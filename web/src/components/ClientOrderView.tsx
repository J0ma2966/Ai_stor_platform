"use client";
import { useCallback, useEffect, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { formatDateTime, formatSum } from "@/lib/money";
import { getOrder, type OrderDetail } from "@/lib/orders";
import { ErrorBox } from "./ErrorBox";
import { StatusBadge, StatusStepper } from "./Status";
import { Items } from "./Items";

export function ClientOrderView({ id }: { id: string }) {
  const t = useTranslations();
  const locale = useLocale();
  const [o, setO] = useState<OrderDetail | null>(null);
  const [error, setError] = useState<unknown>(null);

  const load = useCallback(() => {
    setError(null);
    getOrder("client", id).then(setO, setError);
  }, [id]);
  useEffect(() => {
    load();
    // статус меняет магазин — периодически подтягиваем, пока заказ не завершён
    const timer = setInterval(() => document.visibilityState === "visible" && load(), 60000);
    return () => clearInterval(timer);
  }, [load]);

  if (error) return <ErrorBox error={error} onRetry={load} />;
  if (!o) return <p className="muted">{t("app.loading")}</p>;
  const d = o.delivery;
  const addr = [`${d.street}, ${d.house}`, d.entrance && `${t("window.entrance")} ${d.entrance}`, d.floor && `${t("window.floor")} ${d.floor}`, d.apartment && `${t("window.apartment")} ${d.apartment}`]
    .filter(Boolean)
    .join(", ");

  return (
    <div className="stack">
      <header className="row between">
        <h1>{t("orders.number", { number: o.number })}</h1>
        <StatusBadge status={o.status} />
      </header>
      <p className="muted">{o.store_name} · {formatDateTime(o.created_at, locale)}</p>

      <section className="card">
        <StatusStepper status={o.status} step={o.step} />
        {o.status === "pending" && <p>{t("orders.pendingHint")}</p>}
        {(o.status === "rejected" || o.status === "cancelled") && o.rejection_reason && (
          <p>{t("orders.rejectedReason", { reason: o.rejection_reason })}</p>
        )}
      </section>

      <section className="card">
        <h2>{t("orders.deliveryAddress")}</h2>
        <p>{addr}</p>
        {d.address_comment && <p className="pre">{d.address_comment}</p>}
        {o.delivery_time_comment && <p><span className="muted">{t("orders.timeComment")}: </span><span className="pre">{o.delivery_time_comment}</span></p>}
        {o.customer_comment && <p><span className="muted">{t("orders.yourComment")}: </span><span className="pre">{o.customer_comment}</span></p>}
        {d.photo_url && <img className="photo" src={d.photo_url} alt={t("window.photo")} />}
      </section>

      <section className="card">
        <Items items={o.items} reviewLabel={o.status === "delivered" ? t("reviews.rateItem") : undefined} />
        <p className="total">{t("orders.total")}: {formatSum(o.items_total, locale)}</p>
      </section>

      <section className="card">
        <h2>{t("orders.history")}</h2>
        <ol className="plain history">
          {o.history.map((h, i) => (
            <li key={i}>
              <span className="muted small">{formatDateTime(h.created_at, locale)}</span> {t(`status.${h.to_status}`)}
              {h.reason && <span className="muted"> · {h.reason}</span>}
            </li>
          ))}
        </ol>
      </section>
    </div>
  );
}
