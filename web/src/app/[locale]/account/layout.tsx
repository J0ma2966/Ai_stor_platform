"use client";
import type { ReactNode } from "react";
import { useTranslations } from "next-intl";
import { AuthGate } from "@/components/AuthGate";
import { CabNav, useAccountNav } from "@/components/CabNav";
import { usePathname } from "@/i18n/navigation";

// Кабинет клиента: только роль client (прямой переход по URL другим ролям закрыт).
export default function AccountLayout({ children }: { children: ReactNode }) {
  const t = useTranslations("account");
  const nav = useAccountNav();
  const pathname = usePathname();
  if (pathname.startsWith("/account/welcome")) return <AuthGate role="client">{children}</AuthGate>;
  return (
    <AuthGate role="client">
      <div className="cab-layout">
        <CabNav items={nav} label={t("title")} />
        <div className="stack">{children}</div>
      </div>
    </AuthGate>
  );
}
