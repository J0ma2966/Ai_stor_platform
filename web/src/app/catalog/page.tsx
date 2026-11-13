import type { Metadata } from "next";
import { getLocale, getTranslations } from "next-intl/server";
import { catName, fetchCategories, fetchProducts, fetchStores, FILTER_KEYS, type CatalogFilters as F } from "@/lib/catalog";
import { alternates } from "@/lib/site";
import { ProductGrid } from "@/components/ProductGrid";
import { LoadMore } from "@/components/LoadMore";
import { CatalogFilters } from "@/components/CatalogFilters";

type SP = Promise<Record<string, string | string[] | undefined>>;

function readFilters(sp: Record<string, string | string[] | undefined>): F {
  const f: F = {};
  for (const k of FILTER_KEYS) {
    const v = sp[k];
    const s = (Array.isArray(v) ? v[0] : v)?.trim();
    if (s) f[k] = s.slice(0, 120);
  }
  if (!f.sort) f.sort = "popular";
  return f;
}

export async function generateMetadata({ searchParams }: { searchParams: SP }): Promise<Metadata> {
  const locale = await getLocale();
  const f = readFilters(await searchParams);
  const t = await getTranslations("seo");
  const tc = await getTranslations("catalog");
  let title = t("catalogTitle");
  if (f.q) title = tc("resultsFor", { q: f.q });
  else if (f.category) {
    const c = (await fetchCategories().catch(() => [])).find((x) => x.id === f.category);
    if (c) title = catName(c, locale);
  }
  const filtered = Object.keys(f).some((k) => k !== "sort" && k !== "category");
  return {
    title,
    description: t("catalogDesc"),
    alternates: alternates(locale, f.category && !filtered ? `/catalog?category=${f.category}` : "/catalog"),
    // страницы поиска и комбинаций фильтров не индексируем (дубли)
    robots: filtered ? { index: false, follow: true } : undefined,
  };
}

export default async function CatalogPage({ searchParams }: { searchParams: SP }) {
  const locale = await getLocale();
  const f = readFilters(await searchParams);
  const t = await getTranslations("catalog");
  const [cats, stores] = await Promise.all([fetchCategories().catch(() => []), fetchStores().catch(() => [])]);
  let page: { items: Awaited<ReturnType<typeof fetchProducts>>["items"]; nextCursor: string } | null = null;
  try {
    page = await fetchProducts(f);
  } catch {
    page = null;
  }
  const cat = cats.find((c) => c.id === f.category);
  // Фильтру нужны только id и название: остальные поля магазинов (описание, контакты, адрес)
  // иначе уезжали бы в HTML каждой страницы каталога как свойства клиентского компонента.
  const storeOptions = stores.map(({ id, name }) => ({ id, name }));
  const heading = f.q ? t("resultsFor", { q: f.q }) : cat ? catName(cat, locale) : t("title");
  const hasFilters = Object.keys(f).some((k) => k !== "sort");
  return (
    <div className="catalog">
      <h1>{heading}</h1>
      <div className="catalog-layout">
        <aside>
          <CatalogFilters key={JSON.stringify(f)} filters={f} categories={cats} stores={storeOptions} />
        </aside>
        <section aria-live="polite">
          {!page ? (
            <div role="alert" className="alert"><p>{t("error")}</p></div>
          ) : page.items.length === 0 ? (
            <p className="empty">{hasFilters ? t("empty") : t("emptyCatalog")}</p>
          ) : (
            <>
              <ProductGrid items={page.items} priority={4} />
              <LoadMore key={JSON.stringify(f)} filters={f} cursor={page.nextCursor} />
            </>
          )}
        </section>
      </div>
    </div>
  );
}
