import type { Metadata } from "next";
import { getLocale } from "next-intl/server";
import { legalDoc } from "@/content/legal";
import { alternates } from "@/lib/site";
import { LegalPage } from "@/components/LegalPage";


export async function generateMetadata(): Promise<Metadata> {
  const locale = await getLocale();
  return { title: legalDoc(locale, "contacts").title, alternates: alternates(locale, "/contacts") };
}

export default async function Page() {
  const locale = await getLocale();
  return <LegalPage locale={locale} doc="contacts" />;
}
