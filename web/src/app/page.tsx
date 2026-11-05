import type { Metadata } from "next";
import { getLocale, getTranslations } from "next-intl/server";
import { Link } from "@/i18n/navigation";
import { catName, fetchCategories, fetchProducts, fetchStores, type Category, type Product, type Store } from "@/lib/catalog";
import { alternates, siteUrl } from "@/lib/site";
import { ProductGrid } from "@/components/ProductGrid";
import { StoreCard } from "@/components/StoreCard";
import { JsonLd } from "@/components/JsonLd";
import { TRYON_ENABLED } from "@/lib/fitting";
import { Icon, type IconName } from "@/components/Icon";

export async function generateMetadata(): Promise<Metadata> {
  const locale = await getLocale();
  return { alternates: alternates(locale, "/") };
}

const zones = new Set(["headwear", "top", "bottom", "shoes"]);
const advIcon: Record<number, IconName> = { 1: "store", 2: "sparkle", 3: "truck", 4: "sparkle", 5: "wallet" };

async function safe<T>(p: Promise<T>, fallback: T): Promise<T> {
  try {
    return await p;
  } catch {
    return fallback;
  }
}

export default async function Home() {
  const locale = await getLocale();
  const t = await getTranslations("home");
  const [cats, popular, fresh, stores] = await Promise.all([
    safe<Category[]>(fetchCategories(), []),
    safe<{ items: Product[] }>(fetchProducts({ sort: "popular" }, "", 10), { items: [] }),
    safe<{ items: Product[] }>(fetchProducts({ sort: "new" }, "", 10), { items: [] }),
    safe<Store[]>(fetchStores(), []),
  ]);
  return (
    <div className="stack-lg">
      <JsonLd data={{ "@context": "https://schema.org", "@type": "Organization", name: "Butik", url: siteUrl() }} />
      <div className="home-top">
        <section className="hero" aria-labelledby="h-hero">
          <span className="hero-tag">{t("heroTag")}</span>
          <h1 id="h-hero">{TRYON_ENABLED ? t("heroTitle") : t("heroTitleShop")}</h1>
          <p>{TRYON_ENABLED ? t("heroText") : t("heroTextShop")}</p>
          <div className="row wrap">
            {TRYON_ENABLED ? (
              <>
                <Link href="/fitting" className="btn btn-primary btn-lg">{t("heroCta")}</Link>
                <Link href="/catalog" className="btn btn-ghost btn-lg">{t("heroCatalog")}</Link>
              </>
            ) : (
              <>
                <Link href="/catalog" className="btn btn-primary btn-lg">{t("heroCatalog")}</Link>
                <Link href="/stores" className="btn btn-ghost btn-lg">{t("allStores")}</Link>
              </>
            )}
          </div>
        </section>
        <div className="promo-col">
          <Link href="/catalog?sort=new" className="promo promo-yellow">
            <span className="promo-title">{t("promoNew")}</span>
            <span className="promo-go">{t("promoGo")} <Icon name="arrow" size={18} strokeWidth={2.2} /></span>
          </Link>
          <Link href="/stores" className="promo promo-white">
            <span className="promo-title">{t("promoStores")}</span>
            <span className="promo-go">{t("promoGo")} <Icon name="arrow" size={18} strokeWidth={2.2} /></span>
          </Link>
        </div>
      </div>

      {cats.length > 0 && (
        <section aria-labelledby="h-cats">
          <div className="section-head"><h2 id="h-cats">{t("categories")}</h2></div>
          <ul className="cat-grid plain">
            {cats.map((c) => (
              <li key={c.id}>
                <Link href={`/catalog?category=${c.id}`} className="cat-tile">
                  <span className={`cat-icon cat-${zones.has(c.zone) ? c.zone : "other"}`} aria-hidden="true">
                    <Icon name={zones.has(c.zone) ? (c.zone as IconName) : "hanger"} strokeWidth={1.6} />
                  </span>
                  <span>{catName(c, locale)}</span>
                </Link>
              </li>
            ))}
          </ul>
        </section>
      )}

      <section aria-labelledby="h-pop">
        <div className="section-head"><h2 id="h-pop">{t("popular")}</h2><Link href="/catalog?sort=popular" className="see-all">{t("seeAll")} <Icon name="arrow" size={18} /></Link></div>
        {popular.items.length ? <ProductGrid items={popular.items} priority={4} /> : <p className="muted">{t("emptyCatalog")}</p>}
      </section>

      {TRYON_ENABLED && (
      <section className="how" aria-labelledby="h-how">
        <h2 id="h-how">{t("howTitle")}</h2>
        <ol className="how-steps plain">
          {[1, 2, 3].map((i) => (
            <li key={i} className="card">
              <span className="how-num" aria-hidden="true">{i}</span>
              <h3>{t(`how${i}`)}</h3>
              <p className="muted">{t(`how${i}t`)}</p>
            </li>
          ))}
        </ol>
      </section>
      )}

      {fresh.items.length > 0 && (
        <section aria-labelledby="h-new">
          <div className="section-head"><h2 id="h-new">{t("new")}</h2><Link href="/catalog?sort=new" className="see-all">{t("seeAll")} <Icon name="arrow" size={18} /></Link></div>
          <ProductGrid items={fresh.items} />
        </section>
      )}

      {stores.length > 0 && (
        <section aria-labelledby="h-stores">
          <div className="section-head"><h2 id="h-stores">{t("stores")}</h2><Link href="/stores" className="see-all">{t("allStores")} <Icon name="arrow" size={18} /></Link></div>
          <ul className="store-grid plain">
            {stores.slice(0, 8).map((s) => <li key={s.id}><StoreCard s={s} /></li>)}
          </ul>
        </section>
      )}

      <section aria-labelledby="h-adv">
        <div className="section-head"><h2 id="h-adv">{t("advTitle")}</h2></div>
        <ul className="benefits plain">
          {(TRYON_ENABLED ? [1, 2, 3, 4] : [1, 3, 5]).map((i) => (
            <li key={i} className="benefit">
              <span className="benefit-icon" aria-hidden="true"><Icon name={advIcon[i]} size={24} /></span>
              <div>
                <h3>{t(`adv${i}`)}</h3>
                <p className="muted small">{t(`adv${i}t`)}</p>
              </div>
            </li>
          ))}
        </ul>
      </section>

      {TRYON_ENABLED && (
        <section className="cta card">
          <h2>{t("ctaTitle")}</h2>
          <p className="muted">{t("ctaText")}</p>
          <Link href="/fitting" className="btn btn-primary btn-lg">{t("ctaBtn")}</Link>
        </section>
      )}
    </div>
  );
}
