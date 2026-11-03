"use client";
import { useTranslations } from "next-intl";
import { OrderList } from "@/components/OrderList";

export default function AdminOrdersPage() {
  const t = useTranslations("admin");
  return (
    <>
      <h1>{t("navOrders")}</h1>
      <OrderList scope="admin" />
    </>
  );
}
