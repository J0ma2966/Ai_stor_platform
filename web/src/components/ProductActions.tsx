"use client";
import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Link, usePathname, useRouter } from "@/i18n/navigation";
import { frontImage, type Product } from "@/lib/catalog";
import { addToFitting, getFitting, TRYON_ENABLED } from "@/lib/fitting";
import { useSession } from "@/lib/session";
import { setCartQty } from "@/lib/shop";
import { ErrorBox } from "./ErrorBox";
import { FavoriteButton } from "./FavoriteButton";

// «В корзину», «В избранное», «В примерочную» (ТЗ 4.4). Корзина и примерочная — разные модули.
export function ProductActions({ product: p }: { product: Product }) {
  const t = useTranslations("product");
  const { me, setCartQty: setBadge } = useSession();
  const router = useRouter();
  const pathname = usePathname();
  const [inCart, setInCart] = useState(false);
  const [inFitting, setInFitting] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    const sync = () => setInFitting(!!p.zone && getFitting()[p.zone]?.id === p.id);
    sync();
    window.addEventListener("fitting:change", sync);
    return () => window.removeEventListener("fitting:change", sync);
  }, [p.id, p.zone]);

  if (me && me.role !== "client") return null;

  async function toCart() {
    if (inCart) {
      router.push("/cart");
      return;
    }
    if (!me) {
      router.push(`/login?next=${encodeURIComponent(pathname)}`);
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const cart = await setCartQty(p.id, 1);
      setBadge(cart.total_qty);
      setInCart(true);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }

  function toFitting() {
    if (!p.zone) return;
    addToFitting({ id: p.id, title: p.title, brand: p.brand, price: p.price, image_url: frontImage(p), zone: p.zone, store_name: p.store_name });
  }

  return (
    <div className="stack-sm">
      <div className="row wrap actions">
        <button type="button" className="btn btn-primary btn-lg" disabled={busy || !p.in_stock} onClick={toCart}>
          {inCart ? t("inCart") : t("toCart")}
        </button>
        {TRYON_ENABLED && p.zone && (
          inFitting ? (
            <Link href="/fitting" className="btn btn-lg btn-secondary">{t("inFitting")} →</Link>
          ) : (
            <button type="button" className="btn btn-lg btn-accent2" onClick={toFitting}>{t("toFitting")}</button>
          )
        )}
        <FavoriteButton productId={p.id} withText className="btn btn-lg" />
      </div>
      <p role="status" className="ok-text">{inCart ? t("added") : ""}</p>
      <ErrorBox error={error} />
    </div>
  );
}
