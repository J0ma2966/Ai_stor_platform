"use client";
import { useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { fetchProducts, type CatalogFilters, type Product } from "@/lib/catalog";
import { ProductCardView } from "./ProductCardView";
import { ErrorBox } from "./ErrorBox";

// Следующие страницы каталога (курсор), первая отрендерена на сервере.
export function LoadMore({ filters, cursor }: { filters: CatalogFilters; cursor: string }) {
  const t = useTranslations();
  const locale = useLocale();
  const [items, setItems] = useState<Product[]>([]);
  const [next, setNext] = useState(cursor);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  async function more() {
    setBusy(true);
    setError(null);
    try {
      const r = await fetchProducts(filters, next);
      setItems((p) => [...p, ...r.items]);
      setNext(r.nextCursor);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      {items.length > 0 && (
        <ul className="pgrid plain">
          {items.map((p) => (
            <li key={p.id}><ProductCardView p={p} locale={locale} outOfStock={t("product.outOfStock")} /></li>
          ))}
        </ul>
      )}
      <ErrorBox error={error} onRetry={more} />
      {next && (
        <div className="center">
          <button type="button" className="btn" disabled={busy} onClick={more}>{busy ? t("app.loading") : t("app.more")}</button>
        </div>
      )}
    </>
  );
}
