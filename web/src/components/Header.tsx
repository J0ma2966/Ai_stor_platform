"use client";
import { useEffect, useState, type FormEvent, type MouseEvent } from "react";
import { useSearchParams } from "next/navigation";
import { useLocale, useTranslations } from "next-intl";
import { Link, usePathname, useRouter } from "@/i18n/navigation";
import { localizePath, type Locale } from "@/i18n/routing";
import { useLocalePending, useSwitchLocale } from "@/i18n/switch";
import { api } from "@/lib/api";
import { useSession } from "@/lib/session";
import { TRYON_ENABLED } from "@/lib/fitting";

function Badge({ n }: { n: number }) {
  if (n <= 0) return null;
  return <span className="count-badge" aria-hidden="true">{n > 99 ? "99+" : n}</span>;
}

export function Header() {
  const t = useTranslations();
  const locale = useLocale();
  const pathname = usePathname();
  const router = useRouter();
  const sp = useSearchParams();
  const { me, ready, cartQty, unread, signOut, setMe } = useSession();
  const [q, setQ] = useState(sp.get("q") ?? "");
  const [open, setOpen] = useState(false);
  const other: Locale = locale === "ru" ? "uz" : "ru";
  const switching = useLocalePending();
  const switchLocale = useSwitchLocale();

  useEffect(() => setOpen(false), [pathname]);
  const spQ = sp.get("q") ?? "";
  useEffect(() => setQ(spQ), [spQ]);

  function search(e: FormEvent) {
    e.preventDefault();
    const v = q.trim();
    router.push(v ? `/catalog?q=${encodeURIComponent(v)}` : "/catalog");
  }

  function changeLang(e: MouseEvent<HTMLAnchorElement>) {
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey || e.button !== 0) return; // «открыть в новой вкладке»
    e.preventDefault();
    switchLocale(other);
    // язык клиента нужен и серверу: на нём приходят SMS и уведомления
    if (me?.role === "client" && me.lang !== other) {
      api("/v1/me", { method: "PATCH", body: { lang: other } }).then(() => setMe({ ...me, lang: other }), () => {});
    }
  }

  const role = me?.role;
  const cabinet = role === "store" ? "/store" : role === "admin" ? "/admin" : "/account";
  const notifHref = role === "store" ? "/store/notifications" : "/account/notifications";
  const query = sp.toString();

  return (
    <header className="site-header">
      <a href="#main" className="skip-link">{t("app.skip")}</a>
      <div className="container header-row">
        <Link href="/" className="logo" aria-label={t("app.name")}>
          <span className="logo-mark" aria-hidden="true">B</span>
          <span>{t("app.name")}</span>
        </Link>
        <nav className="header-nav" aria-label={t("nav.menu")}>
          <Link href="/catalog">{t("nav.catalog")}</Link>
          <Link href="/stores">{t("nav.stores")}</Link>
          {TRYON_ENABLED && <Link href="/fitting">{t("nav.fitting")}</Link>}
        </nav>
        <form role="search" className="header-search" action={localizePath("/catalog", locale as Locale)} method="get" onSubmit={search}>
          <label htmlFor="site-search" className="sr-only">{t("nav.search")}</label>
          <input id="site-search" name="q" type="search" placeholder={t("nav.searchPh")} value={q} maxLength={100} onChange={(e) => setQ(e.target.value)} />
          <button className="btn btn-search" aria-label={t("nav.search")}>⌕</button>
        </form>
        <div className="header-actions">
          {/* обычная ссылка на ту же страницу на другом языке (для роботов и «открыть в новой вкладке»),
              а по клику язык меняется на месте, без перехода */}
          <a
            href={localizePath(pathname + (query ? `?${query}` : ""), other)}
            hrefLang={other}
            lang={other}
            className="hdr-btn lang"
            aria-busy={switching || undefined}
            onClick={changeLang}
          >
            {t("app.lang")}
          </a>
          {role !== "store" && role !== "admin" && (
            <>
              <Link href="/account/favorites" className="hdr-btn" aria-label={t("nav.favorites")} title={t("nav.favorites")}>
                <span aria-hidden="true">♡</span>
                <span className="hdr-label">{t("nav.favorites")}</span>
              </Link>
              <Link href="/cart" className="hdr-btn" aria-label={`${t("nav.cart")}${cartQty ? ` (${cartQty})` : ""}`} title={t("nav.cart")}>
                <span aria-hidden="true">🛍</span>
                <span className="hdr-label">{t("nav.cart")}</span>
                <Badge n={cartQty} />
              </Link>
            </>
          )}
          {me && (
            <Link href={notifHref} className="hdr-btn" aria-label={`${t("nav.notifications")}${unread ? ` (${unread})` : ""}`} title={t("nav.notifications")}>
              <span aria-hidden="true">🔔</span>
              <Badge n={unread} />
            </Link>
          )}
          {!ready ? (
            <span className="hdr-btn muted" aria-hidden="true">…</span>
          ) : me ? (
            <div className="hdr-user">
              <button type="button" className="hdr-btn" aria-expanded={open} aria-haspopup="menu" onClick={() => setOpen((v) => !v)}>
                <span aria-hidden="true">👤</span>
                <span className="hdr-label">{role === "store" ? t("nav.store") : role === "admin" ? t("nav.admin") : t("nav.account")}</span>
              </button>
              {role === "client" && (
                <Link href="/account/balance" className="token-pill" title={t("account.balance")}>
                  {t("nav.tokens", { n: me.token_balance })}
                </Link>
              )}
              {open && (
                <ul className="menu" role="menu">
                  <li role="none"><Link role="menuitem" href={cabinet}>{role === "store" ? t("nav.store") : role === "admin" ? t("nav.admin") : t("nav.account")}</Link></li>
                  {role === "client" && <li role="none"><Link role="menuitem" href="/account/orders">{t("account.orders")}</Link></li>}
                  {role === "store" && <li role="none"><Link role="menuitem" href="/store/orders">{t("storeCab.orders")}</Link></li>}
                  <li role="none">
                    <button role="menuitem" type="button" className="btn-link" onClick={async () => { await signOut(); router.replace("/"); }}>
                      {t("app.logout")}
                    </button>
                  </li>
                </ul>
              )}
            </div>
          ) : (
            <Link href={`/login?next=${encodeURIComponent(pathname)}`} className="btn btn-light">{t("nav.login")}</Link>
          )}
        </div>
      </div>
    </header>
  );
}
