"use client";
import { useTranslations } from "next-intl";
import { AuthGate } from "@/components/AuthGate";
import { OrderList } from "@/components/OrderList";

export default function MyOrdersPage() {
  const t = useTranslations("app");
  return (
    <AuthGate role="client">
      <h1>{t("myOrders")}</h1>
      <OrderList scope="client" />
    </AuthGate>
  );
}
