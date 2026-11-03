"use client";
import { useEffect, useState, type FormEvent } from "react";
import { useTranslations } from "next-intl";
import { ErrorBox } from "@/components/ErrorBox";
import { getSettings, putSettings, type Settings } from "@/lib/adminCat";

export default function AdminSettings() {
  const t = useTranslations("adminCat");
  const ta = useTranslations("app");
  const [s, setS] = useState<Settings | null>(null);
  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState<unknown>(null);
  useEffect(() => { getSettings().then(setS).catch(setError); }, []);

  async function save(e: FormEvent) {
    e.preventDefault();
    if (!s) return;
    setBusy(true);
    setSaved(false);
    setError(null);
    try {
      setS(await putSettings({
        generation_cost: Number(s.generation_cost ?? 0), welcome_bonus: Number(s.welcome_bonus ?? 0),
        support_phone: s.support_phone ?? "", support_email: s.support_email ?? "", support_telegram: s.support_telegram ?? "",
      }));
      setSaved(true);
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  }
  if (!s) return error ? <ErrorBox error={error} /> : <p className="muted">{ta("loading")}</p>;
  const num = (k: "generation_cost" | "welcome_bonus", label: string) => (
    <div className="field"><label htmlFor={k}>{label}</label><input id={k} type="number" min={0} max={100000} required value={s[k] ?? ""} onChange={(e) => setS({ ...s, [k]: e.target.value === "" ? undefined : Number(e.target.value) })} /></div>
  );
  const txt = (k: "support_phone" | "support_email" | "support_telegram", label: string, type = "text") => (
    <div className="field"><label htmlFor={k}>{label}</label><input id={k} type={type} maxLength={200} value={s[k] ?? ""} onChange={(e) => setS({ ...s, [k]: e.target.value })} /></div>
  );
  return (
    <>
      <h1>{t("settings")}</h1>
      <form className="card stack" onSubmit={save}>
        <div className="grid-2">{num("generation_cost", t("generationCost"))}{num("welcome_bonus", t("welcomeBonus"))}</div>
        <p className="muted small">{t("settingsHint")}</p>
        <div className="grid-2">{txt("support_phone", t("supportPhone"), "tel")}{txt("support_email", t("supportEmail"), "email")}</div>
        {txt("support_telegram", t("supportTelegram"))}
        <ErrorBox error={error} />
        <div className="row"><button className="btn btn-primary" disabled={busy}>{ta("save")}</button>{saved && <span className="ok-text" role="status">{ta("saved")}</span>}</div>
      </form>
    </>
  );
}
