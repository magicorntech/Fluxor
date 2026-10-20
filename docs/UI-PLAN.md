# Fluxor — Profesyonel UI ve Uygulama Yönetimi Planı

Durum: **plan** (kod yok). Bu belge, dashboard'u baştan profesyonel bir ürün arayüzüne çevirmeyi ve bunun için
gereken uygulama, depo (Git) ve topoloji özelliklerini sıralar. Ant Design 6.6.7 korunur.

## 1. İstenenler ve nasıl anlaşıldığı

| İstek | Bu planda karşılığı |
|---|---|
| UI çok kötü, profesyonel olacak | Yeni tasarım dili, uygulama kabuğu, ortak bileşen kütüphanesi, koyu tema (§4) |
| **Cluster › Cluster için uygulamalar › Uygulama hangi cluster'lara dağıtılacak › Uygulama Kubernetes topolojisi** | Dört kademeli gezinme: Cluster detayı → o cluster'ın uygulamaları → uygulamanın **Hedefler** sekmesi → **Topoloji** sekmesi (§2, §3) |
| Ana ekranda bilgi gösteren **grafikler** | Genel Bakış: KPI kartları + sağlık/senkron halkaları, cluster başına uygulama, dağıtım zaman serisi, dikkat listesi (§3.1, §8) |
| **Repo tanımlama, Git alanı** | **Depolar** sayfası ve sihirbazı: URL, kimlik doğrulama, bağlantı testi, dal/etiket ve yol gezgini (§3.6, §9) |

**Anlaşılan akış (lütfen doğrulayın):** gezinme iki yönlü çalışır. Bir cluster'a girince orada çalışan uygulamalar
görünür; bir uygulamaya girince hangi cluster'lara dağıtıldığı (**hedefler**) ve her hedefteki canlı Kubernetes
kaynak ağacı (**topoloji**) görünür. Yani "uygulama" = bir Git deposundaki manifest kümesi + (cluster, namespace)
hedefleri listesi.

## 2. Bilgi mimarisi

```
Genel Bakış
Cluster'lar ──► Cluster detayı ──► [Uygulamalar sekmesi] ──► Uygulama detayı
Uygulamalar ──────────────────────────────────────────────► Uygulama detayı
                                                              ├─ Özet
                                                              ├─ Hedefler      (hangi cluster'lara, hangi namespace)
                                                              ├─ Topoloji      (canlı kaynak ağacı, hedef başına)
                                                              ├─ Kaynak        (depo, revizyon, yol, tür)
                                                              ├─ Geçmiş        (dağıtımlar, fark)
                                                              └─ Ayarlar
Depolar                                                      (Git depo tanımları)
Yönetim: Kullanıcılar · Ekipler · Gruplar · Yetkiler · Denetim   (mevcut)
```

- **Cluster detayı** sekmeleri: Özet · Uygulamalar · Workload'lar · Namespace'ler · Düğümler · Olaylar · Ayarlar.
- **Uygulama detayı** her zaman şu başlığı taşır: ad, sağlık ve senkron rozetleri, kaynak (`depo@revizyon:yol`),
  hedef sayısı, hızlı eylemler (Senkronize et, Yenile, Geri al). Alt sekmeler bunun altında.
- Her iki yönde de bağlantılar vardır: uygulama sayfasında hedef cluster'a, cluster sayfasında uygulamaya geçilir.
- Mevcut "Olaylar" sayfası, cluster/uygulama zaman çizelgelerine bölünür; genel akış **Etkinlik** olarak kalır.

## 3. Ekranlar

### 3.1 Genel Bakış

