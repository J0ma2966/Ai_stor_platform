import type { Metadata } from "next";
import { getTranslations, setRequestLocale } from "next-intl/server";
import { Link } from "@/i18n/navigation";
import { catName, fetchCategories, fetchProducts, fetchStores, type Category, type Product, type Store } from "@/lib/catalog";
import { alternates, siteUrl } from "@/lib/site";
import { ProductGrid } from "@/components/ProductGrid";
import { StoreCard } from "@/components/StoreCard";
import { JsonLd } from "@/components/JsonLd";
import { TRYON_ENABLED } from "@/lib/fitting";

export async function generateMetadata({ params }: { params: Promise<{ locale: string }> }): Promise<Metadata> {
  const { locale } = await params;
  return { alternates: alternates(locale, "/") };
}

const zoneIcon: Record<string, string> = { headwear: "🧢", top: "🧥", bottom: "👖", shoes: "👟" };

async function safe<T>(p: Promise<T>, fallback: T): Promise<T> {
  try {
    return await p;
  } catch {
    return fallback;
  }
}

export default async function Home({ params }: { params: Promise<{ locale: string }> }) {
  const { locale } = await params;
  setRequestLocale(locale);
  const t = await getTranslations("home");
  const [cats, popular, fresh, stores] = await Promise.all([
    safe<Category[]>(fetchCategories(), []),
    safe<{ items: Product[] }>(fetchProducts({ sort: "popular" }, "", 8), { items: [] }),
    safe<{ items: Product[] }>(fetchProducts({ sort: "new" }, "", 8), { items: [] }),
    safe<Store[]>(fetchStores(), []),
  ]);
  return (
    <div className="stack-lg">
      <JsonLd data={{ "@context": "https://schema.org", "@type": "Organization", name: "Butik", url: siteUrl() }} />
      <section className="hero">
        <div className="hero-text">
          <h1>{TRYON_ENABLED ? t("heroTitle") : t("heroTitleShop")}</h1>
          <p>{TRYON_ENABLED ? t("heroText") : t("heroTextShop")}</p>
          <div className="row wrap">
            {TRYON_ENABLED ? (
              <>
                <Link href="/fitting" className="btn btn-primary btn-lg">{t("heroCta")}</Link>
                <Link href="/catalog" className="btn btn-light btn-lg">{t("heroCatalog")}</Link>
              </>
            ) : (
              <>
                <Link href="/catalog" className="btn btn-primary btn-lg">{t("heroCatalog")}</Link>
                <Link href="/stores" className="btn btn-light btn-lg">{t("allStores")}</Link>
              </>
            )}
          </div>
        </div>
        <div className="hero-art" aria-hidden="true">
          <div className="mannequin">
            <span className="zone z-head">🧢</span>
            <span className="zone z-top">🧥</span>
            <span className="zone z-bottom">👖</span>
            <span className="zone z-shoes">👟</span>
          </div>
        </div>
      </section>

      {cats.length > 0 && (
        <section aria-labelledby="h-cats">
          <h2 id="h-cats">{t("categories")}</h2>
          <ul className="cat-grid plain">
            {cats.map((c) => (
              <li key={c.id}>
                <Link href={`/catalog?category=${c.id}`} className="cat-tile">
                  <span className="cat-icon" aria-hidden="true">{zoneIcon[c.zone] ?? "👗"}</span>
                  <span>{catName(c, locale)}</span>
                </Link>
              </li>
            ))}
          </ul>
        </section>
      )}

      <section aria-labelledby="h-pop">
        <div className="row between"><h2 id="h-pop">{t("popular")}</h2><Link href="/catalog?sort=popular">{t("seeAll")}</Link></div>
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
          <div className="row between"><h2 id="h-new">{t("new")}</h2><Link href="/catalog?sort=new">{t("seeAll")}</Link></div>
          <ProductGrid items={fresh.items} />
        </section>
      )}

      {stores.length > 0 && (
        <section aria-labelledby="h-stores">
          <div className="row between"><h2 id="h-stores">{t("stores")}</h2><Link href="/stores">{t("allStores")}</Link></div>
          <ul className="store-grid plain">
            {stores.slice(0, 8).map((s) => <li key={s.id}><StoreCard s={s} /></li>)}
          </ul>
        </section>
      )}

      <section aria-labelledby="h-adv">
        <h2 id="h-adv">{t("advTitle")}</h2>
        <ul className="adv-grid plain">
          {[1, 2, 3, 4].filter((i) => TRYON_ENABLED || i !== 2).map((i) => (
            <li key={i} className="card">
              <h3>{t(`adv${i}`)}</h3>
              <p className="muted">{t(`adv${i}t`)}</p>
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
