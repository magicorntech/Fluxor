import { describe, expect, it } from 'vitest'
import { ApiError, toError } from './errors'

const envelope = (code: string, message: string) => ({ error: { code, message } })

describe('toError', () => {
  it('shows known server codes in Turkish, whatever the server wrote', () => {
    expect(toError(envelope('invalid_credentials', 'invalid username or password')).message).toBe('Kullanıcı adı veya parola hatalı')
    expect(toError(envelope('forbidden', 'you do not have permission')).message).toBe('Bu işlem için yetkiniz yok')
    expect(toError(envelope('last_admin', 'x')).message).toContain('etkin bir yönetici')
  })
  it('explains rate limiting and the token limit', () => {
    expect(toError(envelope('rate_limited', 'too many attempts, try again in 7 seconds')).message).toContain('Çok fazla deneme')
    expect(toError(envelope('limit_reached', 'x')).message).toContain('20 API token')
  })
  it('keeps the code, so callers can react to it', () => {
    const e = toError(envelope('invalid_current_password', 'x'))
    expect(e).toBeInstanceOf(ApiError)
    expect(e.code).toBe('invalid_current_password')
  })
  it("falls back to the server's own text for codes without a translation (validation details)", () => {
    expect(toError(envelope('invalid_argument', 'name must be 1-63 characters')).message).toBe('name must be 1-63 characters')
  })
  it('copes with a response that is not an error envelope', () => {
    for (const odd of [undefined, null, {}, { error: {} }, 'oops', 42]) {
      const e = toError(odd)
      expect(e.code).toBe('unknown')
      expect(e.message).toBe('Beklenmeyen bir hata oluştu')
    }
  })
})
