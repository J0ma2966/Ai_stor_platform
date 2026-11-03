// Языки сайта и правила адресов.
//
// Русский — язык по умолчанию и живёт без префикса (/catalog), узбекский — под /uz (/uz/catalog).
// Префикс нужен поисковикам и ссылкам «поделиться»; в файловой структуре app/ сегмента [locale]
// нет — middleware переписывает /uz/... в /... и передаёт язык заголовком. Поэтому при смене языка
// дерево страниц Next.js остаётся тем же: страница переводится на месте, без новой страницы,
// без потери введённых данных и прокрутки (см. useSwitchLocale).

export const locales = ["ru", "uz"] as const;
export type Locale = (typeof locales)[number];
export const defaultLocale: Locale = "ru";

/** Cookie с выбранным языком: middleware по ней возвращает человека на его язык. */
export const LOCALE_COOKIE = "NEXT_LOCALE";
/** Заголовок запроса, через который middleware сообщает язык серверным компонентам. */
export const LOCALE_HEADER = "x-locale";

export function isLocale(v: unknown): v is Locale {
  return typeof v === "string" && (locales as readonly string[]).includes(v);
}

/** Язык из префикса адреса: "/uz/catalog" → "uz", "/catalog" → null. */
export function prefixLocale(pathname: string): Locale | null {
  const seg = pathname.split("/")[1];
  return isLocale(seg) ? seg : null;
}

/** Язык, который показывает адрес: префикс или язык по умолчанию. */
export function localeFromPath(pathname: string): Locale {
  return prefixLocale(pathname) ?? defaultLocale;
}

/** Убирает языковой префикс: "/uz/catalog" → "/catalog", "/uz" → "/". */
export function stripLocale(pathname: string): string {
  const l = prefixLocale(pathname);
  if (!l) return pathname || "/";
  const rest = pathname.slice(l.length + 1);
  return rest === "" ? "/" : rest;
}

/**
 * Внутренний адрес для языка: localizePath("/catalog?q=1", "uz") → "/uz/catalog?q=1".
 * Внешние ссылки, якоря и относительные адреса не трогает.
 */
export function localizePath(href: string, locale: Locale): string {
  if (!href.startsWith("/") || href.startsWith("//")) return href;
  const i = href.search(/[?#]/);
  const path = stripLocale(i < 0 ? href : href.slice(0, i));
  const tail = i < 0 ? "" : href.slice(i);
  if (locale === defaultLocale) return path + tail;
  return `/${locale}${path === "/" ? "" : path}` + tail;
}
