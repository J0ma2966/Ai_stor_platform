"use client";
import { useLocale, useTranslations } from "next-intl";
import { formatSum } from "@/lib/money";
import type { OrderItem } from "@/lib/orders";

export function Items({ items }: { items: OrderItem[] }) {
  const locale = useLocale();
  const t = useTranslations("checkout");
  return (
    <ul className="plain items">
      {items.map((it, i) => (
        <li key={i} className="item">
          {it.image_url && <img src={it.image_url} alt="" width={48} height={64} />}
          <span className="grow">{it.brand} · {it.title}{it.size && <strong> · {t("size", { size: it.size })}</strong>}</span>
          <span>×{it.qty}</span>
          <span>{formatSum(it.unit_price * it.qty, locale)}</span>
        </li>
      ))}
    </ul>
  );
}
