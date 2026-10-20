# Fluxor Mimarisi

Amaç: **birden fazla Kubernetes cluster'ının deployment'larını tek uygulamadan yönetmek.**
İki parça, birbirinden bağımsız çalışır ve dağıtılır:

| Parça | Ne | Teknoloji | Port (dev) |
|---|---|---|---|
| **server** | API, cluster yönetimi, GitOps motoru (reconcile), kimlik, depolama | Go | `:8080` |
| **dashboard** | Web arayüzü, yalnızca server'ın HTTP API'sini kullanır | React + TS + Ant Design 6.6.7 | `:5173` |

Dashboard hiçbir cluster'a doğrudan bağlanmaz; her şey server üzerinden geçer.

## 1. Genel görünüm

```
 Kullanıcı
    │
    ▼
┌──────────────┐   REST + SSE (OpenAPI)   ┌─────────────────────────────────────┐
│  dashboard   │ ───────────────────────► │               server                │
│ (SPA, antd)  │                          │  httpapi ─► service katmanı         │
└──────────────┘                          │     │            │                  │
                                          │     ▼            ▼                  │
                                          │   auth        engine (reconciler)   │
                                          │     │         ┌───┴────┐            │
                                          │     ▼         ▼        ▼            │
                                          │   store    render   clustermgr      │
                                          │ (MongoDB) (git,   (client havuzu, │
                                          │            helm,    informer, sağlık)│
                                          │            kustomize)    │          │
                                          └──────────────────────────┼──────────┘
                                                   │                 │
                                                   ▼                 ▼
                                               Git repo'ları   Cluster A, B, C …
```

Server tek süreç olarak başlar, ama içeride modül sınırları net tutulur. Böylece `engine`
ve `render` ileride ayrı süreçlere (controller, repo-server) bölünebilir.

## 2. Kararlar

1. **Kaynak-doğruluk (source of truth) server'ın veritabanıdır ve veritabanı MongoDB'dir** (`Cluster`, `Application`,
   `Release`, kullanıcı, ekip, yetki ve audit kayıtları hep orada). Önceki "SQLite/Postgres" kararının yerini alır.
   "Tek app, çok cluster" hedefi için bir yönetim cluster'ına bağımlı olmak gereksiz. YAML içe/dışa aktarma ve
   deklaratif config repo'su sonraki fazda eklenir. Kullanıcı ve yetki tasarımı: [AUTH.md](AUTH.md).
2. **Cluster bağlantısı: push modeli** (MVP). Server, kayıtlı cluster'ın kimlik bilgisiyle (ServiceAccount token
   veya kubeconfig) API server'a doğrudan bağlanır. ArgoCD ile aynı model, en hızlı yol.
   Güvenlik duvarı arkasındaki cluster'lar için **pull tabanlı agent** Faz 3'te eklenir; bu yüzden
   `clustermgr` arayüzünün arkasında "transport" soyutlaması tutulur.
3. **API sözleşmesi önce**: `api/openapi.yaml` yazılır; Go handler'ları ve dashboard'un TypeScript istemcisi
   (`openapi-typescript`) buna göre üretilir. İki taraf birbirinden bağımsız geliştirilir.
