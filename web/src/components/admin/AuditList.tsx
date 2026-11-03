"use client";
import { useLocale, useTranslations } from "next-intl";
import { Link } from "@/i18n/navigation";
import { formatDateTime } from "@/lib/money";
import type { AuditEntry } from "@/lib/admin";

// Ссылка на объект журнала, если для него есть страница в панели.
function entityHref(e: AuditEntry): string | null {
  if (!e.entity_id) return null;
  switch (e.entity_type) {
    case "store":
      return `/admin/stores/${e.entity_id}`;
    case "user":
      return `/admin/users/${e.entity_id}`;
    case "order":
      return `/admin/orders/${e.entity_id}`;
  }
  return null;
}

export function useActionLabel() {
  const t = useTranslations("admin.actionNames");
  // next-intl не допускает точек в ключах: "store.create" -> "store_create"
  return (a: string) => {
    const k = a.replace(/\./g, "_");
    return t.has(k) ? t(k) : a;
  };
}

// Детали записи: пары ключ=значение, только текстом.
function dataText(d: Record<string, unknown>): string {
  return Object.entries(d ?? {})
    .filter(([, v]) => v !== "" && v !== null && v !== undefined)
    .map(([k, v]) => `${k}: ${typeof v === "string" ? v : JSON.stringify(v)}`)
    .join(" · ");
}

export function AuditList({ items, showEntity = true }: { items: AuditEntry[]; showEntity?: boolean }) {
  const t = useTranslations("admin");
  const locale = useLocale();
  const label = useActionLabel();
  if (items.length === 0) return <p className="muted">{t("auditEmpty")}</p>;
  return (
    <ul className="plain audit-list">
      {items.map((e) => {
        const href = showEntity ? entityHref(e) : null;
        const details = dataText(e.data);
        return (
          <li key={e.id} className="audit-row">
            <div className="row between wrap">
              <strong>{label(e.action)}</strong>
              <span className="muted small">{formatDateTime(e.created_at, locale)}</span>
            </div>
            <div className="small">
              {t("auditBy", { who: e.actor_login ?? t("system"), role: e.actor_role ?? "—" })}
              {href && (
                <>
                  {" · "}
                  <Link href={href}>{t("auditOpen")}</Link>
                </>
              )}
            </div>
            {details && <div className="small muted pre">{details}</div>}
          </li>
        );
      })}
    </ul>
  );
}
