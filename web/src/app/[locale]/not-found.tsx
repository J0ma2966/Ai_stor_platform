import { getTranslations } from "next-intl/server";
import { Link } from "@/i18n/navigation";

export default async function NotFound() {
  const t = await getTranslations("notFound");
  return (
    <div className="card narrow stack center-text">
      <h1>{t("title")}</h1>
      <p className="muted">{t("text")}</p>
      <Link href="/" className="btn btn-primary">{t("home")}</Link>
    </div>
  );
}
