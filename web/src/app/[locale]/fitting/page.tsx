"use client";
import { useTranslations } from "next-intl";
import { FittingRoom } from "@/components/FittingRoom";

// /fitting — примерочная: доступна без входа, вход нужен для запуска генерации.
export default function FittingPage() {
  const t = useTranslations("tryon");
  return (
    <>
      <h1>{t("roomTitle")}</h1>
      <FittingRoom />
    </>
  );
}
