"use client";
import { useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { uploadImage } from "@/lib/storeCab";
import { useErrorText } from "./ErrorBox";

// Одна зона загрузки: предпросмотр, замена. Файл сразу уходит на сервер (перекодирование, без EXIF).
export function ImagePicker({ label, url, purpose, onUploaded, onClear, clearLabel }: {
  label: string; url?: string; purpose: "product" | "store"; onUploaded: (key: string, url: string) => void; onClear?: () => void; clearLabel?: string;
}) {
  const t = useTranslations("productForm");
  const errText = useErrorText();
  const input = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  async function pick(f: File | undefined) {
    if (!f) return;
    setBusy(true);
    setErr("");
    try {
      const r = await uploadImage(f, purpose);
      onUploaded(r.key, r.url);
    } catch (e) {
      setErr(errText(e));
    } finally {
      setBusy(false);
      if (input.current) input.current.value = "";
    }
  }
  return (
    <div className="photo-slot">
      <span className="label">{label}</span>
      {url ? <img src={url} alt={label} /> : <div className="img-ph" aria-hidden="true" />}
      <input ref={input} type="file" accept="image/jpeg,image/png,image/webp" className="sr-only" id={`pick-${label}`} onChange={(e) => pick(e.target.files?.[0])} />
      <label htmlFor={`pick-${label}`} className="btn btn-secondary" aria-disabled={busy}>{busy ? t("uploading") : url ? t("replace") : t("upload")}</label>
      {url && onClear && <button type="button" className="btn-link small" onClick={onClear}>{clearLabel}</button>}
      {err && <span className="field-error" role="alert">{err}</span>}
    </div>
  );
}
