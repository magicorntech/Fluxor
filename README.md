# Fluxor

Ortam ve akış-merkezli, açık kaynak GitOps sürekli dağıtım aracı.
ArgoCD'nin yaptığı işi yapar; farkı: promosyon pipeline'ları, onboarding sihirbazı, zaman yolculuğu, Git write-back ve filo görünümü.

Repo: https://github.com/magicorntech/Fluxor

Durum: erken aşama (Faz 0). Yol haritası için [ROADMAP.md](ROADMAP.md).

Yığın: Go (backend, controller, CLI) + React/TypeScript + [Ant Design](https://ant.design/) 6.6.7 (UI).

Geliştirme (server ve dashboard ayrı süreçlerdir, iki terminal kullanın):

```bash
make dev-server          # API: http://localhost:8080, yerel MongoDB'ye bağlı, kalıcı geliştirme anahtarıyla (hack/dev-server.sh)
make server              # API: http://localhost:8080 (bellek store'u, veri kalıcı değil)
FLUXOR_MONGO_URI=mongodb://localhost:27017 make server   # MongoDB ile, veri kalıcı
make dashboard-install   # ilk seferde
make dashboard           # UI:  http://localhost:5173 (/api isteklerini server'a proxy'ler)
make test                # Go testleri + dashboard birim testleri + build
make test-mongo          # store testleri gerçek MongoDB'ye karşı
make api-gen             # api/openapi.yaml değişince TS tiplerini yeniden üret
```

### İlk çalıştırma ve giriş

Veritabanı boşken server ilk yöneticiyi (`admin`) oluşturur. `FLUXOR_ADMIN_PASSWORD` verilmezse rastgele bir parola
üretip **yalnızca bir kez** log'a yazar. İlk girişte parolanın değiştirilmesi zorunludur.

| Değişken | Anlamı | Varsayılan |
|---|---|---|
| `FLUXOR_ADDR` | Dinleme adresi | `:8080` |
| `FLUXOR_MONGO_URI` / `FLUXOR_MONGO_DB` | MongoDB bağlantısı / veritabanı adı. URI yoksa bellek store'u kullanılır | – / `fluxor` |
| `FLUXOR_JWT_SECRET` | Access token imza anahtarı (≥ 32 bayt). **Üretimde zorunlu**; yoksa her açılışta rastgele üretilir | – |
| `FLUXOR_ADMIN_USERNAME` / `FLUXOR_ADMIN_PASSWORD` | İlk yönetici (yalnızca kullanıcı yoksa) | `admin` / üretilir |
| `FLUXOR_COOKIE_SECURE` | Refresh çerezini yalnızca HTTPS'e bağla; HTTPS arkasında `true` olmalı | `false` |
| `FLUXOR_CORS_ORIGINS` | Dashboard farklı origin'deyse izinli origin'ler (virgülle) | – |
| `FLUXOR_TRUSTED_PROXIES` | Önünüzde reverse proxy varsa adresleri/CIDR'leri (virgülle). Yalnızca bunlardan gelen `X-Forwarded-For` dikkate alınır; **proxy arkasında verilmezse hız sınırı herkes için tek kova olur** | – |
| `FLUXOR_RATE_LOGIN_PER_MIN` / `FLUXOR_RATE_REFRESH_PER_MIN` | Adres başına giriş / yenileme denemesi sınırı (`0` = sınırsız) | `10` / `60` |

### Yetkiler

Yetki, **rol bağlamalarıyla** verilir: bir kullanıcıya, bir rol, bir kapsamda (tüm cluster'lar ya da belirli bir cluster). Yetkiler toplanır, yasaklama yoktur.
Roller: `viewer`, `deployer`, `maintainer`, `cluster-admin`, `admin`. İlk hesap otomatik olarak `admin` olur.
Rol bir kullanıcıya ya da bir **ekibe** (tüm üyeler alır) verilir; kapsam tüm cluster'lar, tek bir cluster ya da bir **cluster grubu** olabilir.
Kullanıcı, ekip, grup ve yetki yönetimi dashboard'daki **Yönetim** menüsünden yapılır (yalnızca yönetici görür); aynı işlemler API'de de var (`/api/v1/users`, `/teams`, `/resource-groups`, `/role-bindings`).

### Denetim ve API token'ları

Her değişiklik (ve oturum açmış bir kullanıcının reddedilen her denemesi) **Yönetim → Denetim** sayfasında kayıtlıdır; kayıtlarda sır bulunmaz.
Betikler için **API token'larım** sayfasından token oluşturabilirsiniz; token, sahibinin yetkileriyle çalışır:

```bash
curl -H "Authorization: Bearer flx_..." http://localhost:8080/api/v1/clusters
```

Mimari: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) · Kullanıcı ve yetki: [docs/AUTH.md](docs/AUTH.md)
