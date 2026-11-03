import type { Metadata } from "next";
import { getTranslations, setRequestLocale } from "next-intl/server";
import { fetchStores } from "@/lib/catalog";
import { alternates } from "@/lib/site";
import { StoreCard } from "@/components/StoreCard";

export async function generateMetadata({ params }: { params: Promise<{ locale: string }> }): Promise<Metadata> {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "seo" });
  return { title: t("storesTitle"), description: t("storesDesc"), alternates: alternates(locale, "/stores") };
}

export default async function StoresPage({ params, searchParams }: { params: Promise<{ locale: string }>; searchParams: Promise<{ q?: string }> }) {
  const { locale } = await params;
  setRequestLocale(locale);
  const { q = "" } = await searchParams;
  const t = await getTranslations("stores");
  const stores = await fetchStores(q.slice(0, 100)).catch(() => null);
  return (
    <div className="stack">
      <h1>{t("title")}</h1>
      <form className="row" role="search">
        <label htmlFor="store-q" className="sr-only">{t("searchPh")}</label>
        <input id="store-q" name="q" type="search" defaultValue={q} placeholder={t("searchPh")} maxLength={100} />
        <button className="btn">⌕</button>
      </form>
      {stores === null ? (
        <div role="alert" className="alert"><p>{t("empty")}</p></div>
      ) : stores.length === 0 ? (
        <p className="empty">{t("empty")}</p>
      ) : (
        <ul className="store-grid plain">
          {stores.map((s) => <li key={s.id}><StoreCard s={s} /></li>)}
        </ul>
      )}
    </div>
  );
}
