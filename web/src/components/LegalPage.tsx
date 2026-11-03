import { getTranslations } from "next-intl/server";
import { legalDoc, type LegalKey } from "@/content/legal";
import { fetchPlatform } from "@/lib/catalog";

export async function LegalPage({ locale, doc }: { locale: string; doc: LegalKey }) {
  const t = await getTranslations("legal");
  const d = legalDoc(locale, doc);
  const p = doc === "contacts" ? await fetchPlatform() : {};
  return (
    <article className="legal card">
      <h1>{d.title}</h1>
      <p className="muted small">{t("updated", { date: d.updated })}</p>
      {d.sections.map((s) => (
        <section key={s.h}>
          <h2>{s.h}</h2>
          {s.p.map((x, i) => <p key={i}>{x}</p>)}
        </section>
      ))}
      {doc === "contacts" && (
        <ul className="plain stack-sm">
          {p.support_phone && <li>☎ <a href={`tel:${p.support_phone}`}>{p.support_phone}</a></li>}
          {p.support_email && <li>✉ <a href={`mailto:${p.support_email}`}>{p.support_email}</a></li>}
          {p.support_telegram && <li>Telegram: <a href={`https://t.me/${p.support_telegram.replace(/^@/, "")}`} target="_blank" rel="noopener noreferrer">{p.support_telegram}</a></li>}
        </ul>
      )}
    </article>
  );
}
