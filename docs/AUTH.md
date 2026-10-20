# Fluxor Kullanıcı ve Yetki Sistemi

Kararlar:
- **Veri deposu: MongoDB, tüm veri için** (kullanıcı, ekip, grup, yetki, cluster, uygulama, release, audit).
  Önceki "SQLite/Postgres" planının yerini alır.
- **Giriş: yerel hesap** (parola + JWT oturum). OIDC/SSO sonraki fazda eklenir.
- **Yetki modeli: rol + kapsam, yalnızca izin (allow-only).** Yasaklama (deny) kuralı yok; yetkiler toplanır.

## 1. Gereksinimlerin karşılığı

| İstek | Karşılığı |
|---|---|
| Admin, diğer kullanıcılar için hesap oluşturur | `users` + yalnızca `admin` rolü olanların kullanabildiği `/users` API'si; ilk girişte parola değişimi zorunlu |
| Hangi cluster / hangi uygulama üzerinde yetki | `role_bindings`: kime (kullanıcı/ekip), hangi rol, hangi **cluster kapsamı** ve **uygulama kapsamı** |
| Uygulamaları ve cluster'ları gruplama | `resource_groups` (`kind: cluster` veya `application`); bir kapsam tek kaynak, grup ya da "hepsi" olabilir |
| Ekipler oluşturma ve atama | `teams` (üyeler kullanıcılar); rol bir ekibe atanırsa tüm üyeler alır |
| Yetki gruplandırma sistemi | **Roller** (izin kümeleri) + **kapsamlar** (kaynak kümeleri) + **bağlamalar** (kim, ne yapabilir, nerede) |

## 2. Veri modeli (MongoDB koleksiyonları)

```
users            _id, username(uniq), email(uniq), displayName, passwordHash, mustChangePassword,
                 disabled, createdAt, createdBy, lastLoginAt, failedLogins, lockedUntil
teams            _id, name(uniq), description, members:[userId]            (index: members)
resource_groups  _id, kind: "cluster"|"application", name, description, members:[resourceId]
                 (unique: kind+name). Sonra: label selector ile dinamik üyelik.
roles            _id, name(uniq), builtin:bool, permissions:[string]
role_bindings    _id, subject:{kind:"user"|"team", id}, roleId,
                 scope:{ cluster: Ref, application: Ref }, createdAt, createdBy
                 Ref = {kind:"any"} | {kind:"id", id} | {kind:"group", id}
sessions         _id, userId, refreshTokenHash(uniq), expiresAt(TTL), userAgent, ip, createdAt
api_tokens       _id, userId, name, tokenHash(uniq), expiresAt, lastUsedAt, createdAt
audit_events     _id, time, actor, action, subject, result, ip, details   (index: time desc)
clusters, applications, releases   (mevcut domain; Mongo'ya taşınır)
```

`passwordHash` hiçbir API yanıtında yer almaz (domain tipinde `json:"-"`).

## 3. Yetki modeli

**İzinler** (`kaynak:eylem`):

| Alan | İzinler |
|---|---|
| Cluster | `cluster:view`, `cluster:manage` (ekle/düzenle/sil) |
| Uygulama | `app:view`, `app:sync`, `app:rollback`, `app:manage` (oluştur/düzenle/sil) |
| Sistem (yalnızca global kapsam) | `user:manage`, `team:manage`, `group:manage`, `rbac:manage`, `audit:view` |

**Yerleşik roller** (ilk sürümde değiştirilemez; özel rol sonra):

| Rol | İzinler |
|---|---|
| `viewer` | `cluster:view`, `app:view` |
| `deployer` | viewer + `app:sync`, `app:rollback` |
| `maintainer` | deployer + `app:manage` |
| `cluster-admin` | `cluster:view`, `cluster:manage` |
| `admin` | `*` (her şey, her kapsam) |

**Kapsam: iki boyut.** Bir bağlama `scope.cluster` ve `scope.application` taşır. Örnekler:
- "Ekip *payments*, `deployer`, uygulama grubu *payments-apps*, cluster grubu *prod*" → yalnızca o uygulamaları yalnızca prod cluster'larda dağıtabilir.
- "Ayşe, `viewer`, cluster *any*, uygulama *any*" → her şeyi görür.
- "Mehmet, `cluster-admin`, cluster *staging-us*" → yalnızca o cluster'ı yönetir (uygulama boyutu cluster izinleri için yok sayılır).

