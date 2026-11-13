// Content-Security-Policy с nonce на каждый запрос.
// Скрипты: только с nonce (Next проставляет его своим скриптам) + 'strict-dynamic'.
// 'unsafe-inline' оставлен только для стилей.

function origins(v: string | undefined, def: string): string {
  return (v ?? def)
    .split(/[\s,;]+/)
    .map((s) => s.trim())
    .filter((s) => /^https?:\/\/[^\s;'"]+$/.test(s))
    .join(" ");
}

export function buildCsp(nonce: string, dev = process.env.NODE_ENV === "development"): string {
  const api = origins(process.env.NEXT_PUBLIC_API_URL, "http://localhost:8080");
  // список через запятую: публичный CDN картинок товаров + хранилище приватных фото
  const media = origins(process.env.NEXT_PUBLIC_MEDIA_ORIGIN, "http://localhost:9000");
  return [
    "default-src 'self'",
    // Google Identity Services (кнопка «Войти с Google» на /login)
    `connect-src 'self' ${api} https://accounts.google.com/gsi/`,
    "frame-src https://accounts.google.com/gsi/",
    `img-src 'self' data: blob: ${media}`,
    `script-src 'self' 'nonce-${nonce}' 'strict-dynamic'${dev ? " 'unsafe-eval'" : ""}`,
    "style-src 'self' 'unsafe-inline' https://accounts.google.com/gsi/style",
    "font-src 'self'",
    "object-src 'none'",
    "frame-ancestors 'none'",
    "base-uri 'self'",
    "form-action 'self'",
  ].join("; ");
}

export function newNonce(): string {
  const b = new Uint8Array(16);
  crypto.getRandomValues(b);
  return btoa(String.fromCharCode(...b));
}
