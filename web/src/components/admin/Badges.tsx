"use client";
import { useTranslations } from "next-intl";
import type { StoreStatus, UserStatus } from "@/lib/admin";

const storeTone: Record<StoreStatus, string> = { pending_activation: "warn", active: "ok", blocked: "bad", archived: "info" };

export function StoreStatusBadge({ status }: { status: StoreStatus }) {
  const t = useTranslations("admin.storeStatus");
  return <span className={`badge badge-${storeTone[status]}`}>{t(status)}</span>;
}

export function UserStatusBadge({ status }: { status: UserStatus }) {
  const t = useTranslations("admin.userStatus");
  return <span className={`badge badge-${status === "active" ? "ok" : "bad"}`}>{t(status)}</span>;
}
