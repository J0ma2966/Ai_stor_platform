"use client";
import { useEffect, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Link } from "@/i18n/navigation";
import { ErrorBox } from "@/components/ErrorBox";
import { ProductCardView } from "@/components/ProductCardView";
import { getFavorites, type CartItem } from "@/lib/shop";
import type { Product } from "@/lib/catalog";

export default function Favorites() {
  const t = useTranslations();
  const locale = useLocale();
  const [items, setItems] = useState<CartItem[] | null>(null);
  const [error, setError] = useState<unknown>(null);
  const load = () => {
    setError(null);
    getFavorites().then((r) => setItems(r.items)).catch(setError);
  };
  useEffect(load, []);
  const asProduct = (c: CartItem): Product => ({
    id: c.product_id, store_id: c.store_id, store_name: c.store_name, category_id: c.category_id, title: c.title,
    brand: c.brand, price: c.price, in_stock: c.available, images: c.image_url ? { front: c.image_url } : {}, created_at: c.added_at,
  });
  return (
    <>
      <h1>{t("account.favorites")}</h1>
      <ErrorBox error={error} onRetry={load} />
      {items === null ? (
        !error && <p className="muted">{t("app.loading")}</p>
      ) : items.length === 0 ? (
        <div className="empty stack-sm center-text"><p>{t("account.favEmpty")}</p><Link href="/catalog" className="btn btn-primary">{t("cart.toCatalog")}</Link></div>
      ) : (
        <ul className="pgrid plain">
          {items.map((c) => <li key={c.product_id}><ProductCardView p={asProduct(c)} locale={locale} outOfStock={c.unavailable_reason === "unavailable" ? t("cart.unavailable") : t("product.outOfStock")} /></li>)}
        </ul>
      )}
    </>
  );
}
