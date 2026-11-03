import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { getLocale, getTranslations } from "next-intl/server";
import { fetchCategories, fetchProducts, fetchStore, FILTER_KEYS, type CatalogFilters as F } from "@/lib/catalog";
import { alternates } from "@/lib/site";
import { ProductGrid } from "@/components/ProductGrid";
import { LoadMore } from "@/components/LoadMore";
import { CatalogFilters } from "@/components/CatalogFilters";
import { JsonLd } from "@/components/JsonLd";

type P = Promise<{ slug: string }>;

export async function generateMetadata({ params }: { params: P }): Promise<Metadata> {
  const { slug } = await params;
  const locale = await getLocale();
  const s = await fetchStore(slug).catch(() => null);
  if (!s) return { title: "404", robots: { index: false } };
  return {
    title: s.name,
    description: s.description.slice(0, 160) || s.name,
    alternates: alternates(locale, `/stores/${s.slug}`),
    openGraph: { title: s.name, images: s.cover_url ? [{ url: s.cover_url }] : s.logo_url ? [{ url: s.logo_url }] : undefined },
  };
}

function safeUrl(u?: string): string | null {
  if (!u) return null;
  try {
    const x = new URL(u);
    return x.protocol === "https:" || x.protocol === "http:" ? x.toString() : null;
  } catch {
    return null;
  }
}

export default async function StorePage({ params, searchParams }: { params: P; searchParams: Promise<Record<string, string | undefined>> }) {
  const { slug } = await params;
  const locale = await getLocale();
  const s = await fetchStore(slug);
  if (!s) notFound();
  const sp = await searchParams;
  const f: F = { store: s.id, sort: "popular" };
  for (const k of FILTER_KEYS) if (k !== "store" && sp[k]) f[k] = String(sp[k]).slice(0, 120);
  const t = await getTranslations("stores");
  const [cats, page] = await Promise.all([fetchCategories().catch(() => []), fetchProducts(f).catch(() => ({ items: [], nextCursor: "" }))]);
  const site = safeUrl(s.contacts.website);
  const tg = s.contacts.telegram?.replace(/^@/, "").replace(/[^A-Za-z0-9_]/g, "");
  const ig = s.contacts.instagram?.replace(/^@/, "").replace(/[^A-Za-z0-9_.]/g, "");
  return (
    <div className="stack-lg">
      <JsonLd data={{ "@context": "https://schema.org", "@type": "Store", name: s.name, description: s.description, image: s.logo_url, telephone: s.phone, address: s.address }} />
      <section className="store-hero">
        {s.cover_url && <img src={s.cover_url} alt="" className="store-cover" width={1200} height={300} />}
        <div className="row store-head">
          {s.logo_url ? <img src={s.logo_url} alt="" width={80} height={80} className="store-logo lg" /> : <span className="store-logo lg ph" aria-hidden="true">{s.name.slice(0, 1)}</span>}
          <div>
            <h1>{s.name}</h1>
            <p className="muted">{t("products", { n: s.product_count })}</p>
          </div>
        </div>
        {s.description && <p className="pre">{s.description}</p>}
        <dl className="kv">
          {s.address && (
            <div><dt>{t("address")}</dt><dd>{s.address}{s.lat != null && s.lng != null && <> · <a href={`https://yandex.uz/maps/?pt=${s.lng},${s.lat}&z=17&l=map`} target="_blank" rel="noopener noreferrer">{t("onMap")}</a></>}</dd></div>
          )}
          {s.phone && <div><dt>{t("phone")}</dt><dd><a href={`tel:${s.phone}`}>{s.phone}</a></dd></div>}
          {s.contacts.hours && <div><dt>{t("hours")}</dt><dd>{s.contacts.hours}</dd></div>}
          {(site || tg || ig) && (
            <div><dt>{t("contacts")}</dt><dd className="row wrap">
              {site && <a href={site} target="_blank" rel="noopener noreferrer nofollow">{t("website")}</a>}
              {tg && <a href={`https://t.me/${tg}`} target="_blank" rel="noopener noreferrer nofollow">Telegram</a>}
              {ig && <a href={`https://instagram.com/${ig}`} target="_blank" rel="noopener noreferrer nofollow">Instagram</a>}
            </dd></div>
          )}
        </dl>
      </section>
      <section>
        <h2>{t("catalog")}</h2>
        <div className="catalog-layout">
          <aside><CatalogFilters key={JSON.stringify(f)} filters={f} categories={cats} stores={[]} fixed={{ store: s.id }} /></aside>
          <section>
            {page.items.length ? (
              <>
                <ProductGrid items={page.items} />
                <LoadMore key={JSON.stringify(f)} filters={f} cursor={page.nextCursor} />
              </>
            ) : (
              <p className="empty">{t("empty")}</p>
            )}
          </section>
        </div>
      </section>
    </div>
  );
}
