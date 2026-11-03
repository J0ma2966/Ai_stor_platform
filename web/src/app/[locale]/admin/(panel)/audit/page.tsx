"use client";
import { useState } from "react";
import { useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { adminApi } from "@/lib/admin";
import { ErrorBox } from "@/components/ErrorBox";
import { AuditList } from "@/components/admin/AuditList";
import { useCursorList } from "@/components/admin/useCursorList";
import { useDebounced } from "@/components/admin/useDebounced";

// Фильтр по группе действий: "store." — все действия над магазинами и т. п.
const GROUPS = ["", "store.", "user.", "tokens.", "order."] as const;

export default function AdminAuditPage() {
  const t = useTranslations("admin");
  const sp = useSearchParams();
  const [group, setGroup] = useState<string>("");
  const [entity, setEntity] = useState(sp.get("entity_id") ?? "");
  const de = useDebounced(entity.trim(), 300);
  const list = useCursorList((cursor) => adminApi.audit({ action: group, entity_id: de, cursor }), `${group}|${de}`);

  return (
    <>
      <h1>{t("navAudit")}</h1>
      <div className="tabs" role="tablist">
        {GROUPS.map((g) => (
          <button key={g || "all"} role="tab" aria-selected={group === g} className={group === g ? "tab tab-on" : "tab"} onClick={() => setGroup(g)}>
            {t(`auditGroup.${g ? g.slice(0, -1) : "all"}`)}
          </button>
        ))}
      </div>
      <input type="search" aria-label={t("entityId")} placeholder={t("entityId")} value={entity} onChange={(e) => setEntity(e.target.value)} maxLength={64} />
      <ErrorBox error={list.error} onRetry={list.reload} />
      {!list.loading || list.items.length > 0 ? <section className="card"><AuditList items={list.items} /></section> : null}
      {list.hasMore && <button className="btn btn-secondary" disabled={list.loading} onClick={list.more}>{t("more")}</button>}
    </>
  );
}
