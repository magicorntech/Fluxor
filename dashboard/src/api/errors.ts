// Error type for failed API calls. `code` is the machine-readable code from the server's
// error envelope (e.g. "invalid_credentials"); `message` is meant to be shown to the user.
export class ApiError extends Error {
  code: string

  constructor(code: string, message: string) {
    super(message)
    this.name = 'ApiError'
    this.code = code
  }
}

// The server's messages are in English; known codes get a Turkish message here. Anything else
// (validation details such as "name must be ...") falls back to the server's own text.
const messages: Record<string, string> = {
  invalid_credentials: 'Kullanıcı adı veya parola hatalı',
  invalid_current_password: 'Mevcut parola hatalı',
  unauthenticated: 'Oturumunuz sona erdi, lütfen tekrar giriş yapın',
  password_change_required: 'Devam etmeden önce parolanızı değiştirmelisiniz',
  forbidden: 'Bu işlem için yetkiniz yok',
  not_found: 'Kayıt bulunamadı',
  already_exists: 'Bu kayıt zaten var',
  last_admin: 'Bu işlem sistemde etkin bir yönetici bırakmayacağı için yapılamaz',
  rate_limited: 'Çok fazla deneme yapıldı, biraz bekleyip tekrar deneyin',
  limit_reached: 'En fazla 20 API token oluşturabilirsiniz; önce birini silin',
  internal: 'Sunucuda bir hata oluştu',
}

export function toError(error: unknown): ApiError {
  const e = (error as { error?: { code?: string; message?: string } } | undefined)?.error
  const code = e?.code ?? 'unknown'
  return new ApiError(code, messages[code] ?? e?.message ?? 'Beklenmeyen bir hata oluştu')
}
