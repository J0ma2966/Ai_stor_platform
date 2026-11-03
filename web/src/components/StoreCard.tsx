import { getTranslations } from "next-intl/server";
import { Link } from "@/i18n/navigation";
import type { Store } from "@/lib/catalog";

export async function StoreCard({ s }: { s: Store }) {
  const t = await getTranslations("stores");
  return (
    <Link href={`/stores/${s.slug}`} className="store-card card">
      {s.logo_url ? <img src={s.logo_url} alt="" width={56} height={56} loading="lazy" className="store-logo" /> : <span className="store-logo ph" aria-hidden="true">{s.name.slice(0, 1)}</span>}
      <span className="stack-xs">
        <strong>{s.name}</strong>
        <span className="muted small">{t("products", { n: s.product_count })}</span>
      </span>
    </Link>
  );
}