**Karar kuralı** `Can(user, izin, hedef)`:
1. Etkin bağlamalar = kullanıcının kendi bağlamaları + üyesi olduğu ekiplerin bağlamaları. Devre dışı kullanıcı hiçbir şey yapamaz.
2. Bağlamanın rolü izni içeriyorsa (veya `*`) ve kapsamı hedefi kapsıyorsa **izin verilir**; hiçbir bağlama eşleşmezse **reddedilir**. Yasaklama yoktur.
3. Hedef türüne göre kapsam eşleşmesi:
   - cluster izinleri: yalnızca `scope.cluster` hedef cluster'ı kapsamalı;
   - uygulama izinleri, bir hedef için (`sync`, `rollback`, `diff`): hem `scope.application` uygulamayı hem `scope.cluster` o hedef cluster'ı kapsamalı;
   - `app:manage` (tanım düzenleme): uygulamanın **tüm** hedef cluster'ları kapsama girmeli (kapsam dışı bir cluster'a hedef eklenemez).
4. Liste uçları **filtreleyerek** döner: kullanıcı yalnızca görme izni olduğu cluster'ları ve uygulamaları (ve uygulamanın yalnızca izinli hedeflerini) görür.
5. Sistem izinleri yalnızca `any/any` kapsamlı bağlamayla verilebilir (doğrulanır).

**Korumalar:** son aktif admin silinemez/devre dışı bırakılamaz/rolü alınamaz; kullanıcı kendi yetkisini yükseltemez; yetki ataması yapan, atadığı izne ve kapsama kendisi de sahip olmalı (ilk sürümde `rbac:manage` yalnızca `admin` rolünde olduğundan pratikte yalnızca admin atama yapar).

## 4. Kimlik doğrulama

