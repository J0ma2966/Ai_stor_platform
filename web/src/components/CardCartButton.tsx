"use client";
import { useState } from "react";
import { useTranslations } from "next-intl";
import { usePathname, useRouter } from "@/i18n/navigation";
import { useSession } from "@/lib/session";
import { setCartQty } from "@/lib/shop";

// Кнопка «В корзину» прямо в карточке каталога: не нужно открывать товар, чтобы его купить.
export function CardCartButton({ productId, inStock }: { productId: string; inStock: boolean }) {
  const t = useTranslations("product");
  const { me, setCartQty: setBadge } = useSession();
  const router = useRouter();
  const pathname = usePathname();
  const [state, setState] = useState<"idle" | "busy" | "added" | "error">("idle");
  if (me && me.role !== "client") return null;

  async function add() {
    if (state === "added") {
      router.push("/cart");
      return;
    }
    if (!me) {
      router.push(`/login?next=${encodeURIComponent(pathname)}`);
      return;
    }
    setState("busy");
    try {
      const cart = await setCartQty(productId, 1);
      setBadge(cart.total_qty);
      setState("added");
    } catch {
      setState("error");
    }
  }

  return (
    <button
      type="button"
      className={`btn pcard-cart ${state === "added" ? "btn-secondary" : "btn-primary"}`}
      disabled={!inStock || state === "busy"}
      aria-live="polite"
      onClick={add}
    >
      {!inStock ? t("outOfStock") : state === "added" ? t("inCart") : state === "error" ? t("retry") : t("toCart")}
    </button>
  );
}
