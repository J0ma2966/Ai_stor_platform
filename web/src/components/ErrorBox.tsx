"use client";
import { useTranslations } from "next-intl";
import { ApiError } from "@/lib/api";

export function useErrorText() {
  const t = useTranslations("errors");
  return (e: unknown): string => {
    if (e instanceof ApiError) {
      return t.has(e.code) ? t(e.code) : t("unknown", { code: e.code });
    }
    return t("internal");
  };
}

export function useFieldError() {
  const t = useTranslations("fieldErrors");
  return (code: string | undefined) => (code ? (t.has(code) ? t(code) : code) : undefined);
}

export function ErrorBox({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  const t = useTranslations();
  const text = useErrorText();
  if (!error) return null;
  const rid = error instanceof ApiError ? error.requestId : "";
  return (
    <div role="alert" className="alert">
      <p>{text(error)}</p>
      {rid && <p className="muted small">{t("errors.requestId", { id: rid })}</p>}
      {onRetry && (
        <button type="button" className="btn btn-secondary" onClick={onRetry}>
          {t("app.retry")}
        </button>
      )}
    </div>
  );
}
