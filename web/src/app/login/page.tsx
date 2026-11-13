"use client";
// Вход и регистрация клиента: Telegram-бот, Google, почта (docs/AUTH.md).
// Какие способы показывать, решает сервер (GET /v1/auth/methods). Вход по номеру и SMS
// появляется, только если он включён на сервере (в prod выключен).
import { useCallback, useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { api, getAuthMethods, type AuthMethods } from "@/lib/api";
import { safeNext } from "@/lib/safeNext";
import { clientNextStep } from "@/lib/onboarding";
import { useRouter } from "@/i18n/navigation";
import { useSession } from "@/lib/session";
import type { Me } from "@/lib/session";
import { ErrorBox } from "@/components/ErrorBox";
import { TelegramLogin } from "@/components/auth/TelegramLogin";
import { GoogleLogin } from "@/components/auth/GoogleLogin";
import { EmailAuth } from "@/components/auth/EmailAuth";
import { PhoneAuth } from "@/components/auth/PhoneAuth";

export default function ClientLogin() {
  const t = useTranslations("login");
  const router = useRouter();
  const next = safeNext(useSearchParams().get("next"), "/account");
  const { reload } = useSession();
  const [methods, setMethods] = useState<AuthMethods | null>(null);
  const [loadError, setLoadError] = useState<unknown>(null);
  const [tab, setTab] = useState<"email" | "phone">("email");

  useEffect(() => {
    let alive = true;
    getAuthMethods()
      .then((m) => alive && setMethods(m))
      .catch((err) => alive && setLoadError(err));
    return () => {
      alive = false;
    };
  }, []);

  const afterSignIn = useCallback(async () => {
    const me = await api<Me>("/v1/me");
    await reload();
    router.replace(clientNextStep(me, next));
  }, [next, reload, router]);

  const social = !!methods && (!!methods.telegram || !!methods.google);
  const forms = !!methods && (methods.email || methods.sms);
  const shownTab = methods && !methods.email ? "phone" : tab;

  return (
    <div className="card narrow auth-card">
      <div className="stack">
        <header className="auth-head">
          <h1>{t("title")}</h1>
          <p className="muted">{t("subtitle")}</p>
        </header>
        {!methods && !loadError && <div className="auth-skeleton" aria-busy="true" aria-label={t("loading")} />}
        <ErrorBox error={loadError} />
        {methods && (
          <>
            {social && (
              <div className="stack-sm">
                {methods.telegram && <TelegramLogin onSignedIn={afterSignIn} />}
                {methods.google && <GoogleLogin clientId={methods.google.client_id} onSignedIn={afterSignIn} />}
              </div>
            )}
            {social && forms && <div className="auth-divider"><span>{t("or")}</span></div>}
            {methods.email && methods.sms && (
              <div className="auth-tabs" role="tablist">
                <button type="button" role="tab" aria-selected={shownTab === "email"} onClick={() => setTab("email")}>{t("tabEmail")}</button>
                <button type="button" role="tab" aria-selected={shownTab === "phone"} onClick={() => setTab("phone")}>{t("tabPhone")}</button>
              </div>
            )}
            {methods.email && shownTab === "email" && <EmailAuth onSignedIn={afterSignIn} />}
            {methods.sms && shownTab === "phone" && <PhoneAuth onSignedIn={afterSignIn} />}
            {!social && !forms && <p className="muted">{t("noMethods")}</p>}
            <p className="muted small auth-legal">{t("legal")}</p>
          </>
        )}
      </div>
    </div>
  );
}
