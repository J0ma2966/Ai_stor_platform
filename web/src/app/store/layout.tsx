"use client";
import type { ReactNode } from "react";
import { useTranslations } from "next-intl";
import { AuthGate } from "@/components/AuthGate";
import { CabNav } from "@/components/CabNav";
import { usePathname } from "@/i18n/navigation";

// Кабинет магазина: только роль store; страница входа — без проверки.
export default function StoreLayout({ children }: { children: ReactNode }) {
  const t = useTranslations("storeCab");
  const pathname = usePathname();
  if (pathname.startsWith("/store/login")) return <>{children}</>;
  const nav = [
    { href: "/store", label: t("overview"), exact: true },
    { href: "/store/orders", label: t("orders") },
    { href: "/store/products", label: t("products") },
    { href: "/store/import", label: t("import") },
    { href: "/store/profile", label: t("profile") },
    { href: "/store/notifications", label: t("notifications") },
  ];
  return (
    <AuthGate role="store">
      <div className="cab-layout">
        <CabNav items={nav} label={t("title")} />
        <div className="stack">{children}</div>
      </div>
    </AuthGate>
  );
}
