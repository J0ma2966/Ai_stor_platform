// API кабинета магазина.
import { api, apiList, apiRaw } from "./api";
import type { Product, Store } from "./catalog";

export async function uploadImage(file: File, purpose: "product" | "store"): Promise<{ key: string; url: string }> {
  const res = await apiRaw(`/v1/uploads?purpose=${purpose}`, { method: "POST", rawBody: file, headers: { "Content-Type": file.type || "application/octet-stream" } });
  return (await res.json()).data;
}

export const listMyProducts = () => apiList<Product>("/v1/store/products");
export const getMyProduct = (id: string) => api<Product>(`/v1/store/products/${id}`);
export const createProduct = (body: unknown) => api<{ id: string; sku: string; status: string }>("/v1/store/products", { method: "POST", body });
export const updateProduct = (id: string, body: unknown) => api<Product>(`/v1/store/products/${id}`, { method: "PATCH", body });
export const deleteProduct = (id: string) => api<void>(`/v1/store/products/${id}`, { method: "DELETE" });
export const setProductStatus = (id: string, status: "published" | "draft") =>
  api<{ status: string }>(`/v1/store/products/${id}/status`, { method: "PATCH", body: { status } });

export type ImportResult = { total: number; created: number; updated: number; failed: number; errors: { row: number; field?: string; code: string; message?: string }[]; generated?: { row: number; sku: string }[] };
export async function importExcel(file: File): Promise<ImportResult> {
  const res = await apiRaw("/v1/store/products/import", { method: "POST", rawBody: file, headers: { "Content-Type": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" } });
  return (await res.json()).data;
}
export async function downloadTemplate() {
  const res = await apiRaw("/v1/store/products/import/template");
  const url = URL.createObjectURL(await res.blob());
  const a = document.createElement("a");
  a.href = url;
  a.download = "products-template.xlsx";
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
export type BulkResult = { filename: string; product_id?: string; sku?: string; angle?: string; error?: string };
export const bulkImages = (items: { filename: string; key: string }[]) =>
  api<BulkResult[]>("/v1/store/products/images/bulk", { method: "POST", body: { items } });

export const getStoreProfile = () => api<Store & { logo_key?: string; cover_key?: string }>("/v1/store/profile");
export const updateStoreProfile = (body: unknown) => api<Store>("/v1/store/profile", { method: "PATCH", body });

export type StoreStats = {
  from: string; to: string; products_total: number; products_published: number; views: number; fitting_adds: number;
  tryons: number; orders: number; orders_delivered: number; revenue: number; views_all_time: number; fitting_adds_all_time: number;
  top_products: { id: string; title: string; brand: string; status: string; views: number; fitting_adds: number; ordered: number }[];
  daily: { day: string; views: number; fitting_adds: number }[];
};
export const getStoreStats = (from: string, to: string) => api<StoreStats>(`/v1/store/stats?from=${from}&to=${to}`);

// ParseImageName — как на сервере: "A-100_front.jpg" → sku, angle (для предпросмотра).
const ANGLES: Record<string, string> = { front: "front", f: "front", "1": "front", спереди: "front", side: "side", s: "side", "2": "side", сбоку: "side", back: "back", b: "back", "3": "back", сзади: "back" };
export function parseImageName(name: string): { sku: string; angle: string } | null {
  const base = name.replace(/\\/g, "/").split("/").pop()!.replace(/\.[^.]+$/, "");
  const i = base.lastIndexOf("_");
  if (i <= 0 || i === base.length - 1) return null;
  const angle = ANGLES[base.slice(i + 1).toLowerCase()];
  return angle ? { sku: base.slice(0, i).trim(), angle } : null;
}
