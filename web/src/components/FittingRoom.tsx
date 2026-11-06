"use client";
import { useCallback, useEffect, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Link, useRouter } from "@/i18n/navigation";
import { api, apiList, currentRole, refreshSession } from "@/lib/api";
import { formatSum } from "@/lib/money";
import { addToFitting, FITTING_EVENT, fittingItems, getFitting, hasSet, removeFromFitting, visibleZones, type Fitting, type FittingItem, type Zone } from "@/lib/fitting";
import { getQuote, type Quote } from "@/lib/tryon";
import { Mannequin } from "@/components/Mannequin";
import { ErrorBox } from "@/components/ErrorBox";

export function useFitting(): Fitting {
  const [f, setF] = useState<Fitting>({});
  useEffect(() => {
    const sync = () => setF(getFitting());
    sync();
    window.addEventListener(FITTING_EVENT, sync);
    window.addEventListener("storage", sync); // другие вкладки
    return () => {
      window.removeEventListener(FITTING_EVENT, sync);
      window.removeEventListener("storage", sync);
    };
  }, []);
  return f;
}

type Category = { id: string; zone: Zone; name_ru: string; name_uz: string };
type Product = { id: string; title: string; brand: string; price: number; store_name: string; is_set?: boolean; images: Record<string, string>; thumbs?: Record<string, string> };

// Вкладки выбора: «Двойки» — товары зоны «верх» с флагом is_set (верх и низ на одном фото).
type Tab = Zone | "set";
const TABS: Tab[] = ["headwear", "top", "set", "bottom", "shoes"];
const tabZone = (tab: Tab): Zone => (tab === "set" ? "top" : tab);

// Экран примерочной: манекен, карточки у зон, выбор товаров, запуск генерации.
export function FittingRoom() {
  const t = useTranslations("tryon");
  const locale = useLocale();
  const router = useRouter();
  const fitting = useFitting();
  const items = fittingItems(fitting);
  const [quote, setQuote] = useState<Quote | null>(null);
  const [authed, setAuthed] = useState(false);

  useEffect(() => {
    (async () => {
      const ok = currentRole() === "client" || ((await refreshSession()) && currentRole() === "client");
      setAuthed(ok);
      if (ok) setQuote(await getQuote().catch(() => null));
    })();
  }, []);

  const unsupported = quote ? items.filter((i) => !quote.supported_zones.includes(i.zone)) : [];

  const start = () => {
    if (!authed) {
      router.push(`/login?next=${encodeURIComponent("/fitting/new")}`);
      return;
    }
    router.push("/fitting/new");
  };

  return (
    <div className="fitting">
      <section className="card fitting-room" aria-label={t("roomTitle")}>
        <div className="mannequin-wrap">
          <Mannequin gender={quote?.gender ?? null} label={t(quote?.gender === "female" ? "mannequinFemale" : "mannequinMale")} />
          <ul className="plain zone-slots">
            {visibleZones(fitting).map((z) => {
              const it = fitting[z];
              return (
                <li key={z} className={`zone-slot zone-${z}${it?.is_set ? " zone-set" : ""}`}>
                  <span className="small muted">{it?.is_set ? t("setLabel") : t(`zone.${z}`)}</span>
                  {it ? (
                    <div className="zone-card">
                      {it.image_url && <img src={it.image_url} alt="" width={56} height={56} />}
                      <div className="grow">
                        <div className="small clamp">{it.title}</div>
                        <div className="small muted">{formatSum(it.price, locale)}</div>
                      </div>
                      <button type="button" className="btn-link small" onClick={() => removeFromFitting(z)} aria-label={t("removeItem", { title: it.title })}>
                        ✕
                      </button>
                    </div>
                  ) : (
                    <div className="zone-empty small muted">{t("emptyZone")}</div>
                  )}
                </li>
              );
            })}
          </ul>
        </div>
        <div className="stack">
          {unsupported.length > 0 && <p className="small warn-text">{t("unsupportedZones", { zones: unsupported.map((i) => t(`zone.${i.zone}`)).join(", ") })}</p>}
          <p className="small muted">{t("disclaimer")}</p>
          <button type="button" className="btn btn-primary" disabled={items.length === 0} onClick={start}>
            {t("generate")}
            {quote ? ` · ${t("costTokens", { n: quote.cost })}` : ""}
          </button>
          {items.length === 0 && <p className="small muted">{t("pickFirst")}</p>}
          {authed && (
            <p className="small">
              <Link href="/account/tryons">{t("history")}</Link> · <Link href="/account/photos">{t("myPhotos")}</Link>
            </p>
          )}
        </div>
      </section>
      <ProductPicker fitting={fitting} />
    </div>
  );
}

