"use client";
import { useEffect, useState, type FormEvent } from "react";
import { useTranslations } from "next-intl";
import { ErrorBox } from "@/components/ErrorBox";
import { createCategory, listAdminCategories, updateCategory, type AdminCategory } from "@/lib/adminCat";

const ZONES = ["headwear", "top", "bottom", "shoes"] as const;
const empty = { slug: "", zone: "top", name_ru: "", name_uz: "", sort: 100 };

export default function AdminCategories() {
  const t = useTranslations("adminCat");
  const ta = useTranslations("app");
  const [items, setItems] = useState<AdminCategory[] | null>(null);
  const [f, setF] = useState(empty);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const load = () => listAdminCategories().then((r) => setItems(r.items)).catch(setError);
  useEffect(() => void load(), []);

  async function add(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await createCategory({ ...f, slug: f.slug.trim().toLowerCase() });
      setF(empty);
      await load();
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  }
  async function patch(c: AdminCategory, b: Partial<AdminCategory>) {
    setError(null);
    try {
      await updateCategory(c.id, b);
      setItems((p) => p?.map((x) => (x.id === c.id ? { ...x, ...b } : x)) ?? p);
    } catch (err) {
      setError(err);
    }
  }
  return (
    <>
      <h1>{t("categories")}</h1>
      <ErrorBox error={error} />
      {items === null ? <p className="muted">{ta("loading")}</p> : (
        <div className="table-wrap">
          <table className="tbl">
            <thead><tr><th>{t("catSlug")}</th><th>{t("catRu")}</th><th>{t("catUz")}</th><th>{t("catZone")}</th><th>{t("catSort")}</th><th>{t("catProducts")}</th><th>{t("catActive")}</th></tr></thead>
            <tbody>
              {items.map((c) => (
                <tr key={c.id}>
                  <td><code>{c.slug}</code></td>
                  <td><input aria-label={t("catRu")} defaultValue={c.name_ru} maxLength={100} onBlur={(e) => e.target.value.trim() && e.target.value !== c.name_ru && patch(c, { name_ru: e.target.value.trim() })} /></td>
                  <td><input aria-label={t("catUz")} defaultValue={c.name_uz} maxLength={100} onBlur={(e) => e.target.value.trim() && e.target.value !== c.name_uz && patch(c, { name_uz: e.target.value.trim() })} /></td>
                  <td>
                    <select aria-label={t("catZone")} value={c.zone} onChange={(e) => patch(c, { zone: e.target.value })}>
                      {ZONES.map((z) => <option key={z} value={z}>{t(`zone_${z}`)}</option>)}
                    </select>
                  </td>
                  <td><input aria-label={t("catSort")} type="number" min={0} max={10000} defaultValue={c.sort} style={{ width: 90 }} onBlur={(e) => Number(e.target.value) !== c.sort && patch(c, { sort: Number(e.target.value) })} /></td>
                  <td>{c.products}</td>
                  <td><input type="checkbox" aria-label={t("catActive")} checked={c.is_active} onChange={(e) => patch(c, { is_active: e.target.checked })} /></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <form className="card stack-sm" onSubmit={add}>
        <h2>{t("catAdd")}</h2>
        <div className="grid-4">
          <div className="field"><label htmlFor="c-slug">{t("catSlug")}</label><input id="c-slug" required pattern="[a-z0-9][a-z0-9-]{1,62}" value={f.slug} onChange={(e) => setF({ ...f, slug: e.target.value })} /></div>
          <div className="field"><label htmlFor="c-ru">{t("catRu")}</label><input id="c-ru" required maxLength={100} value={f.name_ru} onChange={(e) => setF({ ...f, name_ru: e.target.value })} /></div>
          <div className="field"><label htmlFor="c-uz">{t("catUz")}</label><input id="c-uz" required maxLength={100} value={f.name_uz} onChange={(e) => setF({ ...f, name_uz: e.target.value })} /></div>
          <div className="field"><label htmlFor="c-zone">{t("catZone")}</label>
            <select id="c-zone" value={f.zone} onChange={(e) => setF({ ...f, zone: e.target.value })}>{ZONES.map((z) => <option key={z} value={z}>{t(`zone_${z}`)}</option>)}</select>
          </div>
        </div>
        <div><button className="btn btn-primary" disabled={busy}>{t("catAdd")}</button></div>
      </form>
    </>
  );
}
