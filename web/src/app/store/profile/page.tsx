"use client";
import { useEffect, useState, type FormEvent } from "react";
import { useTranslations } from "next-intl";
import { Link } from "@/i18n/navigation";
import { ApiError } from "@/lib/api";
import type { StoreContacts } from "@/lib/catalog";
import { getStoreProfile, updateStoreProfile } from "@/lib/storeCab";
import { ErrorBox, useFieldError } from "@/components/ErrorBox";
import { ImagePicker } from "@/components/ImagePicker";
import { TelegramConnect } from "@/components/TelegramConnect";

type Form = { name: string; description: string; phone: string; address: string; lat: string; lng: string } & Required<StoreContacts>;
const CONTACTS = ["telegram", "instagram", "website", "email", "hours"] as const;

export default function StoreProfile() {
  const t = useTranslations("storeProfile");
  const ta = useTranslations("app");
  const fe = useFieldError();
  const [slug, setSlug] = useState("");
  const [f, setF] = useState<Form | null>(null);
  const [img, setImg] = useState<{ logo?: string; cover?: string }>({});
  const [patch, setPatch] = useState<Record<string, unknown>>({});
  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState<unknown>(null);

  const load = () =>
    getStoreProfile()
      .then((s) => {
        setSlug(s.slug);
        setImg({ logo: s.logo_url, cover: s.cover_url });
        setF({
          name: s.name, description: s.description, phone: s.phone ?? "", address: s.address ?? "",
          lat: s.lat?.toString() ?? "", lng: s.lng?.toString() ?? "",
          telegram: s.contacts.telegram ?? "", instagram: s.contacts.instagram ?? "", website: s.contacts.website ?? "", email: s.contacts.email ?? "", hours: s.contacts.hours ?? "",
        });
      })
      .catch(setError);
  useEffect(() => void load(), []);
  const fields = error instanceof ApiError ? error.fields : {};

  async function save(e: FormEvent) {
    e.preventDefault();
    if (!f) return;
    setBusy(true);
    setError(null);
    setSaved(false);
    const contacts: StoreContacts = {};
    for (const k of CONTACTS) if (f[k].trim()) contacts[k] = f[k].trim();
    try {
      await updateStoreProfile({
        name: f.name, description: f.description, address: f.address,
        ...(f.phone.trim() ? { phone: f.phone.trim() } : {}),
        ...(f.lat && f.lng ? { lat: Number(f.lat), lng: Number(f.lng) } : {}),
        contacts, ...patch,
      });
      setPatch({});
      setSaved(true);
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  }
  if (!f) return error ? <ErrorBox error={error} onRetry={load} /> : <p className="muted">{ta("loading")}</p>;
  const input = (k: keyof Form, type = "text", max = 200) => (
    <div className="field">
      <label htmlFor={`sp-${k}`}>{t(k)}</label>
      <input id={`sp-${k}`} type={type} step={type === "number" ? "any" : undefined} maxLength={max} value={f[k]} onChange={(e) => setF({ ...f, [k]: e.target.value })} aria-invalid={!!fields[k] || !!fields[`contacts.${k}`]} />
      {(fields[k] || fields[`contacts.${k}`]) && <span className="field-error">{fe(fields[k] ?? fields[`contacts.${k}`])}</span>}
    </div>
  );
  return (
    <>
      <div className="row between wrap">
        <h1>{t("title")}</h1>
        {slug && <Link href={`/stores/${slug}`}>{t("publicPage")} ↗</Link>}
      </div>
      <TelegramConnect />
      <form className="stack" onSubmit={save}>
        <fieldset className="card stack" disabled={busy}>
          {input("name")}
          <div className="field">
            <label htmlFor="sp-d">{t("description")}</label>
            <textarea id="sp-d" rows={5} maxLength={5000} value={f.description} onChange={(e) => setF({ ...f, description: e.target.value })} />
          </div>
          <div className="photo-slots">
            <ImagePicker label={t("logo")} url={img.logo} purpose="store" onUploaded={(k, u) => { setPatch((p) => ({ ...p, logo_key: k, clear_logo: false })); setImg((p) => ({ ...p, logo: u })); }}
              onClear={() => { setPatch((p) => ({ ...p, clear_logo: true, logo_key: undefined })); setImg((p) => ({ ...p, logo: undefined })); }} clearLabel={t("removeImage")} />
            <ImagePicker label={t("cover")} url={img.cover} purpose="store" onUploaded={(k, u) => { setPatch((p) => ({ ...p, cover_key: k, clear_cover: false })); setImg((p) => ({ ...p, cover: u })); }}
              onClear={() => { setPatch((p) => ({ ...p, clear_cover: true, cover_key: undefined })); setImg((p) => ({ ...p, cover: undefined })); }} clearLabel={t("removeImage")} />
          </div>
        </fieldset>
        <fieldset className="card stack" disabled={busy}>
          <div className="grid-2">{input("phone", "tel", 16)}{input("email", "email")}</div>
          <div className="grid-2">{input("telegram", "text", 64)}{input("instagram", "text", 64)}</div>
          <div className="grid-2">{input("website", "url")}{input("hours")}</div>
          {input("address", "text", 300)}
          <p className="muted small">{t("addressHint")}</p>
          <fieldset className="plain-fieldset">
            <legend className="label">{t("coords")}</legend>
            <div className="row">{input("lat", "number")}{input("lng", "number")}</div>
          </fieldset>
        </fieldset>
        <ErrorBox error={error} />
        <div className="row">
          <button className="btn btn-primary" disabled={busy}>{ta("save")}</button>
          {saved && <span className="ok-text" role="status">{ta("saved")}</span>}
        </div>
      </form>
    </>
  );
}
