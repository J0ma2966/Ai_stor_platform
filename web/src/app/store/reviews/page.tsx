"use client";
import { Suspense, useCallback, useEffect, useId, useState } from "react";
import { useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { Link } from "@/i18n/navigation";
import { ErrorBox } from "@/components/ErrorBox";
import { ReviewCard } from "@/components/ProductReviews";
import { deleteReply, listStoreReviews, MAX_REVIEW, replyReview, type FullReview } from "@/lib/reviews";

// Кабинет магазина: отзывы о своих товарах и ответы на них. Скрыть отзыв магазин не может —
// это делает модерация платформы (жалоба через поддержку).
export default function StoreReviewsPage() {
  return (
    <Suspense>
      <StoreReviews />
    </Suspense>
  );
}

function StoreReviews() {
  const t = useTranslations("reviews");
  const ta = useTranslations("app");
  const product = useSearchParams().get("product") ?? "";
  const [unanswered, setUnanswered] = useState(false);
  const [rating, setRating] = useState("");
  const [items, setItems] = useState<FullReview[] | null>(null);
  const [cursor, setCursor] = useState("");
  const [error, setError] = useState<unknown>(null);

  const load = useCallback(async (from = "") => {
    setError(null);
    try {
      const r = await listStoreReviews({ product, rating, unanswered: unanswered ? "true" : "", cursor: from });
      setItems((p) => (from ? [...(p ?? []), ...r.items] : r.items));
      setCursor(r.nextCursor);
    } catch (e) {
      setError(e);
    }
  }, [product, rating, unanswered]);
  useEffect(() => void load(), [load]);

  const update = (id: string, patch: Partial<FullReview>) => setItems((p) => p?.map((x) => (x.id === id ? { ...x, ...patch } : x)) ?? p);

  return (
    <>
      <h1>{t("storeTitle")}</h1>
      <p className="muted small">{t("storeHint")}</p>
      <div className="row wrap">
        <label className="row"><input type="checkbox" checked={unanswered} onChange={(e) => setUnanswered(e.target.checked)} style={{ width: "auto" }} /> {t("unanswered")}</label>
        <select aria-label={t("yourRating")} value={rating} onChange={(e) => setRating(e.target.value)} style={{ maxWidth: 200 }}>
          <option value="">{t("allRatings")}</option>
          {[5, 4, 3, 2, 1].map((n) => <option key={n} value={n}>{t(`star${n}`)} ({n}★)</option>)}
        </select>
        {product && <Link href="/store/reviews">{t("allProducts")}</Link>}
      </div>
      <ErrorBox error={error} onRetry={() => load()} />
      {items === null ? (
        !error && <p className="muted">{ta("loading")}</p>
      ) : items.length === 0 ? (
        <p className="empty">{t("storeEmpty")}</p>
      ) : (
        <ul className="plain stack">
          {items.map((r) => (
            <li key={r.id}>
              <ReviewCard
                r={r}
                extra={
                  <>
                    <p className="small muted">
                      {t("aboutProduct")} <Link href={`/product/${r.product_id}#reviews`} target="_blank">{r.product_title}</Link>
                    </p>
                    <ReplyBox r={r} onChange={(patch) => update(r.id, patch)} />
                  </>
                }
              />
            </li>
          ))}
        </ul>
      )}
      {cursor && <div className="center"><button type="button" className="btn" onClick={() => load(cursor)}>{ta("more")}</button></div>}
    </>
  );
}

function ReplyBox({ r, onChange }: { r: FullReview; onChange: (patch: Partial<FullReview>) => void }) {
  const t = useTranslations("reviews");
  const ta = useTranslations("app");
  const id = useId();
  const [open, setOpen] = useState(false);
  const [text, setText] = useState(r.reply ?? "");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  async function run(fn: () => Promise<void>) {
    setBusy(true);
    setError(null);
    try {
      await fn();
      setOpen(false);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }

  if (!open) {
    return (
      <div className="row wrap">
        <button type="button" className="btn btn-secondary" onClick={() => { setText(r.reply ?? ""); setOpen(true); }}>{r.reply ? t("editReply") : t("reply")}</button>
        {r.reply && (
          <button type="button" className="btn btn-secondary" disabled={busy}
            onClick={() => run(async () => { await deleteReply(r.id); onChange({ reply: null, reply_at: null }); })}>{t("deleteReply")}</button>
        )}
        <ErrorBox error={error} />
      </div>
    );
  }
  return (
    <form className="stack-sm" onSubmit={(e) => {
      e.preventDefault();
      const v = text.trim();
      if (v) void run(async () => { await replyReview(r.id, v); onChange({ reply: v, reply_at: new Date().toISOString() }); });
    }}>
      <div className="field">
        <label htmlFor={id}>{t("replyLabel")}</label>
        <textarea id={id} rows={3} maxLength={MAX_REVIEW} value={text} onChange={(e) => setText(e.target.value)} autoFocus />
        <span className="muted small">{t("replyHint")}</span>
      </div>
      <ErrorBox error={error} />
      <div className="row">
        <button className="btn btn-primary" disabled={busy || !text.trim()}>{t("send")}</button>
        <button type="button" className="btn btn-secondary" onClick={() => setOpen(false)}>{ta("cancel")}</button>
      </div>
    </form>
  );
}
