"use client";
import { useEffect, useState, type FormEvent } from "react";
import { useSearchParams } from "next/navigation";
import { useLocale, useTranslations } from "next-intl";
import { Link, useRouter } from "@/i18n/navigation";
import { api } from "@/lib/api";
import { safeNext } from "@/lib/safeNext";
import { clientNextStep } from "@/lib/onboarding";
import { useSession, type Me } from "@/lib/session";
import { ErrorBox } from "@/components/ErrorBox";

// Завершение регистрации: пол (обязательно) + согласие на обработку ПД → приветственные токены,
// затем пароль (/account/password) — только тем, кто вошёл по SMS (needs_password).
export default function Welcome() {
  const t = useTranslations("welcome");
  const locale = useLocale();
  const router = useRouter();
  const next = safeNext(useSearchParams().get("next"), "/account");
  const { me, setMe } = useSession();
  const [gender, setGender] = useState<"female" | "male" | "">("");
  const [consent, setConsent] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  useEffect(() => {
    if (me?.registration_complete) router.replace(clientNextStep(me, next));
  }, [me, next, router]);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!gender || !consent) return;
    setBusy(true);
    setError(null);
    try {
      const m = await api<Me>("/v1/me/complete-registration", { method: "POST", body: { gender, pd_consent: true, lang: locale } });
      setMe(m);
      // следующий шаг — пароль для входа без SMS
      router.replace(clientNextStep(m, next));
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  }
  return (
    <form className="card stack narrow" onSubmit={submit}>
      <h1>{t("title")}</h1>
      <p className="muted">{t("text")}</p>
      <fieldset className="plain-fieldset">
        <legend className="label">{t("gender")}</legend>
        <div className="radio-row">
          {(["female", "male"] as const).map((g) => (
            <label key={g}><input type="radio" name="gender" value={g} checked={gender === g} onChange={() => setGender(g)} required />{t(g)}</label>
          ))}
        </div>
      </fieldset>
      <label className="row check">
        <input type="checkbox" checked={consent} onChange={(e) => setConsent(e.target.checked)} required />
        <span>
          {t.rich("consent", {
            privacy: (c) => <Link href="/legal/privacy" target="_blank">{c}</Link>,
            terms: (c) => <Link href="/legal/terms" target="_blank">{c}</Link>,
          })}
        </span>
      </label>
      <p className="ok-text">{t("bonus", { n: 20 })}</p>
      <ErrorBox error={error} />
      <button className="btn btn-primary" disabled={busy || !gender || !consent}>{t("submit")}</button>
    </form>
  );
}
