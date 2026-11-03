import { Suspense, type ReactNode } from "react";
import type { Metadata, Viewport } from "next";
import { NextIntlClientProvider } from "next-intl";
import { getLocale, getTranslations } from "next-intl/server";
import { headers } from "next/headers";
import { LocaleSync } from "@/i18n/switch";
import { Header } from "@/components/Header";
import { Footer } from "@/components/Footer";
import { StaleWorkerGuard } from "@/components/StaleWorkerGuard";
import { SessionProvider } from "@/lib/session";
import { alternates, siteUrl } from "@/lib/site";
import "./globals.css";

export async function generateMetadata(): Promise<Metadata> {
  const locale = await getLocale();
  const t = await getTranslations({ locale, namespace: "seo" });
  return {
    metadataBase: new URL(siteUrl()),
    title: { default: t("homeTitle"), template: `%s — Butik` },
    description: t("homeDesc"),
    alternates: alternates(locale, "/"),
    openGraph: { type: "website", siteName: "Butik", locale: locale === "uz" ? "uz_UZ" : "ru_RU", title: t("homeTitle"), description: t("homeDesc") },
    robots: { index: true, follow: true },
  };
}

export const viewport: Viewport = { width: "device-width", initialScale: 1, themeColor: "#5b2a86" };

// Корневой layout общий для обоих языков: при смене языка он не пересоздаётся,
// а только получает новые сообщения (см. src/i18n/switch.tsx).
export default async function RootLayout({ children }: { children: ReactNode }) {
  const locale = await getLocale();
  // страницы рендерятся на каждый запрос: иначе в статическом HTML не будет nonce для CSP
  await headers();
  return (
    <html lang={locale}>
      <head>
        <StaleWorkerGuard />
      </head>
      <body>
        <NextIntlClientProvider>
          <LocaleSync />
          <SessionProvider>
            <Suspense fallback={<div className="site-header" />}>
              <Header />
            </Suspense>
            <main id="main" className="container main">
              {children}
            </main>
            <Footer />
          </SessionProvider>
        </NextIntlClientProvider>
      </body>
    </html>
  );
}
