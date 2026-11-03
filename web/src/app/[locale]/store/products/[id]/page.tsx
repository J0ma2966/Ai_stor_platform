"use client";
import { use, useCallback, useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { Link, useRouter } from "@/i18n/navigation";
import { ErrorBox } from "@/components/ErrorBox";
import { ProductForm } from "@/components/ProductForm";
import { ProductStatusBadge } from "@/components/ProductStatusBadge";
import type { Product } from "@/lib/catalog";
import { deleteProduct, getMyProduct, setProductStatus } from "@/lib/storeCab";

export default function EditProduct({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const t = useTranslations("storeProducts");
  const tf = useTranslations("productForm");
  const ta = useTranslations("app");
  const router = useRouter();
  const created = useSearchParams().get("created") === "1";
  const [p, setP] = useState<Product | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [actErr, setActErr] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);

  const load = useCallback(() => {
    setError(null);
    getMyProduct(id).then(setP).catch(setError);
  }, [id]);
  useEffect(load, [load]);

  async function status(s: "published" | "draft") {
    setBusy(true);
    setActErr(null);
    try {
      await setProductStatus(id, s);
      load();
    } catch (e) {
      setActErr(e);
    } finally {
      setBusy(false);
    }
  }
  async function del() {
    if (!p || !window.confirm(t("confirmDelete", { title: p.title }))) return;
    setBusy(true);
    try {
      await deleteProduct(id);
      router.replace("/store/products");
    } catch (e) {
      setActErr(e);
      setBusy(false);
    }
  }

  if (error) return <ErrorBox error={error} onRetry={load} />;
  if (!p) return <p className="muted">{ta("loading")}</p>;
  return (
    <>
      <div className="row between wrap">
        <h1>{tf("editTitle")}</h1>
        <ProductStatusBadge status={p.status ?? "draft"} />
      </div>
      {created && <p className="ok-text" role="status">{tf("created")}</p>}
      {p.status === "unpublished" && (
        <div className="alert" role="alert"><p>{p.moderation_reason && t("reason", { reason: p.moderation_reason })}</p><p className="muted small">{t("fixHint")}</p></div>
      )}
      {p.status === "blocked" && (
        <div className="alert" role="alert"><p>{p.moderation_reason && t("reason", { reason: p.moderation_reason })}</p><p className="muted small">{t("blockedHint")}</p></div>
      )}
      <div className="row wrap">
        {p.status === "draft" && <button type="button" className="btn btn-primary" disabled={busy} onClick={() => status("published")}>{t("publish")}</button>}
        {p.status === "published" && <>
          <button type="button" className="btn" disabled={busy} onClick={() => status("draft")}>{t("unpublish")}</button>
          <Link href={`/product/${p.id}`} className="btn btn-secondary">↗</Link>
        </>}
        {p.status !== "blocked" && <button type="button" className="btn btn-danger" disabled={busy} onClick={del}>{ta("delete")}</button>}
      </div>
      <ErrorBox error={actErr} />
      <ProductForm key={p.id} product={p} onSaved={load} />
    </>
  );
}
