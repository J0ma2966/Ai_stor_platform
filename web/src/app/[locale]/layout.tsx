import { Suspense, type ReactNode } from "react";
import type { Metadata, Viewport } from "next";
import { notFound } from "next/navigation";
import { NextIntlClientProvider, hasLocale } from "next-intl";
import { getTranslations, setRequestLocale } from "next-intl/server";
import { headers } from "next/headers";
import { routing } from "@/i18n/routing";
import { Header } from "@/components/Header";
import { Footer } from "@/components/Footer";
import { SessionProvider } from "@/lib/session";
import { alternates, siteUrl } from "@/lib/site";
import "../globals.css";

export async function generateMetadata({ params }: { params: Promise<{ locale: string }> }): Promise<Metadata> {
  const { locale } = await params;
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

export default async function LocaleLayout({ children, params }: { children: ReactNode; params: Promise<{ locale: string }> }) {
  const { locale } = await params;
  if (!hasLocale(routing.locales, locale)) notFound();
  setRequestLocale(locale);
  // страницы рендерятся на каждый запрос: иначе в статическом HTML не будет nonce для CSP
  await headers();
  return (
    <html lang={locale}>
      <body>
        <NextIntlClientProvider>
          <SessionProvider>
            <Suspense fallback={<div className="site-header" />}>
              <Header />
            </Suspense>
            <main id="main" className="container main">
              <Suspense>{children}</Suspense>
            </main>
            <Footer />
          </SessionProvider>
        </NextIntlClientProvider>
      </body>
    </html>
  );
}
