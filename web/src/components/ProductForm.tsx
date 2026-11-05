"use client";
import { useEffect, useState, type FormEvent } from "react";
import { useLocale, useTranslations } from "next-intl";
import { useRouter } from "@/i18n/navigation";
import { catName, fetchCategories, type Category, type Product } from "@/lib/catalog";
import { createProduct, setProductStatus, updateProduct } from "@/lib/storeCab";
import { ApiError } from "@/lib/api";
import { ErrorBox, useFieldError } from "./ErrorBox";
import { ImagePicker } from "./ImagePicker";
import { SizesEditor, type SizeRow } from "./SizesEditor";

const ANGLES = ["front", "side", "back"] as const;
type Angle = (typeof ANGLES)[number];

// Форма товара: обязательные поля ТЗ 5.3 и три ракурса фото.
export function ProductForm({ product, onSaved }: { product?: Product; onSaved?: () => void }) {
  const t = useTranslations("productForm");
  const locale = useLocale();
  const router = useRouter();
  const ta = useTranslations("app");
  const fe = useFieldError();
  const [cats, setCats] = useState<Category[]>([]);
  const [f, setF] = useState({
    title: product?.title ?? "", description: product?.description ?? "", category_id: product?.category_id ?? "",
    price: product ? String(Math.round(product.price / 100)) : "", brand: product?.brand ?? "", sku: product?.sku ?? "",
    in_stock: product?.in_stock ?? true,
  });
  const [sizes, setSizes] = useState<SizeRow[]>((product?.sizes ?? []).map((s) => ({ size: s.size, stock: String(s.stock ?? 0) })));
  const [keys, setKeys] = useState<Partial<Record<Angle, string>>>({});
  const [urls, setUrls] = useState<Partial<Record<Angle, string>>>(product?.images ?? {});
  const [publish, setPublish] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [ok, setOk] = useState("");
  useEffect(() => { fetchCategories().then(setCats).catch(() => {}); }, []);
  const fields = error instanceof ApiError ? error.fields : {};
  const set = (k: keyof typeof f) => (e: { target: { value: string } }) => setF((p) => ({ ...p, [k]: e.target.value }));

  async function submit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setOk("");
    const price = Math.round(Number(f.price.replace(/\s/g, "").replace(",", ".")) * 100);
    if (!product && ANGLES.some((a) => !keys[a])) {
      setError(new ApiError(422, "validation_failed", "", { images: "required" }));
      return;
    }
    setBusy(true);
    try {
      // Пустой артикул у нового товара — сервер выдаст его автоматически.
      const sku = f.sku.trim();
      const sizeList = sizes.map((s) => ({ size: s.size.trim(), stock: Math.max(0, Number.parseInt(s.stock, 10) || 0) }));
      const body = { ...f, price, sku, sizes: sizeList, ...(Object.keys(keys).length ? { images: keys } : {}) };
      if (product) {
        await updateProduct(product.id, body);
        setOk(t("saved"));
        setKeys({});
        onSaved?.();
      } else {
        const r = await createProduct({ ...body, images: keys });
        if (publish) await setProductStatus(r.id, "published").catch(() => {});
        router.replace(`/store/products/${r.id}?created=1`);
      }
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  }
  const field = (k: "title" | "brand" | "sku", max: number, hint?: string, optional = false, placeholder?: string) => (
    <div className="field">
      <label htmlFor={`pf-${k}`}>{t(k)}</label>
      <input id={`pf-${k}`} required={!optional} placeholder={placeholder} maxLength={max} value={f[k]} onChange={set(k)} aria-invalid={!!fields[k]} aria-describedby={hint ? `pf-${k}-h` : undefined} />
      {hint && <span id={`pf-${k}-h`} className="muted small">{hint}</span>}
      {fields[k] && <span className="field-error">{fe(fields[k])}</span>}
    </div>
  );
  const locked = product?.status === "blocked";
  return (
    <form className="stack" onSubmit={submit}>
      <fieldset className="card stack" disabled={locked || busy}>
        {field("title", 300)}
        <div className="field">
          <label htmlFor="pf-desc">{t("description")}</label>
          <textarea id="pf-desc" required rows={5} maxLength={10000} value={f.description} onChange={set("description")} aria-invalid={!!fields.description} />
        </div>
        <div className="grid-2">
          <div className="field">
            <label htmlFor="pf-cat">{t("category")}</label>
            <select id="pf-cat" required value={f.category_id} onChange={set("category_id")} aria-invalid={!!fields.category_id}>
              <option value="">{t("choose")}</option>
              {cats.map((c) => <option key={c.id} value={c.id}>{catName(c, locale)}</option>)}
            </select>
            {fields.category_id && <span className="field-error">{fe(fields.category_id)}</span>}
          </div>
          <div className="field">
            <label htmlFor="pf-price">{t("price")}</label>
            <input id="pf-price" required inputMode="decimal" pattern="[0-9 ]+([.,][0-9]{1,2})?" value={f.price} onChange={set("price")} aria-invalid={!!fields.price} />
            {fields.price && <span className="field-error">{fe(fields.price)}</span>}
          </div>
        </div>
        <div className="grid-2">
          {field("brand", 120)}
          {product ? field("sku", 64, t("skuHint")) : field("sku", 64, t("skuAutoHint"), true, t("skuAuto"))}
        </div>
        {sizes.length === 0 && (
          <label className="row check"><input type="checkbox" checked={f.in_stock} onChange={(e) => setF((p) => ({ ...p, in_stock: e.target.checked }))} /><span>{t("inStock")}</span></label>
        )}
      </fieldset>
      <fieldset className="card stack" disabled={locked || busy}>
        <legend>{t("sizes")}</legend>
        <SizesEditor rows={sizes} onChange={setSizes} errors={fields} />
      </fieldset>
      <fieldset className="card stack" disabled={locked || busy}>
        <legend>{t("photos")}</legend>
        <div className="photo-slots">
          {ANGLES.map((a) => (
            <ImagePicker key={a} label={t(a)} url={urls[a]} purpose="product" onUploaded={(k, u) => { setKeys((p) => ({ ...p, [a]: k })); setUrls((p) => ({ ...p, [a]: u })); }} />
          ))}
        </div>
        {fields.images && <span className="field-error" role="alert">{t("needPhotos")}</span>}
      </fieldset>
      <ErrorBox error={error && !(error instanceof ApiError && error.code === "validation_failed" && fields.images) ? error : null} />
      {ok && <p className="ok-text" role="status">{ok}</p>}
      {!locked && (
        <div className="row wrap">
          {!product && <label className="row check"><input type="checkbox" checked={publish} onChange={(e) => setPublish(e.target.checked)} /><span>{t("publishNow")}</span></label>}
          <button className="btn btn-primary btn-lg" disabled={busy}>{product ? ta("save") : t("create")}</button>
        </div>
      )}
    </form>
  );
}
