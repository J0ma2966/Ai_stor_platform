"use client";
import { useCallback, useEffect, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Link } from "@/i18n/navigation";
import { ErrorBox } from "@/components/ErrorBox";
import { ProductStatusBadge } from "@/components/ProductStatusBadge";
import { formatSum } from "@/lib/money";
import { listAdminProducts, moderateProduct, type AdminProduct } from "@/lib/adminCat";

const STATUSES = ["", "published", "draft", "unpublished", "blocked"] as const;

// Модерация: все товары всех магазинов, снятие/блокировка с причиной (её видит магазин).
export default function AdminProducts() {
  const t = useTranslations("adminCat");
  const ts = useTranslations("storeProducts");
  const ta = useTranslations("app");
  const ta2 = useTranslations("admin");
  const locale = useLocale();
  const [q, setQ] = useState("");
  const [status, setStatus] = useState("");
  const [items, setItems] = useState<AdminProduct[] | null>(null);
  const [cursor, setCursor] = useState("");
  const [error, setError] = useState<unknown>(null);
  const [acting, setActing] = useState<{ id: string; status: "unpublished" | "blocked" } | null>(null);
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async (from = "") => {
    setError(null);
    try {
      const r = await listAdminProducts({ q: q.trim(), status, cursor: from });
      setItems((p) => (from ? [...(p ?? []), ...r.items] : r.items));
      setCursor(r.nextCursor);
    } catch (e) {
      setError(e);
    }
  }, [q, status]);
  useEffect(() => {
    const id = setTimeout(() => load(), 300);
    return () => clearTimeout(id);
  }, [load]);

  async function act(id: string, st: "unpublished" | "blocked" | "draft", why: string) {
    setBusy(true);
    setError(null);
    try {
      await moderateProduct(id, st, why);
      setItems((p) => p?.map((x) => (x.id === id ? { ...x, status: st, moderation_reason: why || null } : x)) ?? p);
      setActing(null);
      setReason("");
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <h1>{t("products")}</h1>
      <div className="row wrap">
        <input type="search" aria-label={t("search")} placeholder={t("search")} value={q} onChange={(e) => setQ(e.target.value)} style={{ maxWidth: 320 }} />
        <select aria-label={t("status")} value={status} onChange={(e) => setStatus(e.target.value)}>
          {STATUSES.map((s) => <option key={s} value={s}>{s ? ts(`st_${s}`) : ts("filterAll")}</option>)}
        </select>
      </div>
      <ErrorBox error={error} onRetry={() => load()} />
      {items === null ? (
        !error && <p className="muted">{ta("loading")}</p>
      ) : items.length === 0 ? (
        <p className="empty">{t("empty")}</p>
      ) : (
        <div className="table-wrap">
          <table className="tbl">
            <thead><tr><th /><th>{ts("title")}</th><th>{t("store")}</th><th>{ts("price")}</th><th>{t("status")}</th><th /></tr></thead>
            <tbody>
              {items.map((p) => (
                <tr key={p.id}>
                  <td>{p.image_url ? <img src={p.image_url} alt="" width={40} height={53} loading="lazy" /> : null}</td>
                  <td>
                    {p.status === "published" ? <Link href={`/product/${p.id}`} target="_blank">{p.title}</Link> : p.title}
                    <br /><span className="muted small">{p.brand} · {p.sku}</span>
                    {p.moderation_reason && <><br /><span className="small">{ts("reason", { reason: p.moderation_reason })}</span></>}
                  </td>
                  <td>{p.store_name}</td>
                  <td>{formatSum(p.price, locale)}</td>
                  <td><ProductStatusBadge status={p.status} /></td>
                  <td>
                    {acting?.id === p.id ? (
                      <form className="stack-sm" onSubmit={(e) => { e.preventDefault(); if (reason.trim().length >= 3) act(p.id, acting.status, reason.trim()); }}>
                        <input aria-label={t("reasonPh")} placeholder={t("reasonPh")} value={reason} maxLength={500} onChange={(e) => setReason(e.target.value)} autoFocus />
                        {reason.trim().length > 0 && reason.trim().length < 3 && <span className="field-error">{t("reasonRequired")}</span>}
                        <div className="row">
                          <button className="btn btn-danger" disabled={busy || reason.trim().length < 3}>{acting.status === "blocked" ? t("block") : t("unpublish")}</button>
                          <button type="button" className="btn btn-secondary" onClick={() => setActing(null)}>{ta("cancel")}</button>
                        </div>
                      </form>
                    ) : (
                      <div className="row wrap">
                        {p.status !== "unpublished" && p.status !== "blocked" && <button type="button" className="btn btn-secondary" onClick={() => { setActing({ id: p.id, status: "unpublished" }); setReason(""); }}>{t("unpublish")}</button>}
                        {p.status !== "blocked" && <button type="button" className="btn btn-danger" onClick={() => { setActing({ id: p.id, status: "blocked" }); setReason(""); }}>{t("block")}</button>}
                        {(p.status === "unpublished" || p.status === "blocked") && <button type="button" className="btn" disabled={busy} onClick={() => act(p.id, "draft", "")}>{t("restore")}</button>}
                        {p.status === "published" && (
                          <Link href={`/admin/promotions?product=${p.id}&title=${encodeURIComponent(p.title)}`} className="btn btn-secondary">{ta2("navPromotions")}</Link>
                        )}
                      </div>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {cursor && <div className="center"><button type="button" className="btn" onClick={() => load(cursor)}>{ta("more")}</button></div>}
    </>
  );
}