```
┌ Genel Bakış ─────────────────────────────────────────────── [Son 30 gün ▾] [↻] ┐
│ ┌ Cluster ──┐ ┌ Uygulama ─┐ ┌ Sağlıklı ──┐ ┌ Senkron dışı ┐ ┌ Son 24s dağıtım ┐│
│ │ 6 / 7     │ │ 42        │ │ %93        │ │ 5            │ │ 18  ✓16 ✗2      ││
│ │ 1 ulaşılmaz│ │ 118 hedef │ │ ▁▂▃▅▆ trend│ │ 3 uygulama   │ │                 ││
│ └───────────┘ └───────────┘ └────────────┘ └──────────────┘ └─────────────────┘│
│ ┌ Sağlık dağılımı ──┐ ┌ Senkron durumu ──┐ ┌ Cluster başına uygulama ──────────┐ │
│ │   ◔ halka grafiği │ │   ◔ halka        │ │ prod-eu  ████████████ 14          │ │
│ │ Healthy/Degraded… │ │ Synced/OutOfSync │ │ prod-us  █████████ 11             │ │
│ └───────────────────┘ └──────────────────┘ │ staging  ██████ 8   (yığılmış: sağlık)│
│ ┌ Dağıtımlar (günlük, başarılı/başarısız) ─┴────────────────────────────────────┐│
│ │ alan/çizgi grafiği                                                            ││
│ └───────────────────────────────────────────────────────────────────────────────┘│
│ ┌ Dikkat gerektirenler ───────────────────┐ ┌ Son etkinlik ────────────────────┐ │
│ │ ● payments-api   prod-eu  Degraded  2s  │ │ ayse  → payments-api senkronize  │ │
│ │ ● search         staging  OutOfSync     │ │ deploy-bot → web geri aldı       │ │
│ └─────────────────────────────────────────┘ └──────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────────────────┘
```
Halka dilimine tıklamak, süzülmüş listeye götürür (`/applications?health=Degraded`). Tüm sayılar **kullanıcının
yetkisine göre** hesaplanır: görmeye izni olmadığı cluster ve uygulamalar grafiklerde de yoktur.

### 3.2 Cluster'lar ve cluster detayı
- **Liste:** ad, bağlantı durumu, Kubernetes sürümü, düğüm sayısı, uygulama sayısı, etiketler, grup. Kart/tablo
  görünümü, durum ve etikete göre süzme.
- **Detay › Özet:** bağlantı sağlığı (son başarılı temas), sürüm, düğüm ve namespace sayıları, workload sağlığı özeti,
  bu cluster'daki uygulamaların sağlık dağılımı (küçük halka).
- **Detay › Uygulamalar:** bu cluster'a hedeflenmiş uygulamalar; sütunlar: uygulama, namespace, sağlık, senkron,
  son dağıtım. Satıra tıklamak uygulama sayfasının **Topoloji** sekmesini bu cluster seçili olarak açar.
- **Detay › Workload'lar:** canlı Deployment/StatefulSet/DaemonSet listesi (Adım B). Fluxor'un yönetmediği
  kaynaklar "yönetilmiyor" rozetiyle ayrılır.

### 3.3 Uygulamalar
- **Liste:** ad, kaynak (`depo@rev:yol`), **hedef çipleri** (her cluster için sağlık renginde küçük rozet),
  genel sağlık, senkron, son dağıtım. Süzgeçler: sağlık, senkron, cluster, depo, etiket. İki görünüm: **Tablo** ve
  **Matris** (satır uygulama × sütun cluster, hücre = durum).
- **Oluşturma sihirbazı** (§3.7).

### 3.4 Uygulama › Hedefler ("hangi cluster'lara dağıtılacak")
```
Hedefler                                              [+ Hedef ekle]  [Cluster grubu ekle]
┌────────────┬───────────────┬────────────┬──────────┬───────────────┬───────────────┐
│ Cluster    │ Namespace     │ Sağlık     │ Senkron  │ Son dağıtım   │               │
├────────────┼───────────────┼────────────┼──────────┼───────────────┼───────────────┤
│ prod-eu    │ payments      │ ● Healthy  │ ✓ Synced │ 2 dk önce     │ Sync · Topoloji│
│ prod-us    │ payments      │ ◐ Progress │ ✓ Synced │ şimdi         │ Sync · Topoloji│
│ staging    │ payments-test │ ● Degraded │ ⚠ Out of │ 3 sa önce     │ Sync · Topoloji│
└────────────┴───────────────┴────────────┴──────────┴───────────────┴───────────────┘
```
Hedef ekleme: cluster (veya mevcut **cluster grubu**) + namespace (+ "namespace yoksa oluştur"). Kullanıcının
`app:manage` yetkisinin **kapsadığı** cluster'lar seçilebilir (yetki modeli: [AUTH.md](AUTH.md) §3).

