import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { setRequestLocale } from "next-intl/server";
import { LEGAL_KEYS, legalDoc, type LegalKey } from "@/content/legal";
import { alternates } from "@/lib/site";
import { LegalPage } from "@/components/LegalPage";

type P = Promise<{ locale: string; doc: string }>;
const isKey = (d: string): d is LegalKey => (LEGAL_KEYS as string[]).includes(d);

export async function generateMetadata({ params }: { params: P }): Promise<Metadata> {
  const { locale, doc } = await params;
  if (!isKey(doc)) return {};
  return { title: legalDoc(locale, doc).title, alternates: alternates(locale, `/legal/${doc}`) };
}

export default async function Page({ params }: { params: P }) {
  const { locale, doc } = await params;
  if (!isKey(doc)) notFound();
  setRequestLocale(locale);
  return <LegalPage locale={locale} doc={doc} />;
}
