"use client";
import { useEffect, useState, type ReactNode } from "react";
import { useTranslations } from "next-intl";
import { currentRole, refreshSession, setOnAuthLost, type Role } from "@/lib/api";
import { useRouter, usePathname } from "@/i18n/navigation";

// Пускает дальше только пользователя нужной роли. Access-токен в памяти,
// поэтому после перезагрузки страницы сначала пробуем refresh по cookie.
export function AuthGate({ role, children }: { role: Role; children: ReactNode }) {
  const t = useTranslations("app");
  const router = useRouter();
  const pathname = usePathname();
  const [ok, setOk] = useState(currentRole() === role);
  const loginPath = role === "client" ? "/login" : role === "store" ? "/store/login" : "/admin/login";

  useEffect(() => {
    // возвращаемся на ту же страницу вместе с параметрами (например, ?items= на оформлении)
    const toLogin = () => router.replace(`${loginPath}?next=${encodeURIComponent(pathname + window.location.search)}`);
    setOnAuthLost(toLogin);
    if (currentRole() === role) {
      setOk(true);
    } else {
      refreshSession().then((good) => (good && currentRole() === role ? setOk(true) : toLogin()));
    }
    return () => setOnAuthLost(null);
  }, [role, loginPath, pathname, router]);

  if (!ok) return <p className="muted">{t("loading")}</p>;
  return <>{children}</>;
}
