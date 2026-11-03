// Покупка AI-токенов: тарифы, платежи Click/Payme.
import { api, apiList } from "./api";

export type Provider = "click" | "payme";
export type PaymentStatus = "pending" | "paid" | "declined" | "cancelled";

export type Tariff = {
  id: string;
  name_ru: string;
  name_uz: string;
  description: string;
  tokens: number;
  price: number; // тийины
  highlighted: boolean;
};

export type Payment = {
  id: string;
  number: number;
  provider: Provider;
  amount: number;
  tokens: number;
  status: PaymentStatus;
  paid_at: string | null;
  created_at: string;
  checkout_url?: string;
};

export const listTariffs = () => apiList<Tariff>("/v1/tariffs").then((r) => r.items);
export const getBalance = () => api<{ token_balance: number }>("/v1/me").then((m) => m.token_balance);
export const getPayment = (id: string) => api<Payment>(`/v1/payments/${encodeURIComponent(id)}`);
export const listPayments = (cursor = "") =>
  apiList<Payment>(`/v1/payments?limit=20${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ""}`);

export function createPayment(tariffId: string, provider: Provider, idempotencyKey: string) {
  return api<Payment>("/v1/payments", {
    method: "POST",
    body: { tariff_id: tariffId, provider },
    headers: { "Idempotency-Key": idempotencyKey },
  });
}

// Уходим только на платёжные страницы провайдеров — ссылка приходит с API, но
// проверяем хост, чтобы ошибка или подмена ответа не превратились в open redirect.
const CHECKOUT_HOSTS = new Set(["my.click.uz", "checkout.paycom.uz", "checkout.test.paycom.uz"]);

export function safeCheckoutUrl(u: string | undefined): string | null {
  if (!u) return null;
  try {
    const url = new URL(u);
    return url.protocol === "https:" && CHECKOUT_HOSTS.has(url.hostname) ? url.toString() : null;
  } catch {
    return null;
  }
}
