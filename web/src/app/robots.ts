import type { MetadataRoute } from "next";
import { siteUrl } from "@/lib/site";

export default function robots(): MetadataRoute.Robots {
  const closed = ["account", "store", "admin", "cart", "checkout", "login", "fitting"].flatMap((p) => [`/${p}`, `/uz/${p}`]);
  return {
    rules: [{ userAgent: "*", allow: "/", disallow: closed }],
    sitemap: `${siteUrl()}/sitemap.xml`,
    host: siteUrl(),
  };
}
