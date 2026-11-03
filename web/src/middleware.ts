import { NextResponse, type NextRequest } from "next/server";
import { defaultLocale, isLocale, LOCALE_COOKIE, LOCALE_HEADER, localizePath, prefixLocale, stripLocale, type Locale } from "./i18n/routing";
import { buildCsp, newNonce } from "./lib/csp";

// Язык по Accept-Language — только при первом заходе и только если узбекский стоит первым.
function preferredLocale(req: NextRequest): Locale | null {
  const first = (req.headers.get("accept-language") ?? "").split(",")[0]?.trim().toLowerCase() ?? "";
  return first.startsWith("uz") ? "uz" : null;
}

function withCsp(res: NextResponse, csp: string) {
  res.headers.set("Content-Security-Policy", csp);
  return res;
}

export default function middleware(req: NextRequest) {
  const { pathname, search } = req.nextUrl;
  const nonce = newNonce();
  const csp = buildCsp(nonce);
  const prefix = prefixLocale(pathname);

  // Старые адреса /ru/... — навсегда на адрес без префикса (русский — язык по умолчанию).
  if (prefix === defaultLocale) {
    const url = req.nextUrl.clone();
    url.pathname = stripLocale(pathname);
    const res = NextResponse.redirect(url, 308);
    // явная ссылка на русскую версию — это выбор языка: иначе cookie=uz вернула бы на /uz/...
    res.cookies.set(LOCALE_COOKIE, defaultLocale, { path: "/", maxAge: 31536000, sameSite: "lax" });
    return withCsp(res, csp);
  }

  const saved = req.cookies.get(LOCALE_COOKIE)?.value;
  // Запрос роутера Next.js (fetch из открытой вкладки), а не открытие страницы: заголовок RSC
  // middleware не видит, поэтому смотрим на Sec-Fetch-Dest (fetch → "empty", страница → "document").
  const isRsc = req.headers.get("sec-fetch-dest") === "empty";
  let locale: Locale = prefix ?? defaultLocale;
  if (isRsc && isLocale(saved)) {
    // Переходы внутри открытой вкладки (RSC-запросы роутера) идут на языке, который сейчас выбран
    // во вкладке: переключатель и <LocaleSync> держат cookie в актуальном состоянии. Так все
    // догрузки страницы после смены языка, включая служебные перезапросы Next.js по старому адресу,
    // приходят на одном языке.
    locale = saved;
  } else if (!prefix) {
    // Адрес без префикса: возвращаем человека на выбранный им язык. У поисковых роботов cookie нет,
    // поэтому они всегда видят русскую страницу по этому адресу и узбекскую — по /uz/...
    const want = isLocale(saved) ? saved : preferredLocale(req);
    if (want && want !== defaultLocale && !isRsc) {
      const url = req.nextUrl.clone();
      url.pathname = localizePath(pathname, want);
      return withCsp(NextResponse.redirect(url, 307), csp);
    }
    locale = defaultLocale;
  }

  // Next читает nonce из CSP-заголовка запроса и ставит его на свои <script>.
  const headers = new Headers(req.headers);
  headers.set("x-nonce", nonce);
  headers.set("Content-Security-Policy", csp);
  headers.set(LOCALE_HEADER, locale);

  let res: NextResponse;
  if (prefix) {
    // /uz/catalog → /catalog: в app/ нет сегмента [locale], язык передаётся заголовком
    const url = req.nextUrl.clone();
    url.pathname = stripLocale(pathname);
    url.search = search;
    res = NextResponse.rewrite(url, { request: { headers } });
  } else {
    res = NextResponse.next({ request: { headers } });
  }
  return withCsp(res, csp);
}

export const config = {
  // запросы предзагрузки тоже проходят через middleware: без переписывания /uz/... они получили бы 404
  matcher: ["/((?!api|_next/static|_next/image|favicon.ico|.*\\..*).*)"],
};
