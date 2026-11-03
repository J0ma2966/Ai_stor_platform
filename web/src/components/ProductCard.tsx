import { getLocale, getTranslations } from "next-intl/server";
import type { Product } from "@/lib/catalog";
import { ProductCardView } from "./ProductCardView";

// Карточка товара в сетке: фото, цена, название, бренд, магазин (ТЗ 4.3).
export async function ProductCard({ p, priority = false }: { p: Product; priority?: boolean }) {
  const locale = await getLocale();
  const t = await getTranslations("product");
  return <ProductCardView p={p} locale={locale} outOfStock={t("outOfStock")} priority={priority} />;
}
