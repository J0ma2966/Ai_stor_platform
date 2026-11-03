// Публичный адрес сайта для canonical, hreflang, sitemap и OpenGraph.
export function siteUrl(): string {
  return (process.env.NEXT_PUBLIC_SITE_URL ?? "http://localhost:3000").replace(/\/+$/, "");
}

export function localeAlternates(path: string) {
  const base = siteUrl();
  const p = path === "/" ? "" : path;
  return {
    canonical: undefined as string | undefined,
    languages: { ru: `${base}/ru${p}`, uz: `${base}/uz${p}`, "x-default": `${base}/ru${p}` },
  };
}

export function alternates(locale: string, path: string) {
  const a = localeAlternates(path);
  a.canonical = `${siteUrl()}/${locale}${path === "/" ? "" : path}`;
  return a;
}
