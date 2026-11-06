"use client";
import { Suspense, useCallback, useEffect, useId, useState } from "react";
import { useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { ErrorBox } from "@/components/ErrorBox";
import { PromotionCard } from "@/components/PromotionCard";
import type { Product } from "@/lib/catalog";
import { listMyProducts } from "@/lib/storeCab";
import { cancelPromotion, DAY_PRESETS, listStorePromotions, MAX_NOTE, PROMO_STATES, requestPromotion, type Promotion } from "@/lib/promotions";

// Кабинет магазина: заявки на рекламу своих товаров. Решение принимает суперадмин;
// одобренный товар показывается в карусели «Реклама» на главной (сайт и приложение).
export default function StorePromotionsPage() {
  return (
    <Suspense>
      <StorePromotions />
    </Suspense>
  );
}

function StorePromotions() {
  const t = useTranslations("storePromo");
  const tp = useTranslations("promo");
  const ta = useTranslations("app");
  const [state, setState] = useState("");
  const [items, setItems] = useState<Promotion[] | null>(null);
  const [cursor, setCursor] = useState("");
  const [error, setError] = useState<unknown>(null);

  const load = useCallback(async (from = "") => {
    setError(null);
    try {
      const r = await listStorePromotions({ state, cursor: from });
      setItems((p) => (from ? [...(p ?? []), ...r.items] : r.items));
      setCursor(r.nextCursor);
    } catch (e) {
      setError(e);
    }
  }, [state]);
  useEffect(() => void load(), [load]);

  async function cancel(id: string) {
    setError(null);
    try {
      await cancelPromotion(id);
      setItems((p) => p?.filter((x) => x.id !== id) ?? p);
    } catch (e) {
      setError(e);
    }
  }

  return (
    <>
      <h1>{t("title")}</h1>
      <p className="muted">{t("hint")}</p>
      <RequestForm onCreated={(p) => setItems((prev) => [p, ...(prev ?? [])])} />

      <h2>{t("listTitle")}</h2>
      <div className="tabs" role="tablist">
        {["", ...PROMO_STATES].map((s) => (
          <button key={s} role="tab" aria-selected={state === s} className={state === s ? "tab tab-on" : "tab"} onClick={() => setState(s)}>
            {s ? tp(`st_${s}`) : tp("allStates")}
          </button>
        ))}
      </div>
      <ErrorBox error={error} onRetry={() => load()} />
      {items === null ? (
        !error && <p className="muted">{ta("loading")}</p>
      ) : items.length === 0 ? (
        <p className="empty">{t("empty")}</p>
      ) : (
        <ul className="plain stack">
          {items.map((p) => (
            <li key={p.id}>
              <PromotionCard p={p} extra={p.state === "pending" && (
                <div><button type="button" className="btn btn-secondary" onClick={() => cancel(p.id)}>{t("cancel")}</button></div>
              )} />
            </li>
          ))}
        </ul>
      )}
      {cursor && <div className="center"><button type="button" className="btn" onClick={() => load(cursor)}>{ta("more")}</button></div>}
    </>
  );
}

function RequestForm({ onCreated }: { onCreated: (p: Promotion) => void }) {
  const t = useTranslations("storePromo");
  const tp = useTranslations("promo");
  const id = useId();
  const preset = useSearchParams().get("product") ?? "";
  const [products, setProducts] = useState<Product[] | null>(null);
  const [product, setProduct] = useState(preset);
  const [days, setDays] = useState<number>(7);
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    listMyProducts()
      .then((r) => setProducts(r.items.filter((p) => p.status === "published")))
      .catch(setError);
  }, []);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (!product) return;
    setBusy(true);
    setError(null);
    setDone(false);
    try {
      onCreated(await requestPromotion(product, days, note.trim()));
      setDone(true);
      setNote("");
      setProduct("");
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="card stack" onSubmit={submit} aria-labelledby={`${id}-h`}>
      <h2 id={`${id}-h`}>{t("newTitle")}</h2>
      {products !== null && products.length === 0 ? (
        <p className="muted">{t("noPublished")}</p>
      ) : (
        <>
          <div className="field">
            <label htmlFor={`${id}-p`}>{t("product")}</label>
            <select id={`${id}-p`} value={product} onChange={(e) => setProduct(e.target.value)} required disabled={products === null}>
              <option value="">{t("productPh")}</option>
              {(products ?? []).map((p) => <option key={p.id} value={p.id}>{p.title}{p.sku ? ` · ${p.sku}` : ""}</option>)}
            </select>
          </div>
          <fieldset className="field" style={{ border: 0, padding: 0, margin: 0 }}>
            <legend>{t("days")}</legend>
            <div className="tabs" role="radiogroup" aria-label={t("days")}>
              {DAY_PRESETS.map((d) => (
                <button key={d} type="button" role="radio" aria-checked={days === d} className={days === d ? "tab tab-on" : "tab"} onClick={() => setDays(d)}>
                  {tp("daysN", { n: d })}
                </button>
              ))}
            </div>
          </fieldset>
          <div className="field">
            <label htmlFor={`${id}-n`}>{t("note")}</label>
            <textarea id={`${id}-n`} rows={2} maxLength={MAX_NOTE} placeholder={t("notePh")} value={note} onChange={(e) => setNote(e.target.value)} />
            <span className="muted small">{t("payNote")}</span>
          </div>
          <ErrorBox error={error} />
          {done && <p className="small" role="status">{t("sent")}</p>}
          <div><button className="btn btn-primary" disabled={busy || !product}>{t("send")}</button></div>
        </>
      )}
    </form>
  );
}
