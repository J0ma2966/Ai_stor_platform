import { api, apiList, apiRaw, newIdempotencyKey } from "@/lib/api";
import type { Zone } from "@/lib/fitting";

export type Angle = "front" | "side" | "back";
export const ANGLES: Angle[] = ["front", "side", "back"];

export type Quote = {
  cost: number;
  balance: number;
  enough: boolean;
  consent: boolean;
  registered: boolean;
  gender: "male" | "female" | null;
  active: number;
  max_active: number;
  supported_zones: Zone[];
  // Отдельное согласие на передачу фото внешнему AI-провайдеру (OpenAI и т.п.).
  transfer?: { required: boolean; recipient: string; consent: boolean };
  retention: { photo_hours: number; saved_photo_days: number; result_days: number; saved_look_days: number };
};

export type SavedPhoto = { id: string; angle: Angle; key: string; url: string; expires_at: string };

export type GenStatus = "queued" | "running" | "succeeded" | "failed";

export type Generation = {
  id: string;
  status: GenStatus;
  cost: number;
  error_code: string | null;
  refunded: boolean;
  saved: boolean;
  result_url: string | null;
  download_url?: string | null;
  result_expires_at: string | null;
  photos: { angle: Angle; key: string; url: string }[];
  products: {
    id: string; title: string; brand: string; price: number; zone: Zone; is_set?: boolean;
    image_url: string; store_name: string; store_slug: string; available: boolean;
  }[];
  body: { height_cm?: number; weight_kg?: number; params?: Record<string, number> };
  created_at: string;
  finished_at: string | null;
};

export type BodyInput = { height_cm?: number; weight_kg?: number; params?: Record<string, number> };

export const getQuote = () => api<Quote>("/v1/tryon/quote");
export const giveConsent = () => api<void>("/v1/tryon/consent", { method: "POST" });
export const giveTransferConsent = () => api<void>("/v1/tryon/consent/transfer", { method: "POST" });
export const revokeConsent = () => api<void>("/v1/tryon/consent", { method: "DELETE" });
export const listPhotos = async () => (await apiList<SavedPhoto>("/v1/tryon/photos")).items;
export const savePhoto = (angle: Angle, key: string) => api<SavedPhoto[]>(`/v1/tryon/photos/${angle}`, { method: "PUT", body: { key } });
export const deletePhoto = (angle: Angle) => api<void>(`/v1/tryon/photos/${angle}`, { method: "DELETE" });

export async function uploadBodyPhoto(file: Blob): Promise<{ key: string; url: string }> {
  const res = await apiRaw("/v1/uploads?purpose=body", { method: "POST", rawBody: file, headers: { "Content-Type": file.type || "image/jpeg" } });
  return (await res.json()).data;
}

export type CreateInput = {
  product_ids: string[];
  photos: Record<Angle, string>;
  save_photos: boolean;
  body?: BodyInput;
};

// Ключ идемпотентности создаёт вызывающий и переиспользует при повторе после сетевой
// ошибки — так повторное нажатие не спишет токены дважды.
export const createGeneration = (input: CreateInput, idemKey: string = newIdempotencyKey()) =>
  api<Generation>("/v1/tryon/generations", { method: "POST", body: input, headers: { "Idempotency-Key": idemKey } });

export const getGeneration = (id: string) => api<Generation>(`/v1/tryon/generations/${encodeURIComponent(id)}`);
export const listGenerations = (saved: boolean, cursor = "") =>
  apiList<Generation>(`/v1/tryon/generations?limit=12${saved ? "&saved=true" : ""}${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ""}`);
export const setSaved = (id: string, saved: boolean) =>
  api<Generation>(`/v1/tryon/generations/${encodeURIComponent(id)}/save`, { method: saved ? "POST" : "DELETE" });
export const deleteGeneration = (id: string) => api<void>(`/v1/tryon/generations/${encodeURIComponent(id)}`, { method: "DELETE" });

export const isActive = (s: GenStatus) => s === "queued" || s === "running";
