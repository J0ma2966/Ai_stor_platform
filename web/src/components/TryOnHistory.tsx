"use client";
import { useCallback, useEffect, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Link } from "@/i18n/navigation";
import { formatDateTime } from "@/lib/money";
import { deleteGeneration, isActive, listGenerations, type Generation } from "@/lib/tryon";
import { ErrorBox } from "@/components/ErrorBox";
import { TryOnStatus } from "@/components/TryOnStatus";

// История примерок и сохранённые образы (ТЗ 10: «История примерок», «Избранные образы»).
export function TryOnHistory() {
  const t = useTranslations("tryon");
  const locale = useLocale();
  const [saved, setSavedTab] = useState(false);
  const [items, setItems] = useState<Generation[] | null>(null);
  const [cursor, setCursor] = useState("");
  const [error, setError] = useState<unknown>(null);

  const load = useCallback(async (more = false) => {
    setError(null);
    try {
      const r = await listGenerations(saved, more ? cursor : "");
      setItems((prev) => (more && prev ? [...prev, ...r.items] : r.items));
      setCursor(r.nextCursor);
    } catch (e) {
      setError(e);
    }
  }, [saved, cursor]);

  useEffect(() => {
    setItems(null);
    load(false);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [saved]);

  const remove = async (id: string) => {
    if (!window.confirm(t("deleteConfirm"))) return;
    try {
      await deleteGeneration(id);
      setItems((xs) => xs?.filter((x) => x.id !== id) ?? null);
    } catch (e) {
      setError(e);
    }
  };

  return (
    <div className="stack">
      <div className="row" role="tablist">
        <button type="button" role="tab" aria-selected={!saved} className={`btn ${!saved ? "btn-primary" : "btn-secondary"}`} onClick={() => setSavedTab(false)}>{t("allTryons")}</button>
        <button type="button" role="tab" aria-selected={saved} className={`btn ${saved ? "btn-primary" : "btn-secondary"}`} onClick={() => setSavedTab(true)}>{t("savedLooks")}</button>
      </div>
      <ErrorBox error={error} onRetry={() => load(false)} />
      {!items && !error && <p className="muted">{t("loading")}</p>}
      {items?.length === 0 && (
        <div className="card stack">
          <p className="muted">{saved ? t("noLooks") : t("noTryons")}</p>
          <Link href="/fitting" className="btn btn-primary">{t("toRoom")}</Link>
        </div>
      )}
      <ul className="plain history-grid">
        {items?.map((g) => (
          <li key={g.id} className="card stack-sm">
            <Link href={`/fitting/${g.id}`} className="history-thumb">
              {g.result_url ? <img src={g.result_url} alt={t("resultAlt")} loading="lazy" /> : <div className="photo-empty" />}
            </Link>
            <div className="row between">
              <TryOnStatus status={g.status} />
              {g.saved && <span className="badge badge-ok">{t("saved")}</span>}
            </div>
            <div className="small muted">{formatDateTime(g.created_at, locale)}</div>
            <div className="small clamp">{g.products.map((p) => p.title).join(", ")}</div>
            {!isActive(g.status) && (
              <button type="button" className="btn-link small" onClick={() => remove(g.id)}>{t("delete")}</button>
            )}
          </li>
        ))}
      </ul>
      {cursor && <button type="button" className="btn btn-secondary" onClick={() => load(true)}>{t("more")}</button>}
    </div>
  );
}
