// Тема сайта: как в системе (по умолчанию), светлая или тёмная — как в мобильных приложениях.
// Выбор хранится в cookie, чтобы сервер сразу отдал <html data-theme> и страница не мигала при загрузке.
export const THEME_COOKIE = "theme";
export const THEMES = ["system", "light", "dark"] as const;
export type Theme = (typeof THEMES)[number];

export function parseTheme(v: string | undefined | null): Theme {
  return v === "light" || v === "dark" ? v : "system";
}

/** data-theme для <html>: у «как в системе» атрибута нет, работает prefers-color-scheme. */
export function themeAttr(t: Theme): "light" | "dark" | undefined {
  return t === "system" ? undefined : t;
}

export function themeCookie(t: Theme): string {
  const age = t === "system" ? 0 : 60 * 60 * 24 * 365;
  return `${THEME_COOKIE}=${t === "system" ? "" : t}; path=/; max-age=${age}; samesite=lax`;
}

export function applyTheme(t: Theme, doc: Document = document): void {
  const a = themeAttr(t);
  if (a) doc.documentElement.dataset.theme = a;
  else delete doc.documentElement.dataset.theme;
  doc.cookie = themeCookie(t);
}