### 3.5 Uygulama › Topoloji (Kubernetes kaynak ağacı)
```
Cluster: [prod-eu ▾]   Görünüm: (● Ağaç)(○ Ağ)(○ Liste)   Süz: [sağlık ▾][tür ▾][ara…]   [⤢ sığdır] [harita]
                                                           ┌──────────────────────────┐
  payments-api ─┬─ Ingress  payments ──── Service payments │  Deployment payments-api │
   (uygulama)   │                                │         │  ● Healthy   ✓ Synced    │
                ├─ ConfigMap payments-cfg        │         │  Replika 3/3             │
                ├─ Secret   payments-db          ▼         │  Görüntü  api:1.8.2      │
                └─ Deployment payments-api ─ ReplicaSet ─┬─ Pod payments-api-7d9… ●   │
                                                          ├─ Pod payments-api-7d9… ●   │
                                                          └─ Pod payments-api-7d9… ◐   │
                                                           [Özet][Manifest][Olaylar][Günlük]
```
- **Düğüm:** tür simgesi, ad, sağlık rengi + simge (yalnız renk değil), senkron durumu, replika/görüntü özeti.
- **Kenarlar:** sahiplik (`ownerReferences`) ağaç görünümünde; **Ağ** görünümünde Ingress → Service → Pod.
- **Etkileşim:** sürükle/yakınlaştır, mini harita, düğüm seç → sağ çekmecede Özet / Manifest (canlı ↔ istenen fark) /
  Olaylar / Günlük. Çok sayıda Pod varsa ReplicaSet altında "+37 Pod" olarak daraltılır, tıklayınca açılır.
- **Çok hedef:** üstteki seçici tek cluster'ı gösterir; "Tümü" cluster başına yan yana şeritler.
- **Canlı:** sunucudan olay akışıyla (SSE) güncellenir; yenilemeden değişen sağlık görünür.

### 3.6 Depolar (Git alanı)
```
Depolar                                                              [+ Depo ekle]
┌ payments-charts ─ https://git.acme.io/platform/charts.git ─ ● Bağlı ─ main ─ 7 uygulama ┐
│ Kimlik: HTTPS token · Son kontrol: 2 dk önce            [Test et] [Düzenle] [Sil]       │
└──────────────────────────────────────────────────────────────────────────────────────────┘
Depo ekle (sihirbaz)
 1 Adres      →  URL (https:// veya ssh)           [Devam]
 2 Kimlik     →  (●) Herkese açık (○) HTTPS kullanıcı/token (○) SSH anahtarı
 3 Bağlantı   →  [Test et]  ✓ erişildi · 12 dal · 40 etiket   (hata: neden açıkça yazılır)
 4 Ayarlar    →  Ad · varsayılan dal · açıklama                [Kaydet]
```
Kayıttan sonra **depo gezgini**: dal/etiket seçilir, klasör ağacı gezilir; `Chart.yaml` → Helm,
`kustomization.yaml` → Kustomize, aksi halde düz YAML olarak **otomatik algılanır** (uygulama oluştururken yol seçmek için).

### 3.7 Uygulama oluşturma sihirbazı
1. **Kaynak:** depo → revizyon (dal/etiket/commit) → yol (gezgin) → tür (otomatik algılanır, değiştirilebilir) →
   Helm değerleri / Kustomize ayarları. Önizleme: üretilen manifestlerin tür/ad listesi.
2. **Hedefler:** cluster(lar) ve namespace; cluster grubu seçilebilir.
3. **Senkronizasyon:** elle / otomatik, prune, self-heal.
4. **Özet ve oluştur.**

## 4. Görsel tasarım dili ("profesyonel" ne demek)

**Kabuk:** daraltılabilir sol menü (simge + ad, gruplu), üstte **breadcrumb**, genel arama (`⌘K`: cluster, uygulama, depo),
tema anahtarı, kullanıcı menüsü. İçerik alanı geniş ekranlarda sınırlı genişlikte ve ortalı değil, tam genişlik tablolar.

