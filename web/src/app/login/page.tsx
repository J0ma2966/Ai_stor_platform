"use client";
import { useState, type FormEvent } from "react";
import { useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { loginPassword, requestOtp, resetPassword, verifyOtp } from "@/lib/api";
import { safeNext } from "@/lib/safeNext";
import { clientNextStep } from "@/lib/onboarding";
import { useRouter } from "@/i18n/navigation";
import { ErrorBox } from "@/components/ErrorBox";
import { useSession } from "@/lib/session";
import { api } from "@/lib/api";
import type { Me } from "@/lib/session";
import { EmailAuthForm } from "@/components/EmailAuthForm"; // ВРЕМЕННО: вход по почте на время тестирования

// password — обычный вход по телефону и паролю (без SMS);
// sms — регистрация или вход по коду (для тех, кто ещё не задал пароль);
// reset — «Забыли пароль?»: код из SMS + новый пароль.
type Mode = "password" | "sms" | "reset";

export default function ClientLogin() {
  const t = useTranslations("login");
  const router = useRouter();
  const next = safeNext(useSearchParams().get("next"), "/account");
  const { reload } = useSession();
  const [mode, setMode] = useState<Mode>("password");
  const [phone, setPhone] = useState("+998");
  const [password, setPasswordValue] = useState("");
  const [code, setCode] = useState("");
  const [sent, setSent] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  async function afterSignIn() {
    const me = await api<Me>("/v1/me");
    await reload();
    router.replace(clientNextStep(me, next));
  }

  function switchMode(m: Mode) {
    setMode(m);
    setSent(false);
    setCode("");
    setPasswordValue("");
    setError(null);
  }

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      if (mode === "password") {
        await loginPassword(phone.trim(), password);
        await afterSignIn();
      } else if (!sent) {
        await requestOtp(phone.trim());
        setSent(true);
      } else if (mode === "sms") {
        await verifyOtp(phone.trim(), code.trim());
        await afterSignIn();
      } else {
        await resetPassword(phone.trim(), code.trim(), password);
        await afterSignIn();
      }
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  }

  const title = mode === "password" ? t("title") : mode === "sms" ? t("smsTitle") : t("resetTitle");
  const button = mode === "password" ? t("signIn") : !sent ? t("sendCode") : mode === "sms" ? t("signIn") : t("resetSubmit");

  return (
    <>
      <form onSubmit={submit} className="card stack narrow">
        <h1>{title}</h1>
        {mode === "sms" && <p className="muted small">{t("smsHint")}</p>}
        {mode === "reset" && <p className="muted small">{t("resetHint")}</p>}
        <div className="field">
          <label htmlFor="phone">{t("phone")}</label>
          <input id="phone" type="tel" inputMode="tel" autoComplete="tel" value={phone} disabled={sent} onChange={(e) => setPhone(e.target.value)} />
        </div>
        {mode !== "password" && sent && (
          <div className="field">
            <label htmlFor="code">{t("code")}</label>
            <input id="code" inputMode="numeric" autoComplete="one-time-code" maxLength={6} value={code} onChange={(e) => setCode(e.target.value)} />
          </div>
        )}
        {(mode === "password" || (mode === "reset" && sent)) && (
          <div className="field">
            <label htmlFor="password">{mode === "reset" ? t("newPassword") : t("password")}</label>
            <input
              id="password"
              type="password"
              required
              minLength={mode === "reset" ? 10 : 1}
              maxLength={128}
              autoComplete={mode === "reset" ? "new-password" : "current-password"}
              value={password}
              onChange={(e) => setPasswordValue(e.target.value)}
            />
            {mode === "reset" && <p className="muted small">{t("passwordHint")}</p>}
          </div>
        )}
        <ErrorBox error={error} />
        <button className="btn btn-primary" disabled={busy}>{button}</button>
        <div className="stack-sm">
          {mode === "password" ? (
            <>
              <button type="button" className="btn btn-secondary" onClick={() => switchMode("sms")}>{t("toSms")}</button>
              <button type="button" className="btn btn-link" onClick={() => switchMode("reset")}>{t("forgot")}</button>
              <p className="muted small">{t("noPasswordHint")}</p>
            </>
          ) : (
            <button type="button" className="btn btn-secondary" onClick={() => switchMode("password")}>{t("toPassword")}</button>
          )}
        </div>
      </form>
      <EmailAuthForm onSignedIn={afterSignIn} />
    </>
  );
}
