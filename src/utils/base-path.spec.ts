import { Request } from 'express'
import { injectBasePath, normalizeBasePath, resolveBasePath } from './base-path'

describe('normalizeBasePath', () => {
  it('defaults empty / missing / root inputs to "/"', () => {
    expect(normalizeBasePath(undefined)).toBe('/')
    expect(normalizeBasePath('')).toBe('/')
    expect(normalizeBasePath('   ')).toBe('/')
    expect(normalizeBasePath('/')).toBe('/')
  })

  it('adds a leading and trailing slash', () => {
    expect(normalizeBasePath('gpu-ui')).toBe('/gpu-ui/')
    expect(normalizeBasePath('/gpu-ui')).toBe('/gpu-ui/')
    expect(normalizeBasePath('gpu-ui/')).toBe('/gpu-ui/')
    expect(normalizeBasePath('/gpu-ui/')).toBe('/gpu-ui/')
    expect(normalizeBasePath('/a/b/c')).toBe('/a/b/c/')
  })

  it('trims whitespace and collapses duplicate slashes', () => {
    expect(normalizeBasePath('  /gpu-ui  ')).toBe('/gpu-ui/')
    expect(normalizeBasePath('//gpu-ui//')).toBe('/gpu-ui/')
  })

  it('takes the first value of an array header', () => {
    expect(normalizeBasePath(['/hdr', '/other'])).toBe('/hdr/')
  })

  it('strips characters that could break out of the HTML/JS context', () => {
    const out = normalizeBasePath('/gpu-ui"><script>alert(1)</script>')
    expect(out).not.toMatch(/["'<>]/)
  })
})

describe('resolveBasePath', () => {
  const req = (headers: Record<string, unknown>) =>
    ({ headers } as unknown as Request)

  it('prefers the X-Forwarded-Prefix header when present', () => {
    expect(resolveBasePath(req({ 'x-forwarded-prefix': '/gpu-ui' }))).toBe(
      '/gpu-ui/'
    )
  })

  it('falls back to the environment default (root by default)', () => {
    expect(resolveBasePath(req({}))).toBe('/')
  })
})

describe('injectBasePath', () => {
  const html =
    '<!DOCTYPE html><html><head><base href="/"/><title>HAMi</title></head><body></body></html>'

  it('injects <base> and window.__BASE_PATH__ for a sub-path', () => {
    const out = injectBasePath(html, '/gpu-ui/')
    expect(out).toContain('<base href="/gpu-ui/">')
    expect(out).toContain('window.__BASE_PATH__="/gpu-ui/"')
    // the original hard-coded root <base> must be gone
    expect(out).not.toContain('<base href="/"/>')
  })

  it('is idempotent-safe at the site root', () => {
    const out = injectBasePath(html, '/')
    expect(out).toContain('<base href="/">')
    expect(out).toContain('window.__BASE_PATH__="/"')
  })

  it('injects immediately after <head> so the base precedes asset refs', () => {
    const out = injectBasePath(html, '/gpu-ui/')
    expect(out.indexOf('<base href="/gpu-ui/">')).toBeLessThan(
      out.indexOf('<title>')
    )
  })
})
