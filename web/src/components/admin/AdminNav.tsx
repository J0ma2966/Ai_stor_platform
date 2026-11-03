"use client";
import { useTranslations } from "next-intl";
import { Link, usePathname, useRouter } from "@/i18n/navigation";
import { logout } from "@/lib/api";

// Разделы панели. products/categories/settings делает модуль модерации каталога.
export const ADMIN_SECTIONS = [
  ["/admin", "navDashboard"],
  ["/admin/stores", "navStores"],
  ["/admin/users", "navUsers"],
  ["/admin/orders", "navOrders"],
  ["/admin/products", "navProducts"],
  ["/admin/categories", "navCategories"],
  ["/admin/tariffs", "navTariffs"],
  ["/admin/payments", "navPayments"],
  ["/admin/audit", "navAudit"],
  ["/admin/settings", "navSettings"],
] as const;

export function AdminNav() {
  const t = useTranslations("admin");
  const tApp = useTranslations("app");
  const pathname = usePathname();
  const router = useRouter();
  const active = (href: string) => (href === "/admin" ? pathname === "/admin" : pathname === href || pathname.startsWith(href + "/"));

  async function signOut() {
    try {
      await logout();
    } finally {
      router.replace("/admin/login");
    }
  }

  return (
    <nav className="admin-nav" aria-label={t("navLabel")}>
      <ul className="plain">
        {ADMIN_SECTIONS.map(([href, key]) => (
          <li key={href}>
            <Link href={href} className={active(href) ? "admin-link admin-link-on" : "admin-link"} aria-current={active(href) ? "page" : undefined}>
              {t(key)}
            </Link>
          </li>
        ))}
        <li>
          <button type="button" className="admin-link btn-link" onClick={signOut}>{tApp("logout")}</button>
        </li>
      </ul>
    </nav>
  );
}