**Tema (antd 6 `ConfigProvider` token'ları):** nötr gri-mavi zemin + tek marka vurgu rengi; köşe yarıçapı 6; taban
yazı 14 px, tablolarda sıkı satır yüksekliği (kompakt), sayılarda sabit genişlikli rakam (`tabular-nums`), kimlik/hash/YAML
için tek aralıklı yazı tipi. Açık ve **koyu** tema (antd algoritmaları), sistem tercihini izler.

**Durum dili (her yerde aynı):**

| Anlam | Renk | Simge | Metin |
|---|---|---|---|
| Healthy / Synced / Connected | yeşil | ✓ | Sağlıklı / Senkron / Bağlı |
| Progressing | mavi | ◐ (dönen) | Ilerliyor |
| Degraded / Failed / Unreachable | kırmızı | ✕ | Bozuk / Başarısız / Ulaşılamıyor |
| OutOfSync / Warning | turuncu | ⚠ | Senkron dışı |
| Missing / Unknown | gri | ? | Eksik / Bilinmiyor |

Renk **tek başına** anlam taşımaz (simge + metin de var). Tek bir `StatusBadge` bileşeni her ekranda kullanılır.

**Ortak bileşenler** (`dashboard/src/components/`): `PageHeader` (başlık, açıklama, breadcrumb, eylemler), `StatusBadge`,
`KindIcon` (Kubernetes türü simgeleri), `EmptyState` (açıklama + birincil eylem), `FilterBar` (çip süzgeçler, URL'de saklanır),
`DataTable` (sarmalayıcı: yükleniyor iskeleti, hata, boş durum, sütun seçici), `KeyValue`, `CopyText`, `RelativeTime`
(göreli + tam zaman ipucu), `ConfirmDelete`. Sayfalar bunlardan kurulur, ham `Table`/`Alert` çağrıları azalır.

**Kalite çıtası:** yükleme iskeletleri (boş ekran yok), anlamlı boş durumlar, hata durumunda yeniden dene, klavyeyle
gezinme ve görünür odak, kontrast AA, duyarlı düzen (tablet genişliği), tek yerde toplanmış Türkçe metinler (sonradan çeviriye hazır),
**rota bazlı kod bölme** (şu an tek paket 1,1 MB; grafik ve topoloji kitaplıkları ağır, yalnız kendi sayfalarında yüklenir).

## 5. Veri modeli ve API değişiklikleri

**Yeni/genişleyen kayıtlar (MongoDB):**
```
repositories   id, name(uniq), url, kind:"git", auth{type: none|https|ssh, credentialRef}, defaultRevision,
               status{state, message, checkedAt}, createdAt, createdBy
applications   id, name(uniq), source{repoId, revision, path, type: helm|kustomize|plain, helm{values,valueFiles}},
               targets[{clusterId, namespace, createNamespace}], syncPolicy{auto, prune, selfHeal}, labels, createdAt
releases       id, appId, clusterId, revision, status, startedAt, finishedAt, message, diffSummary
credentials    id, kind, ciphertext, nonce, keyId                    (AES-256-GCM; asla API'den dönmez)
```
`Cluster` genişler: `nodeCount`, `namespaceCount`, `lastContactAt`, `credentialRef` (Adım B).

**Yeni uçlar** ([api/openapi.yaml](../api/openapi.yaml)'a eklenir, TS tipleri `make api-gen`):
```
GET/POST/PATCH/DELETE /repositories            POST /repositories/{id}/test
GET  /repositories/{id}/refs                   (dal, etiket)       GET /repositories/{id}/tree?rev=&path=
GET/POST/PATCH/DELETE /applications            GET /applications/{id}?include=targets,status
PUT  /applications/{id}/targets                POST /applications/{id}/sync?cluster=
GET  /applications/{id}/topology?cluster=      GET /applications/{id}/topology/stream   (SSE)
GET  /applications/{id}/releases               GET /clusters/{id}/applications   GET /clusters/{id}/workloads
GET  /overview?range=7d|30d|90d                (Genel Bakış'ın tek çağrısı, yetkiye göre süzülmüş)
```

## 6. Motor önkoşulları (UI bunlara dayanır)

| Parça | İş | Not |
|---|---|---|
| `internal/clustermgr` | cluster başına istemci havuzu, bağlantı sağlığı, bilgi toplama, **informer önbelleği** | Adım B; topoloji ve cluster detayının canlı verisi |
| `internal/secrets` | AES-256-GCM, anahtar `FLUXOR_SECRET_KEY` | cluster kimlik bilgisi + depo kimlik bilgisi için ortak |
| `internal/gitrepo` (yeni) | klonlama/getirme önbelleği (`go-git`), `ls-remote`, dal/etiket, ağaç | yalnızca okuma, sığ klon |
| `internal/render` | düz YAML → Helm → Kustomize | Adım D |
| `internal/engine` | diff, SSA uygulama, prune, sağlık | Adım D/E |
| `internal/topology` (yeni) | canlı kaynaklardan düğüm/kenar grafı üretir | §7 |
| `internal/overview` (yeni) | Genel Bakış sayıları ve zaman serileri | §8 |

## 7. Topoloji tasarımı

**Hangi kaynaklar uygulamaya ait?** Fluxor uyguladığı her kaynağa `fluxor.io/app-id` ve `fluxor.io/target` açıklaması (annotation)
yazar ve son senkronda bir **envanter** tutar. Kökler envanterdeki kaynaklardır; çocuklar `ownerReferences` ile bulunur
(Deployment → ReplicaSet → Pod, Job → Pod, CronJob → Job, StatefulSet/DaemonSet → Pod). Service → Pod ve Ingress → Service
bağlantıları (Ağ görünümü) etiket seçicilerinden ve `backend` alanlarından türetilir.

**Düğüm yükü:** `{id(uid), cluster, group, kind, namespace, name, health{status,message}, sync{status}, createdAt, replicas{ready,desired},
images[], parents[], collapsed?}`; kenarlar `{from, to, type: owner|network}`.

**Sağlık:** tür başına kurallar (`internal/health`): Deployment (kullanılabilir ≥ istenen), Pod (faz, hazır, yeniden başlatma, `CrashLoopBackOff`),
Job, PVC, Ingress. **Senkron:** istenen (render edilen) ile canlı farkı: Synced / OutOfSync / Missing (istenen var canlı yok) / Extra (canlı var, istenmiyor).

**Ölçek:** istek başına düğüm tavanı (varsayılan 2000); aşılırsa Pod'lar ReplicaSet başına sayıya daraltılır
(`collapsed:{total,healthy}`), istemci `?expand=<düğüm>` ile açar. Canlı akış yalnızca **görünen** uygulama için açılır ve sekme kapanınca kapanır.

**İstemci:** React Flow (`@xyflow/react`) + otomatik yerleşim (`elkjs` ya da `@dagrejs/dagre`); özel düğüm kartları antd
tokenlarıyla boyanır; düğüm sayısı eşiğini aşınca sanallaştırma. Paket sürümleri uygulama sırasında doğrulanıp sabitlenir.

**Yetki:** topoloji `app:view` ister ve yalnızca kullanıcının **hem uygulamayı hem hedef cluster'ı** kapsayan kapsamındaki hedefleri döner
(evaluator'daki "uygulama izni iki boyuta bakar" kuralı). Manifest/Secret içeriği: Secret değerleri hiçbir zaman döndürülmez, yalnızca anahtar adları.

## 8. Genel Bakış grafikleri

Kitaplık: **`@ant-design/charts`** (Ant Design ekosistemi, G2 tabanlı; halka, sütun, alan, çizgi). Renkler §4'teki durum paletinden gelir.

| Bileşen | Veri | Kaynak |
|---|---|---|
| KPI: Cluster (bağlı/toplam) | cluster durumları | clustermgr |
| KPI: Uygulama / hedef sayısı | uygulamalar | store |
| KPI: Sağlıklı oranı + mini trend | hedef sağlıkları, günlük örnek | informer önbelleği + günlük özet |
| KPI: Senkron dışı | hedef senkron durumları | engine |
| KPI: Son 24 saat dağıtım | releases | store |
| Halka: sağlık dağılımı, senkron durumu | hedef sayıları | önbellek |
| Sütun (yığılmış): cluster başına uygulama, sağlığa göre | hedefler | önbellek |
| Alan: günlük dağıtımlar (başarılı/başarısız) | releases | store |
| Tablo: dikkat gerektirenler (Degraded, OutOfSync, Unreachable) | önbellek | — |
| Akış: son etkinlik | releases + (yetkili ise) denetim kaydı | store |

Tek çağrı `GET /overview` (15–30 sn önbellek); sayılar **çağıranın yetkisine göre** hesaplanır. Zaman serisi için günlük özetler
`overview_daily` koleksiyonunda tutulur (yeniden başlatmada kaybolmasın). Grafiklerde veri yoksa anlamlı boş durum (ör. "Henüz uygulama yok — [Depo ekle]").

## 9. Depo (Git) tanımı

**Kimlik doğrulama (v1):** herkese açık · HTTPS kullanıcı adı/token · SSH özel anahtarı (+ `known_hosts` bilgisi). GitHub App ve
dağıtım anahtarları sonraya. Kimlik bilgisi `secrets` ile şifrelenir; **API hiçbir zaman geri döndürmez** (yalnızca "tanımlı" bilgisi), güncelleme yeni değer ister.

**Bağlantı testi:** `git ls-remote` ile; sonuç ve **açık hata nedeni** (kimlik reddedildi, ana makine bulunamadı, zaman aşımı, sertifika)
`status` alanına yazılır. Düzenli yeniden kontrol (varsayılan 5 dk) ve "Test et" düğmesi.

**Güvenlik:**
- Yalnızca `https://` ve `ssh://` / `git@host:` adresleri; `file://`, `git://`, `http://` (şifresiz) reddedilir (`http` için açık bir ayar).
- Sunucu tarafı istek sahteciliğine (SSRF) karşı: bulut meta-veri ve bağlantı-yerel adresler (`169.254.0.0/16` vb.) her zaman engellenir; özel
  ağlar (kendi GitLab'ınız) izinlidir, istenirse `FLUXOR_REPO_ALLOWED_HOSTS` ile daraltılır.
- SSH ana makine anahtarı doğrulanır (kayıtlı `known_hosts`); "doğrulamayı atla" seçeneği yok.
- Klonlama zaman aşımı ve boyut sınırı, sığ klon, depo başına kilit (aynı anda tek getirme).
- Helm/Kustomize çalıştırma: yürütülebilir eklentiler kapalı, zaman ve bellek sınırlı (Adım D).
- Depo URL'si ve adı denetim kaydına yazılır; kimlik bilgisi asla.

## 10. Yetkilendirme etkisi

- Yeni izin **`repo:manage`** (sistem sınıfı: depo ekle/düzenle/sil, kimlik bilgisi). Depoları **görmek** için `repo:manage` ya da herhangi bir
  kapsamda `app:manage` yeterli (listede yalnızca ad, adres, durum; kimlik bilgisi yok).
- Yeni yerleşik rol önerisi **"Platform yöneticisi"**: `repo:manage` + cluster yönetimi. (Sistem izni içerdiğinden kapsamı daraltılamaz — mevcut kural.)
- `app:*` izinleri ve kapsam modeli aynen kalır; uygulama kapsamları ve **uygulama grupları** artık açılır (`authz.ApplicationsAvailable = true`),
  cluster grubu mantığıyla aynı. Cluster detayındaki "Uygulamalar" listesi, kullanıcının görebildiği uygulamalarla süzülür.
- Denetim: depo/uygulama/hedef değişiklikleri ve her senkron `s.handle(...)` ile günlüğe yazılır ([AUTH.md](AUTH.md) §7.5).

## 11. Uygulama sırası

Önceki yol haritasındaki B/D/E adımları bu sırayla yeniden düzenlenir. Boyut: S ≈ birkaç gün, M ≈ 1–2 hafta, L ≈ 2+ hafta.

| Adım | İçerik | Boyut | Çıkış kriteri |
|---|---|---|---|
| **UI-0 Tasarım temeli** | tema ve koyu tema, yeni kabuk (menü, breadcrumb, `⌘K`), ortak bileşenler, `StatusBadge`, kod bölme; mevcut sayfalar yeni bileşenlere taşınır | M | Tüm mevcut sayfalar yeni kabukta; bileşen birim testleri; paket boyutu ilk yüklemede küçülmüş |
| **B Cluster bağlantısı + detay** | `clustermgr`, `secrets`, bağlantı testi; **Cluster detay sayfası** (Özet, Workload'lar, Namespace'ler, Düğümler, Olaylar) | L | Gerçek bir cluster eklenip sağlığı ve workload'ları görülüyor |
| **R Depolar** | `repositories` (Mongo), kimlik bilgisi şifreleme, `ls-remote` testi, dal/etiket/ağaç uçları, **Depolar sayfası + sihirbaz + gezgin**, `repo:manage` izni | M | Herkese açık ve özel (token/SSH) depo eklenip test edilebiliyor, yol gezilebiliyor |
| **D Uygulama + oluşturma** | `applications`, render (düz YAML → Helm → Kustomize), tek hedefe SSA senkron, `releases`; **Uygulamalar listesi, oluşturma sihirbazı, Kaynak ve Geçmiş sekmeleri** | L | Depodan bir uygulama oluşturulup tek cluster'a dağıtılıyor, drift düzeliyor |
| **E Çok hedefli** | hedef yönetimi (**Hedefler sekmesi**), N cluster'a paralel senkron, hedef başına durum, **Matris görünümü**, uygulama kapsamı/grupları açılır | M | Aynı uygulama N cluster'a dağıtılıp tek ekranda izleniyor |
| **T Topoloji** | `topology` + sağlık/diff, SSE, React Flow ekranı (Ağaç/Ağ/Liste, çekmece, daraltma) | L | Bir uygulamanın canlı kaynak ağacı hedef başına görülüyor ve canlı güncelleniyor |
| **H Genel Bakış grafikleri** | `overview` + `overview_daily`, `@ant-design/charts`, süzülmüş listelere bağlantılar | M | Ana ekran gerçek verilerle, yetkiye göre doğru sayılarla çalışıyor |
| **P Cila** | boş durum/hata/iskelet taraması, erişilebilirlik (AA), klavye, duyarlılık, performans (2000 düğüm), Playwright ile 4–5 kritik akış | M | Kontrol listesi tamam |

Sıra gerekçesi: UI-0 bağımsızdır ve hemen başlayabilir (her sonraki ekran onun üstüne kurulur). B canlı veri sağlar, R ve D uygulama
verisini, T ikisine birden dayanır; H en sona kalır çünkü anlamlı veri ister. UI-0 sonrası her adım **kullanıcıya görünür bir sonuç** verir.

## 12. Test stratejisi
- **Go:** her yeni paket için tablo testleri; depo ve cluster erişimi sahte sunucu/`envtest` ile (Docker gerekmez); mevcut uyumluluk testi paketi yeni store'lara genişler.
  Yetki ve gizlilik (kimlik bilgisi sızmıyor, topoloji yetkiye göre süzülüyor, genel bakış sayıları yetkiye göre) için ayrı testler + mutasyon denemesi.
- **Dashboard:** saf mantık vitest ile (şimdiki gibi); bileşenler için `@testing-library/react` + jsdom (yeni); gerçek tarayıcı akışları için **Playwright** (P adımında).
- **Elle doğrulama:** her adım gerçek Mongo ve (B'den sonra) gerçek bir test cluster'ıyla tarayıcıda denenir.

## 13. Riskler
| Risk | Önlem |
|---|---|
| Topoloji büyük uygulamalarda yavaşlar/anlaşılmaz olur | düğüm tavanı + daraltma, görünüm süzgeçleri, sanallaştırma, önce küçük uygulamalarla ölçüm |
| Informer'lar çok cluster'da bellek yer | yalnızca yönetilen tür ve çocukları izlenir; cluster başına sınır; ölçüm ve ayar |
| Git kimlik bilgisi sızıntısı | şifreli saklama, API'den dönmeme, denetim ve günlüklerde yok, testle kanıt |
| Depo/Helm üzerinden sunucuda keyfi çalıştırma veya SSRF | adres şeması ve ağ kısıtları, eklenti kapalı, zaman/bellek sınırı |
| Tasarım dili yüzeysel kalır ("antd varsayılanı" görünümü) | UI-0'da token, bileşen ve boşluk sistemi birlikte; tasarım gözden geçirmesi çıkış kriteri |
| Kapsam büyür | her adım tek başına yayımlanabilir; sıra ve çıkış kriterleri sabit |

## 14. Açık sorular (önerimle)
1. **Gezinme anlayışı** (§1) doğru mu: cluster → uygulamalar → hedefler → topoloji, iki yönlü?
2. **Marka/renk:** elinizde logo ve vurgu rengi var mı? *Öneri:* yoksa nötr gri-mavi + indigo vurgu ile başlanır, sonra değiştirmek tek token.
3. **Koyu tema** ilk sürümde olsun mu? *Öneri:* evet (antd algoritmasıyla düşük maliyet).
4. **Topoloji kitaplığı:** React Flow + ELK/dagre mi, AntV G6 mı? *Öneri:* React Flow (geniş topluluk, özel düğüm kolaylığı).
5. **Git kimlik doğrulaması v1:** herkese açık + HTTPS token + SSH yeterli mi, GitHub App şart mı? *Öneri:* ilk üçü.
6. **Depo yetkisi:** `repo:manage` + "Platform yöneticisi" rolü uygun mu? *Öneri:* evet.
7. **E2E:** Playwright kurulumu (tarayıcı indirmesi ~150 MB) kabul mü? *Öneri:* evet, P adımında.
8. **Uygulama tanımı** yalnızca Fluxor'un veritabanında mı yaşasın, yoksa Git'ten içe aktarma ("app-of-apps") sonra mı? *Öneri:* önce veritabanı, içe aktarma Faz 4.
