"use client";
import { useState } from "react";
import { useTranslations } from "next-intl";
import { usePathname, useRouter } from "@/i18n/navigation";
import { useSession } from "@/lib/session";
import { addFavorite, removeFavorite } from "@/lib/shop";
import { Icon } from "./Icon";

export function FavoriteButton({ productId, className = "", withText = false }: { productId: string; className?: string; withText?: boolean }) {
  const t = useTranslations("product");
  const { me, favorites, toggleFavoriteLocal } = useSession();
  const router = useRouter();
  const pathname = usePathname();
  const [busy, setBusy] = useState(false);
  if (me && me.role !== "client") return null;
  const on = favorites.has(productId);

  async function toggle() {
    if (!me) {
      router.push(`/login?next=${encodeURIComponent(pathname)}`);
      return;
    }
    setBusy(true);
    toggleFavoriteLocal(productId, !on);
    try {
      await (on ? removeFavorite(productId) : addFavorite(productId));
    } catch {
      toggleFavoriteLocal(productId, on);
    } finally {
      setBusy(false);
    }
  }
  const label = on ? t("inFavorites") : t("toFavorites");
  return (
    <button type="button" className={`fav-btn ${on ? "fav-on" : ""} ${className}`} aria-pressed={on} aria-label={label} title={label} disabled={busy} onClick={toggle}>
      <Icon name="heart" size={20} filled={on} />
      {withText && <span>{label}</span>}
    </button>
  );
}
