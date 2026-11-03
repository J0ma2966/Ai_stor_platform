"use client";
import { useState, type FormEvent } from "react";
import { useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { loginStaff } from "@/lib/api";
import { safeNext } from "@/lib/safeNext";
import { useRouter } from "@/i18n/navigation";
import { ErrorBox } from "@/components/ErrorBox";
import { useSession } from "@/lib/session";

export default function StoreLogin() {
  const t = useTranslations("login");
  const router = useRouter();
  const next = safeNext(useSearchParams().get("next"), "/store");
  const { reload } = useSession();
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
      await reload();
      router.replace(s.user.role === "admin" && next.startsWith("/store") ? "/admin" : next);
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} className="card stack narrow">
      <h1>{t("storeTitle")}</h1>
      <div className="field">
        <label htmlFor="login">{t("login")}</label>
        <input id="login" autoComplete="username" value={login} onChange={(e) => setLogin(e.target.value)} />
      </div>
      <div className="field">
        <label htmlFor="password">{t("password")}</label>
        <input id="password" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} />
      </div>
      <ErrorBox error={error} />
      <button className="btn btn-primary" disabled={busy}>{t("signIn")}</button>
    </form>
  );
}
