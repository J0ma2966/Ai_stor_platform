"use client";
import { useTranslations } from "next-intl";
import { TryOnGate } from "@/components/TryOnGate";
import { AuthGate } from "@/components/AuthGate";
import { BodyPhotos } from "@/components/BodyPhotos";

export default function PhotosPage() {
  const t = useTranslations("tryon");
  return (
    <TryOnGate>
      <AuthGate role="client">
        <h1>{t("myPhotos")}</h1>
        <BodyPhotos />
      </AuthGate>
    </TryOnGate>
  );
}
