"use client";
import { useCallback, useEffect, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Link, useRouter } from "@/i18n/navigation";
import { currentRole, refreshSession } from "@/lib/api";
import { getCart, type CartItem } from "@/lib/shop";
import { formatSum } from "@/lib/money";
import { addToFitting, FITTING_EVENT, fittingItems, getFitting, removeFromFitting, visibleZones, ZONES, type Fitting, type Zone } from "@/lib/fitting";
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



// Экран примерочной: манекен, карточки у зон, выбор товаров, запуск генерации.
export function FittingRoom() {
  const t = useTranslations("tryon");
  const locale = useLocale();
  const router = useRouter();
  const fitting = useFitting();
  const items = fittingItems(fitting);
  const [quote, setQuote] = useState<Quote | null>(null);
  const [authed, setAuthed] = useState<boolean | null>(null); // null — ещё проверяем вход

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
                <li key={z} className={`zone-slot zone-${z}`}>
                  <span className="small muted">{t(`zone.${z}`)}</span>
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
      <ProductPicker fitting={fitting} authed={authed} />
    </div>
  );
}

// Выбор вещей — только из корзины покупателя (не весь каталог), по зонам манекена.
// Добавление — кнопкой, без drag-and-drop (ТЗ 6.2). Одна вещь корзины в разных
// размерах показывается один раз.
function ProductPicker({ fitting, authed }: { fitting: Fitting; authed: boolean | null }) {
  const t = useTranslations("tryon");
  const locale = useLocale();
  const [zone, setZone] = useState<Zone>("top");
  // Выбор подстраивается сам: если выбранный верх — комплект (верх и низ на одном
  // фото), низ уже подобран и вкладка «Низ» скрыта.
  const zones = visibleZones(fitting);
  useEffect(() => {
    if (!zones.includes(zone)) setZone("top");
  }, [zones, zone]);
  const [items, setItems] = useState<CartItem[] | null>(null);
  const [error, setError] = useState<unknown>(null);

  const load = useCallback(async () => {
    setError(null);
    try {
      const cart = await getCart();
      const seen = new Set<string>();
      setItems(cart.items.filter((i) => ZONES.includes(i.zone as Zone) && !seen.has(i.product_id) && !!seen.add(i.product_id)));
    } catch (e) {
      setError(e);
    }
  }, []);

  useEffect(() => {
    if (authed) void load();
  }, [authed, load]);

  const add = (c: CartItem) =>
    addToFitting({ id: c.product_id, title: c.title, brand: c.brand, price: c.price, image_url: c.image_url ?? "", zone: c.zone as Zone, store_name: c.store_name, is_set: c.is_set });

  const inZone = items?.filter((i) => i.zone === zone) ?? [];
  return (
    <section className="card stack" aria-label={t("pickTitle")}>
      <h2>{t("fromCart")}</h2>
      {authed === false && (
        <p className="muted">
          {t("cartLogin")} <Link href={`/login?next=${encodeURIComponent("/fitting")}`}>{t("login")}</Link>
        </p>
      )}
      {authed && (
        <>
          <div className="row wrap" role="tablist">
            {zones.map((z) => (
              <button key={z} type="button" role="tab" aria-selected={zone === z} className={`btn ${zone === z ? "btn-primary" : "btn-secondary"}`} onClick={() => setZone(z)}>
                {t(`zone.${z}`)}
                {items ? ` · ${items.filter((i) => i.zone === z).length}` : ""}
              </button>
            ))}
          </div>
          <ErrorBox error={error} onRetry={load} />
          {!items && !error && <p className="muted">{t("loading")}</p>}
          {items?.length === 0 && (
            <p className="muted">
              {t("cartEmpty")} <Link href="/catalog">{t("toCatalog")}</Link>
            </p>
          )}
          {!!items?.length && inZone.length === 0 && <p className="muted">{t("cartZoneEmpty")}</p>}
          <ul className="plain product-grid">
            {inZone.map((c) => {
              const chosen = fitting[zone]?.id === c.product_id;
              return (
                <li key={c.product_id} className="product-tile">
                  {c.image_url && <img src={c.image_url} alt="" loading="lazy" />}
                  <div className="small clamp">{c.title}</div>
                  <div className="small muted">{c.brand} · {formatSum(c.price, locale)}</div>
                  <button type="button" className="btn btn-secondary small" disabled={chosen} onClick={() => add(c)}>
                    {chosen ? t("added") : fitting[zone] ? t("replace") : t("add")}
                  </button>
                </li>
              );
            })}
          </ul>
        </>
      )}
    </section>
  );
}
