"use client";
import { useCallback, useEffect, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Link } from "@/i18n/navigation";
import { formatDateTime } from "@/lib/money";
import { useSession } from "@/lib/session";
import { getNotifications, markRead, notificationHref, type Notification } from "@/lib/shop";
import { ErrorBox } from "./ErrorBox";

export function useNotificationText() {
  const t = useTranslations("notifications");
  const ts = useTranslations("status");
  return (n: Notification) => {
    const d = n.data as Record<string, string | number>;
    const status = String(d.status ?? "");
    const args = {
      number: String(d.number ?? ""),
      title: String(d.title ?? ""),
      status: n.kind === "order_status" ? (ts.has(status) ? ts(status) : status) : n.kind === "product_moderated" && t.has(`modStatus.${status}`) ? t(`modStatus.${status}`) : status,
    };
    const key = `kinds.${n.kind}`;
    return t.has(key) ? t(key, args) : t("kinds.system");
  };
}

export function Notifications() {
  const t = useTranslations("notifications");
  const ta = useTranslations("app");
  const locale = useLocale();
  const { me, setUnread } = useSession();
  const text = useNotificationText();
  const [items, setItems] = useState<Notification[] | null>(null);
  const [cursor, setCursor] = useState("");
  const [error, setError] = useState<unknown>(null);

  const load = useCallback(async (from = "") => {
    setError(null);
    try {
      const r = await getNotifications(from);
      setItems((p) => (from ? [...(p ?? []), ...r.items] : r.items));
      setCursor(r.nextCursor);
    } catch (e) {
      setError(e);
    }
  }, []);
  useEffect(() => void load(), [load]);

  async function readAll() {
    try {
      await markRead("all");
      setItems((p) => p?.map((n) => ({ ...n, read: true })) ?? p);
      setUnread(0);
    } catch (e) {
      setError(e);
    }
  }
  async function open(n: Notification) {
    if (n.read) return;
    setItems((p) => p?.map((x) => (x.id === n.id ? { ...x, read: true } : x)) ?? p);
    markRead([n.id]).catch(() => {});
  }

  return (
    <div className="stack">
      <div className="row between">
        <h1>{t("title")}</h1>
        {items?.some((n) => !n.read) && <button type="button" className="btn btn-secondary" onClick={readAll}>{t("markAll")}</button>}
      </div>
      <ErrorBox error={error} onRetry={() => load()} />
      {items === null ? (
        !error && <p className="muted">{ta("loading")}</p>
      ) : items.length === 0 ? (
        <p className="empty">{t("empty")}</p>
      ) : (
        <ul className="plain stack-sm">
          {items.map((n) => {
            const href = notificationHref(n, me?.role ?? "client");
            const reason = (n.data as Record<string, string>).reason;
            const body = (
              <>
                <span className="grow stack-xs">
                  <strong>{text(n)}</strong>
                  {reason && <span className="small">{t("reason", { reason })}</span>}
                  <span className="muted small">{formatDateTime(n.created_at, locale)}</span>
                </span>
                {!n.read && <span className="badge badge-warn">{t("new")}</span>}
              </>
            );
            return (
              <li key={n.id}>
                {href ? (
                  <Link href={href} className={`card notif ${n.read ? "" : "notif-unread"}`} onClick={() => open(n)}>{body}</Link>
                ) : (
                  <div className={`card notif ${n.read ? "" : "notif-unread"}`} onMouseEnter={() => open(n)}>{body}</div>
                )}
              </li>
            );
          })}
        </ul>
      )}
      {cursor && <div className="center"><button type="button" className="btn" onClick={() => load(cursor)}>{ta("more")}</button></div>}
    </div>
  );
}
