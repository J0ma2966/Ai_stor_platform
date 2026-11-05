import { TRYON_ENABLED } from "@/lib/fitting";
import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { localizePath, type Locale } from "@/i18n/routing";
import { getLocale, getTranslations } from "next-intl/server";
import { Link } from "@/i18n/navigation";
import { catName, fetchCategories, fetchProduct, fetchProducts, frontImage } from "@/lib/catalog";
import { formatSum } from "@/lib/money";
import { alternates, siteUrl } from "@/lib/site";
import { Gallery } from "@/components/Gallery";
import { ProductActions } from "@/components/ProductActions";
import { ProductGrid } from "@/components/ProductGrid";
import { JsonLd } from "@/components/JsonLd";
import { ProductReviews } from "@/components/ProductReviews";
import { Stars } from "@/components/Stars";
import { formatRating } from "@/lib/reviews";

type P = Promise<{ id: string }>;

export async function generateMetadata({ params }: { params: P }): Promise<Metadata> {
  const { id } = await params;
  const locale = await getLocale();
  const p = await fetchProduct(id).catch(() => null);
  if (!p) return { title: "404", robots: { index: false } };
  const desc = `${p.brand} · ${formatSum(p.price, locale)} · ${p.store_name}. ${(p.description ?? "").slice(0, 140)}`;
  const img = frontImage(p);
  return {
    title: `${p.title} — ${p.brand}`,
    description: desc,
    alternates: alternates(locale, `/product/${p.id}`),
    openGraph: { type: "website", title: p.title, description: desc, images: img ? [{ url: img, width: 600, height: 800 }] : undefined },
  };
}

export default async function ProductPage({ params }: { params: P }) {
  const { id } = await params;
  const locale = await getLocale();
  const p = await fetchProduct(id);
  if (!p) notFound();
  const t = await getTranslations("product");
  const tr = await getTranslations("reviews");
  const ratingCount = p.rating_count ?? 0;
  const ratingAvg = p.rating_avg ?? 0;
  const [cats, more] = await Promise.all([
    fetchCategories().catch(() => []),
    fetchProducts({ store: p.store_id, sort: "popular" }, "", 9).catch(() => ({ items: [], nextCursor: "" })),
  ]);
  const cat = cats.find((c) => c.id === p.category_id);
  const others = more.items.filter((x) => x.id !== p.id).slice(0, 8);
  const url = siteUrl() + localizePath(`/product/${p.id}`, locale as Locale);
  return (
    <div className="stack-lg">
      <JsonLd
        data={{
          "@context": "https://schema.org",
          "@type": "Product",
          name: p.title,
          description: p.description,
          brand: { "@type": "Brand", name: p.brand },
          image: Object.values(p.images),
          sku: p.id,
          category: cat ? catName(cat, locale) : undefined,
          aggregateRating: ratingCount > 0
            ? { "@type": "AggregateRating", ratingValue: ratingAvg.toFixed(2), reviewCount: ratingCount, bestRating: 5, worstRating: 1 }
            : undefined,
          offers: {
            "@type": "Offer",
            url,
            priceCurrency: "UZS",
            price: (p.price / 100).toFixed(2),
            availability: p.in_stock ? "https://schema.org/InStock" : "https://schema.org/OutOfStock",
            seller: { "@type": "Organization", name: p.store_name },
          },
        }}
      />
      <nav aria-label="breadcrumbs" className="crumbs small">
        <Link href="/catalog">{t("category")}</Link>
        {cat && <> / <Link href={`/catalog?category=${cat.id}`}>{catName(cat, locale)}</Link></>}
      </nav>
      <div className="product-layout">
        <Gallery images={p.images} title={p.title} />
        <div className="stack product-info">
          <p className="muted">{p.brand}</p>
          <h1>{p.title}</h1>
          <a href="#reviews" className="rating-link small">
            {ratingCount > 0 ? (
              <>
                <Stars value={ratingAvg} label={tr("avgLabel", { avg: formatRating(ratingAvg), max: 5 })} />
                <span>{formatRating(ratingAvg)}</span>
                <span className="muted">{tr("count", { n: ratingCount })}</span>
              </>
            ) : (
              <span className="muted">{tr("noneShort")}</span>
            )}
          </a>
          <p className="price price-lg">{formatSum(p.price, locale)}</p>
          <p>
            <span className={`badge ${p.in_stock ? "badge-ok" : "badge-bad"}`}>{p.in_stock ? t("inStock") : t("outOfStock")}</span>
          </p>
          <ProductActions product={p} />
          {TRYON_ENABLED && <p className="muted small">{t("fittingNote")}</p>}
          <dl className="kv">
            <div><dt>{t("brand")}</dt><dd>{p.brand}</dd></div>
            {cat && <div><dt>{t("category")}</dt><dd>{catName(cat, locale)}</dd></div>}
            <div><dt>{t("seller")}</dt><dd>{p.store_slug ? <Link href={`/stores/${p.store_slug}`}>{p.store_name}</Link> : p.store_name}</dd></div>
          </dl>
          {p.description && (
            <section>
              <h2>{t("description")}</h2>
              <p className="pre">{p.description}</p>
            </section>
          )}
        </div>
      </div>
      <ProductReviews productId={p.id} avg={ratingAvg} count={ratingCount} dist={p.rating_dist ?? [0, 0, 0, 0, 0]} />
      {others.length > 0 && (
        <section>
          <div className="row between">
            <h2>{t("moreFromStore", { store: p.store_name })}</h2>
            {p.store_slug && <Link href={`/stores/${p.store_slug}`}>{t("goStore")}</Link>}
          </div>
          <ProductGrid items={others} />
        </section>
      )}
    </div>
  );
}
