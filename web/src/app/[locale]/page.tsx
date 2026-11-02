import { redirect } from "@/i18n/navigation";

// Главную и каталог делает отдельная задача; пока ведём в кабинет заказов.
export default async function Home({ params }: { params: Promise<{ locale: string }> }) {
  const { locale } = await params;
  redirect({ href: "/account/orders", locale });
}
