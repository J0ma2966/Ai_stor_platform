"use client";
import { useState, type FormEvent } from "react";
import { useLocale, useTranslations } from "next-intl";
import { adminApi, sumToTiyin, type Tariff, type TariffInput } from "@/lib/admin";
import { formatSum } from "@/lib/money";
import { ErrorBox } from "@/components/ErrorBox";
import { useCursorList } from "@/components/admin/useCursorList";

type Draft = { name_ru: string; name_uz: string; description: string; tokens: string; price: string; highlighted: boolean; is_published: boolean };
const empty: Draft = { name_ru: "", name_uz: "", description: "", tokens: "", price: "", highlighted: false, is_published: false };
const toDraft = (x: Tariff): Draft => ({ ...x, tokens: String(x.tokens), price: String(x.price / 100) });

function TariffForm({ initial, onSave, onCancel }: { initial: Draft; onSave: (b: TariffInput) => Promise<void>; onCancel: () => void }) {
  const t = useTranslations("admin");
  const [d, setD] = useState(initial);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const tokens = Number.parseInt(d.tokens, 10);
  const price = sumToTiyin(d.price);
  const valid = d.name_ru.trim() && d.name_uz.trim() && Number.isInteger(tokens) && tokens > 0 && price !== null;

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!valid) return;
    setBusy(true);
    setError(null);
    try {
      await onSave({ name_ru: d.name_ru.trim(), name_uz: d.name_uz.trim(), description: d.description.trim(), tokens, price: price!, highlighted: d.highlighted, is_published: d.is_published });
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  }
  const text = (k: "name_ru" | "name_uz" | "description" | "tokens" | "price", label: string, extra: Record<string, unknown> = {}) => (
    <div className="field">
      <label htmlFor={`tf-${k}`}>{label}</label>
      <input id={`tf-${k}`} value={d[k]} onChange={(e) => setD({ ...d, [k]: e.target.value })} {...extra} />
    </div>
  );
  return (
    <form className="card" onSubmit={submit} noValidate>
      {text("name_ru", t("nameRu"))}
      {text("name_uz", t("nameUz"))}
      {text("description", t("description"))}
      {text("tokens", t("tokensCount"), { inputMode: "numeric" })}
      {text("price", t("priceSum"), { inputMode: "decimal" })}
      <label className="row small"><input type="checkbox" checked={d.highlighted} onChange={(e) => setD({ ...d, highlighted: e.target.checked })} /> {t("highlighted")}</label>
      <label className="row small"><input type="checkbox" checked={d.is_published} onChange={(e) => setD({ ...d, is_published: e.target.checked })} /> {t("published")}</label>
      <ErrorBox error={error} />
      <div className="row wrap">
        <button className="btn btn-primary" disabled={busy || !valid}>{t("save")}</button>
        <button type="button" className="btn btn-secondary" onClick={onCancel}>{t("cancel")}</button>
      </div>
    </form>
  );
}

export default function AdminTariffsPage() {
  const t = useTranslations("admin");
  const locale = useLocale();
  const list = useCursorList(() => adminApi.tariffs(), "tariffs");
  const [editing, setEditing] = useState<string | "new" | null>(null);

  return (
    <>
      <div className="row between wrap">
        <h1>{t("navTariffs")}</h1>
        {editing !== "new" && <button className="btn btn-primary" onClick={() => setEditing("new")}>{t("newTariff")}</button>}
      </div>
      {editing === "new" && (
        <TariffForm
          initial={empty}
          onCancel={() => setEditing(null)}
          onSave={async (b) => {
            await adminApi.createTariff(b);
            setEditing(null);
            list.reload();
          }}
        />
      )}
      <ErrorBox error={list.error} onRetry={list.reload} />
      {!list.loading && !list.error && list.items.length === 0 && <p className="muted">{t("nothing")}</p>}
      <ul className="plain stack">
        {list.items.map((x) =>
          editing === x.id ? (
            <li key={x.id}>
              <TariffForm
                initial={toDraft(x)}
                onCancel={() => setEditing(null)}
                onSave={async (b) => {
                  await adminApi.updateTariff(x.id, b);
                  setEditing(null);
                  list.reload();
                }}
              />
            </li>
          ) : (
            <li key={x.id} className="card order-row">
              <span className="order-main">
                <strong>{locale === "uz" ? x.name_uz : x.name_ru}{x.highlighted && " ★"}</strong>
                <span className="small">{t("tokensN", { n: x.tokens })} · {formatSum(x.price, locale)}</span>
                {x.description && <span className="small muted">{x.description}</span>}
              </span>
              <span className="order-side">
                <span className={`badge badge-${x.is_published ? "ok" : "warn"}`}>{x.is_published ? t("published") : t("hidden")}</span>
                <button className="btn-link" onClick={() => setEditing(x.id)}>{t("edit")}</button>
              </span>
            </li>
          ),
        )}
      </ul>
    </>
  );
}
