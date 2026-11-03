"use client";
import { useState, type FormEvent } from "react";
import { useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { ApiError, loginStaff, logout } from "@/lib/api";
import { safeNext } from "@/lib/safeNext";
import { useRouter } from "@/i18n/navigation";
import { ErrorBox } from "@/components/ErrorBox";

export default function AdminLogin() {
  const t = useTranslations("login");
  const ta = useTranslations("admin");
  const router = useRouter();
  const next = safeNext(useSearchParams().get("next"), "/admin");
  const [login, setLogin] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const s = await loginStaff(login.trim(), password);
      if (s.user.role !== "admin") {
        // магазин ввёл свой логин на странице админа: сессию не оставляем
        await logout();
        throw new ApiError(403, "forbidden", "not an admin");
      }
      router.replace(next);
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} className="card stack narrow">
      <h1>{ta("loginTitle")}</h1>
      <div className="field">
        <label htmlFor="login">{t("login")}</label>
        <input id="login" autoComplete="username" value={login} onChange={(e) => setLogin(e.target.value)} required />
      </div>
      <div className="field">
        <label htmlFor="password">{t("password")}</label>
        <input id="password" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} required />
      </div>
      <ErrorBox error={error} />
      <button className="btn btn-primary" disabled={busy}>{t("signIn")}</button>
    </form>
  );
}
