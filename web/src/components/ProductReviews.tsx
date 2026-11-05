"use client";
import { useCallback, useEffect, useId, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Link, usePathname } from "@/i18n/navigation";
import { ApiError } from "@/lib/api";
import { formatDateTime } from "@/lib/money";
import { useSession } from "@/lib/session";
import {
  deleteReview, formatRating, getMyReview, listReviews, MAX_RATING, MAX_REVIEW, saveReview,
  type MyReview, type Review,
} from "@/lib/reviews";
import { ErrorBox } from "./ErrorBox";
import { StarInput, Stars } from "./Stars";

type Props = { productId: string; avg: number; count: number; dist: number[] };

// Блок «Отзывы» на странице товара: сводка, свой отзыв (форма), список с фильтром по звёздам.
export function ProductReviews({ productId, avg: avg0, count: count0, dist: dist0 }: Props) {
  const t = useTranslations("reviews");
  const ta = useTranslations("app");
  const [summary, setSummary] = useState({ avg: avg0, count: count0, dist: dist0 });
  const [filter, setFilter] = useState("");
  const [items, setItems] = useState<Review[] | null>(null);
  const [cursor, setCursor] = useState("");
  const [error, setError] = useState<unknown>(null);

  const load = useCallback(async (from = "") => {
    setError(null);
    try {
      const r = await listReviews(productId, { rating: filter, cursor: from });
      setItems((p) => (from ? [...(p ?? []), ...r.items] : r.items));
      setCursor(r.nextCursor);
    } catch (e) {
      setError(e);
    }
  }, [productId, filter]);
  useEffect(() => void load(), [load]);

  // после своего отзыва обновляем сводку локально, не дожидаясь кэша карточки товара
  function onMineChanged(before: number | null, after: number | null) {
    setSummary((s) => {
      const dist = [...s.dist];
      let sum = s.avg * s.count;
      let count = s.count;
      if (before) { dist[before - 1] = Math.max(0, dist[before - 1] - 1); sum -= before; count--; }
      if (after) { dist[after - 1]++; sum += after; count++; }
      return { avg: count > 0 ? sum / count : 0, count, dist };
    });
    void load();
  }

  return (
    <section id="reviews" className="stack reviews" aria-labelledby="reviews-h">
      <h2 id="reviews-h">{t("title")} {summary.count > 0 && <span className="muted">{summary.count}</span>}</h2>
      <div className="reviews-layout">
        <aside className="stack-sm reviews-summary">
          {summary.count > 0 ? (
            <>
              <p className="rating-big">
                <strong>{formatRating(summary.avg)}</strong>
                <Stars value={summary.avg} size="lg" label={t("avgLabel", { avg: formatRating(summary.avg), max: MAX_RATING })} />
              </p>
              <p className="muted small">{t("count", { n: summary.count })}</p>
              <ul className="plain stack-xs" aria-label={t("filterLabel")}>
                {[5, 4, 3, 2, 1].map((n) => {
                  const c = summary.dist[n - 1] ?? 0;
                  const on = filter === String(n);
                  return (
                    <li key={n}>
                      <button type="button" className={`dist-row${on ? " dist-on" : ""}`} aria-pressed={on} disabled={c === 0 && !on}
                        onClick={() => setFilter(on ? "" : String(n))}>
                        <span>{n}★</span>
                        <span className="dist-bar" aria-hidden="true"><span style={{ width: `${summary.count ? (c / summary.count) * 100 : 0}%` }} /></span>
                        <span className="muted small">{c}</span>
                        <span className="sr-only">{t("filterBy", { n })}</span>
                      </button>
                    </li>
                  );
                })}
              </ul>
            </>
          ) : (
            <p className="muted">{t("none")}</p>
          )}
          <MyReviewBox productId={productId} onChanged={onMineChanged} />
        </aside>
        <div className="stack">
          {filter && (
            <p className="row">
              <span>{t("filtered", { n: filter })}</span>
              <button type="button" className="btn btn-secondary" onClick={() => setFilter("")}>{t("showAll")}</button>
            </p>
          )}
          <ErrorBox error={error} onRetry={() => load()} />
          {items === null ? (
            !error && <p className="muted">{ta("loading")}</p>
          ) : items.length === 0 ? (
            summary.count > 0 && <p className="muted">{t("emptyFilter")}</p>
          ) : (
            <ul className="plain stack">
              {items.map((r) => <li key={r.id}><ReviewCard r={r} /></li>)}
            </ul>
          )}
          {cursor && <div className="center"><button type="button" className="btn" onClick={() => load(cursor)}>{ta("more")}</button></div>}
        </div>
      </div>
    </section>
  );
}

export function ReviewCard({ r, extra }: { r: Review; extra?: React.ReactNode }) {
  const t = useTranslations("reviews");
  const locale = useLocale();
  return (
    <article className="card stack-sm review">
      <div className="row between wrap">
        <span className="row">
          <Stars value={r.rating} label={t("ratingLabel", { n: r.rating, max: MAX_RATING })} />
          <strong>{r.author_name || t("anonymous")}</strong>
          <span className="badge badge-ok small">{t("verified")}</span>
        </span>
        <span className="muted small">{formatDateTime(r.created_at, locale)}</span>
      </div>
      {r.body && <p className="pre">{r.body}</p>}
      {r.reply && (
        <div className="review-reply">
          <p className="small"><strong>{t("storeReply")}</strong>{r.reply_at && <span className="muted"> · {formatDateTime(r.reply_at, locale)}</span>}</p>
          <p className="pre">{r.reply}</p>
        </div>
      )}
      {extra}
    </article>
  );
}

