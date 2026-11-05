"use client";
import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { useLocale, useTranslations } from "next-intl";
import { ApiError, api } from "@/lib/api";
import { formatSum } from "@/lib/money";
import {
  checkout, newIdempotencyKey, uploadDeliveryPhoto,
  itemKey, type CheckoutInput, type CheckoutItem, type CheckoutResult,
} from "@/lib/orders";
import { ErrorBox, useErrorText, useFieldError } from "./ErrorBox";
import { Link } from "@/i18n/navigation";

type Product = { id: string; title: string; brand: string; price: number; in_stock: boolean; store_name: string; images: Record<string, string> };

// Лимиты совпадают с сервером (docs/ORDERS.md).
const LIMITS = {
  customer_name: 120, customer_phone: 16, street: 200, house: 30, entrance: 20, floor: 10,
  apartment: 20, intercom: 30, address_comment: 1000, customer_comment: 1000, delivery_time_comment: 500,
} as const;

type Fields = Record<keyof typeof LIMITS, string>;
const empty: Fields = {
  customer_name: "", customer_phone: "", street: "", house: "", entrance: "", floor: "",
  apartment: "", intercom: "", address_comment: "", customer_comment: "", delivery_time_comment: "",
};

export function CheckoutForm({ items }: { items: CheckoutItem[] }) {
  const t = useTranslations("checkout");
  const tf = useTranslations("fieldErrors");
  const locale = useLocale();
  const errText = useErrorText();
  const fieldErr = useFieldError();

  const [f, setF] = useState<Fields>(empty);
  const [loc, setLoc] = useState<{ lat: number; lng: number } | null>(null);
  const [locError, setLocError] = useState(false);
  const [photo, setPhoto] = useState<{ key: string; preview: string } | null>(null);
  const [photoBusy, setPhotoBusy] = useState(false);
  const [photoErr, setPhotoErr] = useState<unknown>(null);
  const [products, setProducts] = useState<Record<string, Product>>({});
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<CheckoutResult | null>(null);
  // Ключ идемпотентности живёт столько же, сколько форма: повторная отправка не создаст дубль.
  const idemKey = useRef<string>("");
  if (!idemKey.current) idemKey.current = newIdempotencyKey();

  useEffect(() => {
    let alive = true;
    Promise.all(items.map((it) => api<Product>(`/v1/products/${it.product_id}`).catch(() => null))).then((list) => {
      if (!alive) return;
      const m: Record<string, Product> = {};
      list.forEach((p) => p && (m[p.id] = p));
      setProducts(m);
    });
    return () => {
      alive = false;
    };
  }, [items]);

  const total = useMemo(
    () => items.reduce((s, it) => s + (products[it.product_id]?.price ?? 0) * it.qty, 0),
    [items, products],
  );

  const set = (k: keyof Fields) => (e: { target: { value: string } }) => {
    setF((prev) => ({ ...prev, [k]: e.target.value }));
    // ошибка поля уходит, как только его начали исправлять
    setErrors((prev) => {
      if (!(k in prev) && !(`delivery.${k}` in prev)) return prev;
      const next = { ...prev };
      delete next[k];
      delete next[`delivery.${k}`];
      return next;
    });
    // при изменении данных — новый ключ, иначе сервер вернёт idempotency_key_reused
    idemKey.current = newIdempotencyKey();
  };

  function locate() {
    setLocError(false);
    if (!("geolocation" in navigator)) return setLocError(true);
    navigator.geolocation.getCurrentPosition(
      (p) => {
        setLoc({ lat: Number(p.coords.latitude.toFixed(6)), lng: Number(p.coords.longitude.toFixed(6)) });
        idemKey.current = newIdempotencyKey();
      },
      () => setLocError(true),
      { enableHighAccuracy: true, timeout: 15000 },
    );
  }

  async function onPhoto(file: File | undefined) {
    if (!file) return;
    setPhotoErr(null);
    if (!["image/jpeg", "image/png", "image/webp"].includes(file.type)) {
      setPhotoErr(new ApiError(415, "unsupported_type", ""));
      return;
    }
    if (file.size > 10 << 20) {
      setPhotoErr(new ApiError(413, "file_too_large", ""));
      return;
    }
    setPhotoBusy(true);
    try {
      const key = await uploadDeliveryPhoto(file);
      if (photo) URL.revokeObjectURL(photo.preview);
      setPhoto({ key, preview: URL.createObjectURL(file) });
      idemKey.current = newIdempotencyKey();
    } catch (e) {
      setPhotoErr(e);
    } finally {
      setPhotoBusy(false);
    }
  }

  function validate(): Record<string, string> {
    const e: Record<string, string> = {};
    if (!f.customer_name.trim()) e.customer_name = "required";
    if (!f.street.trim()) e["delivery.street"] = "required";
    if (!f.house.trim()) e["delivery.house"] = "required";
    if (f.customer_phone.trim() && !/^\+[1-9][0-9]{7,14}$/.test(f.customer_phone.trim())) e.customer_phone = "e164";
    return e;
  }

  async function submit(ev: FormEvent) {
    ev.preventDefault();
    if (busy) return;
    const local = validate();
    setErrors(local);
    setError(null);
    if (Object.keys(local).length) return;
    const input: CheckoutInput = {
      items,
      customer_name: f.customer_name,
      customer_phone: f.customer_phone.trim() || undefined,
      customer_comment: f.customer_comment,
      delivery_time_comment: f.delivery_time_comment,
      delivery: {
        street: f.street, house: f.house, entrance: f.entrance, floor: f.floor, apartment: f.apartment,
        intercom: f.intercom, address_comment: f.address_comment,
        ...(loc ? { lat: loc.lat, lng: loc.lng } : {}),
        ...(photo ? { photo_key: photo.key } : {}),
      },
    };
    setBusy(true);
    try {
      setResult(await checkout(input, idemKey.current));
    } catch (e) {
      if (e instanceof ApiError && e.code === "validation_failed") setErrors(e.fields);
      setError(e);
    } finally {
      setBusy(false);
    }
  }

  if (!items.length) return <p className="muted">{t("empty")}</p>;

  if (result) {
    return (
      <section className="card" aria-live="polite">
        <h2>{t("successTitle")}</h2>
        {result.orders.length > 1 && <p>{t("successSplit")}</p>}
        <ul className="plain">
          {result.orders.map((o) => (
            <li key={o.id}>
              <Link href={`/account/orders/${o.id}`}>
                {t("orderLine", { number: o.number, store: o.store_name, total: formatSum(o.items_total, locale) })}
              </Link>
            </li>
          ))}
        </ul>
        <Link className="btn" href="/account/orders">{t("toOrders")}</Link>
      </section>
    );
  }

  const unavailable = error instanceof ApiError && error.code === "products_unavailable" ? error.fields : {};
  const input = (k: keyof Fields, label: string, opts: { errKey?: string; required?: boolean; placeholder?: string; inputMode?: "tel" | "numeric" | "text"; area?: boolean } = {}) => {
    const errKey = opts.errKey ?? k;
    const err = fieldErr(errors[errKey]);
    const id = `f-${k}`;
    const common = {
      id, name: k, value: f[k], onChange: set(k), maxLength: LIMITS[k], placeholder: opts.placeholder,
      "aria-invalid": err ? true : undefined, "aria-describedby": err ? `${id}-err` : undefined, required: opts.required,
    };
    return (
      <div className="field">
        <label htmlFor={id}>{label}{opts.required && " *"}</label>
        {opts.area ? <textarea rows={3} {...common} /> : <input type="text" inputMode={opts.inputMode} {...common} />}
        {err && <span id={`${id}-err`} className="field-error">{err}</span>}
      </div>
    );
  };

  return (
    <form onSubmit={submit} noValidate className="stack">
      <section className="card">
        <ul className="plain items">
          {items.map((it) => {
            const p = products[it.product_id];
            const bad = unavailable[itemKey(it)] ?? unavailable[it.product_id];
            return (
              <li key={itemKey(it)} className={bad ? "item item-bad" : "item"}>
                {p?.images?.front && <img src={p.images.front} alt="" width={48} height={64} />}
                <span className="grow">
                  {p ? `${p.brand} · ${p.title}` : it.product_id}
                  {it.size && <span> · {t("size", { size: it.size })}</span>}
                  {p && <span className="muted small"> · {p.store_name}</span>}
                  {bad && <span className="field-error"> {tf(bad)}</span>}
                </span>
                <span>×{it.qty}</span>
                {p && <span>{formatSum(p.price * it.qty, locale)}</span>}
              </li>
            );
          })}
        </ul>
        <p className="total">{formatSum(total, locale)}</p>
      </section>

      <fieldset className="card">
        <legend>{t("contact")}</legend>
        {input("customer_name", t("name"), { required: true })}
        {input("customer_phone", t("phone"), { inputMode: "tel", placeholder: "+998901234567" })}
        <p className="muted small">{t("phoneHint")}</p>
      </fieldset>

      <fieldset className="card">
        <legend>{t("address")}</legend>
        <div className="grid-2">
          {input("street", t("street"), { required: true, errKey: "delivery.street" })}
          {input("house", t("house"), { required: true, errKey: "delivery.house" })}
        </div>
        <div className="grid-4">
          {input("entrance", t("entrance"), { errKey: "delivery.entrance" })}
          {input("floor", t("floor"), { errKey: "delivery.floor" })}
          {input("apartment", t("apartment"), { errKey: "delivery.apartment" })}
          {input("intercom", t("intercom"), { errKey: "delivery.intercom" })}
        </div>
        {input("address_comment", t("addressComment"), { area: true, errKey: "delivery.address_comment" })}

        <div className="field">
          <span className="label">{t("location")}</span>
          {loc ? (
            <p>
              {t("locationSet", { lat: loc.lat, lng: loc.lng })}{" "}
              <button type="button" className="btn-link" onClick={() => setLoc(null)}>{t("locationClear")}</button>
            </p>
          ) : (
            <button type="button" className="btn btn-secondary" onClick={locate}>{t("useLocation")}</button>
          )}
          {locError && <span className="field-error">{t("locationDenied")}</span>}
          {errors["delivery.location"] && <span className="field-error">{fieldErr(errors["delivery.location"])}</span>}
        </div>

        <div className="field">
          <label htmlFor="f-photo">{t("photo")}</label>
          <p className="muted small">{t("photoHint")}</p>
          {photo ? (
            <div className="photo-preview">
              <img src={photo.preview} alt={t("photo")} />
              <button type="button" className="btn-link" onClick={() => { URL.revokeObjectURL(photo.preview); setPhoto(null); }}>
                {t("photoRemove")}
              </button>
            </div>
          ) : (
            <input id="f-photo" type="file" accept="image/jpeg,image/png,image/webp" capture="environment"
              disabled={photoBusy} onChange={(e) => onPhoto(e.target.files?.[0])} />
          )}
          {photoBusy && <span className="muted small">{t("photoUploading")}</span>}
          {photoErr ? <span className="field-error">{errText(photoErr)}</span> : null}
        </div>
      </fieldset>

      <fieldset className="card">
        <legend>{t("comments")}</legend>
        {input("customer_comment", t("orderComment"), { area: true })}
        {input("delivery_time_comment", t("timeComment"), { area: true, placeholder: t("timeCommentPh") })}
      </fieldset>

      <ErrorBox error={error} />
      <button type="submit" className="btn btn-primary" disabled={busy || photoBusy}>
        {busy ? t("submitting") : t("submit")}
      </button>
    </form>
  );
}
