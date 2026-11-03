"use client";
import { useTranslations } from "next-intl";
import type { GenStatus } from "@/lib/tryon";

const tone: Record<GenStatus, string> = { queued: "info", running: "warn", succeeded: "ok", failed: "bad" };

export function TryOnStatus({ status }: { status: GenStatus }) {
  const t = useTranslations("tryon.status");
  return <span className={`badge badge-${tone[status]}`}>{t(status)}</span>;
}
