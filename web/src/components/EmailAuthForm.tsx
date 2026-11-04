"use client";
// ВРЕМЕННО: вход и регистрация по почте на период тестирования (backend/internal/auth/email_temp.go).
// Форма показывается, только если сервер отвечает enabled=true (EMAIL_AUTH_ENABLED).
// Удаляется вместе с ключами emailAuth и errors.email_taken в messages.
import { useEffect, useState, type FormEvent } from "react";
import { useTranslations } from "next-intl";
import { api, setSession } from "@/lib/api";
import { ErrorBox } from "@/components/ErrorBox";

type SessionResp = { access_token: string; user: { id: string; role: "client" } };

export function EmailAuthForm({ onSignedIn }: { onSignedIn: () => Promise<void> }) {
  const t = useTranslations("emailAuth");
  const [enabled, setEnabled] = useState(false);
  const [mode, setMode] = useState<"login" | "register">("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    let alive = true;
    api<{ enabled: boolean }>("/v1/auth/email/status")
      .then((s) => alive && setEnabled(s.enabled))
      .catch(() => alive && setEnabled(false));
    return () => {
      alive = false;
    };
  }, []);

  if (!enabled) return null;

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const s = await api<SessionResp>(`/v1/auth/email/${mode}`, {
        method: "POST",
        body: { email: email.trim().toLowerCase(), password },
      });
      setSession(s.access_token, s.user.role);
      await onSignedIn();
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} className="card stack narrow">
      <h2>{mode === "login" ? t("loginTitle") : t("registerTitle")}</h2>
      <p className="muted small">{t("testingNote")}</p>
      <div className="field">
        <label htmlFor="email">{t("email")}</label>
        <input id="email" type="email" autoComplete="email" required maxLength={254} value={email} onChange={(e) => setEmail(e.target.value)} />
      </div>
      <div className="field">
        <label htmlFor="email-password">{t("password")}</label>
        <input
          id="email-password"
          type="password"
          required
          minLength={mode === "register" ? 10 : 1}
          maxLength={128}
          autoComplete={mode === "register" ? "new-password" : "current-password"}
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
        {mode === "register" && <p className="muted small">{t("passwordHint")}</p>}
      </div>
      <ErrorBox error={error} />
      <button className="btn btn-primary" disabled={busy}>{mode === "login" ? t("signIn") : t("register")}</button>
      <button
        type="button"
        className="btn btn-secondary"
        onClick={() => {
          setMode(mode === "login" ? "register" : "login");
          setError(null);
        }}
      >
        {mode === "login" ? t("toRegister") : t("toLogin")}
      </button>
    </form>
  );
}
