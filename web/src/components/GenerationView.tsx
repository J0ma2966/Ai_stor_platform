"use client";
import { useCallback, useEffect, useRef, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Link, useRouter } from "@/i18n/navigation";
import { newIdempotencyKey } from "@/lib/api";
import { formatDateTime, formatSum } from "@/lib/money";
import { addToFitting, clearFitting } from "@/lib/fitting";
import { ANGLES, createGeneration, getGeneration, isActive, setSaved, type Generation } from "@/lib/tryon";
import { ErrorBox } from "@/components/ErrorBox";
import { TryOnStatus } from "@/components/TryOnStatus";

const POLL_MS = 3000;

// Состояние генерации: ожидает → выполняется → результат или ошибка (ТЗ 6.5, 6.8).
export function GenerationView({ id }: { id: string }) {
  const t = useTranslations("tryon");
  const locale = useLocale();
  const router = useRouter();
  const [g, setG] = useState<Generation | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const retryKey = useRef(newIdempotencyKey());

  const load = useCallback(async () => {
    try {
      setG(await getGeneration(id));
      setError(null);
    } catch (e) {
      setError(e);
    }
  }, [id]);

  useEffect(() => {
    load();
  }, [load]);

  // опрос, пока генерация в очереди/в работе; интерфейс не блокируется
  useEffect(() => {
    if (!g || !isActive(g.status)) return;
    const timer = setTimeout(load, POLL_MS);
    return () => clearTimeout(timer);
  }, [g, load]);

  if (!g) return error ? <ErrorBox error={error} onRetry={load} /> : <p className="muted">{t("loading")}</p>;

  const toggleSave = async () => {
    setBusy(true);
    try {
      setG(await setSaved(g.id, !g.saved));
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  // «Изменить образ»: вернуть вещи в примерочную
  const edit = () => {
    clearFitting();
    g.products.forEach((p) => addToFitting({ id: p.id, title: p.title, brand: p.brand, price: p.price, image_url: p.image_url, zone: p.zone, store_name: p.store_name }));
    router.push("/fitting");
  };

  // «Повторить генерацию»: те же вещи и фото, новое списание по актуальной цене
  const repeat = async () => {
    if (g.photos.length !== 3) {
      edit();
      router.push("/fitting/new");
      return;
    }
    setBusy(true);
    try {
      const keys = Object.fromEntries(g.photos.map((p) => [p.angle, p.key])) as Record<"front" | "side" | "back", string>;
      const n = await createGeneration({ product_ids: g.products.map((p) => p.id), photos: keys, save_photos: false, body: g.body }, retryKey.current);
      router.push(`/fitting/${n.id}`);
    } catch (e) {
      setError(e);
      setBusy(false);
    }
  };

  return (
    <div className="stack">
      <div className="row between wrap">
        <TryOnStatus status={g.status} />
        <span className="small muted">{formatDateTime(g.created_at, locale)}</span>
      </div>

      {isActive(g.status) && (
        <div className="card stack" aria-live="polite">
          <div className="spinner" aria-hidden="true" />
          <p>{t(g.status === "queued" ? "queuedText" : "runningText")}</p>
          <p className="small muted">{t("canLeave")}</p>
        </div>
      )}

      {g.status === "failed" && (
        <div className="alert" role="alert">
          <p>{t.has(`genError.${g.error_code}`) ? t(`genError.${g.error_code}`) : t("genError.provider_error")}</p>
          {g.refunded && <p className="ok-text">{t("refunded", { n: g.cost })}</p>}
        </div>
      )}

      {g.status === "succeeded" && g.result_url && (
        <figure className="card result">
          <img src={g.result_url} alt={t("resultAlt")} />
          <figcaption className="result-labels small muted">
            {ANGLES.map((a) => <span key={a}>{t(`angle.${a}`)}</span>)}
          </figcaption>
          {g.result_expires_at && <p className="small muted">{t("expires", { date: formatDateTime(g.result_expires_at, locale) })}</p>}
        </figure>
      )}
      {g.status === "succeeded" && !g.result_url && <p className="muted">{t("resultDeleted")}</p>}
      {g.status === "succeeded" && <p className="small muted">{t("disclaimer")}</p>}

      <ErrorBox error={error} />

      {!isActive(g.status) && (
        <div className="row wrap">
          {g.status === "succeeded" && g.result_url && (
            <button type="button" className="btn btn-primary" onClick={toggleSave} disabled={busy}>
              {g.saved ? t("unsaveLook") : t("saveLook")}
            </button>
          )}
          <button type="button" className="btn btn-secondary" onClick={edit} disabled={busy}>{t("editLook")}</button>
          <button type="button" className="btn btn-secondary" onClick={repeat} disabled={busy}>{t("repeat", { n: g.cost })}</button>
        </div>
      )}

      <section className="card">
        <h2>{t("usedItems")}</h2>
        <ul className="plain stack">
          {g.products.map((p) => (
            <li key={p.id} className="row">
              {p.image_url && <img src={p.image_url} alt="" width={56} height={56} className="thumb" />}
              <div className="grow">
                <Link href={`/products/${p.id}`}>{p.title}</Link>
                <div className="small muted">{p.brand} · {p.store_name}</div>
              </div>
              <div className="stack-sm">
                <span>{formatSum(p.price, locale)}</span>
                {p.available ? (
                  <Link href={`/products/${p.id}`} className="btn btn-secondary small">{t("toCart")}</Link>
                ) : (
                  <span className="small muted">{t("unavailable")}</span>
                )}
              </div>
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}
