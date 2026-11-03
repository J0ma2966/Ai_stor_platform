"use client";
import { useTranslations } from "next-intl";
import { ProductForm } from "@/components/ProductForm";

export default function NewProduct() {
  const t = useTranslations("productForm");
  return (
    <>
      <h1>{t("newTitle")}</h1>
      <ProductForm />
    </>
  );
}