- **Parola:** argon2id (`golang.org/x/crypto/argon2`), kullanıcı başına tuz, asgari 12 karakter; kullanıcı adı yoksa da sabit süreli karşılaştırma (kullanıcı sayımını önlemek için aynı hata ve süre).
- **Oturum:** kısa ömürlü **access JWT** (15 dk, HS256, `sub`, `exp`; `Authorization: Bearer`, tarayıcıda yalnızca bellekte) + **refresh token** (opak, 7 gün, Mongo'da hash'li, her yenilemede döndürülür; `httpOnly`, `SameSite=Strict` çerez, yalnızca `/api/v1/auth` yolunda). Yenileme ve çıkış isteği özel bir başlık ister (CSRF savunması). Çıkış oturum kaydını siler.
- **Kaba kuvvet:** başarısız denemelerde artan gecikme ve geçici kilit (`failedLogins`, `lockedUntil`); IP bazlı hız sınırı.
- **API token'ları** (CLI/CI için): kullanıcıya bağlı, yalnızca oluşturulurken bir kez gösterilir, hash'li saklanır, kullanıcının yetkisini aşamaz.
- **İlk kurulum (bootstrap):** hiç kullanıcı yoksa server `FLUXOR_ADMIN_USERNAME` / `FLUXOR_ADMIN_PASSWORD` ile admin oluşturur; verilmediyse rastgele parola üretip **bir kez** log'a yazar; `mustChangePassword=true`.
- **Gizli anahtarlar:** `FLUXOR_JWT_SECRET` (üretimde zorunlu; dev'de yoksa rastgele üretilir ve uyarı verilir).
- **Denetim:** giriş/çıkış, başarısız giriş, kullanıcı/ekip/grup/bağlama değişiklikleri ve tüm sync/rollback işlemleri `audit_events`'e yazılır.

## 5. API (OpenAPI'ye eklenecek)

```
POST /api/v1/auth/login            POST /api/v1/auth/refresh        POST /api/v1/auth/logout
GET  /api/v1/auth/me               # kullanıcı, ekipleri, etkin izin özeti (UI menüsü için)
PUT  /api/v1/auth/password
GET/POST /api/v1/users             GET/PATCH/DELETE /api/v1/users/{id}
POST /api/v1/users/{id}/reset-password      # admin geçici parola belirler
GET/POST /api/v1/teams             GET/PATCH/DELETE /api/v1/teams/{id}      PUT /api/v1/teams/{id}/members
GET/POST /api/v1/resource-groups   GET/PATCH/DELETE ...{id}                 PUT .../{id}/members
GET  /api/v1/roles
GET/POST /api/v1/role-bindings     DELETE /api/v1/role-bindings/{id}        # ?subjectKind=&subjectId=
GET/POST/DELETE /api/v1/tokens
GET  /api/v1/audit
```
Mevcut `/clusters` uçları kimlik ister ve yukarıdaki kurallarla yetkilendirilir (`/healthz` açık kalır).
Yeni hata kodları: `401 unauthenticated`, `403 forbidden`.

## 6. Kod yapısı

```
internal/domain/        user.go team.go resource_group.go role.go binding.go   (saf tipler)
internal/authz/         permissions.go  roles.go (yerleşik)  evaluator.go  evaluator_test.go
                        → Mongo'dan bağımsız, tablo testli saf mantık
internal/auth/          password.go (argon2id)  jwt.go  sessions.go  tokens.go  bootstrap.go
internal/store/         arayüzler: UserStore TeamStore GroupStore RoleStore BindingStore
                        SessionStore TokenStore AuditStore (+ ClusterStore)
internal/store/mongo/   client.go (bağlantı, indeksler)  users.go teams.go ... clusters.go
internal/store/memory/  testler için tüm arayüzlerin bellek içi uygulaması
internal/httpapi/       authn middleware, authorize yardımcısı, users.go teams.go groups.go rbac.go auth.go audit.go
internal/config/        FLUXOR_MONGO_URI, FLUXOR_MONGO_DB, FLUXOR_JWT_SECRET, FLUXOR_ADMIN_*
dashboard/src/
  features/auth/        Login sayfası, AuthProvider (bellekte token, açılışta refresh), RequireAuth, usePermissions
  features/admin/       Kullanıcılar, Ekipler, Gruplar, Yetkiler (bağlama tablosu + form)
```
Sürücü: `go.mongodb.org/mongo-driver/v2`. Yönetim menüsü, `/auth/me` izinlerine göre görünür; asıl denetim her zaman server'dadır.

## 7. Uygulama sırası

| Adım | İçerik | Çıkış kriteri |
|---|---|---|
| **U1. Mongo temeli** | Sürücü, bağlantı + indeksler, `Store` arayüzlerinin Mongo uygulaması (önce clusters), config; testler `FLUXOR_TEST_MONGO_URI` ile (yoksa atlanır) | Cluster'lar yeniden başlatmada kalıyor |
| **U2. Kimlik** | `users`, argon2id, JWT + refresh, bootstrap admin, authn middleware, `/auth/*`, dashboard giriş sayfası + route koruması | Giriş yapmadan API 401; admin girip parola değiştirebiliyor |
| **U3. Yetki çekirdeği** | `authz` paketi (izinler, yerleşik roller, evaluator + testler), `roles`/`role_bindings`, mevcut uçlara enforcement + liste filtreleme | Kapsam dışı cluster 403/gizli; evaluator testleri yeşil |
| **U4. Ekip ve gruplar** | `teams`, `resource_groups`, üyelik uçları, ekip üzerinden yetki | Ekibe verilen rol üyelere yansıyor |
| **U5. Yönetim arayüzü** | Dashboard: Kullanıcılar, Ekipler, Gruplar, Yetkiler sayfaları, `usePermissions` ile menü/buton gizleme | Admin tüm akışı UI'dan yönetebiliyor |
| **U6. Audit ve token'lar** | `audit_events`, API token'ları, hız sınırı/kilit | Önemli işlemler audit'te; PAT ile CLI erişimi |

Sonra: Adım B (clustermgr, gerçek cluster bağlantısı), D (Application + sync). OIDC/SSO ve özel roller sonraki faz.

## 7.1 U2 uygulama notları (tasarımdan farklar ve bilinen sınırlar)
- ~~Yönetici işareti yok~~ → **U3'te çözüldü** (aşağıdaki §7.2).
- ~~Kullanıcı CRUD API'si yok~~ → **U3'te eklendi** (silme yok; hesaplar devre dışı bırakılır).
- **Hesap kilidi U2'de yapıldı** (5 yanlış denemede 15 dk; sayaç başarılı girişte sıfırlanır; kilitli/devre dışı hesap da genel "hatalı kullanıcı adı veya parola" hatasını alır). ~~IP bazlı hız sınırı U6'da~~ → **U6'da yapıldı** (§7.5).
- **Access token'lar iptal edilemez, 15 dk içinde kendiliğinden biter.** Çıkış ve parola değişimi yalnızca refresh oturumlarını siler. Hesabı devre dışı bırakmak ise anında etkilidir (kullanıcı her istekte yüklenir).
- ~~Yenileme belirteci yeniden kullanımı: aile iptali yok~~ → **U6'da yapıldı** (§7.5).
- **CSRF:** çerezle yapılan `refresh` ve `logout` çağrıları `X-Fluxor-Request` başlığı ister; çerez `SameSite=Strict`, `HttpOnly`, yalnızca `/api/v1/auth` yoluna gider.
- **Güvenlik:** parola sınırı 12–256 karakter; bilinmeyen kullanıcıda da argon2 doğrulaması yapılır (zamanlama farkını azaltmak için); `X-Forwarded-For` güvenilmez, oturum IP'si doğrudan bağlantıdan alınır.

## 7.2 U3 uygulama notları
- **Yetki kararı (`authz.Access.Can`)** saf bir fonksiyondur ve Mongo'dan bağımsızdır; sınıflar: *cluster* izinleri yalnızca bağlamanın cluster boyutuna, *uygulama* izinleri hem uygulama hem cluster boyutuna bakar, *sistem* izinleri (`user:manage`, `rbac:manage`, ...) yalnızca kapsamı hiç daraltılmamış (any/any) bağlamayla verilir. Bilinmeyen izin ve bilinmeyen rol hiçbir şey vermez.
- **Henüz var olmayan kaynak** (yeni cluster kaydı) boş `ClusterID` ile sorulur; yalnızca "tüm cluster'lar" kapsamı bunu karşılar. `app:manage` gibi uygulama tanımı işlemleri için çağıran, uygulamanın **her hedef cluster'ı** için ayrı ayrı sormalıdır (uygulama kaynağı geldiğinde).
- **Cluster uçları:** liste filtrelenir (yetkisiz kullanıcı boş liste görür, hata değil); görme izni olmayan cluster 404 döner ve var olmayanla birebir aynı gövdeyi taşır; görüp yönetemeyen 403 alır. Cluster silinince ona bağlı bağlamalar da silinir.
- **Bağlama doğrulama** (`ValidateBinding`): rol var olmalı, kullanıcı ve cluster var olmalı, `admin` daraltılamaz, uygulama izni olmayan rol (`cluster-admin`) uygulama kapsamıyla sınırlanamaz. **Ekip, kaynak grubu ve uygulama kapsamı U4'e kadar reddedilir** (`teamsAvailable`, `groupsAvailable`, `applicationsAvailable` bayrakları `authz/service.go`'da; evaluator grup referanslarını zaten destekliyor ve sahte üyelikle test edildi).
- **Aynı yetki iki kez verilemez** (kullanıcı+rol+kapsam benzersiz `key` ile).
- **Yönetici bağlaması:** başlangıçta hiç `admin` bağlaması yoksa `createdBy: bootstrap` olan en eski kullanıcı `admin`e bağlanır (sabit ID `bootstrap-admin`, çoklu kopyada güvenli). Bu, U2'den kalan veritabanlarını da onarır. Yönetici varken bir şey vermez.
- **Korumalar:** son aktif yöneticinin admin bağlaması silinemez (409 `last_admin`; devre dışı yöneticiler sayılmaz); yönetici kendi hesabını devre dışı bırakamaz ve kendi parolasını sıfırlayamaz (kendi parolası için `/auth/password`). Hesabı devre dışı bırakırken "son yönetici" kontrolü **bilerek yok**: `user:manage` yalnızca aktif yöneticide olduğundan ve kendini devre dışı bırakamadığından hedef hiçbir zaman son yönetici olamaz; ekip veya özel rol `user:manage` verebildiği anda `AdminUserIDs` ile eklenmeli.
- **Bilinen sınır:** son-yönetici kontrolü okuma-sonra-yazma yapar (TOCTOU); iki yönetici aynı anda birbirinin rolünü silerse ikisi de geçebilir. Tek bir yönetici varken bu yarış mümkün değildir.
- **Dashboard:** yetkisiz işlemler Türkçe hata verir; düğmeleri/menüyü izne göre gizleme U5'te (`/auth/me` izin özeti ile).

