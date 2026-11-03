// Корзина, избранное, уведомления (только для вошедших; клиентская сторона).
import { api, apiList } from "./api";

export type CartItem = {
  product_id: string;
  title: string;
  brand: string;
  price: number;
  store_id: string;
  store_name: string;
  category_id: string;
  zone: string;
  image_url?: string;
  in_stock: boolean;
  available: boolean;
  unavailable_reason?: "out_of_stock" | "unavailable";
  qty: number;
  added_at: string;
};
export type Cart = { items: CartItem[]; total_qty: number; total: number; can_checkout: boolean; has_unavailable: boolean };

export const getCart = () => api<Cart>("/v1/me/cart");
export const setCartQty = (productId: string, qty: number) => api<Cart>(`/v1/me/cart/${productId}`, { method: "PUT", body: { qty } });
export const removeFromCart = (productId: string) => api<Cart>(`/v1/me/cart/${productId}`, { method: "DELETE" });

export const getFavorites = () => apiList<CartItem>("/v1/me/favorites");
export const getFavoriteIds = () => api<string[]>("/v1/me/favorites/ids");
export const addFavorite = (id: string) => api<void>(`/v1/me/favorites/${id}`, { method: "PUT" });
export const removeFavorite = (id: string) => api<void>(`/v1/me/favorites/${id}`, { method: "DELETE" });

export type Notification = { id: number; kind: string; data: Record<string, unknown>; read: boolean; created_at: string };
export const getNotifications = (cursor = "") => apiList<Notification>(`/v1/me/notifications${cursor ? `?cursor=${cursor}` : ""}`);
export const getUnread = () => api<{ unread: number }>("/v1/me/notifications/unread-count");
export const markRead = (ids: number[] | "all") =>
  api<void>("/v1/me/notifications/read", { method: "POST", body: ids === "all" ? { all: true } : { ids } });

// Ссылка, куда ведёт уведомление (по виду и роли).
export function notificationHref(n: Notification, role: string): string | null {
  const d = n.data as Record<string, string>;
  switch (n.kind) {
    case "order_created":
      return d.order_id ? `/store/orders/${d.order_id}` : "/store/orders";
    case "order_status":
      return d.order_id ? `/account/orders/${d.order_id}` : "/account/orders";
    case "product_moderated":
      return d.product_id ? `/store/products/${d.product_id}` : "/store/products";
    case "payment_paid":
    case "payment_failed":
      return "/account/balance";
    case "generation_succeeded":
    case "generation_failed":
      return d.generation_id ? `/account/tryons/${d.generation_id}` : "/account/tryons";
    default:
      return role === "store" ? "/store" : null;
  }
}
