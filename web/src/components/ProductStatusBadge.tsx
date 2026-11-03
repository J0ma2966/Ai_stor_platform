"use client";
import { useTranslations } from "next-intl";

const tone: Record<string, string> = { draft: "warn", published: "ok", unpublished: "bad", blocked: "bad" };

export function ProductStatusBadge({ status }: { status: string }) {
  const t = useTranslations("storeProducts");
  return <span className={`badge badge-${tone[status] ?? "info"}`}>{t(`st_${status}`)}</span>;
}
