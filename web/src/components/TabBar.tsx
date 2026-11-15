"use client";
// Нижняя панель как в Android-приложении покупателя: Главная, Каталог, Корзина, Заказы, Профиль.
// Видна только на телефонах (CSS, до 760 px) и только гостю и покупателю; в кабинетах
// магазина и админа её нет.
import { useTranslations } from "next-intl";
import { Link, usePathname } from "@/i18n/navigation";
import { useSession } from "@/lib/session";
import { Icon, type IconName } from "./Icon";

type Tab = { href: string; label: string; icon: IconName; match: (p: string) => boolean };

const ROOTS = ["/", "/catalog", "/cart", "/account/orders", "/account"];

/** Экран — одна из вкладок (на нём Telegram не показывает кнопку «Назад»). */
export function isTabRoot(pathname: string): boolean {
  return ROOTS.includes(pathname.replace(/\/+$/, "") || "/");
}

export function TabBar() {
  const t = useTranslations("nav");
  const pathname = usePathname();
  const { me, ready, cartQty } = useSession();
  const role = me?.role;
  const staff = (p: string) => pathname === p || pathname.startsWith(p + "/"); // «/stores» — витрина, не кабинет
  if (role === "store" || role === "admin" || staff("/store") || staff("/admin")) return null;

  const tabs: Tab[] = [
    { href: "/", label: t("home"), icon: "home", match: (p) => p === "/" },
    { href: "/catalog", label: t("catalog"), icon: "grid", match: (p) => p.startsWith("/catalog") },
    { href: "/cart", label: t("cart"), icon: "bag", match: (p) => p.startsWith("/cart") || p.startsWith("/checkout") },
    { href: "/account/orders", label: t("orders"), icon: "truck", match: (p) => p.startsWith("/account/orders") },
    {
      href: ready && !me ? "/login" : "/account",
      label: t("profile"),
      icon: "user",
      match: (p) => p === "/login" || (p.startsWith("/account") && !p.startsWith("/account/orders")),
    },
  ];

  return (
    <nav className="tabbar" aria-label={t("menu")}>
      {tabs.map((tab) => {
        const on = tab.match(pathname);
        return (
          <Link key={tab.icon} href={tab.href} className="tabbar-item" aria-current={on ? "page" : undefined}>
            <span className="tabbar-icon">
              <Icon name={tab.icon} size={22} strokeWidth={on ? 2.2 : 1.8} />
              {tab.icon === "bag" && cartQty > 0 && <span className="count-badge" aria-hidden="true">{cartQty > 99 ? "99+" : cartQty}</span>}
            </span>
            <span className="tabbar-label">{tab.label}</span>
            {tab.icon === "bag" && cartQty > 0 && <span className="sr-only">({cartQty})</span>}
          </Link>
        );
      })}
    </nav>
  );
}
