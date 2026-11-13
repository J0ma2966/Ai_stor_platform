// Реклама товаров магазинов (docs/PROMOTIONS.md): заявки магазина, решения суперадмина,
// карусель «Реклама» на главной.
import { api, apiList } from "./api";
import { apiBase, serverHeaders, type Product } from "./catalog";

// Границы — те же, что в backend/internal/promotions (MinDays, MaxDays, MaxNote).
export const MIN_DAYS = 1;
export const MAX_DAYS = 90;
export const MAX_NOTE = 500;
export const DAY_PRESETS = [3, 7, 14, 30] as const;

export type PromoState = "pending" | "scheduled" | "active" | "ended" | "rejected" | "stopped";
export const PROMO_STATES: PromoState[] = ["pending", "active", "scheduled", "ended", "rejected", "stopped"];

export type Promotion = {
  id: string;
  product_id: string;
  product_title: string;
  product_price: number;
  image_url?: string;
  store_id: string;
  store_name: string;
  status: "pending" | "approved" | "rejected" | "stopped";
  state: PromoState;
  days: number;
  store_note: string;
  admin_reason: string | null;
  starts_at: string | null;
  ends_at: string | null;
  decided_at: string | null;
  created_at: string;
};

function qs(f: Record<string, string | undefined>) {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(f)) if (v) p.set(k, v);
  return p.size ? `?${p}` : "";
}

// Карусель на главной (серверный рендеринг, короткий кэш: остановленная реклама уходит за минуту).
export async function fetchPromoted(): Promise<Product[]> {
  const init: RequestInit & { next?: { revalidate: number } } = { headers: serverHeaders() };
  if (typeof window === "undefined") init.next = { revalidate: 60 };
  const res = await fetch(apiBase() + "/v1/promotions", init);
  if (!res.ok) return [];
  const body = (await res.json()) as { data?: Product[] };
  return body.data ?? [];
}

// магазин
export const listStorePromotions = (f: { state?: string; cursor?: string } = {}) => apiList<Promotion>(`/v1/store/promotions${qs(f)}`);
export const requestPromotion = (productId: string, days: number, note: string) =>
  api<Promotion>("/v1/store/promotions", { method: "POST", body: { product_id: productId, days, note } });
export const cancelPromotion = (id: string) => api<void>(`/v1/store/promotions/${id}`, { method: "DELETE" });

// суперадмин
export const listAdminPromotions = (f: { state?: string; store?: string; cursor?: string }) => apiList<Promotion>(`/v1/admin/promotions${qs(f)}`);
export const createPromotion = (productId: string, startsAt: string | null, endsAt: string | null) =>
  api<Promotion>("/v1/admin/promotions", { method: "POST", body: { product_id: productId, starts_at: startsAt, ends_at: endsAt } });
export const approvePromotion = (id: string, startsAt: string | null, endsAt: string | null) =>
  api<Promotion>(`/v1/admin/promotions/${id}/approve`, { method: "POST", body: { starts_at: startsAt, ends_at: endsAt } });
export const rejectPromotion = (id: string, reason: string) => api<Promotion>(`/v1/admin/promotions/${id}/reject`, { method: "POST", body: { reason } });
export const stopPromotion = (id: string, reason: string) => api<Promotion>(`/v1/admin/promotions/${id}/stop`, { method: "POST", body: { reason } });

// <input type="date"> ↔ ISO. Дата начала — с начала дня по Ташкенту (UTC+5, без перехода на летнее время),
// дата окончания — до конца выбранного дня.
const TZ = "+05:00";
export function dateToStart(d: string): string | null {
  return d ? new Date(`${d}T00:00:00${TZ}`).toISOString() : null;
}
export function dateToEnd(d: string): string | null {
  return d ? new Date(`${d}T23:59:59${TZ}`).toISOString() : null;
}
export function isoToDate(iso: string | Date): string {
  const t = new Date(new Date(iso).getTime() + 5 * 3600_000);
  return t.toISOString().slice(0, 10);
}
export function addDays(iso: string | Date, days: number): Date {
  return new Date(new Date(iso).getTime() + days * 86400_000);
}
const dayFormat = new Intl.DateTimeFormat("ru-RU", { day: "numeric", month: "short", year: "numeric", timeZone: "Asia/Tashkent" });
export function formatDay(iso: string | null): string {
  if (!iso) return "";
  return dayFormat.format(new Date(iso));
}