// Список товаров по зонам (справа на десктопе, ниже на мобильном). Добавление — кнопкой,
// без drag-and-drop (ТЗ 6.2).
function ProductPicker({ fitting }: { fitting: Fitting }) {
  const t = useTranslations("tryon");
  const locale = useLocale();
  const [cats, setCats] = useState<Category[]>([]);
  const [tab, setTab] = useState<Tab>("top");
  const zone = tabZone(tab);
  const setChosen = hasSet(fitting);
  // выбрана двойка — вкладка «Низ» скрыта
  useEffect(() => {
    if (setChosen && tab === "bottom") setTab("set");
  }, [setChosen, tab]);
  const [q, setQ] = useState("");
  const [products, setProducts] = useState<Product[] | null>(null);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    api<Category[]>("/v1/categories").then(setCats).catch(setError);
  }, []);

  const load = useCallback(async () => {
    const zoneCats = cats.filter((c) => c.zone === zone);
    if (zoneCats.length === 0) return;
    setError(null);
    setProducts(null);
    try {
      const lists = await Promise.all(
        zoneCats.map((c) =>
          apiList<Product>(`/v1/products?limit=12&category=${c.id}${zone === "top" ? `&set=${tab === "set"}` : ""}${q ? `&q=${encodeURIComponent(q)}` : ""}`),
        ),
      );
      setProducts(lists.flatMap((l) => l.items));
    } catch (e) {
      setError(e);
    }
  }, [cats, zone, tab, q]);

  useEffect(() => {
    const id = setTimeout(load, 250);
    return () => clearTimeout(id);
  }, [load]);

  const add = (p: Product) => {
    const item: FittingItem = { id: p.id, title: p.title, brand: p.brand, price: p.price, image_url: p.thumbs?.front ?? p.images?.front ?? "", zone, store_name: p.store_name, is_set: tab === "set" };
    addToFitting(item);
  };

  return (
    <section className="card stack" aria-label={t("pickTitle")}>
      <div className="row wrap" role="tablist">
        {TABS.filter((x) => !(setChosen && x === "bottom")).map((x) => (
          <button key={x} type="button" role="tab" aria-selected={tab === x} className={`btn ${tab === x ? "btn-primary" : "btn-secondary"}`} onClick={() => setTab(x)}>
            {x === "set" ? t("setsTab") : t(`zone.${x}`)}
          </button>
        ))}
      </div>
      {tab === "set" && <p className="small muted">{t("setsHint")}</p>}
      <input type="search" placeholder={t("search")} value={q} onChange={(e) => setQ(e.target.value)} aria-label={t("search")} />
      <ErrorBox error={error} onRetry={load} />
      {!products && !error && <p className="muted">{t("loading")}</p>}
      {products?.length === 0 && <p className="muted">{t("noProducts")}</p>}
      <ul className="plain product-grid">
        {products?.map((p) => {
          const chosen = fitting[zone]?.id === p.id;
          return (
            <li key={p.id} className="product-tile">
              {(p.thumbs?.front ?? p.images?.front) && <img src={p.thumbs?.front ?? p.images.front} alt="" loading="lazy" />}
              <div className="small clamp">{p.title}</div>
              <div className="small muted">{p.brand} · {formatSum(p.price, locale)}</div>
              <button type="button" className="btn btn-secondary small" disabled={chosen} onClick={() => add(p)}>
                {chosen ? t("added") : fitting[zone] ? t("replace") : t("add")}
              </button>
            </li>
          );
        })}
      </ul>
    </section>
  );
}
