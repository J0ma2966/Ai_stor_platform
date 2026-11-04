"use client";
import { useState } from "react";
import { useTranslations } from "next-intl";
import { Link } from "@/i18n/navigation";
import { ErrorBox } from "@/components/ErrorBox";
import { bulkImages, downloadTemplate, importExcel, parseImageName, uploadImage, type BulkResult, type ImportResult } from "@/lib/storeCab";

const MAX_FILES = 300;

export default function ImportPage() {
  const t = useTranslations("import");
  const [res, setRes] = useState<ImportResult | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null);
  const [photoRes, setPhotoRes] = useState<BulkResult[] | null>(null);
  const [photoErr, setPhotoErr] = useState<unknown>(null);

  async function onExcel(f?: File) {
    if (!f) return;
    setBusy(true);
    setError(null);
    setRes(null);
    try {
      setRes(await importExcel(f));
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }

  // Загружаем фото по одному (сервер принимает одну загрузку на пользователя за раз),
  // затем одним запросом привязываем ключи к товарам по артикулу в имени файла.
  async function onPhotos(list: FileList | null) {
    if (!list?.length) return;
    const files = Array.from(list).slice(0, MAX_FILES);
    setPhotoErr(null);
    setPhotoRes(null);
    setProgress({ done: 0, total: files.length });
    const uploaded: { filename: string; key: string }[] = [];
    const failed: BulkResult[] = [];
    for (const [i, f] of files.entries()) {
      if (!parseImageName(f.name)) {
        failed.push({ filename: f.name, error: "bad_name" });
      } else {
        try {
          uploaded.push({ filename: f.name, key: (await uploadImage(f, "product")).key });
        } catch {
          failed.push({ filename: f.name, error: "upload" });
        }
      }
      setProgress({ done: i + 1, total: files.length });
    }
    try {
      const r = uploaded.length ? await bulkImages(uploaded) : [];
      setPhotoRes([...r, ...failed]);
    } catch (e) {
      setPhotoErr(e);
    }
  }

  const okCount = photoRes?.filter((r) => !r.error).length ?? 0;
  return (
    <>
      <h1>{t("title")}</h1>
      <section className="card stack-sm">
        <h2>{t("excelTitle")}</h2>
        <p>{t("excelText")}</p>
        <p className="muted small">{t("columns")}</p>
        <div className="row wrap">
          <button type="button" className="btn" onClick={() => downloadTemplate().catch(setError)}>{t("template")}</button>
          <input id="xlsx" type="file" className="sr-only" accept=".xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" onChange={(e) => { onExcel(e.target.files?.[0]); e.target.value = ""; }} />
          <label htmlFor="xlsx" className="btn btn-primary" aria-disabled={busy}>{busy ? t("uploading") : t("chooseFile")}</label>
        </div>
        <ErrorBox error={error} />
        {res && (
          <div role="status" className="stack-sm">
            <p className={res.failed ? "" : "ok-text"}>{t("result", { total: res.total, created: res.created, updated: res.updated, failed: res.failed })}</p>
            {res.generated && res.generated.length > 0 && (
              <details>
                <summary>{t("generatedTitle")} ({res.generated.length})</summary>
                <ul className="small">
                  {res.generated.slice(0, 500).map((g) => <li key={g.row}>{t("row", { row: g.row })}: <code>{g.sku}</code></li>)}
                </ul>
              </details>
            )}
            {res.errors.length > 0 && (
              <details open>
                <summary>{t("errorsTitle")} ({res.errors.length})</summary>
                <ul className="small">
                  {res.errors.slice(0, 200).map((e, i) => (
                    <li key={i}>{t("row", { row: e.row })}: {e.field && <code>{e.field}</code>} — {t.has(`codes.${e.code}`) ? t(`codes.${e.code}`) : e.code}{e.message ? ` (${e.message})` : ""}</li>
                  ))}
                </ul>
              </details>
            )}
          </div>
        )}
      </section>
      <section className="card stack-sm">
        <h2>{t("photosTitle")}</h2>
        <p>{t("photosText")}</p>
        <div className="row">
          <input id="photos" type="file" multiple className="sr-only" accept="image/jpeg,image/png,image/webp" onChange={(e) => { onPhotos(e.target.files); e.target.value = ""; }} />
          <label htmlFor="photos" className="btn btn-primary">{t("choosePhotos")}</label>
          {progress && <span role="status">{t("progress", progress)}</span>}
        </div>
        <ErrorBox error={photoErr} />
        {photoRes && (
          <div className="stack-sm">
            <p className={photoRes.length === okCount ? "ok-text" : ""}>{t("photosResult", { ok: okCount, bad: photoRes.length - okCount })}</p>
            {photoRes.some((r) => r.error) && (
              <ul className="small">
                {photoRes.filter((r) => r.error).map((r, i) => <li key={i}><code>{r.filename}</code> — {t(`imgErr.${r.error}`)}</li>)}
              </ul>
            )}
            <p className="muted small">{t("publishHint")} <Link href="/store/products">→</Link></p>
          </div>
        )}
      </section>
    </>
  );
}
