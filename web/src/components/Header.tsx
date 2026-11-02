"use client";
import { useLocale, useTranslations } from "next-intl";
import { Link, usePathname } from "@/i18n/navigation";

export function Header() {
  const t = useTranslations("app");
  const locale = useLocale();
  const pathname = usePathname();
  const other = locale === "ru" ? "uz" : "ru";
  const isStore = pathname.startsWith("/store");
  return (
    <header className="site-header">
      <div className="container row between">
        <Link href="/" className="logo">{t("name")}</Link>
        <nav className="row">
          {isStore ? <Link href="/store/orders">{t("storeOrders")}</Link> : <Link href="/account/orders">{t("myOrders")}</Link>}
          <Link href={pathname} locale={other} className="lang">{t("lang")}</Link>
        </nav>
      </div>
    </header>
  );
}
