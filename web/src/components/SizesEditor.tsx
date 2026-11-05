"use client";
import { useTranslations } from "next-intl";

export type SizeRow = { size: string; stock: string };

const PRESETS: Record<string, string[]> = {
  presetLetters: ["XS", "S", "M", "L", "XL", "XXL"],
  presetRu: ["42", "44", "46", "48", "50", "52", "54"],
  presetShoes: ["36", "37", "38", "39", "40", "41", "42", "43", "44", "45"],
};

// Размеры и остатки товара в кабинете магазина. Пустой список — товар без размеров.
export function SizesEditor({ rows, onChange, errors }: { rows: SizeRow[]; onChange: (r: SizeRow[]) => void; errors: Record<string, string> }) {
  const t = useTranslations("productForm");
  const tf = useTranslations("fieldErrors");
  const total = rows.reduce((s, r) => s + (Number.parseInt(r.stock, 10) || 0), 0);
  const upd = (i: number, k: keyof SizeRow, v: string) => onChange(rows.map((r, j) => (j === i ? { ...r, [k]: v } : r)));
  const addPreset = (list: string[]) => {
    const have = new Set(rows.map((r) => r.size.trim().toUpperCase()));
    onChange([...rows, ...list.filter((s) => !have.has(s)).map((size) => ({ size, stock: "0" }))]);
  };
  const err = (i: number, k: string) => errors[`sizes[${i}].${k}`];
  return (
    <div className="stack-sm">
      <p className="muted small">{t("sizesHint")}</p>
      <div className="row wrap">
        {Object.entries(PRESETS).map(([k, list]) => (
          <button key={k} type="button" className="btn btn-sm btn-secondary" onClick={() => addPreset(list)}>+ {t(k)}</button>
        ))}
      </div>
      {rows.length === 0 && <p className="muted small">{t("noSizes")}</p>}
      <div className="size-rows">
        {rows.map((r, i) => (
          <div key={i} className="size-row">
            <div className="field">
              <label htmlFor={`sz-${i}`} className="small">{t("sizeName")}</label>
              <input id={`sz-${i}`} maxLength={20} required value={r.size} onChange={(e) => upd(i, "size", e.target.value)} aria-invalid={!!err(i, "size")} />
              {err(i, "size") && <span className="field-error">{tf(err(i, "size")!)}</span>}
            </div>
            <div className="field">
              <label htmlFor={`st-${i}`} className="small">{t("stock")}</label>
              <input id={`st-${i}`} type="number" inputMode="numeric" min={0} max={100000} required value={r.stock}
                onChange={(e) => upd(i, "stock", e.target.value)} aria-invalid={!!err(i, "stock")} />
              {err(i, "stock") && <span className="field-error">{tf(err(i, "stock")!)}</span>}
            </div>
            <button type="button" className="btn btn-sm btn-danger" aria-label={t("removeSize", { size: r.size })}
              onClick={() => onChange(rows.filter((_, j) => j !== i))}>×</button>
          </div>
        ))}
      </div>
      <div className="row wrap between">
        <button type="button" className="btn btn-sm" onClick={() => onChange([...rows, { size: "", stock: "1" }])}>+ {t("addSize")}</button>
        {rows.length > 0 && <span className="small">{t("totalStock", { n: total })}</span>}
      </div>
      {errors.sizes && <span className="field-error">{tf(errors.sizes)}</span>}
    </div>
  );
}
