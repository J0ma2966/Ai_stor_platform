// API суперадмина (/v1/admin/*). Все запросы идут через общий клиент с refresh.
import { api, apiList } from "./api";

export type StoreStatus = "pending_activation" | "active" | "blocked" | "archived";
export type UserRole = "client" | "store" | "admin";
export type UserStatus = "active" | "blocked";

export type Stats = {
  stores: { total: number; active: number; pending_activation: number; blocked: number; archived: number };
  users: { clients: number; clients_new_7d: number; blocked: number };
  orders: { total: number; today: number; last_7d: number; by_status: Record<string, number>; delivered_sum_30d: number };
  products: { published: number; total: number };
  tokens: { issued_30d: number; spent_30d: number };
};

export type AuditEntry = {
  id: number;
  actor_id: string | null;
  actor_role: UserRole | null;
  actor_login: string | null;
  action: string;
  entity_type: string;
  entity_id: string | null;
  data: Record<string, unknown>;
  ip: string | null;
  request_id: string | null;
  created_at: string;
};

export type StoreRow = {
  id: string;
  slug: string;
  name: string;
  phone: string | null;
  status: StoreStatus;
  login: string | null;
  user_id: string | null;
  products_count: number;
  orders_count: number;
  activated_at: string | null;
  created_at: string;
};

export type StoreDetail = StoreRow & {
  description: string;
  address: string | null;
  contacts: Record<string, unknown>;
  user_status: UserStatus | null;
  last_login_at: string | null;
  history: AuditEntry[];
};

export type UserRow = {
  id: string;
  role: UserRole;
  status: UserStatus;
  phone: string | null;
  login: string | null;
  store_id: string | null;
  store_name: string | null;
  gender: "male" | "female" | null;
  lang: "ru" | "uz";
  token_balance: number;
  orders_count: number;
  registration_complete: boolean;
  last_login_at: string | null;
  created_at: string;
};

export type LedgerEntry = { id: number; op: string; amount: number; balance_after: number; reason: string | null; created_at: string };
export type UserDetail = UserRow & { ledger: LedgerEntry[]; history: AuditEntry[] };

// Тарифы и платежи: эндпоинты делает модуль оплаты (Click/Payme).
export type Tariff = {
  id: string;
  name_ru: string;
  name_uz: string;
  description: string;
  tokens: number;
  price: number;
  highlighted: boolean;
  is_published: boolean;
  created_at?: string;
};
export type TariffInput = Omit<Tariff, "id" | "created_at">;

export type Payment = {
  id: string;
  user_id: string;
  user_phone?: string | null;
  tariff_id: string;
  provider: "click" | "payme";
  provider_txn_id: string | null;
  amount: number;
  tokens: number;
  status: "pending" | "paid" | "declined" | "cancelled";
  paid_at: string | null;
  created_at: string;
};

export function qs(params: Record<string, string | undefined>): string {
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) if (v) q.set(k, v);
  const s = q.toString();
  return s ? `?${s}` : "";
}

const id = encodeURIComponent;

export const adminApi = {
  stats: () => api<Stats>("/v1/admin/stats"),

  stores: (f: { status?: string; q?: string; cursor?: string }) => apiList<StoreRow>(`/v1/admin/stores${qs(f)}`),
  store: (sid: string) => api<StoreDetail>(`/v1/admin/stores/${id(sid)}`),
  createStore: (b: { slug: string; name: string; login: string; password: string; phone?: string }) =>
    api<{ store_id: string; user_id: string; status: StoreStatus }>("/v1/admin/stores", { method: "POST", body: b }),
  setStoreStatus: (sid: string, status: Exclude<StoreStatus, "pending_activation">, reason = "") =>
    api<{ id: string; status: StoreStatus }>(`/v1/admin/stores/${id(sid)}/status`, { method: "POST", body: { status, reason } }),
  resetStorePassword: (sid: string, password: string) =>
    api<void>(`/v1/admin/stores/${id(sid)}/password`, { method: "POST", body: { password } }),

  users: (f: { role?: string; status?: string; q?: string; cursor?: string }) => apiList<UserRow>(`/v1/admin/users${qs(f)}`),
  user: (uid: string) => api<UserDetail>(`/v1/admin/users/${id(uid)}`),
  setUserStatus: (uid: string, status: UserStatus, reason: string) =>
    api<{ id: string; status: UserStatus }>(`/v1/admin/users/${id(uid)}/status`, { method: "POST", body: { status, reason } }),
  adjustTokens: (uid: string, amount: number, reason: string) =>
    api<{ user_id: string; token_balance: number }>(`/v1/admin/users/${id(uid)}/tokens`, { method: "POST", body: { amount, reason } }),

  audit: (f: { action?: string; entity_type?: string; entity_id?: string; actor_id?: string; cursor?: string }) =>
    apiList<AuditEntry>(`/v1/admin/audit${qs(f)}`),

  tariffs: () => apiList<Tariff>("/v1/admin/tariffs"),
  createTariff: (b: TariffInput) => api<Tariff>("/v1/admin/tariffs", { method: "POST", body: b }),
  updateTariff: (tid: string, b: Partial<TariffInput>) => api<Tariff>(`/v1/admin/tariffs/${id(tid)}`, { method: "PATCH", body: b }),
  payments: (f: { status?: string; provider?: string; user_id?: string; cursor?: string }) => apiList<Payment>(`/v1/admin/payments${qs(f)}`),
};

// Логин магазина: как требует CHECK в БД.
export const LOGIN_RE = /^[a-z0-9._-]{3,64}$/;
export const SLUG_RE = /^[a-z0-9][a-z0-9-]{1,62}$/;

// Надёжный пароль для нового магазина (показывается админу один раз).
export function generatePassword(len = 14): string {
  const abc = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789";
  const buf = new Uint32Array(len);
  crypto.getRandomValues(buf);
  return Array.from(buf, (n) => abc[n % abc.length]).join("");
}

// Сумма в сумах (ввод админа) → тийины для API.
export function sumToTiyin(s: string): number | null {
  const n = Number(s.replace(/[\s ]/g, "").replace(",", "."));
  if (!Number.isFinite(n) || n <= 0) return null;
  return Math.round(n * 100);
}

export function userTitle(u: Pick<UserRow, "phone" | "login" | "store_name">): string {
  return u.store_name ? `${u.store_name} (${u.login})` : (u.phone ?? u.login ?? "—");
}