4. **Canlı güncelleme: SSE** (`/api/v1/events`). WebSocket gerekmedikçe kullanılmaz.
5. **Uygulama (apply) Server-Side Apply ile**, diff SSA dry-run ile yapılır. Prune, envantere dayanır.
6. **Test için docker gerekmez**: birim testler `client-go` fake'leri ile, entegrasyon testleri `envtest`
   (kube-apiserver + etcd binary'leri, container değil) ile çalışır. kind/e2e sonraya kalır.

## 3. Alan modeli (domain)

```
Cluster        id, name, apiServer, caData, credentialRef, labels, status(Connected|Unreachable), version
Application    id, name, project, source{repoURL, path, revision, type(helm|kustomize|plain), helmValues},
               targets[{clusterId, namespace}], syncPolicy{auto, prune, selfHeal}
Release        id, appId, clusterId, revision, status(Syncing|Synced|Failed), startedAt, finishedAt, message
ResourceState  clusterId, gvk, namespace, name, health, syncStatus, hash     (canlı önbellek, DB'ye yazılmaz)
Event/Audit    id, time, actor, action, subject, details
```
Bir `Application` birden fazla hedefe (`targets`) sahip olabilir: **aynı kaynak, N cluster**.
Uygulama durumu, hedef bazlı durumların toplamıdır (en kötü durum kazanır).

## 4. Server modülleri

| Paket | Sorumluluk |
|---|---|
| `internal/domain` | Saf tipler ve doğrulama; hiçbir bağımlılığı yok |
| `internal/store` | `Store` arayüzleri + `mongo/` (asıl uygulama), `memory/` (test) |
| `internal/secrets` | Cluster kimlik bilgilerinin AES-GCM ile şifrelenmesi (master key env/dosya) |
| `internal/clustermgr` | Cluster başına `dynamic.Interface` + discovery havuzu, bağlantı sağlığı, informer'larla canlı kaynak önbelleği |
| `internal/render` | Git clone/fetch (cache), Helm template, Kustomize build, düz YAML → manifest listesi |
| `internal/engine` | Reconcile döngüsü: render → diff → apply → prune → durum; hedefler arası fan-out ve eşzamanlılık sınırı |
| `internal/health` | Kaynak türü başına sağlık kuralları (Deployment, StatefulSet, Job, Ingress…) |
| `internal/diff` | SSA dry-run tabanlı diff, ignore kuralları |
| `internal/events` | İç olay veriyolu (engine → SSE, audit) |
| `internal/auth` | Token/OIDC doğrulama, RBAC (proje ve cluster bazlı) |
| `internal/httpapi` | Router, handler'lar, SSE, hata biçimi, middleware |
| `internal/config` | Ortam değişkeni ve dosyadan yapılandırma |

Bağımlılık yönü: `httpapi → engine → {render, clustermgr, store, health, diff}` ve hepsi `domain`'e bağlı.
Hiçbir paket `httpapi`'yi import etmez.

## 5. Çoklu cluster akışı (çekirdek)

**Cluster ekleme:** dashboard → `POST /clusters` → server kimlik bilgisini şifreleyip saklar →
`clustermgr` bağlantıyı dener (`/version`, yetki kontrolü) → durum `Connected` veya `Unreachable`.

**Sync (bir Application, N hedef):**
1. `render` kaynağı bir kez çeker ve her hedef için (namespace, değerler) manifestleri üretir.
2. `engine` hedefleri sınırlı eşzamanlılıkla paralel işler; her hedef için: SSA dry-run → diff →
   apply → prune → sağlık bekleme.
3. Her hedef için bir `Release` kaydı yazılır; sonuçlar `events` ile SSE'ye akar.
4. Uygulama durumu hedef durumlarından hesaplanır. Bir cluster'ın başarısızlığı diğerlerini durdurmaz.

**Canlı durum:** `clustermgr`, cluster başına ilgili kaynak türleri için informer çalıştırır;
`engine` drift'i bu önbellekle (API'yi dövmeden) tespit eder. `selfHeal` açıksa drift otomatik düzeltilir.

## 6. HTTP API (ilk sürüm taslağı)

```
GET    /healthz  /readyz
GET    /api/v1/clusters                       POST   /api/v1/clusters
GET    /api/v1/clusters/{id}                  DELETE /api/v1/clusters/{id}
GET    /api/v1/clusters/{id}/workloads        # canlı: namespace ve deployment listesi (salt okunur)
GET    /api/v1/applications                   POST   /api/v1/applications
GET    /api/v1/applications/{id}              PUT/DELETE
POST   /api/v1/applications/{id}/sync         # ?cluster=<id> ile tek hedef
GET    /api/v1/applications/{id}/diff?cluster=
GET    /api/v1/applications/{id}/releases
GET    /api/v1/events                         # SSE
POST   /api/v1/auth/login   GET /api/v1/me
```
Hatalar tek biçimde: `{ "error": { "code": "...", "message": "..." } }`.

## 7. Dashboard yapısı

