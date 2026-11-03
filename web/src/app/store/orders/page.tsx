"use client";
import { useTranslations } from "next-intl";
import { AuthGate } from "@/components/AuthGate";
import { OrderList } from "@/components/OrderList";

export default function StoreOrdersPage() {
  const t = useTranslations("app");
  return (
    <AuthGate role="store">
      <h1>{t("storeOrders")}</h1>
      <OrderList scope="store" />
    </AuthGate>
  );
}
