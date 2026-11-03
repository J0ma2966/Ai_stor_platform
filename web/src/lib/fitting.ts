// Примерочная (выбор вещей на манекене) — на клиенте, в localStorage: выбор сохраняется
// при переходах по каталогу и к загрузке фото (ТЗ 6.3), работает и без входа.
// По одному товару на зону манекена; новый товар той же зоны заменяет старый.
import { API_URL } from "@/lib/api";

// Выключатель AI-примерки (prod запускается без неё): NEXT_PUBLIC_TRYON_ENABLED=false
// при сборке скрывает примерочную, мастер и кнопки «В примерочную».
// Бэкенд при этом запускают с TRYON_PROVIDER=off.
export const TRYON_ENABLED = process.env.NEXT_PUBLIC_TRYON_ENABLED !== "false";

export type Zone = "headwear" | "top" | "bottom" | "shoes";
export const ZONES: Zone[] = ["headwear", "top", "bottom", "shoes"];

export type FittingItem = {
  id: string;
  title: string;
  brand: string;
  price: number; // тийины
  image_url: string;
  zone: Zone;
  store_name?: string;
};

export type Fitting = Partial<Record<Zone, FittingItem>>;

const KEY = "fitting_v1";
export const FITTING_EVENT = "fitting:change";

function isItem(v: unknown): v is FittingItem {
  const o = v as FittingItem;
  return !!o && typeof o.id === "string" && typeof o.title === "string" && ZONES.includes(o.zone);
}

export function getFitting(): Fitting {
  if (typeof window === "undefined") return {};
  try {
    const raw = JSON.parse(window.localStorage.getItem(KEY) ?? "{}") as Record<string, unknown>;
    const out: Fitting = {};
    for (const z of ZONES) if (isItem(raw[z]) && (raw[z] as FittingItem).zone === z) out[z] = raw[z] as FittingItem;
    return out;
  } catch {
    return {};
  }
}

function write(f: Fitting) {
  try {
    window.localStorage.setItem(KEY, JSON.stringify(f));
  } catch {
    /* приватный режим — выбор живёт до перезагрузки */
  }
  window.dispatchEvent(new CustomEvent(FITTING_EVENT, { detail: f }));
}

export function addToFitting(item: FittingItem): void {
  if (!TRYON_ENABLED) return;
  const f = getFitting();
  const replaced = f[item.zone]?.id === item.id;
  f[item.zone] = item;
  write(f);
  if (!replaced) {
    // счётчик для статистики магазина; ошибки не мешают пользователю
    void fetch(`${API_URL}/v1/fitting/adds`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ product_id: item.id }),
      credentials: "include",
    }).catch(() => undefined);
  }
}

export function removeFromFitting(zone: Zone): void {
  const f = getFitting();
  delete f[zone];
  write(f);
}

export function clearFitting(): void {
  write({});
}

export function fittingItems(f: Fitting): FittingItem[] {
  return ZONES.flatMap((z) => (f[z] ? [f[z]!] : []));
}
