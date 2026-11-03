import type { Metadata } from "next";
import { setRequestLocale } from "next-intl/server";
import { legalDoc } from "@/content/legal";
import { alternates } from "@/lib/site";
import { LegalPage } from "@/components/LegalPage";

type P = Promise<{ locale: string }>;

export async function generateMetadata({ params }: { params: P }): Promise<Metadata> {
  const { locale } = await params;
  return { title: legalDoc(locale, "about").title, alternates: alternates(locale, "/about") };
}

export default async function Page({ params }: { params: P }) {
  const { locale } = await params;
  setRequestLocale(locale);
  return <LegalPage locale={locale} doc="about" />;
}
