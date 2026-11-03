"use client";
import { useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Link } from "@/i18n/navigation";
import { adminApi } from "@/lib/admin";
import { formatDateTime, formatSum } from "@/lib/money";
import { ErrorBox } from "@/components/ErrorBox";
import { useCursorList } from "@/components/admin/useCursorList";

const STATUSES = ["", "paid", "pending", "declined", "cancelled"] as const;
const tone: Record<string, string> = { paid: "ok", pending: "warn", declined: "bad", cancelled: "bad" };

export default function AdminPaymentsPage() {
  const t = useTranslations("admin");
  const tp = useTranslations("admin.paymentStatus");
  const locale = useLocale();
  const [status, setStatus] = useState("");
  const [provider, setProvider] = useState("");
  const list = useCursorList((cursor) => adminApi.payments({ status, provider, cursor }), `${status}|${provider}`);

  return (
    <>
      <h1>{t("navPayments")}</h1>
      <div className="row wrap">
        <div className="tabs grow" role="tablist">
          {STATUSES.map((s) => (
            <button key={s || "all"} role="tab" aria-selected={status === s} className={status === s ? "tab tab-on" : "tab"} onClick={() => setStatus(s)}>
              {s ? tp(s) : t("all")}
            </button>
          ))}
        </div>
        <select aria-label={t("provider")} value={provider} onChange={(e) => setProvider(e.target.value)}>
          <option value="">{t("allProviders")}</option>
          <option value="click">Click</option>
          <option value="payme">Payme</option>
        </select>
      </div>
      <ErrorBox error={list.error} onRetry={list.reload} />
      {!list.loading && !list.error && list.items.length === 0 && <p className="muted">{t("nothing")}</p>}
      <ul className="plain stack">
        {list.items.map((p) => (
          <li key={p.id} className="card order-row">
            <span className="order-main">
              <strong>{formatSum(p.amount, locale)} · {t("tokensN", { n: p.tokens })}</strong>
              <span className="muted small">
                {p.provider === "click" ? "Click" : "Payme"}
                {p.provider_txn_id && ` · ${p.provider_txn_id}`} · {formatDateTime(p.paid_at ?? p.created_at, locale)}
              </span>
              <Link href={`/admin/users/${p.user_id}`} className="small">{p.user_phone ?? t("openUser")}</Link>
            </span>
            <span className="order-side">
              <span className={`badge badge-${tone[p.status] ?? "info"}`}>{tp.has(p.status) ? tp(p.status) : p.status}</span>
            </span>
          </li>
        ))}
      </ul>
      {list.hasMore && <button className="btn btn-secondary" disabled={list.loading} onClick={list.more}>{t("more")}</button>}
    </>
  );
}
