"use client";
import { useCallback, useEffect, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Link } from "@/i18n/navigation";
import { AuthGate } from "@/components/AuthGate";
import { ErrorBox } from "@/components/ErrorBox";
import { formatSum } from "@/lib/money";
import { useSession } from "@/lib/session";
import { getCart, removeFromCart, setCartQty, type Cart, type CartItem } from "@/lib/shop";

function CartView() {
  const t = useTranslations("cart");
  const ta = useTranslations("app");
  const locale = useLocale();
  const { setCartQty: setBadge } = useSession();
  const [cart, setCart] = useState<Cart | null>(null);
  const [busy, setBusy] = useState<string>("");
  const [error, setError] = useState<unknown>(null);

  const apply = useCallback((c: Cart) => {
    setCart(c);
    setBadge(c.total_qty + c.items.filter((i) => !i.available).length);
  }, [setBadge]);
  const load = useCallback(() => {
    setError(null);
    getCart().then(apply).catch(setError);
  }, [apply]);
  useEffect(load, [load]);

  async function change(it: CartItem, qty: number) {
    setBusy(it.product_id);
    setError(null);
    try {
      apply(qty <= 0 ? await removeFromCart(it.product_id) : await setCartQty(it.product_id, qty));
    } catch (e) {
      setError(e);
    } finally {
      setBusy("");
    }
  }

  if (!cart) return error ? <ErrorBox error={error} onRetry={load} /> : <p className="muted">{ta("loading")}</p>;
  if (!cart.items.length)
    return (
      <div className="empty stack-sm center-text">
        <p>{t("empty")}</p>
        <Link href="/catalog" className="btn btn-primary">{t("toCatalog")}</Link>
      </div>
    );
  // группируем по магазинам: заказ делится по бутикам
  const groups = new Map<string, CartItem[]>();
  for (const it of cart.items) groups.set(it.store_name, [...(groups.get(it.store_name) ?? []), it]);
  const checkoutHref = `/checkout?items=${cart.items.filter((i) => i.available).map((i) => `${i.product_id}:${i.qty}`).join(",")}`;
  return (
    <div className="cart-layout">
      <div className="stack">
        <ErrorBox error={error} />
        {[...groups.entries()].map(([store, items]) => (
          <section key={store} className="card stack-sm">
            <h2>{t("store", { store })}</h2>
            <ul className="plain stack">
              {items.map((it) => (
                <li key={it.product_id} className={`cart-item ${it.available ? "" : "item-bad"}`}>
                  <Link href={`/product/${it.product_id}`}>{it.image_url ? <img src={it.image_url} alt="" width={80} height={106} loading="lazy" /> : <div className="img-ph" />}</Link>
                  <div className="stack-xs">
                    <Link href={`/product/${it.product_id}`}>{it.title}</Link>
                    <span className="muted small">{it.brand}</span>
                    <span className="price">{formatSum(it.price, locale)}</span>
                    {!it.available && <span className="badge badge-bad">{it.unavailable_reason === "out_of_stock" ? t("outOfStock") : t("unavailable")}</span>}
                  </div>
                  <div className="stack-sm" style={{ alignItems: "flex-end" }}>
                    {it.available && (
                      <div className="qty" role="group" aria-label={t("qty")}>
                        <button type="button" aria-label="−" disabled={busy === it.product_id} onClick={() => change(it, it.qty - 1)}>−</button>
                        <span aria-live="polite">{it.qty}</span>
                        <button type="button" aria-label="+" disabled={busy === it.product_id || it.qty >= 99} onClick={() => change(it, it.qty + 1)}>+</button>
                      </div>
                    )}
                    <button type="button" className="btn-link small" disabled={busy === it.product_id} onClick={() => change(it, 0)}>{t("remove")}</button>
                  </div>
                </li>
              ))}
            </ul>
          </section>
        ))}
      </div>
      <aside className="card stack-sm">
        <p className="row between"><span>{t("items", { n: cart.total_qty })}</span></p>
        <p className="row between"><strong>{t("total")}</strong><span className="price price-lg">{formatSum(cart.total, locale)}</span></p>
        {cart.has_unavailable && <p className="field-error" role="alert">{t("fixProblems")}</p>}
        {cart.can_checkout ? (
          <Link href={checkoutHref} className="btn btn-primary btn-lg">{t("checkout")}</Link>
        ) : (
          <button className="btn btn-primary btn-lg" disabled>{t("checkout")}</button>
        )}
        {groups.size > 1 && <p className="muted small">{t("splitHint")}</p>}
        <p className="muted small">{t("payHint")}</p>
      </aside>
    </div>
  );
}

export default function CartPage() {
  const t = useTranslations("cart");
  return (
    <div className="stack">
      <h1>{t("title")}</h1>
      <AuthGate role="client"><CartView /></AuthGate>
    </div>
  );
}
