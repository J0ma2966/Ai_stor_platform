"use client";
import { useCallback, useEffect, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Link } from "@/i18n/navigation";
import { formatDateTime, formatSum } from "@/lib/money";
import { listOrders, type OrderStatus, type OrderSummary, type Scope } from "@/lib/orders";
import { ErrorBox } from "./ErrorBox";
import { StatusBadge } from "./Status";

const FILTERS: Record<string, OrderStatus[] | undefined> = {
  filterAll: undefined,
  filterNew: ["pending"],
  filterActive: ["confirmed", "assembling", "in_delivery"],
  filterDone: ["delivered", "rejected", "cancelled"],
};

export function OrderList({ scope }: { scope: Scope }) {
  const t = useTranslations("orders");
  const locale = useLocale();
  const [filter, setFilter] = useState("filterAll");
  const [items, setItems] = useState<OrderSummary[]>([]);
  const [cursor, setCursor] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);

  const load = useCallback(
    async (more: boolean, from = "") => {
      setLoading(true);
      setError(null);
      try {
        const r = await listOrders(scope, { status: FILTERS[filter], cursor: more ? from : "" });
        setItems((prev) => (more ? [...prev, ...r.items] : r.items));
        setCursor(r.nextCursor);
      } catch (e) {
        setError(e);
      } finally {
        setLoading(false);
      }
    },
    [scope, filter],
  );

  useEffect(() => {
    load(false);
  }, [load]);

  // Магазин: обновляем список новых заказов раз в 30 секунд, пока вкладка открыта.
  useEffect(() => {
    if (scope === "client") return;
    const id = setInterval(() => document.visibilityState === "visible" && load(false), 30000);
    return () => clearInterval(id);
  }, [scope, load]);

  const href = (o: OrderSummary) => (scope === "client" ? `/account/orders/${o.id}` : scope === "store" ? `/store/orders/${o.id}` : `/admin/orders/${o.id}`);

  return (
    <div className="stack">
      {scope !== "client" && (
        <div className="tabs" role="tablist">
          {Object.keys(FILTERS).map((k) => (
            <button key={k} role="tab" aria-selected={filter === k} className={filter === k ? "tab tab-on" : "tab"} onClick={() => setFilter(k)}>
              {t(k)}
            </button>
          ))}
        </div>
      )}
      <ErrorBox error={error} onRetry={() => load(false)} />
      {!loading && !error && items.length === 0 && <p className="muted">{t("empty")}</p>}
      <ul className="plain stack">
        {items.map((o) => (
          <li key={o.id}>
            <Link href={href(o)} className="card order-row">
              <span className="order-main">
                <strong>{t("number", { number: o.number })}</strong>
                <span className="muted small">
                  {formatDateTime(o.created_at, locale)} · {scope === "client" ? o.store_name : o.customer_name}
                </span>
                {scope !== "client" && o.short_address && <span className="small">{o.short_address}</span>}
              </span>
              <span className="order-side">
                <StatusBadge status={o.status} />
                <span className="small">{t("items", { count: o.items_count })} · {formatSum(o.items_total, locale)}</span>
              </span>
            </Link>
          </li>
        ))}
      </ul>
      {cursor && (
        <button className="btn btn-secondary" disabled={loading} onClick={() => load(true, cursor)}>
          {t("more")}
        </button>
      )}
    </div>
  );
}
