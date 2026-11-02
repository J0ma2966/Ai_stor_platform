import type { NextConfig } from "next";
import createNextIntlPlugin from "next-intl/plugin";

const withNextIntl = createNextIntlPlugin("./src/i18n/request.ts");

const apiOrigin = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";
const mediaOrigin = process.env.NEXT_PUBLIC_MEDIA_ORIGIN ?? "http://localhost:9000";

// CSP: только свои скрипты, API и хранилище картинок. Без inline-скриптов от третьих лиц.
const csp = [
  "default-src 'self'",
  `connect-src 'self' ${apiOrigin}`,
  `img-src 'self' data: blob: ${mediaOrigin}`,
  "script-src 'self' 'unsafe-inline'" + (process.env.NODE_ENV === "development" ? " 'unsafe-eval'" : ""),
  "style-src 'self' 'unsafe-inline'",
  "frame-ancestors 'none'",
  "base-uri 'self'",
  "form-action 'self'",
].join("; ");

const config: NextConfig = {
  reactStrictMode: true,
  poweredByHeader: false,
  output: "standalone",
  async headers() {
    return [
      {
        source: "/:path*",
        headers: [
          { key: "Content-Security-Policy", value: csp },
          { key: "X-Content-Type-Options", value: "nosniff" },
          { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
          { key: "Permissions-Policy", value: "camera=(), microphone=(), geolocation=(self)" },
        ],
      },
    ];
  },
};

export default withNextIntl(config);
