"use client";
import { useLocale } from "next-intl";
import { Link } from "@/i18n/navigation";
import { formatSum } from "@/lib/money";
import type { OrderItem } from "@/lib/orders";

// reviewLabel — у доставленного заказа клиента: ссылка «Оценить товар» к блоку отзывов.
export function Items({ items, reviewLabel }: { items: OrderItem[]; reviewLabel?: string }) {
  const locale = useLocale();
  return (
    <ul className="plain items">
      {items.map((it, i) => (
        <li key={i} className="item">
          {it.image_url && <img src={it.image_url} alt="" width={48} height={64} />}
          <span className="grow">
            {it.brand} · {it.title}
            {reviewLabel && it.product_id && <><br /><Link href={`/product/${it.product_id}#reviews`} className="small">{reviewLabel}</Link></>}
          </span>
          <span>×{it.qty}</span>
          <span>{formatSum(it.unit_price * it.qty, locale)}</span>
        </li>
      ))}
    </ul>
  );
}
