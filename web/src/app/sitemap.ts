import type { MetadataRoute } from "next";
import { apiBase, type Category, type Product, type Store } from "@/lib/catalog";
import { siteUrl } from "@/lib/site";

export const revalidate = 3600;
const MAX_PRODUCTS = 45000; // лимит одного sitemap — 50 000 адресов

async function get<T>(path: string): Promise<{ data: T; next_cursor?: string } | null> {
  try {
    const r = await fetch(apiBase() + path, { next: { revalidate } });
    return r.ok ? r.json() : null;
  } catch {
    return null;
  }
}

export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const base = siteUrl();
  const both = (path: string, extra: Partial<MetadataRoute.Sitemap[number]> = {}): MetadataRoute.Sitemap[number] => ({
    url: `${base}/ru${path}`,
    alternates: { languages: { ru: `${base}/ru${path}`, uz: `${base}/uz${path}` } },
    ...extra,
  });
  const out: MetadataRoute.Sitemap = [
    both("", { changeFrequency: "daily", priority: 1 }),
    both("/catalog", { changeFrequency: "hourly", priority: 0.9 }),
    both("/stores", { changeFrequency: "daily", priority: 0.7 }),
    both("/about"), both("/contacts"), both("/legal/terms"), both("/legal/privacy"), both("/legal/photos"),
  ];
  const cats = await get<Category[]>("/v1/categories");
  for (const c of cats?.data ?? []) out.push(both(`/catalog?category=${c.id}`, { changeFrequency: "daily", priority: 0.8 }));
  const stores = await get<Store[]>("/v1/stores?limit=200");
  for (const s of stores?.data ?? []) out.push(both(`/stores/${s.slug}`, { changeFrequency: "daily", priority: 0.7 }));
  let cursor = "";
  let n = 0;
  do {
    const page = await get<Product[]>(`/v1/products?sort=new&limit=60${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ""}`);
    if (!page) break;
    for (const p of page.data ?? []) {
      out.push(both(`/product/${p.id}`, { lastModified: p.created_at, changeFrequency: "weekly", priority: 0.6 }));
      n++;
    }
    cursor = page.next_cursor ?? "";
  } while (cursor && n < MAX_PRODUCTS);
  return out;
}