> Yeni ekranlar (cluster detayı, uygulamalar, hedefler, topoloji, depolar, grafikli genel bakış) ve tasarım dili için [UI-PLAN.md](UI-PLAN.md).

Dashboard `dashboard/` klasöründedir. API tipleri `api/openapi.yaml`'dan `npm run api:gen` ile üretilir
(`openapi-typescript` projeye bağımlılık olarak eklenmez, `npx` ile sabit sürümde çalışır: TypeScript 5 peer şartı yüzünden).

```
dashboard/src/
  api/          OpenAPI'den üretilen istemci + TanStack Query hook'ları
  app/          router, layout (antd Layout/Menu), tema, oturum
  features/
    clusters/       liste, ekleme formu, bağlantı durumu
    applications/   liste, detay, hedef bazlı sekmeler, sync/diff
    fleet/          uygulama × cluster matrisi (sağlık ısı haritası)
    events/         SSE akışı, zaman çizelgesi
  components/   StatusTag, ResourceTree, DiffViewer (Monaco)…
```
Ekranlar: **Genel Bakış** (filo matrisi) · **Cluster'lar** · **Uygulamalar** · **Uygulama detayı**
(her hedef için durum, kaynak ağacı, diff, sürüm geçmişi) · **Olaylar**.
Dev'de Vite, `/api` isteklerini `:8080`'e proxy'ler; prod'da dashboard statik olarak ayrı servis edilir
(ters vekil ile aynı origin altında ya da CORS ile).

## 8. Hedef repo yapısı

```
Fluxor/
  api/openapi.yaml
  cmd/fluxor-server/        server giriş noktası            
  cmd/fluxor/               CLI                              
  internal/                 yukarıdaki paketler
  dashboard/                React uygulaması                 
  charts/fluxor/            server + dashboard Helm chart'ı (sonra)
  docs/  hack/  e2e/
```
Makefile: `make server` (`go run ./cmd/fluxor-server`), `make dashboard` (`npm run dev`),
`make api-gen`, `make test`.

## 9. Uygulama sırası

| Adım | İçerik | Çıkış kriteri |
|---|---|---|
| **A. İskelet** | `openapi.yaml` (clusters + healthz), `internal/{domain,config,store/memory,httpapi}`, `cmd/fluxor-server`; `ui/` → `dashboard/`, API istemcisi, router, boş sayfalar | `make server` ve `make dashboard` ayrı çalışıyor; dashboard `/healthz` ve cluster listesini gösteriyor |
| **B. Cluster'ları bağla** | `clustermgr`, `secrets`, cluster ekleme/silme/durum, `GET /clusters/{id}/workloads` | Birden fazla gerçek cluster eklenip tüm deployment'lar tek ekranda salt okunur görülüyor |
| **C. Kalıcılık** | MongoDB store (AUTH.md U1 ile birlikte) | Yeniden başlatınca veri duruyor |
| **D. Application + sync** | `render` (düz YAML → Helm → Kustomize), `engine` tek hedefe SSA apply/prune, `Release` kaydı | Bir Git repo'sundan tek cluster'a sync, drift düzeltme |
| **E. Çok hedefli sync** | Fan-out, filo matrisi, SSE olayları, hedef bazlı diff/sync | Aynı uygulama N cluster'a dağıtılıp tek ekranda izleniyor |
| **F. Kimlik ve yetki** | AUTH.md U2–U6 (yerel hesap, rol+kapsam RBAC, ekip, grup, audit); OIDC sonra | Roller dashboard'a yansıyor |

Sonrası (önceki plandan): ortamlar ve promosyon, onboarding sihirbazı, Git write-back,
zaman çizelgesi, pull tabanlı agent, politika motoru.

## 10. Açık noktalar
- Kaynak-doğruluk olarak MongoDB seçildi; deklaratif (config repo / CRD) mod sonra gerekiyor mu?
- Cluster kimlik bilgisi: yalnızca ServiceAccount token mı, kubeconfig yükleme de olsun mu? (öneri: ikisi)
- Çok kiracılık (proje/takım izolasyonu) hangi fazda? (öneri: F ile birlikte, `project` alanı baştan var)
