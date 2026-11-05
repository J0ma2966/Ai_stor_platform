"use client";
import { useEffect, useMemo, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Link } from "@/i18n/navigation";
import { ErrorBox } from "@/components/ErrorBox";
import { ProductStatusBadge } from "@/components/ProductStatusBadge";
import { formatSum } from "@/lib/money";
import type { Product } from "@/lib/catalog";
import { listMyProducts, setProductStatus } from "@/lib/storeCab";

const FILTERS = ["all", "published", "draft", "unpublished", "blocked"] as const;

export default function StoreProducts() {
  const t = useTranslations("storeProducts");
  const tc = useTranslations("storeCab");
  const ta = useTranslations("app");
  const locale = useLocale();
  const [items, setItems] = useState<Product[] | null>(null);
  const [q, setQ] = useState("");
  const [filter, setFilter] = useState<(typeof FILTERS)[number]>("all");
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState("");

  const load = () => {
    setError(null);
    listMyProducts().then((r) => setItems(r.items)).catch(setError);
  };
  useEffect(load, []);
  const shown = useMemo(() => {
    const s = q.trim().toLowerCase();
    return (items ?? []).filter((p) => (filter === "all" || p.status === filter) && (!s || `${p.title} ${p.sku} ${p.brand}`.toLowerCase().includes(s)));
  }, [items, q, filter]);

  async function toggle(p: Product) {
    setBusy(p.id);
    setError(null);
    try {
      const r = await setProductStatus(p.id, p.status === "published" ? "draft" : "published");
      setItems((prev) => prev?.map((x) => (x.id === p.id ? { ...x, status: r.status as Product["status"] } : x)) ?? prev);
    } catch (e) {
      setError(e);
    } finally {
      setBusy("");
    }
  }

  return (
    <>
      <div className="row between wrap">
        <h1>{t("title")}</h1>
        <div className="row">
          <Link href="/store/import" className="btn">{tc("import")}</Link>
          <Link href="/store/products/new" className="btn btn-primary">{tc("addProduct")}</Link>
        </div>
      </div>
      <div className="row wrap">
        <input type="search" aria-label={t("search")} placeholder={t("search")} value={q} onChange={(e) => setQ(e.target.value)} style={{ maxWidth: 320 }} />
        <div className="tabs" role="tablist">
          {FILTERS.map((f) => (
            <button key={f} role="tab" aria-selected={filter === f} className={filter === f ? "tab tab-on" : "tab"} onClick={() => setFilter(f)}>
              {f === "all" ? t("filterAll") : t(`st_${f}`)}
            </button>
          ))}
        </div>
      </div>
      <ErrorBox error={error} onRetry={load} />
      {items === null ? (
        !error && <p className="muted">{ta("loading")}</p>
      ) : items.length === 0 ? (
        <p className="empty">{t("empty")}</p>
      ) : (
        <>
          <p className="muted small">{t("count", { n: shown.length, total: items.length })}</p>
          <div className="table-wrap">
            <table className="tbl">
              <thead><tr><th /><th>{t("title")}</th><th>{t("sku")}</th><th>{t("price")}</th><th>{t("views")}</th><th>{t("status")}</th><th /></tr></thead>
              <tbody>
                {shown.map((p) => (
                  <tr key={p.id}>
                    <td>{p.images.front ? <img src={p.thumbs?.front ?? p.images.front} alt="" width={40} height={53} loading="lazy" /> : <span className="muted small">{t("noPhotos")}</span>}</td>
                    <td><Link href={`/store/products/${p.id}`}>{p.title}</Link><br /><span className="muted small">{p.brand}</span>{p.moderation_reason && (p.status === "unpublished" || p.status === "blocked") && <><br /><span className="field-error">{t("reason", { reason: p.moderation_reason })}</span></>}</td>
                    <td>{p.sku}</td>
                    <td>{formatSum(p.price, locale)}</td>
                    <td>{p.views ?? 0}</td>
                    <td><ProductStatusBadge status={p.status ?? "draft"} /></td>
                    <td>
                      {(p.status === "draft" || p.status === "published") && (
                        <button type="button" className="btn btn-secondary small" disabled={busy === p.id} onClick={() => toggle(p)}>
                          {p.status === "published" ? t("unpublish") : t("publish")}
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}
    </>
  );
}
