"use client";
import { useCallback, useEffect, useRef, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { ApiError, newIdempotencyKey } from "@/lib/api";
import { formatDateTime, formatSum } from "@/lib/money";
import {
  createPayment, getBalance, getPayment, listPayments, listTariffs, safeCheckoutUrl,
  type Payment, type PaymentStatus, type Provider, type Tariff,
} from "@/lib/payments";
import { ErrorBox } from "./ErrorBox";

const POLL_MS = 3000;
const POLL_LIMIT = 40; // ~2 минуты ждём подтверждения от провайдера

const badge: Record<PaymentStatus, string> = { pending: "badge-warn", paid: "badge-ok", declined: "badge-bad", cancelled: "badge-bad" };

// Кабинет клиента: баланс токенов, покупка пакета через Click/Payme и история платежей.
// После оплаты провайдер возвращает клиента сюда с ?payment=<id> — ждём вебхук и показываем итог.
export function BalancePage({ returnedPaymentId }: { returnedPaymentId?: string }) {
  const t = useTranslations("balance");
  const locale = useLocale();
  const [balance, setBalance] = useState<number | null>(null);
  const [tariffs, setTariffs] = useState<Tariff[]>([]);
  const [history, setHistory] = useState<Payment[]>([]);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState<string>("");
  const [returned, setReturned] = useState<Payment | null>(null);
  const [waiting, setWaiting] = useState(false);
  // ключ на пару «тариф+провайдер»: повторный клик/ретрай не создаёт второй платёж
  const keys = useRef(new Map<string, string>());

  const load = useCallback(async () => {
    setError(null);
    try {
      const [b, ts, h] = await Promise.all([getBalance(), listTariffs(), listPayments()]);
      setBalance(b);
      setTariffs(ts);
      setHistory(h.items);
    } catch (e) {
      setError(e);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    if (!returnedPaymentId) return;
    let n = 0;
    let stop = false;
    setWaiting(true);
    const tick = async () => {
      try {
        const p = await getPayment(returnedPaymentId);
        if (stop) return;
        setReturned(p);
        if (p.status !== "pending" || ++n >= POLL_LIMIT) {
          setWaiting(false);
          if (p.status === "paid") load();
          return;
        }
      } catch (e) {
        if (stop) return;
        if (e instanceof ApiError && e.status === 404) {
          setWaiting(false);
          return;
        }
      }
      timer = setTimeout(tick, POLL_MS);
    };
    let timer = setTimeout(tick, 0);
    return () => {
      stop = true;
      clearTimeout(timer);
    };
  }, [returnedPaymentId, load]);

  const buy = async (tariff: Tariff, provider: Provider) => {
    const k = `${tariff.id}:${provider}`;
    if (!keys.current.has(k)) keys.current.set(k, newIdempotencyKey());
    setBusy(k);
    setError(null);
    try {
      const p = await createPayment(tariff.id, provider, keys.current.get(k)!);
      const url = safeCheckoutUrl(p.checkout_url);
      if (!url) throw new ApiError(0, "checkout_unavailable", "no checkout url");
      keys.current.delete(k); // следующий клик — новая покупка
      window.location.assign(url);
    } catch (e) {
      setError(e);
      setBusy("");
    }
  };

  const name = (tf: Tariff) => (locale === "uz" ? tf.name_uz : tf.name_ru);

  return (
    <div className="stack">
      <h1>{t("title")}</h1>
      <section className="card">
        <p className="muted small">{t("current")}</p>
        <p className="balance-value" data-testid="balance">{balance === null ? "…" : t("tokens", { count: balance })}</p>
      </section>

      {returned && (
        <div role="status" className={`alert ${returned.status === "paid" ? "alert-ok" : ""}`}>
          {returned.status === "paid" && <p>{t("paidOk", { count: returned.tokens })}</p>}
          {returned.status === "pending" && <p>{waiting ? t("waiting") : t("stillPending")}</p>}
          {(returned.status === "declined" || returned.status === "cancelled") && <p>{t("failed")}</p>}
        </div>
      )}

      <ErrorBox error={error} onRetry={load} />

      <section className="stack">
        <h2>{t("buy")}</h2>
        {tariffs.length === 0 && !error && <p className="muted">{t("noTariffs")}</p>}
        <ul className="plain tariff-grid">
          {tariffs.map((tf) => (
            <li key={tf.id} className={`card stack ${tf.highlighted ? "tariff-hl" : ""}`}>
              <strong>{name(tf)}</strong>
              {tf.highlighted && <span className="badge badge-info">{t("popular")}</span>}
              <span className="balance-value">{t("tokens", { count: tf.tokens })}</span>
              <span>{formatSum(tf.price, locale)}</span>
              {tf.description && <span className="muted small">{tf.description}</span>}
              <div className="row">
                {(["click", "payme"] as Provider[]).map((pr) => (
                  <button key={pr} className="btn btn-primary" disabled={busy !== ""} onClick={() => buy(tf, pr)}>
                    {busy === `${tf.id}:${pr}` ? t("redirecting") : t(pr)}
                  </button>
                ))}
              </div>
            </li>
          ))}
        </ul>
      </section>

      {history.length > 0 && (
        <section className="stack">
          <h2>{t("history")}</h2>
          <ul className="plain stack">
            {history.map((p) => (
              <li key={p.id} className="card row between">
                <span className="stack-tight">
                  <strong>{t("tokens", { count: p.tokens })} · {formatSum(p.amount, locale)}</strong>
                  <span className="muted small">{formatDateTime(p.created_at, locale)} · {t(p.provider)}</span>
                </span>
                <span className={`badge ${badge[p.status]}`}>{t(`status.${p.status}`)}</span>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  );
}
