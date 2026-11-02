"use client";
import { useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Link } from "@/i18n/navigation";
import { adminApi, userTitle, type UserRole } from "@/lib/admin";
import { formatDateTime } from "@/lib/money";
import { ErrorBox } from "@/components/ErrorBox";
import { UserStatusBadge } from "@/components/admin/Badges";
import { useCursorList } from "@/components/admin/useCursorList";
import { useDebounced } from "@/components/admin/useDebounced";

const ROLES: UserRole[] = ["client", "store", "admin"];

export default function AdminUsersPage() {
  const t = useTranslations("admin");
  const tr = useTranslations("admin.role");
  const tu = useTranslations("admin.userStatus");
  const locale = useLocale();
  const [role, setRole] = useState<UserRole>("client");
  const [status, setStatus] = useState("");
  const [q, setQ] = useState("");
  const dq = useDebounced(q.trim(), 300);
  const list = useCursorList((cursor) => adminApi.users({ role, status, q: dq, cursor }), `${role}|${status}|${dq}`);

  return (
    <>
      <h1>{t("navUsers")}</h1>
      <div className="tabs" role="tablist">
        {ROLES.map((r) => (
          <button key={r} role="tab" aria-selected={role === r} className={role === r ? "tab tab-on" : "tab"} onClick={() => setRole(r)}>
            {tr(r)}
          </button>
        ))}
      </div>
      <div className="row wrap">
        <input type="search" className="grow" aria-label={t("searchUsers")} placeholder={t("searchUsers")} value={q} onChange={(e) => setQ(e.target.value)} maxLength={100} />
        <select aria-label={t("status")} value={status} onChange={(e) => setStatus(e.target.value)}>
          <option value="">{t("allStatuses")}</option>
          <option value="active">{tu("active")}</option>
          <option value="blocked">{tu("blocked")}</option>
        </select>
      </div>
      <ErrorBox error={list.error} onRetry={list.reload} />
      {!list.loading && !list.error && list.items.length === 0 && <p className="muted">{t("nothing")}</p>}
      <ul className="plain stack">
        {list.items.map((u) => (
          <li key={u.id}>
            <Link href={`/admin/users/${u.id}`} className="card order-row">
              <span className="order-main">
                <strong>{userTitle(u)}</strong>
                <span className="muted small">
                  {t("registered", { date: formatDateTime(u.created_at, locale) })}
                  {u.role === "client" && !u.registration_complete && ` · ${t("notCompleted")}`}
                </span>
              </span>
              <span className="order-side">
                <UserStatusBadge status={u.status} />
                {u.role === "client" && <span className="small">{t("tokensAndOrders", { tokens: u.token_balance, orders: u.orders_count })}</span>}
              </span>
            </Link>
          </li>
        ))}
      </ul>
      {list.hasMore && <button className="btn btn-secondary" disabled={list.loading} onClick={list.more}>{t("more")}</button>}
    </>
  );
}
