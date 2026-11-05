"use client";
import { TRYON_ENABLED } from "@/lib/fitting";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Link, useRouter } from "@/i18n/navigation";
import { useSwitchLocale } from "@/i18n/switch";
import type { Locale } from "@/i18n/routing";
import { api } from "@/lib/api";
import { formatDateTime } from "@/lib/money";
import { useSession, type Me } from "@/lib/session";
import { ErrorBox } from "@/components/ErrorBox";

const BODY = ["chest", "waist", "hips"] as const;

export default function Profile() {
  const t = useTranslations("profile");
  const ta = useTranslations("app");
  const locale = useLocale();
  const router = useRouter();
  const switchLocale = useSwitchLocale();
  const { me, setMe, signOut } = useSession();
  const [gender, setGender] = useState(me?.gender ?? "");
  const [lang, setLang] = useState(me?.lang ?? locale);
  const [height, setHeight] = useState(me?.height_cm?.toString() ?? "");
  const [weight, setWeight] = useState(me?.weight_kg?.toString() ?? "");
  const [body, setBody] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [confirmDel, setConfirmDel] = useState(false);

  // Поля заполняем из профиля один раз (после загрузки), чтобы смена языка в шапке —
  // она тоже обновляет профиль — не стирала то, что человек уже начал вводить.
  const loadedFor = useRef<string | null>(null);
  useEffect(() => {
    if (!me || loadedFor.current === me.id) return;
    loadedFor.current = me.id;
    setGender(me.gender ?? "");
    setLang(me.lang);
    setHeight(me.height_cm?.toString() ?? "");
    setWeight(me.weight_kg?.toString() ?? "");
    setBody(Object.fromEntries(Object.entries(me.body_params ?? {}).map(([k, v]) => [k, String(v)])));
  }, [me]);
  // язык интерфейса в форме всегда совпадает с текущим
  useEffect(() => setLang(locale as Locale), [locale]);

  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    setSaved(false);
    const bp: Record<string, number> = {};
    for (const k of BODY) if (body[k] && Number(body[k]) > 0) bp[k] = Number(body[k]);
    try {
      const m = await api<Me>("/v1/me", {
        method: "PATCH",
        body: {
          gender: gender || undefined,
          lang,
          height_cm: height ? Number(height) : undefined,
          weight_kg: weight ? Number(weight) : undefined,
          body_params: bp,
        },
      });
      setMe(m);
      setSaved(true);
      if (lang !== locale) switchLocale(lang as Locale);
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  }

  async function del() {
    setBusy(true);
    setError(null);
    try {
      await api("/v1/me", { method: "DELETE", body: { confirm: true } });
      await signOut();
      router.replace("/");
    } catch (err) {
      setError(err);
      setBusy(false);
    }
  }

  if (!me) return null;
  return (
    <>
      <h1>{t("title")}</h1>
      <form className="card stack" onSubmit={save}>
        <p><span className="muted">{t("phone")}:</span> {me.phone}</p>
        <fieldset className="plain-fieldset">
          <legend className="label">{t("gender")}</legend>
          <div className="radio-row">
            {(["female", "male"] as const).map((g) => (
              <label key={g}><input type="radio" name="gender" checked={gender === g} onChange={() => setGender(g)} />{t(g)}</label>
            ))}
          </div>
          <p className="muted small">{t("genderHint")}</p>
        </fieldset>
        <div className="field">
          <label htmlFor="lang">{t("lang")}</label>
          <select id="lang" value={lang} onChange={(e) => setLang(e.target.value as "ru" | "uz")}>
            <option value="ru">Русский</option>
            <option value="uz">O‘zbekcha</option>
          </select>
        </div>
        <fieldset className="plain-fieldset">
          <legend className="label">{t("body")}</legend>
          <p className="muted small">{t("bodyHint")}</p>
          <div className="grid-4">
            <div className="field"><label htmlFor="h">{t("height")}</label><input id="h" type="number" min={50} max={260} value={height} onChange={(e) => setHeight(e.target.value)} /></div>
            <div className="field"><label htmlFor="w">{t("weight")}</label><input id="w" type="number" min={20} max={400} step="0.1" value={weight} onChange={(e) => setWeight(e.target.value)} /></div>
            {BODY.map((k) => (
              <div className="field" key={k}>
                <label htmlFor={k}>{t(k)}</label>
                <input id={k} type="number" min={30} max={250} value={body[k] ?? ""} onChange={(e) => setBody((p) => ({ ...p, [k]: e.target.value }))} />
              </div>
            ))}
          </div>
        </fieldset>
        <ErrorBox error={error} />
        <div className="row">
          <button className="btn btn-primary" disabled={busy}>{ta("save")}</button>
          {saved && <span className="ok-text" role="status">{ta("saved")}</span>}
        </div>
      </form>

      <section className="card stack-sm">
        <h2>{t("passwordTitle")}</h2>
        <p className="muted">{t("passwordText")}</p>
        <p><Link href="/account/password">{t("passwordLink")}</Link></p>
      </section>

      <section className="card stack-sm">
        <h2>{t("privacy")}</h2>
        {me.pd_consent_at && <p>{t("consentGiven", { date: formatDateTime(me.pd_consent_at, locale) })}</p>}
        <p className="row wrap">
          {TRYON_ENABLED && <Link href="/account/photos">{t("photosLink")}</Link>}
          <Link href="/legal/privacy">{t("privacy")}</Link>
        </p>
      </section>

      <section className="card stack-sm danger-zone">
        <h2>{t("deleteTitle")}</h2>
        <p className="muted">{t("deleteText")}</p>
        <label className="row check">
          <input type="checkbox" checked={confirmDel} onChange={(e) => setConfirmDel(e.target.checked)} />
          <span>{t("deleteConfirm")}</span>
        </label>
        <button type="button" className="btn btn-danger" disabled={!confirmDel || busy} onClick={del}>{t("deleteBtn")}</button>
      </section>
    </>
  );
}
