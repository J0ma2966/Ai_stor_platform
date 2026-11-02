// Разрешаем возврат только на свои внутренние пути (защита от open redirect).
export function safeNext(next: string | null, fallback: string): string {
  if (!next || !next.startsWith("/") || next.startsWith("//") || next.startsWith("/\\") || /[\r\n\t]/.test(next)) return fallback;
  return next;
}
