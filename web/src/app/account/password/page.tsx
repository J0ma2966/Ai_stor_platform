"use client";
import { useState, type FormEvent } from "react";
import { useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { useRouter } from "@/i18n/navigation";
import { api, setPassword } from "@/lib/api";
import { safeNext } from "@/lib/safeNext";
import { useSession, type Me } from "@/lib/session";
import { ErrorBox } from "@/components/ErrorBox";

// Пароль клиента. После регистрации по SMS это обязательный шаг (needs_password=true):
// дальше клиент входит по телефону и паролю, а SMS нужна только для сброса пароля.
// Если пароль уже есть — смена с вводом текущего.
export default function PasswordPage() {
  const t = useTranslations("password");
  const router = useRouter();
  const next = safeNext(useSearchParams().get("next"), "/account/profile");
  const { me, setMe } = useSession();
  const [current, setCurrent] = useState("");
  const [pw, setPw] = useState("");
  const [repeat, setRepeat] = useState("");
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);
  const [error, setError] = useState<unknown>(null);

  if (!me) return null;
  const change = me.has_password;
  const mismatch = repeat !== "" && pw !== repeat;

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (pw.length < 10 || pw !== repeat) return;
    setBusy(true);
    setError(null);
    setDone(false);
    try {
      await setPassword(pw, change ? current : undefined);
      setMe(await api<Me>("/v1/me"));
      if (change) {
        setDone(true);
        setCurrent("");
        setPw("");
        setRepeat("");
      } else {
        router.replace(next);
      }
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="card stack narrow" onSubmit={submit}>
      <h1>{change ? t("changeTitle") : t("setTitle")}</h1>
      <p className="muted">{change ? t("changeText") : t("setText")}</p>
      {change && (
        <div className="field">
          <label htmlFor="current-password">{t("current")}</label>
          <input id="current-password" type="password" required maxLength={128} autoComplete="current-password" value={current} onChange={(e) => setCurrent(e.target.value)} />
        </div>
      )}
      <div className="field">
        <label htmlFor="new-password">{t("new")}</label>
        <input id="new-password" type="password" required minLength={10} maxLength={128} autoComplete="new-password" value={pw} onChange={(e) => setPw(e.target.value)} />
        <p className="muted small">{t("hint")}</p>
      </div>
      <div className="field">
        <label htmlFor="repeat-password">{t("repeat")}</label>
        <input
          id="repeat-password"
          type="password"
          required
          minLength={10}
          maxLength={128}
          autoComplete="new-password"
          value={repeat}
          aria-invalid={mismatch || undefined}
          onChange={(e) => setRepeat(e.target.value)}
        />
        {mismatch && <p className="field-error" role="alert">{t("mismatch")}</p>}
      </div>
      <ErrorBox error={error} />
      <div className="row">
        <button className="btn btn-primary" disabled={busy || pw.length < 10 || pw !== repeat || (change && !current)}>
          {change ? t("changeSubmit") : t("setSubmit")}
        </button>
        {done && <span className="ok-text" role="status">{t("changed")}</span>}
      </div>
    </form>
  );
}
