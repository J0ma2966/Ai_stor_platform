"use client";
import { useState, type FormEvent } from "react";
import { useTranslations } from "next-intl";
import { ApiError } from "@/lib/api";
import { adminApi, generatePassword, LOGIN_RE, SLUG_RE } from "@/lib/admin";
import { copyToClipboard } from "@/lib/orders";
import { ErrorBox, useFieldError } from "@/components/ErrorBox";

type Created = { storeId: string; login: string; password: string; activated: boolean };

// Создание магазина: пароль генерируется и показывается один раз — передайте его владельцу.
export function CreateStoreForm({ onCreated }: { onCreated: () => void }) {
  const t = useTranslations("admin");
  const fe = useFieldError();
  const [f, setF] = useState({ name: "", slug: "", login: "", password: generatePassword(), phone: "" });
  const [activate, setActivate] = useState(true);
  const [fields, setFields] = useState<Record<string, string>>({});
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const [created, setCreated] = useState<Created | null>(null);
  const [copied, setCopied] = useState(false);
  const set = (k: keyof typeof f) => (e: { target: { value: string } }) => setF({ ...f, [k]: e.target.value });

  function validate(): Record<string, string> {
    const out: Record<string, string> = {};
    if (!f.name.trim()) out.name = "required";
    if (!SLUG_RE.test(f.slug)) out.slug = "invalid";
    if (!LOGIN_RE.test(f.login)) out.login = "invalid";
    if (f.password.length < 10) out.password = "min";
    if (f.phone && !/^\+[1-9][0-9]{7,14}$/.test(f.phone)) out.phone = "e164";
    return out;
  }

  async function submit(e: FormEvent) {
    e.preventDefault();
    const v = validate();
    setFields(v);
    if (Object.keys(v).length) return;
    setBusy(true);
    setError(null);
    try {
      const r = await adminApi.createStore({ name: f.name.trim(), slug: f.slug, login: f.login, password: f.password, phone: f.phone || undefined });
      let activated = false;
      if (activate) {
        try {
          await adminApi.setStoreStatus(r.store_id, "active");
          activated = true;
        } catch (err) {
          setError(err); // магазин создан, активировать можно в карточке
        }
      }
      setCreated({ storeId: r.store_id, login: f.login, password: f.password, activated });
      onCreated();
    } catch (err) {
      setError(err);
      if (err instanceof ApiError) {
        if (err.code === "slug_taken") setFields({ slug: "taken" });
        else if (err.code === "login_taken") setFields({ login: "taken" });
        else setFields(err.fields);
      }
    } finally {
      setBusy(false);
    }
  }

  if (created) {
    const text = `${t("credLogin")}: ${created.login}\n${t("credPassword")}: ${created.password}`;
    return (
      <section className="card stack" aria-live="polite">
        <h2>{t("storeCreated")}</h2>
        <p className="small">{created.activated ? t("storeCreatedActive") : t("storeCreatedPending")}</p>
        <pre className="copy-text pre card">{text}</pre>
        <p className="small muted">{t("passwordOnce")}</p>
        <div className="row wrap">
          <button type="button" className="btn btn-primary" onClick={async () => setCopied(await copyToClipboard(text))}>
            {copied ? t("copied") : t("copy")}
          </button>
          <button
            type="button"
            className="btn btn-secondary"
            onClick={() => {
              setCreated(null);
              setCopied(false);
              setF({ name: "", slug: "", login: "", password: generatePassword(), phone: "" });
            }}
          >
            {t("createAnother")}
          </button>
        </div>
      </section>
    );
  }

  const field = (k: keyof typeof f, label: string, extra: Record<string, unknown> = {}, hint?: string) => (
    <div className="field">
      <label htmlFor={`cs-${k}`}>{label}</label>
      <input id={`cs-${k}`} value={f[k]} onChange={set(k)} aria-invalid={!!fields[k]} {...extra} />
      {hint && <span className="small muted">{hint}</span>}
      {fields[k] && <span className="field-error">{fields[k] === "taken" ? t("taken") : fe(fields[k])}</span>}
    </div>
  );

  return (
    <form onSubmit={submit} className="card" noValidate>
      <h2>{t("newStore")}</h2>
      {field("name", t("storeName"))}
      {field("slug", t("storeSlug"), { autoCapitalize: "none", spellCheck: false }, t("slugHint"))}
      {field("login", t("storeLogin"), { autoCapitalize: "none", spellCheck: false, autoComplete: "off" }, t("loginHint"))}
      <div className="field">
        <label htmlFor="cs-password">{t("storePassword")}</label>
        <div className="row">
          <input id="cs-password" className="grow" value={f.password} onChange={set("password")} autoComplete="new-password" spellCheck={false} aria-invalid={!!fields.password} />
          <button type="button" className="btn btn-secondary" onClick={() => setF({ ...f, password: generatePassword() })}>{t("generate")}</button>
        </div>
        {fields.password && <span className="field-error">{t("passwordMin")}</span>}
      </div>
      {field("phone", t("storePhone"), { type: "tel", placeholder: "+998901234567" })}
      <label className="row small">
        <input type="checkbox" checked={activate} onChange={(e) => setActivate(e.target.checked)} /> {t("activateNow")}
      </label>
      <ErrorBox error={error} />
      <button className="btn btn-primary" disabled={busy}>{t("create")}</button>
    </form>
  );
}
