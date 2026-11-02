"use client";
import { useCallback, useEffect, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { ApiError } from "@/lib/api";
import { formatDateTime, formatSum } from "@/lib/money";
import { copyToClipboard, getOrder, setOrderStatus, type OrderDetail, type OrderStatus } from "@/lib/orders";
import { ErrorBox } from "./ErrorBox";
import { StatusBadge } from "./Status";
import { Items } from "./Items";

const NEEDS_REASON: OrderStatus[] = ["rejected", "cancelled"];

// Окно доставки для магазина: всё, что нужно передать курьеру, и смена статуса.
// Все значения выводятся как текст (React экранирует), без dangerouslySetInnerHTML.
export function DeliveryWindow({ id, scope = "store" }: { id: string; scope?: "store" | "admin" }) {
  const t = useTranslations("window");
  const ts = useTranslations("status");
  const ta = useTranslations("action");
  const tApp = useTranslations("app");
  const locale = useLocale();
  const [o, setO] = useState<OrderDetail | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [copied, setCopied] = useState<"" | "ok" | "fail">("");
  const [pending, setPending] = useState<OrderStatus | null>(null);
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(() => {
    setError(null);
    return getOrder(scope, id).then(setO, setError);
  }, [id, scope]);
  useEffect(() => {
    load();
  }, [load]);

  async function copy() {
    if (!o?.copy_text) return;
    setCopied((await copyToClipboard(o.copy_text)) ? "ok" : "fail");
    setTimeout(() => setCopied(""), 4000);
  }

  async function apply(next: OrderStatus) {
    if (!o) return;
    if (NEEDS_REASON.includes(next) && !reason.trim()) return;
    setBusy(true);
    setError(null);
    try {
      setO(await setOrderStatus(scope, o.id, next, o.status, NEEDS_REASON.includes(next) ? reason : ""));
      setPending(null);
      setReason("");
    } catch (e) {
      setError(e);
      if (e instanceof ApiError && (e.code === "status_changed" || e.code === "invalid_transition")) await load();
    } finally {
      setBusy(false);
    }
  }

  if (!o) return error ? <ErrorBox error={error} onRetry={load} /> : <p className="muted">{tApp("loading")}</p>;
  const d = o.delivery;
  const rows: [string, string | undefined][] = [
    [t("street"), `${d.street}, ${d.house}`],
    [t("entrance"), d.entrance],
    [t("floor"), d.floor],
    [t("apartment"), d.apartment],
    [t("intercom"), d.intercom],
    [t("addressComment"), d.address_comment],
  ];

  return (
    <div className="stack">
      <header className="row between">
        <h1>{t("title", { number: o.number })}</h1>
        <StatusBadge status={o.status} />
      </header>
      <p className="muted">{formatDateTime(o.created_at, locale)}</p>

      <section className="card" aria-labelledby="dw-recipient">
        <h2 id="dw-recipient">{t("recipient")}</h2>
        <p>
          {o.customer_name} · <a href={`tel:${o.customer_phone}`}>{o.customer_phone}</a>
        </p>
      </section>

      <section className="card" aria-labelledby="dw-address">
        <h2 id="dw-address">{t("address")}</h2>
        <dl className="kv">
          {rows.map(([k, v]) => (
            <div key={k}>
              <dt>{k}</dt>
              <dd className="pre">{v || t("none")}</dd>
            </div>
          ))}
          <div>
            <dt>{t("location")}</dt>
            <dd>
              {d.maps_url ? (
                <a href={d.maps_url} target="_blank" rel="noopener noreferrer">
                  {t("openMap")} ({d.lat}, {d.lng})
                </a>
              ) : (
                t("noLocation")
              )}
            </dd>
          </div>
          <div>
            <dt>{t("timeComment")}</dt>
            <dd className="pre">{o.delivery_time_comment || t("none")}</dd>
          </div>
          <div>
            <dt>{t("orderComment")}</dt>
            <dd className="pre">{o.customer_comment || t("none")}</dd>
          </div>
        </dl>
      </section>

      <section className="card" aria-labelledby="dw-photo">
        <h2 id="dw-photo">{t("photo")}</h2>
        {d.photo_url ? (
          <a href={d.photo_url} target="_blank" rel="noopener noreferrer">
            <img className="photo" src={d.photo_url} alt={t("photo")} />
          </a>
        ) : (
          d.has_photo ? (
            <button type="button" className="btn btn-secondary" onClick={load}>{tApp("retry")}</button>
          ) : (
            <p className="muted">{t("noPhoto")}</p>
          )
        )}
      </section>

      <section className="card copy-box">
        <button type="button" className="btn btn-primary" onClick={copy}>
          {t("copy")}
        </button>
        <span role="status" aria-live="polite" className={copied === "fail" ? "field-error" : "ok-text"}>
          {copied === "ok" ? t("copied") : copied === "fail" ? t("copyFailed") : ""}
        </span>
        <textarea readOnly className="copy-text" rows={8} value={o.copy_text ?? ""} aria-label={t("copy")} onFocus={(e) => e.currentTarget.select()} />
      </section>

      <section className="card">
        <h2>{t("items")}</h2>
        <Items items={o.items} />
        <p className="total">{formatSum(o.items_total, locale)}</p>
      </section>

      <section className="card" aria-labelledby="dw-status">
        <h2 id="dw-status">{t("status")}: {ts(o.status)}</h2>
        <ErrorBox error={error} />
        {o.next_statuses && o.next_statuses.length > 0 ? (
          <div className="row wrap">
            {o.next_statuses.map((s) => (
              <button
                key={s}
                type="button"
                disabled={busy}
                className={NEEDS_REASON.includes(s) ? "btn btn-danger" : "btn btn-primary"}
                onClick={() => (NEEDS_REASON.includes(s) ? setPending(s) : apply(s))}
              >
                {ta(s)}
              </button>
            ))}
          </div>
        ) : (
          <p className="muted">{t("finished")}</p>
        )}
        {pending && (
          <div className="stack reason-box">
            <label htmlFor="dw-reason">{t("reasonLabel")} *</label>
            <textarea id="dw-reason" rows={2} maxLength={500} value={reason} placeholder={t("reasonPh")} onChange={(e) => setReason(e.target.value)} />
            <div className="row">
              <button type="button" className="btn btn-danger" disabled={busy || !reason.trim()} onClick={() => apply(pending)}>
                {ta(pending)}
              </button>
              <button type="button" className="btn btn-secondary" onClick={() => setPending(null)}>
                {t("cancel")}
              </button>
            </div>
          </div>
        )}
        <ol className="plain history">
          {o.history.map((h, i) => (
            <li key={i}>
              <span className="muted small">{formatDateTime(h.created_at, locale)}</span> {ts(h.to_status)}
              {h.reason && <span className="muted"> · {h.reason}</span>}
            </li>
          ))}
        </ol>
      </section>
    </div>
  );
}
