"use client";
import { useState } from "react";
import { useSearchParams } from "next/navigation";
import { useLocale, useTranslations } from "next-intl";
import { Link } from "@/i18n/navigation";
import { adminApi, type StoreStatus } from "@/lib/admin";
import { formatDateTime } from "@/lib/money";
import { ErrorBox } from "@/components/ErrorBox";
import { StoreStatusBadge } from "@/components/admin/Badges";
import { CreateStoreForm } from "@/components/admin/CreateStoreForm";
import { useCursorList } from "@/components/admin/useCursorList";
import { useDebounced } from "@/components/admin/useDebounced";

const FILTERS: ("" | StoreStatus)[] = ["", "pending_activation", "active", "blocked", "archived"];

export default function AdminStoresPage() {
  const t = useTranslations("admin");
  const ts = useTranslations("admin.storeStatus");
  const locale = useLocale();
  const initial = useSearchParams().get("status") ?? "";
  const [status, setStatus] = useState<string>(FILTERS.includes(initial as StoreStatus) ? initial : "");
  const [q, setQ] = useState("");
  const dq = useDebounced(q.trim(), 300);
  const [showForm, setShowForm] = useState(false);
  const list = useCursorList((cursor) => adminApi.stores({ status, q: dq, cursor }), `${status}|${dq}`);

  return (
    <>
      <div className="row between wrap">
        <h1>{t("navStores")}</h1>
        <button className="btn btn-primary" onClick={() => setShowForm((v) => !v)} aria-expanded={showForm}>
          {showForm ? t("close") : t("newStore")}
        </button>
      </div>
      {showForm && <CreateStoreForm onCreated={list.reload} />}
      <div className="tabs" role="tablist">
        {FILTERS.map((k) => (
          <button key={k || "all"} role="tab" aria-selected={status === k} className={status === k ? "tab tab-on" : "tab"} onClick={() => setStatus(k)}>
            {k ? ts(k) : t("all")}
          </button>
        ))}
      </div>
      <input type="search" aria-label={t("searchStores")} placeholder={t("searchStores")} value={q} onChange={(e) => setQ(e.target.value)} maxLength={100} />
      <ErrorBox error={list.error} onRetry={list.reload} />
      {!list.loading && !list.error && list.items.length === 0 && <p className="muted">{t("nothing")}</p>}
      <ul className="plain stack">
        {list.items.map((s) => (
          <li key={s.id}>
            <Link href={`/admin/stores/${s.id}`} className="card order-row">
              <span className="order-main">
                <strong>{s.name}</strong>
                <span className="muted small">/{s.slug} · {t("loginShort")}: {s.login ?? "—"}</span>
                <span className="muted small">{formatDateTime(s.created_at, locale)}</span>
              </span>
              <span className="order-side">
                <StoreStatusBadge status={s.status} />
                <span className="small">{t("counts", { products: s.products_count, orders: s.orders_count })}</span>
              </span>
            </Link>
          </li>
        ))}
      </ul>
      {list.hasMore && <button className="btn btn-secondary" disabled={list.loading} onClick={list.more}>{t("more")}</button>}
    </>
  );
}
