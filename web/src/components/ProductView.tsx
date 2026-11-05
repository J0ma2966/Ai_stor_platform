"use client";
import { useEffect } from "react";
import { API_URL } from "@/lib/api";

// Отмечает просмотр карточки товара (статистика магазина, сортировка «Популярные»).
// Считает браузер, а не серверный рендеринг: ответ API для страницы кэшируется.
// Повторное открытие той же карточки в этой вкладке не считается.
export function ProductView({ id }: { id: string }) {
  useEffect(() => {
    const key = `pv:${id}`;
    try {
      if (sessionStorage.getItem(key)) return;
      sessionStorage.setItem(key, "1");
    } catch {
      /* приватный режим без sessionStorage — считаем без дедупликации */
    }
    void fetch(`${API_URL}/v1/products/${id}/view`, { method: "POST", keepalive: true }).catch(() => undefined);
  }, [id]);
  return null;
}
