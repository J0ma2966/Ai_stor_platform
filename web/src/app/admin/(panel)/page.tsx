"use client";
import { useCallback, useEffect, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Link } from "@/i18n/navigation";
import { adminApi, type Stats } from "@/lib/admin";
import { formatSum } from "@/lib/money";
import { ErrorBox } from "@/components/ErrorBox";
import { StatusBadge } from "@/components/Status";
import type { OrderStatus } from "@/lib/orders";

const STATUS_ORDER: OrderStatus[] = ["pending", "confirmed", "assembling", "in_delivery", "delivered", "rejected", "cancelled"];

function Tile({ label, value, hint, href }: { label: string; value: string | number; hint?: string; href?: string }) {
  const body = (
    <>
      <span className="muted small">{label}</span>
      <strong className="tile-value">{value}</strong>
      {hint && <span className="small muted">{hint}</span>}
    </>
  );
  return href ? <Link href={href} className="card tile">{body}</Link> : <div className="card tile">{body}</div>;
}

export default function AdminDashboard() {
  const t = useTranslations("admin");
  const tApp = useTranslations("app");
  const locale = useLocale();
  const [s, setS] = useState<Stats | null>(null);
  const [error, setError] = useState<unknown>(null);
  const load = useCallback(() => {
    setError(null);
    adminApi.stats().then(setS, setError);
  }, []);
  useEffect(load, [load]);

  if (!s) return error ? <ErrorBox error={error} onRetry={load} /> : <p className="muted">{tApp("loading")}</p>;
  return (
    <>
      <h1>{t("navDashboard")}</h1>
      <div className="tiles">
        <Tile label={t("statStores")} value={s.stores.active} hint={t("statStoresHint", { total: s.stores.total, pending: s.stores.pending_activation })} href="/admin/stores" />
        <Tile label={t("statClients")} value={s.users.clients} hint={t("statClientsHint", { n: s.users.clients_new_7d })} href="/admin/users" />
        <Tile label={t("statOrdersToday")} value={s.orders.today} hint={t("statOrdersHint", { week: s.orders.last_7d, total: s.orders.total })} href="/admin/orders" />
        <Tile label={t("statRevenue30")} value={formatSum(s.orders.delivered_sum_30d, locale)} />
        <Tile label={t("statProducts")} value={s.products.published} hint={t("statProductsHint", { total: s.products.total })} />
        <Tile label={t("statTokens30")} value={`+${s.tokens.issued_30d} / −${s.tokens.spent_30d}`} />
        <Tile label={t("statBlocked")} value={s.users.blocked + s.stores.blocked} hint={t("statBlockedHint", { users: s.users.blocked, stores: s.stores.blocked })} />
      </div>
      <section className="card">
        <h2>{t("ordersByStatus")}</h2>
        <ul className="plain by-status">
          {STATUS_ORDER.map((st) => [st, s.orders.by_status[st] ?? 0] as const).map(([st, n]) => (
            <li key={st} className="row between">
              <StatusBadge status={st} />
              <strong>{n}</strong>
            </li>
          ))}
        </ul>
      </section>
      {s.stores.pending_activation > 0 && (
        <p>
          <Link href="/admin/stores?status=pending_activation">{t("pendingStoresLink", { n: s.stores.pending_activation })}</Link>
        </p>
      )}
    </>
  );
}