function MyReviewBox({ productId, onChanged }: { productId: string; onChanged: (before: number | null, after: number | null) => void }) {
  const t = useTranslations("reviews");
  const { me, ready } = useSession();
  const pathname = usePathname();
  const [mine, setMine] = useState<MyReview | null>(null);
  const [editing, setEditing] = useState(false);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    if (me?.role !== "client") return;
    getMyReview(productId).then(setMine, setError);
  }, [me?.role, productId]);

  if (!ready) return null;
  if (!me) {
    return <p className="small"><Link href={`/login?next=${encodeURIComponent(pathname + "#reviews")}`}>{t("loginToReview")}</Link></p>;
  }
  if (me.role !== "client") return null;
  if (error) return <ErrorBox error={error} />;
  if (!mine) return null;

  const r = mine.review;
  if (r && !editing) {
    return (
      <div className="card stack-sm">
        <p className="small"><strong>{t("yourReview")}</strong></p>
        {r.status === "hidden" && (
          <p className="alert small">{t("hiddenByModerator")}{r.moderation_reason && ` ${t("reason", { reason: r.moderation_reason })}`}</p>
        )}
        <Stars value={r.rating} label={t("ratingLabel", { n: r.rating, max: MAX_RATING })} />
        {r.body && <p className="pre small">{r.body}</p>}
        <div className="row wrap">
          {r.status !== "hidden" && <button type="button" className="btn btn-secondary" onClick={() => setEditing(true)}>{t("edit")}</button>}
          {/* скрытый модератором отзыв удаляется только текстом: новый отзыв на товар оставить нельзя */}
          {(r.status !== "hidden" || r.body) && <DeleteButton onDelete={async () => {
            await deleteReview(productId);
            setMine({ ...mine, review: r.status === "hidden" ? { ...r, body: "" } : null });
            onChanged(r.status === "visible" ? r.rating : null, null);
          }} />}
        </div>
      </div>
    );
  }
  if (!r && !mine.can_review) return <p className="muted small">{t("onlyBuyers")}</p>;
  return (
    <ReviewForm
      productId={productId}
      initial={r ? { rating: r.rating, body: r.body } : null}
      onCancel={r ? () => setEditing(false) : undefined}
      onSaved={(saved) => {
        setMine({ ...mine, review: saved });
        setEditing(false);
        onChanged(r && r.status === "visible" ? r.rating : null, saved.status === "visible" ? saved.rating : null);
      }}
    />
  );
}

function DeleteButton({ onDelete }: { onDelete: () => Promise<void> }) {
  const t = useTranslations("reviews");
  const ta = useTranslations("app");
  const [ask, setAsk] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  if (!ask) return <button type="button" className="btn btn-secondary" onClick={() => setAsk(true)}>{t("delete")}</button>;
  return (
    <span className="row wrap">
      <span className="small">{t("confirmDelete")}</span>
      <button type="button" className="btn btn-danger" disabled={busy} onClick={async () => {
        setBusy(true);
        setError(null);
        try {
          await onDelete();
        } catch (e) {
          setError(e);
          setBusy(false);
        }
      }}>{t("delete")}</button>
      <button type="button" className="btn btn-secondary" onClick={() => setAsk(false)}>{ta("cancel")}</button>
      <ErrorBox error={error} />
    </span>
  );
}

function ReviewForm({ productId, initial, onSaved, onCancel }: {
  productId: string;
  initial: { rating: number; body: string } | null;
  onSaved: (r: NonNullable<MyReview["review"]>) => void;
  onCancel?: () => void;
}) {
  const t = useTranslations("reviews");
  const ta = useTranslations("app");
  const bodyId = useId();
  const [rating, setRating] = useState(initial?.rating ?? 0);
  const [body, setBody] = useState(initial?.body ?? "");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [needRating, setNeedRating] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (!rating) {
      setNeedRating(true);
      return;
    }
    setBusy(true);
    setError(null);
    try {
      onSaved(await saveReview(productId, rating, body.trim()));
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  }

  const fieldErr = error instanceof ApiError ? error.fields : {};
  return (
    <form className="card stack-sm" onSubmit={submit} noValidate>
      <p className="small"><strong>{initial ? t("editTitle") : t("formTitle")}</strong></p>
      <StarInput value={rating} onChange={(v) => { setRating(v); setNeedRating(false); }} legend={t("yourRating")} labelFor={(n) => t(`star${n}`)} />
      {rating > 0 && <p className="muted small">{t(`star${rating}`)}</p>}
      {(needRating || fieldErr.rating) && <p className="field-error">{t("ratingRequired")}</p>}
      <div className="field">
        <label htmlFor={bodyId}>{t("bodyLabel")}</label>
        <textarea id={bodyId} rows={4} maxLength={MAX_REVIEW} value={body} placeholder={t("bodyPh")} onChange={(e) => setBody(e.target.value)} />
        <span className="muted small">{body.length}/{MAX_REVIEW}</span>
      </div>
      <ErrorBox error={error} />
      <div className="row wrap">
        <button className="btn btn-primary" disabled={busy}>{initial ? t("save") : t("submit")}</button>
        {onCancel && <button type="button" className="btn btn-secondary" onClick={onCancel}>{ta("cancel")}</button>}
      </div>
    </form>
  );
}
