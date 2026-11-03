"use client";
import { use, useCallback, useEffect, useState, type FormEvent } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Link } from "@/i18n/navigation";
import { adminApi, userTitle, type UserDetail } from "@/lib/admin";
import { formatDateTime } from "@/lib/money";
import { ErrorBox } from "@/components/ErrorBox";
import { UserStatusBadge } from "@/components/admin/Badges";
import { ConfirmAction } from "@/components/admin/ConfirmAction";
import { AuditList } from "@/components/admin/AuditList";

export default function AdminUserPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const t = useTranslations("admin");
  const tr = useTranslations("admin.roleOne");
  const to = useTranslations("admin.tokenOp");
  const tApp = useTranslations("app");
  const locale = useLocale();
  const [u, setU] = useState<UserDetail | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const [amount, setAmount] = useState("");
  const [reason, setReason] = useState("");
  const [done, setDone] = useState("");

  const load = useCallback(() => adminApi.user(id).then(setU, setError), [id]);
  useEffect(() => {
    load();
  }, [load]);

  async function run(fn: () => Promise<unknown>, okText = "") {
    setBusy(true);
    setError(null);
    setDone("");
    try {
      await fn();
      await load();
      setDone(okText);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }

  const amt = Number.parseInt(amount, 10);
  const amountOk = Number.isInteger(amt) && amt !== 0 && Math.abs(amt) <= 100000 && /^-?\d+$/.test(amount.trim());
  async function adjust(e: FormEvent) {
    e.preventDefault();
    if (!amountOk || reason.trim().length < 3) return;
    await run(async () => {
      await adminApi.adjustTokens(id, amt, reason.trim());
      setAmount("");
      setReason("");
    }, t("tokensAdjusted"));
  }

  if (!u) return error ? <ErrorBox error={error} onRetry={load} /> : <p className="muted">{tApp("loading")}</p>;
  const rows: [string, string | null][] = [
    [t("roleLabel"), tr(u.role)],
    [t("phone"), u.phone],
    [t("storeLogin"), u.login],
    [t("lang"), u.lang],
    [t("createdAt"), formatDateTime(u.created_at, locale)],
    [t("lastLogin"), u.last_login_at ? formatDateTime(u.last_login_at, locale) : t("never")],
  ];
  if (u.role === "client") {
    rows.push([t("tokenBalance"), String(u.token_balance)], [t("ordersCount"), String(u.orders_count)], [t("registrationComplete"), u.registration_complete ? t("yes") : t("no")]);
  }

  return (
    <>
      <Link href="/admin/users" className="small">← {tApp("back")}</Link>
      <header className="row between wrap">
        <h1>{userTitle(u)}</h1>
        <UserStatusBadge status={u.status} />
      </header>
      <ErrorBox error={error} />
      {done && <p className="ok-text" role="status">{done}</p>}
      <section className="card">
        <dl className="kv">
          {rows.map(([k, v]) => (
            <div key={k}>
              <dt>{k}</dt>
              <dd>{v || "—"}</dd>
            </div>
          ))}
        </dl>
        {u.store_id && <Link href={`/admin/stores/${u.store_id}`} className="small">{t("openStore")}</Link>}
      </section>

      {u.role === "client" && (
        <>
          <section className="card stack">
            <h2>{t("actions")}</h2>
            <div className="row wrap">
              {u.status === "active" ? (
                <ConfirmAction label={t("block")} danger needReason busy={busy} onConfirm={(r) => run(() => adminApi.setUserStatus(id, "blocked", r))}>
                  <p className="small">{t("blockUserHint")}</p>
                </ConfirmAction>
              ) : (
                <ConfirmAction label={t("unblock")} needReason busy={busy} onConfirm={(r) => run(() => adminApi.setUserStatus(id, "active", r))} />
              )}
            </div>
          </section>

          <form className="card" onSubmit={adjust} noValidate>
            <h2>{t("adjustTokens")}</h2>
            <div className="field">
              <label htmlFor="amt">{t("amount")}</label>
              <input id="amt" inputMode="numeric" value={amount} onChange={(e) => setAmount(e.target.value)} placeholder="+20 / -20" aria-invalid={amount !== "" && !amountOk} />
              <span className="small muted">{t("amountHint")}</span>
            </div>
            <div className="field">
              <label htmlFor="rsn">{t("reason")}</label>
              <input id="rsn" value={reason} onChange={(e) => setReason(e.target.value)} maxLength={500} />
            </div>
            <button className="btn btn-primary" disabled={busy || !amountOk || reason.trim().length < 3}>{t("apply")}</button>
          </form>

          <section className="card">
            <h2>{t("ledger")}</h2>
            {u.ledger.length === 0 ? (
              <p className="muted">{t("nothing")}</p>
            ) : (
              <ul className="plain audit-list">
                {u.ledger.map((l) => (
                  <li key={l.id} className="audit-row row between wrap">
                    <span>
                      <strong className={l.amount > 0 ? "ok-text" : ""}>{l.amount > 0 ? `+${l.amount}` : l.amount}</strong> {to.has(l.op) ? to(l.op) : l.op}
                      {l.reason && <span className="small muted"> · {l.reason}</span>}
                    </span>
                    <span className="small muted">
                      {t("balanceAfter", { n: l.balance_after })} · {formatDateTime(l.created_at, locale)}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </section>
        </>
      )}

      <section className="card">
        <h2>{t("history")}</h2>
        <AuditList items={u.history} showEntity={false} />
      </section>
    </>
  );
}
