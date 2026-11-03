"use client";
import { useTranslations } from "next-intl";
import { TryOnGate } from "@/components/TryOnGate";
import { AuthGate } from "@/components/AuthGate";
import { TryOnHistory } from "@/components/TryOnHistory";

export default function TryonsPage() {
  const t = useTranslations("tryon");
  return (
    <TryOnGate>
      <AuthGate role="client">
        <h1>{t("history")}</h1>
        <TryOnHistory />
      </AuthGate>
    </TryOnGate>
  );
}
