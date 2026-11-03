import { getTranslations } from "next-intl/server";
import { Link } from "@/i18n/navigation";
import { fetchPlatform } from "@/lib/catalog";

export async function Footer() {
  const t = await getTranslations();
  const p = await fetchPlatform();
  return (
    <footer className="site-footer">
      <div className="container footer-grid">
        <div>
          <p className="logo">{t("app.name")}</p>
          <p className="muted small">{t("app.tagline")}</p>
        </div>
        <nav aria-label={t("footer.about")}>
          <ul className="plain stack-sm">
            <li><Link href="/about">{t("footer.about")}</Link></li>
            <li><Link href="/catalog">{t("nav.catalog")}</Link></li>
            <li><Link href="/stores">{t("nav.stores")}</Link></li>
            <li><Link href="/store/login">{t("footer.forStores")}</Link></li>
          </ul>
        </nav>
        <nav aria-label={t("footer.terms")}>
          <ul className="plain stack-sm">
            <li><Link href="/legal/terms">{t("footer.terms")}</Link></li>
            <li><Link href="/legal/privacy">{t("footer.privacy")}</Link></li>
            <li><Link href="/legal/photos">{t("footer.photos")}</Link></li>
          </ul>
        </nav>
        <div>
          <p className="footer-title">{t("footer.support")}</p>
          <ul className="plain stack-sm">
            {p.support_phone && <li><a href={`tel:${p.support_phone}`}>{p.support_phone}</a></li>}
            {p.support_email && <li><a href={`mailto:${p.support_email}`}>{p.support_email}</a></li>}
            {p.support_telegram && <li><a href={`https://t.me/${p.support_telegram.replace(/^@/, "")}`} rel="noopener noreferrer" target="_blank">Telegram</a></li>}
            <li><Link href="/contacts">{t("footer.contacts")}</Link></li>
          </ul>
        </div>
      </div>
      <div className="container muted small footer-bottom">{t("footer.rights", { year: new Date().getFullYear() })}</div>
    </footer>
  );
}
