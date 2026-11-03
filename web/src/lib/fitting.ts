// Примерочная (клиентское состояние). Владелец модуля — AI-примерка;
// здесь совместимая версия с тем же API, чтобы витрина могла добавлять товары.
export type Zone = "headwear" | "top" | "bottom" | "shoes";
export type FittingItem = { id: string; title: string; brand: string; price: number; image_url: string; zone: Zone; store_name?: string };

// Выключатель AI-примерки (prod запускается без неё): NEXT_PUBLIC_TRYON_ENABLED=false.
// Совпадает с определением в модуле примерки.
export const TRYON_ENABLED = process.env.NEXT_PUBLIC_TRYON_ENABLED !== "false";

const KEY = "fitting_v1";

export function getFitting(): Partial<Record<Zone, FittingItem>> {
  try {
    const v = JSON.parse(localStorage.getItem(KEY) ?? "{}");
    return v && typeof v === "object" ? v : {};
  } catch {
    return {};
  }
}

function save(v: Partial<Record<Zone, FittingItem>>) {
  try {
    localStorage.setItem(KEY, JSON.stringify(v));
  } catch {
    /* приватный режим — выбор живёт до перезагрузки */
  }
  window.dispatchEvent(new Event("fitting:change"));
}

// В базовой версии — по одному товару на зону манекена: новый заменяет старый.
export function addToFitting(item: FittingItem): void {
  save({ ...getFitting(), [item.zone]: item });
}

export function removeFromFitting(zone: Zone): void {
  const v = getFitting();
  delete v[zone];
  save(v);
}

export function clearFitting(): void {
  save({});
}
