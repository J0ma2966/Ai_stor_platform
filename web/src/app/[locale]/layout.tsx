import { Suspense, type ReactNode } from "react";
import { notFound } from "next/navigation";
import { NextIntlClientProvider, hasLocale } from "next-intl";
import { getTranslations, setRequestLocale } from "next-intl/server";
import { headers } from "next/headers";
import { routing } from "@/i18n/routing";
import { Header } from "@/components/Header";
import "../globals.css";

export async function generateMetadata({ params }: { params: Promise<{ locale: string }> }) {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "app" });
  return { title: t("name") };
}

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
          <Header />
          <main className="container">
            <Suspense>{children}</Suspense>
          </main>
        </NextIntlClientProvider>
      </body>
    </html>
  );
}
