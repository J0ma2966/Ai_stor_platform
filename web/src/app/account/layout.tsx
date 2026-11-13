"use client";
import { useEffect, type ReactNode } from "react";
import { useTranslations } from "next-intl";
import { AuthGate } from "@/components/AuthGate";
import { CabNav, useAccountNav } from "@/components/CabNav";
import { usePathname, useRouter } from "@/i18n/navigation";
import { useSession } from "@/lib/session";

// Кабинет клиента: только роль client (прямой переход по URL другим ролям закрыт).
// Обязательные шаги регистрации (welcome, затем пароль) показываются без меню кабинета.
export default function AccountLayout({ children }: { children: ReactNode }) {
  const t = useTranslations("account");
  const nav = useAccountNav();
  const pathname = usePathname();
  const { me } = useSession();
  const needsPassword = !!me && me.role === "client" && me.registration_complete && !!me.needs_password;
  const onboarding = pathname.startsWith("/account/welcome") || (pathname.startsWith("/account/password") && needsPassword);
  if (onboarding) return <AuthGate role="client">{children}</AuthGate>;
  return (
    <AuthGate role="client">
      <PasswordGuard needsPassword={needsPassword} />
      <div className="cab-layout">
        <CabNav items={nav} label={t("title")} />
        <div className="stack">{children}</div>
      </div>
    </AuthGate>
  );
}

// Клиент, вошедший по SMS без пароля (например, зарегистрированный до появления
// входа по паролю), сначала задаёт пароль.
function PasswordGuard({ needsPassword }: { needsPassword: boolean }) {
  const router = useRouter();
  const pathname = usePathname();
  useEffect(() => {
    if (needsPassword && !pathname.startsWith("/account/password")) {
      router.replace(`/account/password?next=${encodeURIComponent(pathname + window.location.search)}`);
    }
  }, [needsPassword, pathname, router]);
  return null;
}