## 7.3 U4 uygulama notları
- **Ekipler:** kullanıcı kümeleridir; bir ekibe verilen rol her üyeye geçer. Üyelik `PUT /teams/{id}/members` ile **tümüyle değiştirilir** (`members` alanı zorunlu: yazım hatası ekibi sessizce boşaltamaz; `[]` herkesi çıkarır). Üyeler var olan kullanıcılar olmalı, tekrarlar atılır, liste sıralı saklanır (en çok 1000).
- **Kaynak grupları:** şimdilik yalnızca **cluster grupları** (`kind: cluster`); üyeler var olan cluster'lardır. **Uygulama grupları reddedilir** (`authz.ApplicationsAvailable = false`), uygulamalar gelince açılır. Ad kuralı: 1–64 karakter, baş/son boşluk ve denetim karakteri yok; **ad büyük/küçük harfe duyarsız benzersiz** (ekipte genel, grupta tür başına).
- **Kapsamda grup:** bağlamanın cluster boyutu artık `group` olabilir ("payments ekibi, deployer, *prod* cluster grubu"). Evaluator grubun **türünü** denetler: bir uygulama grubu, kimlikleri eşleşse bile cluster erişimi veremez. Silinmiş grup hiçbir şeyle eşleşmez.
- **Anında etki:** `Authorizer.For()` her istekte kullanıcının ekiplerini ve bağlamaların işaret ettiği grupları okur; üyelik veya grup değişikliği bir sonraki istekte geçerlidir. Önbellek yoktur (ölçek gerekirse eklenir).
- **Yönetici sayımı ekipleri kapsar:** `admin` rolü bir ekibe (any/any) verilmişse üyeleri yönetici sayılır (devre dışı olanlar sayılmaz). `AdminUserIDs(AdminChange{...})` bekleyen bir değişikliğin (silinen bağlama, değişen/boşaltılan ekip) sonucunu önceden hesaplar; `WouldLeaveNoAdmin` yönetici zaten yoksa `false` döner, yani bozuk bir durum onarılabilir.
- **Korunan işlemler (409 `last_admin`):** yönetici bağlamasını silmek, yönetici ekibin üyelerini değiştirmek/boşaltmak, yönetici ekibi silmek. Ekibin başka bir *etkin* yöneticiyle değiştirilmesi serbesttir.
- **Temizlik:** ekip silinince ona verilen bağlamalar, grup silinince ona bağlı bağlamalar, cluster silinince hem bağlamalar hem grup üyelikleri (`$pull`) silinir. Bu temizlikler en iyi çabadır (hata loglanır); ilgili kayıtlar zaten hiçbir şeyle eşleşmediğinden yetki açısından zararsızdır.
- **Bilinen sınırlar:** üyelik yazımı "oku–doğrula–yaz" olduğundan son-yönetici kontrolünde §7.2'deki yarış (TOCTOU) ekipler için de geçerlidir. Kullanıcı listesi doğrulaması için `ListUsers` tek sorguyla tüm kullanıcıları okur (binlerce kullanıcıda sayfalama gerekir). Ekip/grup okuma (`GET`) da yönetici izni ister; kullanıcının kendi ekiplerini görmesi `/auth/me` ile U5'te.

