"use client";
import { useTranslations } from "next-intl";
import { AuthGate } from "@/components/AuthGate";
import { BodyPhotos } from "@/components/BodyPhotos";

export default function PhotosPage() {
  const t = useTranslations("tryon");
  return (
    <AuthGate role="client">
      <h1>{t("myPhotos")}</h1>
      <BodyPhotos />
    </AuthGate>
  );
}
