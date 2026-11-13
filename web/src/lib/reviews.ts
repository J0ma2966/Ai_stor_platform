// Отзывы и оценки товаров (docs/REVIEWS.md). Публичный список работает и без входа.
import { api, apiList } from "./api";

// Границы оценки — те же, что в backend/internal/reviews (MinRating, MaxRating).
export const MIN_RATING = 1;
export const MAX_RATING = 5;
export const MAX_REVIEW = 2000;

export type Review = {
  id: string;
  rating: number;
  body: string;
  author_name: string;
  reply: string | null;
  reply_at: string | null;
  created_at: string;
  updated_at: string;
};
export type FullReview = Review & {
  product_id: string;
  product_title?: string;
  image_url?: string;
  store_id?: string;
  store_name?: string;
  status: "visible" | "hidden";
  moderation_reason: string | null;
};
export type MyReview = { can_review: boolean; review: FullReview | null };

function qs(f: Record<string, string | undefined>) {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(f)) if (v) p.set(k, v);
  return p.size ? `?${p}` : "";
}

export const listReviews = (productId: string, f: { rating?: string; cursor?: string } = {}) =>
  apiList<Review>(`/v1/products/${productId}/reviews${qs({ ...f, limit: "10" })}`);

// клиент
export const getMyReview = (productId: string) => api<MyReview>(`/v1/reviews/products/${productId}`);
export const saveReview = (productId: string, rating: number, body: string) =>
  api<FullReview>(`/v1/reviews/products/${productId}`, { method: "PUT", body: { rating, body } });
export const deleteReview = (productId: string) => api<void>(`/v1/reviews/products/${productId}`, { method: "DELETE" });

// магазин
export const listStoreReviews = (f: { rating?: string; unanswered?: string; product?: string; cursor?: string }) =>
  apiList<FullReview>(`/v1/store/reviews${qs(f)}`);
export const replyReview = (id: string, text: string) => api<{ reply: string }>(`/v1/store/reviews/${id}/reply`, { method: "PUT", body: { text } });
export const deleteReply = (id: string) => api<void>(`/v1/store/reviews/${id}/reply`, { method: "DELETE" });

// администратор
export const listAdminReviews = (f: { status?: string; rating?: string; store?: string; cursor?: string }) =>
  apiList<FullReview>(`/v1/admin/reviews${qs(f)}`);
export const moderateReview = (id: string, status: "visible" | "hidden", reason: string) =>
  api<{ status: string }>(`/v1/admin/reviews/${id}/status`, { method: "POST", body: { status, reason } });

// Средняя оценка с одним знаком после запятой: 4.33 → «4,3» (ru-RU для обоих языков, как и деньги).
// Форматтер один на модуль: toLocaleString создавал бы новый на каждую карточку товара.
const ratingFormat = new Intl.NumberFormat("ru-RU", { minimumFractionDigits: 1, maximumFractionDigits: 1 });
export function formatRating(avg: number): string {
  return ratingFormat.format(avg);
}
