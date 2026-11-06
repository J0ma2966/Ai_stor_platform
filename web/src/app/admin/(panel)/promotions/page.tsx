"use client";
import { Suspense, useCallback, useEffect, useId, useState } from "react";
import { useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { ErrorBox } from "@/components/ErrorBox";
import { PromotionCard } from "@/components/PromotionCard";
import { ConfirmAction } from "@/components/admin/ConfirmAction";
import {
  addDays, approvePromotion, createPromotion, dateToEnd, dateToStart, isoToDate, listAdminPromotions, PROMO_STATES,
  rejectPromotion, stopPromotion, type Promotion,
} from "@/lib/promotions";

// Реклама товаров: заявки магазинов (одобрить со сроком / отклонить с причиной),
// идущая и запланированная реклама (остановить), запуск рекламы без заявки.
export default function AdminPromotionsPage() {
  return (
    <Suspense>
      <AdminPromotions />
    </Suspense>
  );
}

function AdminPromotions() {
  const t = useTranslations("admin");
  const tp = useTranslations("promo");
  const ta = useTranslations("app");
  const sp = useSearchParams();
  const product = sp.get("product") ?? "";
  const [state, setState] = useState<string>(product ? "" : "pending");
  const [items, setItems] = useState<Promotion[] | null>(null);
  const [cursor, setCursor] = useState("");
  const [error, setError] = useState<unknown>(null);

  const load = useCallback(async (from = "") => {
    setError(null);
    try {
      const r = await listAdminPromotions({ state, cursor: from });
      setItems((p) => (from ? [...(p ?? []), ...r.items] : r.items));
      setCursor(r.nextCursor);
    } catch (e) {
      setError(e);
    }
  }, [state]);
  useEffect(() => void load(), [load]);

  const replace = (p: Promotion) => setItems((prev) => prev?.map((x) => (x.id === p.id ? p : x)) ?? prev);

  async function run(fn: () => Promise<Promotion>) {
    setError(null);
    try {
      replace(await fn());
    } catch (e) {
      setError(e);
    }
  }

  return (
    <>
      <h1>{t("promoTitle")}</h1>
      <p className="muted">{t("promoHint")}</p>
      {product
        ? <CreateForm productId={product} title={sp.get("title") ?? ""} onCreated={(p) => setItems((prev) => [p, ...(prev ?? [])])} />
        : <p className="small muted">{t("promoFromProducts")}</p>}
      <div className="tabs" role="tablist">
        {["", ...PROMO_STATES].map((s) => (
          <button key={s} role="tab" aria-selected={state === s} className={state === s ? "tab tab-on" : "tab"} onClick={() => setState(s)}>
            {s ? tp(`st_${s}`) : tp("allStates")}
          </button>
        ))}
      </div>
      <ErrorBox error={error} onRetry={() => load()} />
      {items === null ? (
        !error && <p className="muted">{ta("loading")}</p>
      ) : items.length === 0 ? (
        <p className="empty">{t("promoEmpty")}</p>
      ) : (
        <ul className="plain stack">
          {items.map((p) => (
            <li key={p.id}>
              <PromotionCard p={p} showStore extra={
                <div className="row wrap">
                  {p.state === "pending" && (
                    <>
                      <ApproveAction p={p} onDone={replace} onError={setError} />
                      <ConfirmAction label={t("promoReject")} danger needReason onConfirm={(reason) => run(() => rejectPromotion(p.id, reason))} />
                    </>
                  )}
                  {(p.state === "active" || p.state === "scheduled") && (
                    <ConfirmAction label={t("promoStop")} danger needReason onConfirm={(reason) => run(() => stopPromotion(p.id, reason))} />
                  )}
                </div>
              } />
            </li>
          ))}
        </ul>
      )}
      {cursor && <div className="center"><button type="button" className="btn" onClick={() => load(cursor)}>{ta("more")}</button></div>}
    </>
  );
}

// Даты показа: по умолчанию — с сегодняшнего дня на срок из заявки.
function PeriodFields({ start, end, setStart, setEnd }: { start: string; end: string; setStart: (v: string) => void; setEnd: (v: string) => void }) {
  const t = useTranslations("admin");
  const id = useId();
  return (
    <div className="row wrap">
      <div className="field">
        <label htmlFor={`${id}-s`}>{t("promoStart")}</label>
        <input id={`${id}-s`} type="date" value={start} min={isoToDate(new Date())} onChange={(e) => setStart(e.target.value)} required />
      </div>
      <div className="field">
        <label htmlFor={`${id}-e`}>{t("promoEnd")}</label>
        <input id={`${id}-e`} type="date" value={end} min={start} onChange={(e) => setEnd(e.target.value)} required />
      </div>
    </div>
  );
}

function defaultPeriod(days: number): [string, string] {
  const now = new Date();
  // окончание — включительно: 7 дней с сегодняшнего = до дня «сегодня + 6»
  return [isoToDate(now), isoToDate(addDays(now, Math.max(days, 1) - 1))];
}

function ApproveAction({ p, onDone, onError }: { p: Promotion; onDone: (p: Promotion) => void; onError: (e: unknown) => void }) {
  const t = useTranslations("admin");
  const ta = useTranslations("app");
  const [open, setOpen] = useState(false);
  const [[start, end], setPeriod] = useState(() => defaultPeriod(p.days));
  const [busy, setBusy] = useState(false);

  if (!open) return <button type="button" className="btn btn-primary" onClick={() => setOpen(true)}>{t("promoApprove")}</button>;
  return (
    <form className="card confirm-box stack" onSubmit={async (e) => {
      e.preventDefault();
      setBusy(true);
      try {
        // начало «сегодня» — с текущего момента, иначе — с начала выбранного дня
        const s = start === isoToDate(new Date()) ? null : dateToStart(start);
        onDone(await approvePromotion(p.id, s, dateToEnd(end)));
        setOpen(false);
      } catch (err) {
        onError(err);
      } finally {
        setBusy(false);
      }
    }}>
      <PeriodFields start={start} end={end} setStart={(v) => setPeriod([v, end < v ? v : end])} setEnd={(v) => setPeriod([start, v])} />
      <span className="small muted">{t("promoDatesHint")}</span>
      <div className="row wrap">
        <button className="btn btn-primary" disabled={busy}>{t("promoApprove")}</button>
        <button type="button" className="btn btn-secondary" onClick={() => setOpen(false)} disabled={busy}>{ta("cancel")}</button>
      </div>
    </form>
  );
}

function CreateForm({ productId, title, onCreated }: { productId: string; title: string; onCreated: (p: Promotion) => void }) {
  const t = useTranslations("admin");
  const [[start, end], setPeriod] = useState(() => defaultPeriod(7));
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);
  const [error, setError] = useState<unknown>(null);
  return (
    <form className="card stack" onSubmit={async (e) => {
      e.preventDefault();
      setBusy(true);
      setError(null);
      try {
        const s = start === isoToDate(new Date()) ? null : dateToStart(start);
        onCreated(await createPromotion(productId, s, dateToEnd(end)));
        setDone(true);
      } catch (err) {
        setError(err);
      } finally {
        setBusy(false);
      }
    }}>
      <h2>{t("promoCreateTitle")}</h2>
      <p className="muted small">{t("promoCreateHint")}</p>
      {title && <p><strong>{t("promoProduct", { title })}</strong></p>}
      <PeriodFields start={start} end={end} setStart={(v) => setPeriod([v, end < v ? v : end])} setEnd={(v) => setPeriod([start, v])} />
      <ErrorBox error={error} />
      <div><button className="btn btn-primary" disabled={busy || done}>{t("promoCreate")}</button></div>
    </form>
  );
}
