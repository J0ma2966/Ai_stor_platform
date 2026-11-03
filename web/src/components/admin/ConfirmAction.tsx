"use client";
import { useId, useState, type ReactNode } from "react";
import { useTranslations } from "next-intl";

// Кнопка опасного действия: сначала раскрывает подтверждение (и при необходимости
// поле причины), действие выполняется только по второй кнопке. Без window.confirm.
export function ConfirmAction({
  label,
  confirmLabel,
  danger,
  needReason,
  busy,
  onConfirm,
  children,
}: {
  label: string;
  confirmLabel?: string;
  danger?: boolean;
  needReason?: boolean;
  busy?: boolean;
  onConfirm: (reason: string) => Promise<unknown> | void;
  children?: ReactNode;
}) {
  const t = useTranslations("admin");
  const id = useId();
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");
  const tooShort = needReason && reason.trim().length < 3;

  if (!open) {
    return (
      <button type="button" className={danger ? "btn btn-danger" : "btn btn-secondary"} onClick={() => setOpen(true)} disabled={busy}>
        {label}
      </button>
    );
  }
  return (
    <div className="card confirm-box stack">
      {children}
      {needReason && (
        <div className="field">
          <label htmlFor={id}>{t("reason")}</label>
          <textarea id={id} value={reason} onChange={(e) => setReason(e.target.value)} maxLength={500} rows={2} />
          <span className="small muted">{t("reasonHint")}</span>
        </div>
      )}
      <div className="row wrap">
        <button
          type="button"
          className={danger ? "btn btn-danger" : "btn btn-primary"}
          disabled={busy || tooShort}
          onClick={async () => {
            await onConfirm(reason.trim());
            setOpen(false);
            setReason("");
          }}
        >
          {confirmLabel ?? label}
        </button>
        <button type="button" className="btn btn-secondary" onClick={() => setOpen(false)} disabled={busy}>
          {t("cancel")}
        </button>
      </div>
    </div>
  );
}
