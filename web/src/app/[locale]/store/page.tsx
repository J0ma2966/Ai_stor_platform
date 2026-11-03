"use client";
import { useCallback, useEffect, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Link } from "@/i18n/navigation";
import { ErrorBox } from "@/components/ErrorBox";
import { formatSum } from "@/lib/money";
import { getStoreStats, type StoreStats } from "@/lib/storeCab";

function ymd(d: Date) {
  return new Intl.DateTimeFormat("en-CA", { timeZone: "Asia/Tashkent" }).format(d);
}
function daysAgo(n: number) {
  return ymd(new Date(Date.now() - n * 86400000));
}

export default function StoreOverview() {
  const t = useTranslations("storeCab");
  const ta = useTranslations("app");
  const locale = useLocale();
  const [from, setFrom] = useState(daysAgo(29));
  const [to, setTo] = useState(ymd(new Date()));
  const [st, setSt] = useState<StoreStats | null>(null);
  const [error, setError] = useState<unknown>(null);

  const load = useCallback(() => {
    setError(null);
    getStoreStats(from, to).then(setSt).catch(setError);
  }, [from, to]);
  useEffect(load, [load]);

  const preset = (n: number) => { setFrom(daysAgo(n - 1)); setTo(ymd(new Date())); };
  const max = Math.max(1, ...(st?.daily.map((d) => d.views) ?? [1]));
  const tiles: [string, string | number][] = st
    ? [
        ["productsTotal", st.products_total], ["productsPublished", st.products_published], ["views", st.views],
        ["fittingAdds", st.fitting_adds], ["tryons", st.tryons], ["ordersCount", st.orders], ["delivered", st.orders_delivered],
        ["revenue", formatSum(st.revenue, locale)],
      ]
    : [];
  return (
    <>
      <div className="row between wrap">
        <h1>{t("statsTitle")}</h1>
        <Link href="/store/products/new" className="btn btn-primary">{t("addProduct")}</Link>
      </div>
      <p className="muted small">{t("onlyOwn")}</p>
      <div className="card row wrap">
        <span className="label">{t("period")}</span>
        {[7, 30, 90].map((n) => <button key={n} type="button" className="tab" onClick={() => preset(n)}>{t(`last${n}`)}</button>)}
        <label className="row small">{t("from")} <input type="date" value={from} max={to} onChange={(e) => setFrom(e.target.value)} /></label>
        <label className="row small">{t("to")} <input type="date" value={to} min={from} onChange={(e) => setTo(e.target.value)} /></label>
      </div>
      <ErrorBox error={error} onRetry={load} />
      {!st ? (
        !error && <p className="muted">{ta("loading")}</p>
      ) : (
        <>
          <ul className="tiles plain">
            {tiles.map(([k, v]) => <li key={k} className="card tile"><span className="muted small">{t(k)}</span><span className="num">{v}</span></li>)}
          </ul>
          <section className="card">
            <h2>{t("daily")}</h2>
            <div className="bars" role="img" aria-label={t("daily")}>
              {st.daily.map((d) => <span key={d.day} title={`${d.day}: ${d.views}`} style={{ height: `${(d.views / max) * 100}%` }} />)}
            </div>
          </section>
          <section className="card">
            <h2>{t("top")}</h2>
            {st.top_products.length === 0 ? (
              <p className="muted">{t("topEmpty")}</p>
            ) : (
              <div className="table-wrap">
                <table className="tbl">
                  <thead><tr><th>{t("products")}</th><th>{t("views")}</th><th>{t("fittingAdds")}</th><th>{t("ordered")}</th></tr></thead>
                  <tbody>
                    {st.top_products.map((p) => (
                      <tr key={p.id}><td><Link href={`/store/products/${p.id}`}>{p.title}</Link> <span className="muted small">{p.brand}</span></td><td>{p.views}</td><td>{p.fitting_adds}</td><td>{p.ordered}</td></tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </section>
        </>
      )}
    </>
  );
}
