import { defaultLocale, isLocale, localizePath, locales, type Locale } from "@/i18n/routing";

// Публичный адрес сайта для canonical, hreflang, sitemap и OpenGraph.
export function siteUrl(): string {
  return (process.env.NEXT_PUBLIC_SITE_URL ?? "http://localhost:3000").replace(/\/+$/, "");
}

/** Абсолютный адрес страницы на языке: ru — без префикса, uz — /uz/... */
export function localeUrl(path: string, locale: Locale): string {
  return siteUrl() + localizePath(path, locale);
}

export function localeAlternates(path: string) {
  const languages: Record<string, string> = {};
  for (const l of locales) languages[l] = localeUrl(path, l);
  languages["x-default"] = localeUrl(path, defaultLocale);
  return { canonical: undefined as string | undefined, languages };
}

export function alternates(locale: string, path: string) {
  const a = localeAlternates(path);
  a.canonical = localeUrl(path, isLocale(locale) ? locale : defaultLocale);
  return a;
}
