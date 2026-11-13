"use client";
// Почта: вход по паролю, регистрация и восстановление пароля по коду из письма.
import { useState, type FormEvent } from "react";
import { useLocale, useTranslations } from "next-intl";
import { loginEmail, registerEmail, resetEmailPassword, sendEmailCode } from "@/lib/api";
import { ErrorBox } from "@/components/ErrorBox";

type Mode = "login" | "register" | "reset";

export function EmailAuth({ onSignedIn }: { onSignedIn: () => Promise<void> }) {
  const t = useTranslations("emailAuth");
  const locale = useLocale();
  const [mode, setMode] = useState<Mode>("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [repeat, setRepeat] = useState("");
  const [code, setCode] = useState("");
  const [sentTo, setSentTo] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [resent, setResent] = useState(false);

  const addr = email.trim().toLowerCase();
  const mismatch = mode !== "login" && repeat !== "" && password !== repeat;

  function switchMode(m: Mode) {
    setMode(m);
    setSentTo(null);
    setCode("");
    setPassword("");
    setRepeat("");
    setError(null);
    setResent(false);
  }

  async function run(fn: () => Promise<void>) {
    setBusy(true);
    setError(null);
    try {
      await fn();
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  }

  function submit(e: FormEvent) {
    e.preventDefault();
    if (mode === "login") {
      void run(async () => {
        await loginEmail(addr, password);
        await onSignedIn();
      });
    } else if (!sentTo) {
      void run(async () => {
        await sendEmailCode(addr, mode, locale);
        setSentTo(addr);
      });
    } else {
      if (password.length < 10 || password !== repeat) return;
      void run(async () => {
        if (mode === "register") await registerEmail(sentTo, code.trim(), password);
        else await resetEmailPassword(sentTo, code.trim(), password);
        await onSignedIn();
      });
    }
  }

  const title = mode === "login" ? t("loginTitle") : mode === "register" ? t("registerTitle") : t("resetTitle");
  const button = mode === "login" ? t("signIn") : !sentTo ? t("sendCode") : mode === "register" ? t("register") : t("resetSubmit");

  return (
    <form onSubmit={submit} className="stack" noValidate={false}>
      <h2 className="auth-subtitle">{title}</h2>
      {mode !== "login" && !sentTo && <p className="muted small">{mode === "register" ? t("registerHint") : t("resetHint")}</p>}
      <div className="field">
        <label htmlFor="email">{t("email")}</label>
        <input
          id="email"
          type="email"
          autoComplete="email"
          required
          maxLength={254}
          disabled={!!sentTo}
          value={email}
          onChange={(e) => setEmail(e.target.value)}
        />
      </div>
      {sentTo && (
        <>
          <p className="auth-note" role="status">{t("codeSent", { email: sentTo })}</p>
          <div className="field">
            <label htmlFor="email-code">{t("code")}</label>
            <input
              id="email-code"
              inputMode="numeric"
              autoComplete="one-time-code"
              required
              pattern="[0-9]{6}"
              maxLength={6}
              value={code}
              onChange={(e) => setCode(e.target.value.replace(/\D/g, ""))}
            />
          </div>
        </>
      )}
      {(mode === "login" || sentTo) && (
        <div className="field">
          <label htmlFor="email-password">{mode === "login" ? t("password") : t("newPassword")}</label>
          <input
            id="email-password"
            type="password"
            required
            minLength={mode === "login" ? 1 : 10}
            maxLength={128}
            autoComplete={mode === "login" ? "current-password" : "new-password"}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
          {mode !== "login" && <p className="muted small">{t("passwordHint")}</p>}
        </div>
      )}
      {mode !== "login" && sentTo && (
        <div className="field">
          <label htmlFor="email-repeat">{t("repeat")}</label>
          <input
            id="email-repeat"
            type="password"
            required
            maxLength={128}
            autoComplete="new-password"
            aria-invalid={mismatch}
            value={repeat}
            onChange={(e) => setRepeat(e.target.value)}
          />
          {mismatch && <p className="field-error">{t("mismatch")}</p>}
        </div>
      )}
      <ErrorBox error={error} />
      <button className="btn btn-primary" disabled={busy || (!!sentTo && code.length !== 6)}>{button}</button>
      <div className="auth-links">
        {sentTo && (
          <button
            type="button"
            className="btn-link"
            disabled={busy}
            onClick={() =>
              void run(async () => {
                await sendEmailCode(sentTo, mode === "reset" ? "reset" : "register", locale);
                setResent(true);
              })
            }
          >
            {resent ? t("resent") : t("resend")}
          </button>
        )}
        {mode === "login" ? (
          <>
            <button type="button" className="btn-link" onClick={() => switchMode("register")}>{t("toRegister")}</button>
            <button type="button" className="btn-link" onClick={() => switchMode("reset")}>{t("forgot")}</button>
          </>
        ) : (
          <button type="button" className="btn-link" onClick={() => switchMode("login")}>{t("toLogin")}</button>
        )}
      </div>
    </form>
  );
}
