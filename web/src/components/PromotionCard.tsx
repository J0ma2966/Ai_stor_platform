"use client";
import type { ReactNode } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Link } from "@/i18n/navigation";
import { formatSum } from "@/lib/money";
import { formatDay, type Promotion, type PromoState } from "@/lib/promotions";

const tone: Record<PromoState, string> = { pending: "warn", scheduled: "info", active: "ok", ended: "info", rejected: "bad", stopped: "bad" };

export function PromoStateBadge({ state }: { state: PromoState }) {
  const t = useTranslations("promo");
  return <span className={`badge badge-${tone[state]}`}>{t(`st_${state}`)}</span>;
}

// Заявка или реклама: в кабинете магазина и в админке. extra — действия.
export function PromotionCard({ p, showStore, extra }: { p: Promotion; showStore?: boolean; extra?: ReactNode }) {
  const t = useTranslations("promo");
  const locale = useLocale();
  return (
    <article className="card promo-req">
      {p.image_url ? <img src={p.image_url} alt="" width={64} height={85} loading="lazy" /> : <div className="img-ph promo-req-ph" aria-hidden="true" />}
      <div className="stack-sm grow">
        <div className="row wrap between">
          <strong>
            {p.state === "active" || p.state === "scheduled" || p.state === "pending"
              ? <Link href={`/product/${p.product_id}`} target="_blank">{p.product_title}</Link>
              : p.product_title}
          </strong>
          <PromoStateBadge state={p.state} />
        </div>
        <span className="muted small">
          {showStore && <>{p.store_name} · </>}{formatSum(p.product_price, locale)} · {t("requested", { n: p.days })}
        </span>
        {p.starts_at && p.ends_at && <span className="small">{t("period", { from: formatDay(p.starts_at), to: formatDay(p.ends_at) })}</span>}
        {p.store_note && <span className="small">{t("note", { note: p.store_note })}</span>}
        {p.admin_reason && <span className="small field-error">{t("reason", { reason: p.admin_reason })}</span>}
        {extra}
      </div>
    </article>
  );
}
