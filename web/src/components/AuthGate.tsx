"use client";
import { useEffect, useState, type ReactNode } from "react";
import { useTranslations } from "next-intl";
import { currentRole, refreshSessionResult, setOnAuthLost, type ApiError, type Role } from "@/lib/api";
import { ErrorBox } from "@/components/ErrorBox";
import { useRouter, usePathname } from "@/i18n/navigation";

// Пускает дальше только пользователя нужной роли. Access-токен в памяти,
// поэтому после перезагрузки страницы сначала пробуем refresh по cookie.
// Временный сбой refresh (429, 5xx, нет сети) — не выход: показываем ошибку с «Повторить».
export function AuthGate({ role, children }: { role: Role; children: ReactNode }) {
  const t = useTranslations("app");
  const router = useRouter();
  const pathname = usePathname();
  const [ok, setOk] = useState(currentRole() === role);
  const [error, setError] = useState<ApiError | null>(null);
  const [attempt, setAttempt] = useState(0);
  const loginPath = role === "client" ? "/login" : role === "store" ? "/store/login" : "/admin/login";

  useEffect(() => {
    // возвращаемся на ту же страницу вместе с параметрами (например, ?items= на оформлении)
    const toLogin = () => router.replace(`${loginPath}?next=${encodeURIComponent(pathname + window.location.search)}`);
    setOnAuthLost(toLogin);
    if (currentRole() === role) {
      setOk(true);
    } else {
      setError(null);
      refreshSessionResult().then((r) => {
        if (r.ok && currentRole() === role) setOk(true);
        else if (!r.ok && r.error) setError(r.error);
        else toLogin();
      });
    }
    return () => setOnAuthLost(null);
  }, [role, loginPath, pathname, router, attempt]);

  if (!ok && error) return <ErrorBox error={error} onRetry={() => setAttempt((n) => n + 1)} />;
  if (!ok) return <p className="muted">{t("loading")}</p>;
  return <>{children}</>;
}
