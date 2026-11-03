"use client";
import { use, useCallback, useEffect, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Link } from "@/i18n/navigation";
import { adminApi, generatePassword, type StoreDetail, type StoreStatus } from "@/lib/admin";
import { copyToClipboard } from "@/lib/orders";
import { formatDateTime } from "@/lib/money";
import { ErrorBox } from "@/components/ErrorBox";
import { StoreStatusBadge } from "@/components/admin/Badges";
import { ConfirmAction } from "@/components/admin/ConfirmAction";
import { AuditList } from "@/components/admin/AuditList";

export default function AdminStorePage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const t = useTranslations("admin");
  const tApp = useTranslations("app");
  const locale = useLocale();
  const [s, setS] = useState<StoreDetail | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const [newPassword, setNewPassword] = useState("");
  const [shownPassword, setShownPassword] = useState("");
  const [copied, setCopied] = useState(false);

  const load = useCallback(() => adminApi.store(id).then(setS, setError), [id]);
  useEffect(() => {
    load();
  }, [load]);

  async function run(fn: () => Promise<unknown>) {
    setBusy(true);
    setError(null);
    try {
      await fn();
      await load();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  const setStatus = (st: Exclude<StoreStatus, "pending_activation">) => (reason: string) => run(() => adminApi.setStoreStatus(id, st, reason));

  if (!s) return error ? <ErrorBox error={error} onRetry={load} /> : <p className="muted">{tApp("loading")}</p>;
  const rows: [string, string | null | undefined][] = [
    [t("storeSlug"), s.slug],
    [t("storeLogin"), s.login],
    [t("storePhone"), s.phone],
    [t("address"), s.address],
    [t("createdAt"), formatDateTime(s.created_at, locale)],
    [t("activatedAt"), s.activated_at ? formatDateTime(s.activated_at, locale) : null],
    [t("lastLogin"), s.last_login_at ? formatDateTime(s.last_login_at, locale) : t("never")],
    [t("productsOrders"), t("counts", { products: s.products_count, orders: s.orders_count })],
  ];

  return (
    <>
      <Link href="/admin/stores" className="small">← {tApp("back")}</Link>
      <header className="row between wrap">
        <h1>{s.name}</h1>
        <StoreStatusBadge status={s.status} />
      </header>
      <ErrorBox error={error} />
      <section className="card">
        <dl className="kv">
          {rows.map(([k, v]) => (
            <div key={k}>
              <dt>{k}</dt>
              <dd>{v || "—"}</dd>
            </div>
          ))}
        </dl>
      </section>

      <section className="card stack">
        <h2>{t("actions")}</h2>
        <div className="row wrap">
          {(s.status === "pending_activation" || s.status === "blocked" || s.status === "archived") && (
            <ConfirmAction label={s.status === "pending_activation" ? t("activate") : t("unblock")} busy={busy} onConfirm={setStatus("active")} />
          )}
          {s.status !== "blocked" && s.status !== "archived" && (
            <ConfirmAction label={t("block")} danger needReason busy={busy} onConfirm={setStatus("blocked")}>
              <p className="small">{t("blockStoreHint")}</p>
            </ConfirmAction>
          )}
          {s.status !== "archived" && (
            <ConfirmAction label={t("archive")} danger needReason busy={busy} onConfirm={setStatus("archived")}>
              <p className="small">{t("archiveHint")}</p>
            </ConfirmAction>
          )}
          <ConfirmAction
            label={t("resetPassword")}
            busy={busy}
            onConfirm={async () => {
              const pw = newPassword || generatePassword();
              await run(async () => {
                await adminApi.resetStorePassword(id, pw);
                setShownPassword(pw);
                setCopied(false);
              });
              setNewPassword("");
            }}
          >
            <p className="small">{t("resetPasswordHint")}</p>
            <div className="field">
              <label htmlFor="np">{t("newPasswordOptional")}</label>
              <input id="np" value={newPassword} onChange={(e) => setNewPassword(e.target.value)} minLength={10} autoComplete="new-password" />
            </div>
          </ConfirmAction>
        </div>
        {shownPassword && (
          <div className="stack" aria-live="polite">
            <pre className="copy-text pre card">{`${t("credLogin")}: ${s.login}\n${t("credPassword")}: ${shownPassword}`}</pre>
            <p className="small muted">{t("passwordOnce")}</p>
            <button type="button" className="btn btn-secondary" onClick={async () => setCopied(await copyToClipboard(`${s.login}\n${shownPassword}`))}>
              {copied ? t("copied") : t("copy")}
            </button>
          </div>
        )}
      </section>

      <section className="card">
        <h2>{t("history")}</h2>
        <AuditList items={s.history} showEntity={false} />
        <Link href={`/admin/audit?entity_id=${s.id}`} className="small">{t("fullHistory")}</Link>
      </section>
    </>
  );
}
