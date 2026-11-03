"use client";
import { useMemo } from "react";
import { useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { AuthGate } from "@/components/AuthGate";
import { CheckoutForm } from "@/components/CheckoutForm";
import { parseItemsParam } from "@/lib/orders";

// /checkout?items=<product_id>:<qty>,... — корзина передаёт выбранные позиции.
export default function CheckoutPage() {
  const t = useTranslations("checkout");
  const sp = useSearchParams();
  const items = useMemo(() => parseItemsParam(sp.get("items")), [sp]);
  return (
    <AuthGate role="client">
      <h1>{t("title")}</h1>
      <CheckoutForm items={items} />
    </AuthGate>
  );
}
