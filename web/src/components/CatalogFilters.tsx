"use client";
import { useState, type FormEvent } from "react";
import { useLocale, useTranslations } from "next-intl";
import { useRouter } from "@/i18n/navigation";
import { catName, SORTS, type CatalogFilters as F, type Category, type Store } from "@/lib/catalog";

// Фильтры каталога живут в URL: ссылкой можно поделиться, «назад» работает.
export function CatalogFilters({ filters, categories, stores, fixed = {} }: { filters: F; categories: Category[]; stores: Pick<Store, "id" | "name">[]; fixed?: Partial<F> }) {
  const t = useTranslations("catalog");
  const locale = useLocale();
  const router = useRouter();
  const [f, setF] = useState<F>(filters);
  const [open, setOpen] = useState(false);
  const set = (k: keyof F) => (e: { target: { value: string } }) => setF((p) => ({ ...p, [k]: e.target.value }));

  function push(next: F) {
    const p = new URLSearchParams();
    for (const [k, v] of Object.entries({ ...next, ...fixed })) if (v) p.set(k, String(v));
    const s = p.toString();
    router.push(`/catalog${s ? `?${s}` : ""}`);
  }
  function apply(e: FormEvent) {
    e.preventDefault();
    push(f);
    setOpen(false);
  }
  return (
    <div className="filters">
      <div className="row between filters-top">
        <button type="button" className="btn btn-secondary only-mobile" aria-expanded={open} aria-controls="catalog-filters" onClick={() => setOpen((v) => !v)}>
          {open ? t("hideFilters") : t("showFilters")}
        </button>
        <label className="row sort">
          <span className="small muted">{t("sort")}</span>
          <select value={f.sort || "popular"} onChange={(e) => { const n = { ...f, sort: e.target.value }; setF(n); push(n); }}>
            {SORTS.map((s) => <option key={s} value={s}>{t(`sort_${s}`)}</option>)}
          </select>
        </label>
      </div>
      <form id="catalog-filters" className={`card stack filters-form ${open ? "open" : ""}`} onSubmit={apply} aria-label={t("filters")}>
        {!fixed.category && (
          <div className="field">
            <label htmlFor="f-cat">{t("category")}</label>
            <select id="f-cat" value={f.category ?? ""} onChange={set("category")}>
              <option value="">{t("all")}</option>
              {categories.map((c) => <option key={c.id} value={c.id}>{catName(c, locale)}</option>)}
            </select>
          </div>
        )}
        {!fixed.store && (
          <div className="field">
            <label htmlFor="f-store">{t("store")}</label>
            <select id="f-store" value={f.store ?? ""} onChange={set("store")}>
              <option value="">{t("all")}</option>
              {stores.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
            </select>
          </div>
        )}
        <div className="field">
          <label htmlFor="f-brand">{t("brand")}</label>
          <input id="f-brand" type="text" maxLength={120} value={f.brand ?? ""} onChange={set("brand")} />
        </div>
        <fieldset className="field plain-fieldset">
          <legend className="label">{t("price")}</legend>
          <div className="row">
            <input aria-label={`${t("price")} ${t("from")}`} placeholder={t("from")} inputMode="numeric" pattern="\d*" value={f.price_min ?? ""} onChange={set("price_min")} />
            <input aria-label={`${t("price")} ${t("to")}`} placeholder={t("to")} inputMode="numeric" pattern="\d*" value={f.price_max ?? ""} onChange={set("price_max")} />
          </div>
        </fieldset>
        <label className="row check">
          <input type="checkbox" checked={f.in_stock === "true"} onChange={(e) => setF((p) => ({ ...p, in_stock: e.target.checked ? "true" : "" }))} />
          <span>{t("inStock")}</span>
        </label>
        <div className="row">
          <button className="btn btn-primary grow">{t("apply")}</button>
          <button type="button" className="btn btn-secondary" onClick={() => { const n = { q: f.q, sort: f.sort }; setF(n); push(n); setOpen(false); }}>{t("reset")}</button>
        </div>
      </form>
    </div>
  );
}
