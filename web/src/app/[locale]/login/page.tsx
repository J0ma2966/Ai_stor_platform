"use client";
import { useState, type FormEvent } from "react";
import { useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { requestOtp, verifyOtp } from "@/lib/api";
import { safeNext } from "@/lib/safeNext";
import { useRouter } from "@/i18n/navigation";
import { ErrorBox } from "@/components/ErrorBox";

export default function ClientLogin() {
  const t = useTranslations("login");
  const router = useRouter();
  const next = safeNext(useSearchParams().get("next"), "/account/orders");
  const [phone, setPhone] = useState("+998");
  const [code, setCode] = useState("");
  const [sent, setSent] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      if (!sent) {
        await requestOtp(phone.trim());
        setSent(true);
      } else {
        await verifyOtp(phone.trim(), code.trim());
        router.replace(next);
      }
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} className="card stack narrow">
      <h1>{t("title")}</h1>
      <div className="field">
        <label htmlFor="phone">{t("phone")}</label>
        <input id="phone" type="tel" inputMode="tel" autoComplete="tel" value={phone} disabled={sent} onChange={(e) => setPhone(e.target.value)} />
      </div>
      {sent && (
        <div className="field">
          <label htmlFor="code">{t("code")}</label>
          <input id="code" inputMode="numeric" autoComplete="one-time-code" maxLength={6} value={code} onChange={(e) => setCode(e.target.value)} />
        </div>
      )}
      <ErrorBox error={error} />
      <button className="btn btn-primary" disabled={busy}>{sent ? t("signIn") : t("sendCode")}</button>
    </form>
  );
}
