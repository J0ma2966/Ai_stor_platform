"use client";
import { use } from "react";
import { useTranslations } from "next-intl";
import { AuthGate } from "@/components/AuthGate";
import { DeliveryWindow } from "@/components/DeliveryWindow";
import { Link } from "@/i18n/navigation";

export default function StoreOrderPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const t = useTranslations("app");
  return (
    <AuthGate role="store">
      <Link href="/store/orders" className="small">← {t("back")}</Link>
      <DeliveryWindow id={id} />
    </AuthGate>
  );
}
