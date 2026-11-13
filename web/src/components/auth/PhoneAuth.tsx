"use client";
// Вход по номеру телефона и SMS. Показывается, только если на сервере включён
// SMS_AUTH_ENABLED (в prod выключен: клиенты входят через Telegram, почту и Google).
import { useState, type FormEvent } from "react";
import { useTranslations } from "next-intl";
import { loginPassword, requestOtp, resetPassword, verifyOtp } from "@/lib/api";
import { ErrorBox } from "@/components/ErrorBox";

// password — вход по телефону и паролю; sms — регистрация или вход по коду;
// reset — «Забыли пароль?»: код из SMS + новый пароль.
type Mode = "password" | "sms" | "reset";

export function PhoneAuth({ onSignedIn }: { onSignedIn: () => Promise<void> }) {
  const t = useTranslations("login");
  const [mode, setMode] = useState<Mode>("password");
  const [phone, setPhone] = useState("+998");
  const [password, setPasswordValue] = useState("");
  const [code, setCode] = useState("");
  const [sent, setSent] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

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
        await onSignedIn();
      } else if (!sent) {
        await requestOtp(phone.trim());
        setSent(true);
      } else if (mode === "sms") {
        await verifyOtp(phone.trim(), code.trim());
        await onSignedIn();
      } else {
        await resetPassword(phone.trim(), code.trim(), password);
        await onSignedIn();
      }
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  }

  const title = mode === "password" ? t("phoneTitle") : mode === "sms" ? t("smsTitle") : t("resetTitle");
  const button = mode === "password" ? t("signIn") : !sent ? t("sendCode") : mode === "sms" ? t("signIn") : t("resetSubmit");

  return (
    <form onSubmit={submit} className="stack">
      <h2 className="auth-subtitle">{title}</h2>
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
          <label htmlFor="phone-password">{mode === "reset" ? t("newPassword") : t("password")}</label>
          <input
            id="phone-password"
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
      <div className="auth-links">
        {mode === "password" ? (
          <>
            <button type="button" className="btn-link" onClick={() => switchMode("sms")}>{t("toSms")}</button>
            <button type="button" className="btn-link" onClick={() => switchMode("reset")}>{t("forgot")}</button>
          </>
        ) : (
          <button type="button" className="btn-link" onClick={() => switchMode("password")}>{t("toPassword")}</button>
        )}
      </div>
    </form>
  );
}
