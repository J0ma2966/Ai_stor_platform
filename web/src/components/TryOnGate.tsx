"use client";
import type { ReactNode } from "react";
import { useTranslations } from "next-intl";
import { Link } from "@/i18n/navigation";
import { TRYON_ENABLED } from "@/lib/fitting";

// Заглушка вместо примерочной, мастера и истории, пока AI-примерка выключена.
export function TryOnGate({ children, enabled = TRYON_ENABLED }: { children: ReactNode; enabled?: boolean }) {
  const t = useTranslations("tryon");
  if (enabled) return <>{children}</>;
  return (
    <section className="empty" data-testid="tryon-disabled">
      <h1>{t("disabledTitle")}</h1>
      <p>{t("disabledText")}</p>
      <Link href="/" className="btn btn-primary">{t("toCatalog")}</Link>
    </section>
  );
}
