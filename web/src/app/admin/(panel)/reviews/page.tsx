"use client";
import { useCallback, useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Link } from "@/i18n/navigation";
import { ErrorBox } from "@/components/ErrorBox";
import { ReviewCard } from "@/components/ProductReviews";
import { ConfirmAction } from "@/components/admin/ConfirmAction";
import { listAdminReviews, moderateReview, type FullReview } from "@/lib/reviews";

// Модерация отзывов: скрыть (с причиной, её увидит автор) или вернуть. Скрытый отзыв
// не показывается на витрине и не входит в среднюю оценку товара.
export default function AdminReviews() {
  const t = useTranslations("reviews");
  const ta = useTranslations("app");
  const [status, setStatus] = useState("");
  const [rating, setRating] = useState("");
  const [items, setItems] = useState<FullReview[] | null>(null);
  const [cursor, setCursor] = useState("");
  const [error, setError] = useState<unknown>(null);

  const load = useCallback(async (from = "") => {
    setError(null);
    try {
      const r = await listAdminReviews({ status, rating, cursor: from });
      setItems((p) => (from ? [...(p ?? []), ...r.items] : r.items));
      setCursor(r.nextCursor);
    } catch (e) {
      setError(e);
    }
  }, [status, rating]);
  useEffect(() => void load(), [load]);

  async function act(r: FullReview, st: "visible" | "hidden", reason: string) {
    setError(null);
    try {
      await moderateReview(r.id, st, reason);
    } catch (e) {
      setError(e);
      return;
    }
    setItems((p) => p?.map((x) => (x.id === r.id ? { ...x, status: st, moderation_reason: st === "hidden" ? reason : null } : x)) ?? p);
  }

  return (
    <>
      <h1>{t("adminTitle")}</h1>
      <div className="row wrap">
        <select aria-label={t("status")} value={status} onChange={(e) => setStatus(e.target.value)} style={{ maxWidth: 220 }}>
          <option value="">{t("allStatuses")}</option>
          <option value="visible">{t("st_visible")}</option>
          <option value="hidden">{t("st_hidden")}</option>
        </select>
        <select aria-label={t("yourRating")} value={rating} onChange={(e) => setRating(e.target.value)} style={{ maxWidth: 200 }}>
          <option value="">{t("allRatings")}</option>
          {[1, 2, 3, 4, 5].map((n) => <option key={n} value={n}>{n}★</option>)}
        </select>
      </div>
      <ErrorBox error={error} onRetry={() => load()} />
      {items === null ? (
        !error && <p className="muted">{ta("loading")}</p>
      ) : items.length === 0 ? (
        <p className="empty">{t("adminEmpty")}</p>
      ) : (
        <ul className="plain stack">
          {items.map((r) => (
            <li key={r.id}>
              <ReviewCard
                r={r}
                extra={
                  <>
                    <p className="small muted">
                      <Link href={`/product/${r.product_id}`} target="_blank">{r.product_title}</Link> · {r.store_name}
                    </p>
                    {r.status === "hidden" && (
                      <p className="small"><span className="badge badge-bad">{t("st_hidden")}</span> {r.moderation_reason}</p>
                    )}
                    {r.status === "visible" ? (
                      <ConfirmAction label={t("hide")} confirmLabel={t("hide")} danger needReason onConfirm={(reason) => act(r, "hidden", reason)} />
                    ) : (
                      <ConfirmAction label={t("restore")} confirmLabel={t("restore")} onConfirm={() => act(r, "visible", "")} />
                    )}
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