## 7.4 U5 uygulama notları
- **İzin özeti (`/auth/me`, login/refresh/parola yanıtları):** `teams` (üyesi olduğu ekipler), `permissions` (sahip olduğu **sistem** izinleri: `user:manage`, `team:manage`, `group:manage`, `rbac:manage`, `audit:view`) ve `canCreateClusters`. Cluster/uygulama izinleri kaynak başına söylenir: cluster yanıtlarındaki `permissions` (`cluster:view`, `cluster:manage`). Bu alanlar **yalnızca arayüzde bir şeyi göstermek/gizlemek içindir**; her istek sunucuda yeniden denetlenir.
- **Arayüz:** menüde yalnızca izin verilen sayfalar görünür; gizli bir sayfanın adresi elle yazılırsa 403 sayfası çıkar (`RequirePermission`). Düğmeler izne göre gizlenir (cluster ekle/sil). Ekip ve grup üyeleri seçilirken kullanıcı listesi `user:manage`, cluster listesi görme izni ister; yönetici dışında bu izinleri taşıyan rol yoktur.
- **Taze kalma:** `useMe` 60 sn'de bir ve pencere görünür olunca yenilenir. İzin özeti değişirse (`permissionSignature`: sistem izinleri, cluster oluşturma hakkı, ekip kimlikleri) diğer tüm sorgular yeniden çekilir (`usePermissionSync`), böylece yetki verilince/alınınca açık sayfadaki liste ve düğmeler yenileme gerekmeden güncellenir. Gecikme en fazla ~60 sn'dir; bu sürede eski bir düğmeye basılırsa sunucu 403/404 verir. Anlık güncelleme (SSE) U6 sonrasına kalır.
- **Geçici parola:** kullanıcı oluşturma ve parola sıfırlamada üretilen parola yalnızca bir diyalogda gösterilir; kapatınca tekrar görülemez.
- **Rol adları** arayüzde Türkçeleştirilir (`model.ts`), bilinmeyen bir rol sunucunun metniyle gösterilir.
- **Bilinen sınırlar:** uygulama kapsamı ve uygulama grupları arayüzde kapalı (uygulamalar gelince açılacak). Kullanıcı, ekip ve atama tabloları sayfalanmıyor (yüzlerce kayıtta gerekir). Arayüz akışları (`Select` seçimi, modal'lar) için otomatik tarayıcı testi yok; yalnızca saf mantık vitest ile, akışlar elle doğrulandı.

## 7.5 U6 uygulama notları
**Denetim günlüğü**
- **Ne kaydedilir:** her *değiştiren* rota (`s.handle(mux, desen, işlem, konuTürü, işleyici)` ile işlem adına bağlanır): giriş, parola değişikliği, kullanıcı/ekip/grup/yetki/cluster/token oluşturma-güncelleme-silme, üyelik değişikliği. **Okumalar kaydedilmez.** Yeni bir değiştiren rota eklerken `s.handle` ile kaydedilmesi unutulmamalı (dashboard testindeki işlem listesi de güncellenmeli).
- **Ne zaman yazılır:** yanıt belli olduktan sonra, tek ara katmanda (`audit`). Sonuç HTTP durumundan türer: 2xx/3xx `success`; 401/403/429 `denied`; diğer 4xx `failure`; 5xx `error`.
- **Kaydedilmeyenler (bilerek):** oturum açılmamış istekler (401; herkes yapabilir, günlüğü doldurur) ve hız sınırına takılanlar (429; aynı nedenle). Bunlar yalnızca sunucu günlüğüne (`slog`) düşer. Oturum açmış bir kullanıcının yetkisiz denemesi (403) kaydedilir.
- **Başarısız giriş:** olayın `actorName`'i denenen ad, `actorId` boş, `details.reason` gerçek neden (`unknown_user`, `bad_password`, `locked`, `disabled`). İstemci hepsi için aynı yanıtı alır; neden yalnızca günlüktedir.
- **Güvenlik olayı `auth.refresh_reuse`:** kullanılmış bir refresh token 10 sn'den sonra tekrar gelirse yazılır; `actor` sızdırılan oturumun sahibidir.
- **Sır yok:** parola, token, özet (hash) ve erişim belirteci hiçbir olaya girmez (testle sınanır, Mongo belgeleri de elle tarandı). `details` yalnızca kısa gerçekler taşır (`role`, `to`, `scope`, `fields`, `members`, `via: api_token`, ...).
- **Adlar olayda kalır:** silinen kaynağın adı (`subjectName`) olaya yazılır; sonradan aranamaz.
- **Sıralama:** olay ID'leri zamana göre sıralanabilir (nanosaniye + rastgele kuyruk); `(time, id)` sırası, aynı milisaniyedeki olayları da olduğu sırayla verir. Sayfalama imleci `(time, id)` çiftidir.
- **Yazma hatası isteği bozmaz:** günlük yazılamazsa hata sunucu günlüğüne yazılır, yanıt değişmez (erişilebilirlik bütünlüğe tercih edildi). Bir saklama süresi yok; büyürse Mongo'da `audit_events.time` üzerine TTL indeksi eklenebilir.

**API token'ları**
- Biçim `flx_` + 43 karakter (256 bit rastgele). Yalnızca SHA-256 özeti saklanır (yüksek entropili olduğundan yavaş özet gerekmez); ilk 8 karakter listede tanımak için tutulur. Sır yalnızca oluşturulurken bir kez döner.
- **Sahibinin tüm yetkileriyle** çalışır, kapsam (scope) yoktur: yetkiler her istekte yeniden okunur. Süre 1–365 gün (varsayılan 30), kullanıcı başına en çok 20. Süresi dolanlar Mongo TTL indeksiyle silinir.
- **Token yapamaz:** token oluşturma/listeleme/silme ve parola değiştirme yalnızca *oturumla* yapılır (çalınan bir token kendine devam eden yeni bir token üretemesin).
- **İptal:** kullanıcı devre dışı bırakılınca ve yönetici parola sıfırlayınca token'lar silinir (tekrar etkinleştirmek eskilerini geri getirmez); kullanıcı kendi parolasını değiştirince korunur (CI kırılmasın). Son kullanım dakikada en çok bir kez yazılır.
- Bir başkasının token'ı, var olmayan gibi 404 verir ve adı günlükten sızdırılmaz.

**Hız sınırı ve istemci adresi**
- Giriş (varsayılan 10/dk) ve yenileme (60/dk) için adres başına jeton kovası: ilk anda o kadar deneme, sonra dakikada o kadar. Aşılırsa `429 rate_limited` + `Retry-After`. Limit hesap kilidinden *önce* uygulanır (doğru parola da sınırlanır). `FLUXOR_RATE_LOGIN_PER_MIN` / `FLUXOR_RATE_REFRESH_PER_MIN`; `0` sınırı kapatır.
- **Adres:** `X-Forwarded-For` yalnızca doğrudan bağlanan eş `FLUXOR_TRUSTED_PROXIES` içindeyse dikkate alınır (sağdan başlayıp güvenilir olmayan ilk adres). Aksi halde istemci başlıkla adres seçebilir, hız sınırını aşar ve günlüğe sahte adres yazdırırdı. Proxy arkasında bu ayar **verilmezse** herkes proxy'nin adresinden gelmiş görünür ve tek kova paylaşılır.
- Kovalar bellektedir (süreç başına): birden çok sunucu kopyasında limit kopya başınadır.

**Refresh token ailesi**
- Her giriş yeni bir *aile* başlatır, yenilemeler aynı ailede kalır. Bir token kullanılınca silinmez, `usedAt` ile işaretlenir (Mongo'da tek atomik güncelleme: eşzamanlı 6 yenilemeden 1'i kazanır).
- Kullanılmış token **10 sn içinde** tekrar gelirse (iki sekme aynı çerezle yarıştı) yalnızca reddedilir. **Sonra** gelirse sızdı demektir: ailenin tüm oturumları silinir ve `auth.refresh_reuse` yazılır. Aynı kullanıcının başka girişleri (başka aile) etkilenmez. Süresi dolmuş eski token hırsızlık sayılmaz.
- Dashboard: oturum varken yenileme 401 alırsa bir kez 400 ms sonra yeniden dener (yarışan sekme çerezi bu arada değiştirmiş olabilir).
- Bilinen sınır: ailenin silinmesi hırsızı ve meşru kullanıcıyı birlikte çıkarır (meşru kullanıcı yeniden giriş yapar); hangisinin ilk kullandığı anlaşılamaz. Mevcut erişim belirteçleri (15 dk) kendiliğinden biter.

**Bilinen sınırlar:** denetim sayfasında kullanıcı seçici tüm kullanıcıları yükler (sayfalama yok); yönetici, başka kullanıcıların token'larını listeleyemez/silemez (devre dışı bırakma ve parola sıfırlama dolaylı yoldur).

## 8. Geliştirme ortamı notu
Docker olmadan çalışmak için yerel `mongod` gerekir (Homebrew: `mongodb/brew` tap'i, `mongodb-community`). U1'e başlamadan kurulacak.
Birim testleri (evaluator, parola, JWT, httpapi) memory store ile Mongo'suz çalışır.

## 9. Açık noktalar
- Özel rol oluşturma (UI'dan izin seçerek) hangi fazda? (öneri: U5 sonrası)
- Grup üyeliği: yalnızca statik liste mi, label selector ile dinamik grup da olsun mu? (öneri: önce statik)
- Parola sıfırlama e-postası gerekmiyor (admin geçici parola verir); e-posta altyapısı sonraki faza.
