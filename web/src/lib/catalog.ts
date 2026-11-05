// Типы и запросы публичного каталога. Работают и на сервере (SSR), и в браузере.
import type { Zone } from "./fitting";

export type Category = { id: string; slug: string; zone: Zone; name_ru: string; name_uz: string };
export type Product = {
  id: string;
  store_id: string;
  store_name: string;
  store_slug?: string;
  category_id: string;
  zone?: Zone;
  sku?: string;
  title: string;
  description?: string;
  brand: string;
  price: number;
  in_stock: boolean;
  // размеры: покупателю — наличие (low — осталось 1–2 шт.), кабинету магазина — точный остаток
  sizes?: ProductSize[];
  has_sizes?: boolean; // каталог: у товара есть размеры (в корзину — через карточку)
  status?: "draft" | "published" | "unpublished" | "blocked";
  moderation_reason?: string;
  images: Partial<Record<"front" | "side" | "back", string>>;
  views?: number;
  fitting_adds?: number;
  rating_avg?: number; // средняя оценка видимых отзывов, 0 — отзывов нет
  rating_count?: number;
  rating_dist?: number[]; // только карточка товара: сколько отзывов на 1…5 звёзд
  created_at: string;
};
export type ProductSize = { size: string; in_stock: boolean; low?: boolean; stock?: number };
export type StoreContacts = { telegram?: string; instagram?: string; website?: string; email?: string; hours?: string };
export type Store = {
  id: string;
  slug: string;
  name: string;
  description: string;
  logo_url?: string;
  cover_url?: string;
  phone?: string;
  contacts: StoreContacts;
  address?: string;
  lat?: number;
  lng?: number;
  product_count: number;
  status?: string;
  created_at: string;
};
export type PlatformSettings = { generation_cost?: number; support_phone?: string; support_email?: string; support_telegram?: string };

export const SORTS = ["popular", "rating", "new", "price_asc", "price_desc"] as const;
export type Sort = (typeof SORTS)[number];

export type CatalogFilters = {
  q?: string;
  category?: string;
  store?: string;
  brand?: string;
  price_min?: string; // сумы (в URL), в API уходят тийины
  price_max?: string;
  in_stock?: string;
  sort?: string;
};

export const FILTER_KEYS: (keyof CatalogFilters)[] = ["q", "category", "store", "brand", "price_min", "price_max", "in_stock", "sort"];

// Адрес API для серверного рендеринга: внутри Docker-сети он может отличаться от публичного.
export function apiBase(): string {
  if (typeof window === "undefined") return process.env.API_INTERNAL_URL ?? process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";
  return process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";
}

export class FetchError extends Error {
  constructor(public status: number) {
    super(`api ${status}`);
  }
}

async function get<T>(path: string, revalidate = 30): Promise<{ data: T; next_cursor?: string }> {
  const init: RequestInit & { next?: { revalidate: number } } = { headers: { Accept: "application/json" } };
  if (typeof window === "undefined") init.next = { revalidate };
  const res = await fetch(apiBase() + path, init);
  if (!res.ok) throw new FetchError(res.status);
  return res.json();
}

export function toQuery(f: CatalogFilters, extra: Record<string, string> = {}): string {
  const p = new URLSearchParams();
  for (const k of FILTER_KEYS) {
    let v = f[k];
    if (!v) continue;
    if ((k === "price_min" || k === "price_max") && /^\d+$/.test(v)) v = String(Number(v) * 100);
    else if (k === "price_min" || k === "price_max") continue;
    p.set(k, v);
  }
  for (const [k, v] of Object.entries(extra)) if (v) p.set(k, v);
  const s = p.toString();
  return s ? `?${s}` : "";
}

export async function fetchProducts(f: CatalogFilters, cursor = "", limit = 24) {
  const r = await get<Product[]>(`/v1/products${toQuery(f, { cursor, limit: String(limit) })}`);
  return { items: r.data ?? [], nextCursor: r.next_cursor ?? "" };
}
export async function fetchProduct(id: string): Promise<Product | null> {
  if (!/^[0-9a-f-]{36}$/i.test(id)) return null;
  try {
    return (await get<Product>(`/v1/products/${id}`)).data;
  } catch (e) {
    if (e instanceof FetchError && e.status === 404) return null;
    throw e;
  }
}
export async function fetchCategories() {
  return (await get<Category[]>("/v1/categories", 300)).data ?? [];
}
export async function fetchStores(q = "") {
  return (await get<Store[]>(`/v1/stores${q ? `?q=${encodeURIComponent(q)}` : ""}`, 60)).data ?? [];
}
export async function fetchStore(slug: string): Promise<Store | null> {
  if (!/^[a-z0-9][a-z0-9-]{1,62}$/.test(slug)) return null;
  try {
    return (await get<Store>(`/v1/stores/${slug}`, 60)).data;
  } catch (e) {
    if (e instanceof FetchError && e.status === 404) return null;
    throw e;
  }
}
export async function fetchPlatform(): Promise<PlatformSettings> {
  try {
    return (await get<PlatformSettings>("/v1/platform", 60)).data ?? {};
  } catch {
    return {};
  }
}

export function catName(c: Category | undefined, locale: string): string {
  if (!c) return "";
  return locale === "uz" ? c.name_uz : c.name_ru;
}

export function frontImage(p: Pick<Product, "images">): string {
  return p.images.front ?? p.images.side ?? p.images.back ?? "";
}
