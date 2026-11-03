"use client";
import { useTranslations } from "next-intl";

export default function ErrorPage({ reset }: { error: Error; reset: () => void }) {
  const t = useTranslations();
  return (
    <div role="alert" className="card narrow stack center-text">
      <h1>{t("errorPage.title")}</h1>
      <p className="muted">{t("errorPage.text")}</p>
      <button type="button" className="btn btn-primary" onClick={reset}>{t("app.retry")}</button>
    </div>
  );
}
