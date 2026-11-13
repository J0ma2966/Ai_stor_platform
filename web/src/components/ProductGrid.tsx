import { getLocale, getTranslations } from "next-intl/server";
import type { Product } from "@/lib/catalog";
import { ProductCardView } from "./ProductCardView";

// Сетка карточек товара: фото, цена, название, бренд, магазин (ТЗ 4.3).
// Язык и перевод берутся один раз на сетку, а не на каждую карточку: карточка —
// обычный синхронный компонент, серверу не нужно ждать по два промиса на товар.
export async function ProductGrid({ items, priority = 0 }: { items: Product[]; priority?: number }) {
  const locale = await getLocale();
  const t = await getTranslations("product");
  const outOfStock = t("outOfStock");
  return (
    <ul className="pgrid plain">
      {items.map((p, i) => (
        <li key={p.id}>
          <ProductCardView p={p} locale={locale} outOfStock={outOfStock} priority={i < priority} />
        </li>
      ))}
    </ul>
  );
}
