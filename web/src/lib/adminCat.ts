import { api, apiList } from "./api";

export type AdminProduct = {
  id: string; store_id: string; store_name: string; sku: string; title: string; brand: string; price: number; in_stock: boolean;
  status: "draft" | "published" | "unpublished" | "blocked"; moderation_reason: string | null; image_url?: string; views: number; created_at: string;
};
export function listAdminProducts(f: { q?: string; status?: string; store?: string; cursor?: string }) {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(f)) if (v) p.set(k, v);
  return apiList<AdminProduct>(`/v1/admin/products${p.size ? `?${p}` : ""}`);
}
export const moderateProduct = (id: string, status: "unpublished" | "blocked" | "draft", reason: string) =>
  api<{ status: string }>(`/v1/admin/products/${id}/status`, { method: "POST", body: { status, reason } });

export type AdminCategory = { id: string; slug: string; parent_id: string | null; zone: string; name_ru: string; name_uz: string; sort: number; is_active: boolean; products: number };
export const listAdminCategories = () => apiList<AdminCategory>("/v1/admin/categories");
export const createCategory = (b: Partial<AdminCategory>) => api<{ id: string }>("/v1/admin/categories", { method: "POST", body: b });
export const updateCategory = (id: string, b: Partial<AdminCategory>) => api<{ id: string }>(`/v1/admin/categories/${id}`, { method: "PATCH", body: b });

export type Settings = { generation_cost?: number; welcome_bonus?: number; support_phone?: string; support_email?: string; support_telegram?: string };
export const getSettings = () => api<Settings>("/v1/admin/settings");
export const putSettings = (s: Settings) => api<Settings>("/v1/admin/settings", { method: "PUT", body: s });
