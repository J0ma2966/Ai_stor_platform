"use client";
import { use } from "react";
import { useTranslations } from "next-intl";
import { TryOnGate } from "@/components/TryOnGate";
import { AuthGate } from "@/components/AuthGate";
import { GenerationView } from "@/components/GenerationView";
import { TryOnWizard } from "@/components/TryOnWizard";

// /fitting/new — мастер запуска; /fitting/<id> — статус и результат генерации.
export default function FittingStepPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const t = useTranslations("tryon");
  return (
    <TryOnGate>
      <AuthGate role="client">
        <h1>{id === "new" ? t("newTitle") : t("resultTitle")}</h1>
        {id === "new" ? <TryOnWizard /> : <GenerationView id={id} />}
      </AuthGate>
    </TryOnGate>
  );
}
