import { api, apiList, apiRaw, newIdempotencyKey } from "./api";

export type OrderStatus = "pending" | "confirmed" | "rejected" | "assembling" | "in_delivery" | "delivered" | "cancelled";

export const CUSTOMER_STEPS: OrderStatus[] = ["confirmed", "assembling", "in_delivery", "delivered"];

export type OrderSummary = {
  id: string;
  number: number;
  checkout_id: string;
  store_id: string;
  store_name: string;
  status: OrderStatus;
  step: number;
  items_total: number;
  items_count: number;
  customer_name?: string;
  short_address?: string;
  created_at: string;
  updated_at: string;
};

export type Delivery = {
  street: string;
  house: string;
  entrance?: string;
  floor?: string;
  apartment?: string;
  intercom?: string;
  address_comment?: string;
  lat?: number;
  lng?: number;
  maps_url?: string;
  photo_url?: string;
  has_photo: boolean;
};

export type OrderItem = { product_id: string | null; title: string; brand: string; image_url?: string; unit_price: number; qty: number };
export type HistoryEntry = { from_status: OrderStatus | null; to_status: OrderStatus; reason?: string; actor_role?: string; created_at: string };

export type OrderDetail = OrderSummary & {
  customer_phone: string;
  customer_comment: string | null;
  delivery_time_comment: string | null;
  rejection_reason: string | null;
  delivery: Delivery;
  items: OrderItem[];
  history: HistoryEntry[];
  next_statuses?: OrderStatus[];
  copy_text?: string;
};

export type CheckoutInput = {
  items: { product_id: string; qty: number }[];
  customer_name: string;
  customer_phone?: string;
  customer_comment?: string;
  delivery_time_comment?: string;
  delivery: {
    street: string;
    house: string;
    entrance?: string;
    floor?: string;
    apartment?: string;
    intercom?: string;
    address_comment?: string;
    lat?: number;
    lng?: number;
    photo_key?: string;
  };
};

export type CheckoutResult = {
  checkout_id: string;
  orders: { id: string; number: number; store_id: string; store_name: string; status: OrderStatus; items_total: number }[];
};

// Ключ создаётся при открытии формы и переиспользуется при повторной отправке,
// чтобы двойной клик или обрыв сети не создали второй заказ.
export function checkout(input: CheckoutInput, idempotencyKey: string) {
  return api<CheckoutResult>("/v1/orders", { method: "POST", body: input, headers: { "Idempotency-Key": idempotencyKey } });
}
export { newIdempotencyKey };

export async function uploadDeliveryPhoto(file: File): Promise<string> {
  const res = await apiRaw("/v1/uploads?purpose=delivery", { method: "POST", rawBody: file, headers: { "Content-Type": file.type } });
  const body = await res.json();
  return body.data.key as string;
}

export type Scope = "client" | "store" | "admin";
const base = (s: Scope) => (s === "client" ? "/v1/orders" : s === "store" ? "/v1/store/orders" : "/v1/admin/orders");

export function listOrders(scope: Scope, opts: { status?: OrderStatus[]; cursor?: string } = {}) {
  const q = new URLSearchParams();
  if (opts.status?.length) q.set("status", opts.status.join(","));
  if (opts.cursor) q.set("cursor", opts.cursor);
  const qs = q.toString();
  return apiList<OrderSummary>(base(scope) + (qs ? `?${qs}` : ""));
}

export function getOrder(scope: Scope, id: string) {
  return api<OrderDetail>(`${base(scope)}/${encodeURIComponent(id)}`);
}

export function setOrderStatus(scope: Exclude<Scope, "client">, id: string, status: OrderStatus, from: OrderStatus, reason = "") {
  return api<OrderDetail>(`${base(scope)}/${encodeURIComponent(id)}/status`, { method: "POST", body: { status, from, reason } });
}

// Разбор ?items=<uuid>:<qty>,<uuid>:<qty> (корзина передаёт выбранные позиции).
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
export function parseItemsParam(s: string | null): { product_id: string; qty: number }[] {
  if (!s) return [];
  const out: { product_id: string; qty: number }[] = [];
  for (const part of s.split(",").slice(0, 50)) {
    const [id, q] = part.split(":");
    const qty = Number.parseInt(q ?? "1", 10);
    if (UUID.test(id ?? "") && qty >= 1 && qty <= 99 && !out.some((x) => x.product_id === id)) out.push({ product_id: id!, qty });
  }
  return out;
}

// Клиентская копия текста для курьера — на случай, если сервер её не вернул.
export async function copyToClipboard(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    // запасной путь для старых браузеров/HTTP: textarea + execCommand
    const ta = document.createElement("textarea");
    ta.value = text; // только как значение, без HTML
    ta.setAttribute("readonly", "");
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.select();
    const ok = document.execCommand("copy");
    document.body.removeChild(ta);
    return ok;
  }
}
