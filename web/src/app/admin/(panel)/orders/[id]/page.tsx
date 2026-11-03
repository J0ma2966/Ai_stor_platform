"use client";
import { use } from "react";
import { useTranslations } from "next-intl";
import { DeliveryWindow } from "@/components/DeliveryWindow";
import { Link } from "@/i18n/navigation";

export default function AdminOrderPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const t = useTranslations("app");
  return (
    <>
      <Link href="/admin/orders" className="small">← {t("back")}</Link>
      <DeliveryWindow id={id} scope="admin" />
    </>
  );
}
