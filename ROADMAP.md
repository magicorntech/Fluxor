# Fluxor Yol Haritası

Fluxor: ortam ve akış-merkezli, açık kaynak GitOps sürekli dağıtım aracı (Go backend + React/TS UI, kendi motoru).
Tam plan: `~/.claude/plans/imdi-fluxor-argocd-benzeri-pure-piglet.md`
Repo: https://github.com/magicorntech/Fluxor (dal: `master`)
Mimari (server + dashboard ayrı, çoklu cluster): [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)
UI ve uygulama yönetimi planı (cluster › uygulamalar › hedefler › topoloji, grafikler, Git depoları): [docs/UI-PLAN.md](docs/UI-PLAN.md)

Durum işaretleri: `[ ]` yapılacak · `[~]` sürüyor · `[x]` bitti

## Faz 0 — Temel
- [x] ROADMAP.md
- [x] Repo iskeleti: dizinler, Makefile, .gitignore, README, LICENSE (Apache-2.0), `go mod init github.com/magicorntech/Fluxor`
- [x] CI taslağı (GitHub Actions: build + test; lint sonra eklenecek)
- [x] `dashboard/` (eski `ui/`) Vite + React + TS + **Ant Design 6.6.7** (UI kararı: antd; shadcn/Tailwind vazgeçildi)
- [-] kind tabanlı `make dev`: ertelendi (docker şimdilik yok; testler fake client + envtest ile)
- [ ] İsim/marka doğrulaması ("Fluxor" ≈ "Flux" riski)

## Mimari uygulama sırası (docs/ARCHITECTURE.md §9; Faz 1–3'ün yerine geçer)
- [x] A. İskelet: openapi.yaml, `cmd/fluxor-server`, `ui/` → `dashboard/`, API istemcisi (cluster CRUD memory store üzerinde; dashboard Genel Bakış + Cluster'lar sayfaları çalışıyor)
- [ ] B. Cluster'ları bağla: clustermgr, secrets, salt okunur workload listesi *(UI-PLAN §11'de yeniden sıralandı: cluster detay sayfasıyla birlikte)*
- [x] C. Kalıcılık: MongoDB store (U1 ile tamamlandı)
- [ ] D. Application + sync (tek hedef) *(UI-PLAN §11: uygulama listesi, oluşturma sihirbazı, Kaynak/Geçmiş sekmeleri dahil)*
- [ ] E. Çok hedefli sync, filo matrisi, SSE *(UI-PLAN §11: Hedefler sekmesi, Matris görünümü)*
- [ ] F. Kimlik ve yetki → aşağıdaki U1–U6

