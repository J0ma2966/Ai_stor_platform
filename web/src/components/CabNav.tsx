"use client";
import { TRYON_ENABLED } from "@/lib/fitting";
import { useTranslations } from "next-intl";
import { Link, usePathname } from "@/i18n/navigation";

export type NavItem = { href: string; label: string; exact?: boolean };

export function CabNav({ items, label }: { items: NavItem[]; label: string }) {
  const pathname = usePathname();
  return (
    <nav className="cab-nav" aria-label={label}>
      {items.map((it) => {
        const on = it.exact ? pathname === it.href : pathname === it.href || pathname.startsWith(it.href + "/");
        return <Link key={it.href} href={it.href} aria-current={on ? "page" : undefined}>{it.label}</Link>;
      })}
    </nav>
  );
}

export function useAccountNav(): NavItem[] {
  const t = useTranslations("account");
  return [
    { href: "/account", label: t("title"), exact: true },
    { href: "/account/profile", label: t("profile") },
    { href: "/account/balance", label: t("balance") },
    ...(TRYON_ENABLED
      ? [{ href: "/account/tryons", label: t("tryons") }, { href: "/account/photos", label: t("photos") }]
      : []),
    { href: "/account/favorites", label: t("favorites") },
    { href: "/cart", label: t("cart") },
    { href: "/account/orders", label: t("orders") },
    { href: "/account/notifications", label: t("notifications") },
  ];
}