## UI ve uygulama yönetimi (docs/UI-PLAN.md) — plan hazır, onay bekliyor
Sıra: UI-0 → B → R → D → E → T → H → P. Ayrıntı, çıkış kriterleri ve açık sorular için UI-PLAN §11 ve §14.
- [ ] UI-0. Tasarım temeli: tema + koyu tema, yeni kabuk (menü, breadcrumb, ⌘K), ortak bileşenler, rota bazlı kod bölme
- [ ] B. Cluster bağlantısı + **Cluster detay sayfası** (Özet, Workload'lar, Namespace'ler, Düğümler, Olaylar)
- [ ] R. **Depolar** (Git): kayıt, şifreli kimlik bilgisi, bağlantı testi, dal/etiket/yol gezgini, `repo:manage` izni
- [ ] D. Uygulama + oluşturma sihirbazı + render + tek hedefe senkron
- [ ] E. **Hedefler** sekmesi (hangi cluster'lara), çok hedefli senkron, Matris görünümü, uygulama kapsamı/grupları
- [ ] T. **Topoloji** (canlı Kubernetes kaynak ağacı, SSE)
- [ ] H. **Genel Bakış grafikleri** (`@ant-design/charts`, yetkiye göre süzülmüş)
- [ ] P. Cila: boş durum/hata/iskelet, erişilebilirlik, performans, Playwright

## Kullanıcı ve yetki sistemi (docs/AUTH.md; MongoDB, yerel hesap, rol+kapsam RBAC)
Sıra: şimdi U1–U6, sonra Adım B/D/E.
- [x] U1. Mongo temeli (bağlantı, indeksler, clusters Mongo'ya taşındı; `FLUXOR_MONGO_URI` boşsa bellek store'u)
- [x] U2. Kimlik: users, argon2id, JWT+refresh, bootstrap admin, authn middleware, `/auth/*`, dashboard girişi (kullanıcı CRUD API'si U3/U5'te)
- [x] U3. Yetki çekirdeği: izinler, yerleşik roller, evaluator + testler, enforcement, liste filtreleme (+ minimal kullanıcı yönetimi API'si)
- [x] U4. Ekipler ve kaynak grupları (cluster grupları; uygulama grupları uygulamalarla birlikte)
- [x] U5. Yönetim arayüzü (Kullanıcılar, Ekipler, Gruplar, Yetkiler)
- [x] U6. Audit, API token'ları, hız sınırı, refresh token ailesi iptali

## Faz 1 — Motor MVP
- [ ] `Application` CRD tipleri
- [ ] Git + Helm + Kustomize render
- [ ] Server-Side Apply, prune, envanter
- [ ] Sağlık değerlendirmesi
- [ ] `fluxor` CLI (`app create/sync/diff`)
- [ ] Otomatik / manuel sync

## Faz 2 — API + UI MVP
- [ ] Server + SSE
- [ ] Uygulama listesi, detay, kaynak ağacı
- [ ] Diff, sync, rollback, log, olaylar

## Faz 3 — Çoklu cluster ve güvenlik
- [ ] Cluster kaydı + agent (pull)
- [ ] OIDC, RBAC, audit log
- [ ] Secret entegrasyonu (SOPS / External Secrets)

## Faz 4 — Ayrışma özellikleri
- [ ] `Environment` + `Pipeline` CRD, promosyon tahtası
- [ ] Onboarding sihirbazı + generator editörü
- [ ] Git write-back (PR)
- [ ] Filo ısı haritası
- [ ] Zaman çizelgesi / zaman yolculuğu

## Faz 5 — Güvenlik ve süreç
- [ ] Politika motoru (CEL), freeze window, onaylar
- [ ] Bildirimler, preview environment'lar

## Faz 6 — Ekosistem
- [ ] ArgoCD import, Rollouts/Flagger
- [ ] Anlamsal diff, AI hata açıklayıcı
- [ ] Eklenti sistemi, docs sitesi, ilk sürüm

## Günlük
- 2026-10-20: Plan onaylandı (açık kaynak, kendi motor, Go + React/TS). Faz 0 başladı.
- 2026-10-20: UI kütüphanesi Ant Design 6.6.7 olarak belirlendi. go 1.27.3 ve kind 0.33.0 brew ile kuruldu. Go iskeleti (`cmd/cli`, `internal/version`) derleniyor ve test geçiyor; `ui/` build ediyor. LICENSE eklendi.
- 2026-10-20: Mimari dokümanı yazıldı: server ve dashboard ayrı süreçler, kaynak-doğruluk DB, cluster bağlantısı push (agent sonra), OpenAPI sözleşmesi. Docker şimdilik kapsam dışı.
- 2026-10-20: Adım A tamam. `api/openapi.yaml` (healthz + clusters), `internal/{domain,config,store(memory),httpapi}`, `cmd/fluxor-server`; `ui/` → `dashboard/` (react-router, TanStack Query, openapi-fetch; tipler `make api-gen` ile üretiliyor). Go testleri geçiyor, dashboard build ediyor; server ve dashboard ayrı çalıştırılıp Vite proxy üzerinden uçtan uca denendi. Sıradaki: Adım B (clustermgr + secrets + salt okunur workload listesi).
- 2026-10-20: Repo https://github.com/magicorntech/Fluxor olarak belirlendi. Go modül yolu `github.com/magicorntech/Fluxor` yapıldı (GitHub'daki büyük/küçük harfle birebir). Yerel git kuruldu, `origin` eklendi ve uzak `master`'ın (yalnızca README) üzerine oturtuldu; henüz commit/push yok.
- 2026-10-20: Kullanıcı/yetki tasarımı yazıldı (docs/AUTH.md). Kararlar: tüm veri MongoDB'de (SQLite/Postgres planı iptal), yerel hesap + sonra OIDC, rol+kapsam allow-only RBAC, ekipler ve cluster/uygulama grupları. Uygulama sırası U1–U6.
- 2026-10-20: U1 tamam. MongoDB 9.0.4 brew ile kuruldu (mongodb/brew tap'inde yalnızca mongodb-community ve mongodb-database-tools formülleri güvenilir işaretlendi) ve servis olarak çalışıyor. `internal/store/mongo` (bağlantı, ping, unique indeks, ClusterStore), ortak `storetest` uyumluluk paketi (memory + mongo aynı testleri geçiyor; bu paket memory store'daki nil-labels tutarsızlığını yakaladı ve düzeltildi), `FLUXOR_MONGO_URI`/`FLUXOR_MONGO_DB` config'i, `make test-mongo`. Server yeniden başlatıldığında cluster'ların kaldığı elle doğrulandı. Sıradaki: U2 (kimlik).
- 2026-10-20: U2 tamam. `internal/auth` (argon2id, JWT HS256 + dönen opak refresh token, kilitleme, bootstrap), `User`/`Session` için store arayüzleri + memory + Mongo (TTL ve kısmi unique e-posta indeksi) + ortak uyumluluk testleri, `httpapi` authn middleware (varsayılan reddet; yalnızca healthz ve auth uçları açık), `/auth/login|refresh|logout|me|password`, zorunlu ilk parola değişimi (403 `password_change_required`), CORS credentials, OpenAPI 0.2.0. Dashboard: AuthProvider (token yalnızca bellekte, açılışta refresh çerezinden geri yükleme, süre dolmadan yenileme, 401'de tek seferlik yenileme), giriş ve parola değiştirme sayfaları, rota koruması, kullanıcı menüsü. Doğrulama: Go testleri (Mongo'lu/Mongo'suz) ve dashboard tsc/lint/build geçiyor; auth middleware'ine 3 mutasyon denendi, üçü de testlerde yakalandı; tarayıcıda giriş, yanlış parola, zorunlu değişim, sayfa yenileme, 401→refresh→yeniden istek ve çıkış elle doğrulandı. Sıradaki: U3 (yetki çekirdeği).
- 2026-10-20: U3 tamam. `internal/authz` (11 izin, 5 yerleşik rol, saf `Access.Can` evaluator'ı: cluster izinleri yalnızca cluster boyutuna, uygulama izinleri iki boyuta, sistem izinleri yalnızca any/any bağlamaya bakar; `ValidateBinding`, `AdminUserIDs`, `EnsureBootstrapAdmin`), `domain.Binding/Subject/Scope/Ref`, `role_bindings` (memory + Mongo, benzersiz `key` indeksi) ve `ListUsers`, ortak uyumluluk testleri. HTTP: cluster uçlarında yetki denetimi (liste filtrelenir, görülemeyen cluster 404, görülen ama yönetilemeyen 403, yeni cluster kaydı yalnızca tüm cluster'ları kapsayan yetkiyle, cluster silinince bağlamaları da silinir), `/users` (liste, oluştur, getir, güncelle/devre dışı, parola sıfırla), `/roles`, `/role-bindings`; son yönetici ve kendini devre dışı bırakma/parolasını sıfırlama korumaları. `auth.Service.CreateUser/ResetPassword`. Başlangıçta yönetici bağlaması yoksa bootstrap kullanıcısı `admin` rolüne bağlanır (U2 veritabanlarını da onarır). OpenAPI 0.3.0. Doğrulama: Go testleri (Mongo'lu) ve dashboard tsc/lint/build geçiyor; 38 mutasyondan 36'sı testlerde yakalandı, kalan ikisi eşdeğer/erişilemez kod (biri silindi); gerçek Mongo'da 30 maddelik uçtan uca senaryo ve tarayıcıda yetkisiz kullanıcı akışı elle doğrulandı. Sıradaki: U4 (ekipler ve kaynak grupları).
- 2026-10-20: U4 tamam. `domain.Team` / `domain.ResourceGroup`, store arayüzleri (`TeamStore`, `GroupStore`) + memory + Mongo (büyük/küçük harfe duyarsız benzersiz ad indeksleri, çok anahtarlı üyelik indeksleri; profil ve üyelik ayrı güncellenir, böylece eşzamanlı değişiklik ezilmez) + ortak uyumluluk testleri. `authz`: `For()` kullanıcının ekiplerini ve bağlamaların işaret ettiği grupları yükler; evaluator grup türünü bilir (uygulama grubu cluster erişimi veremez); ekip ve cluster grubu bayrakları açıldı (`ApplicationsAvailable` kapalı kalır); `AdminUserIDs` ekip üyelerini sayar, `WouldLeaveNoAdmin` ile bekleyen değişikliğin etkisi önceden hesaplanır. HTTP: `/teams` (CRUD + `PUT /members`), `/resource-groups` (CRUD + `PUT /members`, `?kind=`); ekip silme ve üye değiştirme son-yönetici korumasına tabi; ekip, grup veya cluster silinince bağlamalar ve grup üyelikleri temizlenir. OpenAPI 0.4.0. Doğrulama: Go testleri (Mongo'lu), dashboard tsc/lint/build; 34 mutasyonun hepsi testlerde yakalandı; gerçek Mongo'da 29 maddelik uçtan uca senaryo geçti. Sıradaki: U5 (yönetim arayüzü).
- 2026-10-20: U5 tamam. Sunucu: `/auth/me` ve oturum yanıtları izin özeti taşır (`teams`, sistem `permissions`, `canCreateClusters`); cluster yanıtları kullanıcının o cluster üzerindeki izinlerini (`permissions`) içerir; OpenAPI 0.5.0. Dashboard: `usePermissions` + `useMe` (60 sn'de bir yenilenir), izne göre menü ("Yönetim" grubu) ve `RequirePermission` ile 403 sayfası; Kullanıcılar (oluştur, düzenle, parola sıfırla, devre dışı bırak, geçici parola yalnızca bir kez gösterilir), Ekipler (CRUD + üyelik), Gruplar (cluster grupları), Yetkiler (atamalar tablosu, "yetki ver" formu, roller sekmesi) sayfaları; Cluster'lar sayfasında "Cluster ekle" ve silme düğmeleri izne göre gizlenir; izin özeti değişince açık sayfadaki veriler yeniden çekilir (`usePermissionSync`). Birim test altyapısı eklendi (vitest 5; saf mantık `features/admin/model.ts`, `app/nav.ts`, 44 test). Doğrulama: Go testleri (Mongo'lu), dashboard tsc/lint/vitest/build; 5 mutasyon yakalandı; gerçek Mongo + tarayıcıda yönetici ve yetkisiz kullanıcı akışları elle doğrulandı. Tarayıcı testi iki gerçek eksik buldu ve düzeltildi: izin değişince cluster listesi eski kalıyordu, rol adları İngilizce görünüyordu. Sıradaki: U6 (audit, API token'ları, hız sınırı).
- 2026-10-20: U6 tamam. **Denetim:** `audit_events` (Mongo; zamana göre sıralanabilir ID, `time`/`actor`/`action` indeksleri), her değiştirici rota `s.handle(...)` ile bir işlem adına bağlanır ve tek bir ara katman olayı yazar (kim, ne, neyin üzerinde, sonuç, HTTP durumu, adres, ayrıntılar; silinen nesnenin adı olayda kalır); reddedilen/başarısız denemeler de kaydedilir, oturumsuz (401) ve hız sınırına takılan (429) istekler kaydedilmez; giriş başarısızlık nedeni (`unknown_user`, `bad_password`, `locked`, `disabled`) yalnızca denetim kaydına yazılır, istemciye hep aynı yanıt gider; `GET /audit` (`audit:view`; süzgeçler, imleçle sayfalama). **API token'ları:** `flx_` + 256 bit, yalnızca SHA-256 özeti saklanır, sahibinin yetkileriyle çalışır, 1–365 gün (varsayılan 30), kullanıcı başına 20; token ile token oluşturulamaz/silinemez, parola değiştirilemez; kullanıcı devre dışı bırakılınca ve yönetici parola sıfırlayınca iptal edilir, kendi parolasını değiştirince korunur. **Hız sınırı:** giriş ve yenileme için adres başına jeton kovası (varsayılan 10 ve 60/dk), `FLUXOR_TRUSTED_PROXIES` ile yalnızca güvenilir proxy'lerin `X-Forwarded-For`'u dikkate alınır. **Refresh token ailesi iptali:** kullanılmış token 10 sn'den sonra tekrar gelirse aile iptal edilir ve `auth.refresh_reuse` olayı hesabı adlandırarak yazılır; 10 sn içinde (iki sekme yarışı) yalnızca reddedilir; eşzamanlı yenilemelerde Mongo'da tek kazanan. Dashboard: Denetim sayfası (süzgeçler, "daha eski kayıtları yükle"), API token'larım sayfası (token yalnızca bir kez gösterilir), hata eşlemeleri; yarışan sekme için yenileme bir kez yeniden denenir. OpenAPI 0.6.0. Doğrulama: Go testleri (Mongo'lu), dashboard tsc/lint/vitest (63)/build; Go'da 32 mutasyonun hepsi yakalandı; gerçek Mongo'da 17 + 8 maddelik sunucu senaryosu ve tarayıcıda denetim/token akışları geçti. Sıradaki: Adım B (cluster'lara gerçek bağlantı) ve sonrası.
- 2026-10-20: Uygulama yerel olarak çalıştırıldı: `hack/dev-server.sh` (`make dev-server`) sunucuyu yerel MongoDB'de `fluxor` veritabanıyla ve `.dev-secret` (git'te yok) içindeki kalıcı imza anahtarıyla başlatır; dashboard `make dashboard` ile `:5173`. Boş veritabanında ilk yönetici (`admin`) üretilen parolayla oluşur, parola yalnızca sunucunun günlüğünde bir kez yazılır.
- 2026-10-20: UI ve uygulama yönetimi planı yazıldı (docs/UI-PLAN.md): dört kademeli gezinme (cluster › uygulamalar › hedefler › topoloji), ana ekran grafikleri, Git depo tanımı, tasarım dili, veri modeli ve API, topoloji ve depo güvenliği tasarımı, 8 adımlık sıra (UI-0, B, R, D, E, T, H, P) ve 8 açık soru. Kod yazılmadı; onay bekleniyor.
