"use client";
import { useTranslations } from "next-intl";
import { CUSTOMER_STEPS, type OrderStatus } from "@/lib/orders";

const tone: Record<OrderStatus, string> = {
  pending: "warn", confirmed: "info", assembling: "info", in_delivery: "info",
  delivered: "ok", rejected: "bad", cancelled: "bad",
};

export function StatusBadge({ status }: { status: OrderStatus }) {
  const t = useTranslations("status");
  return <span className={`badge badge-${tone[status]}`}>{t(status)}</span>;
}

// Шкала для клиента: подтверждён → собирается → передан в доставку → доставлен.
export function StatusStepper({ status, step }: { status: OrderStatus; step: number }) {
  const t = useTranslations("status");
  if (status === "rejected" || status === "cancelled") return null;
  return (
    <ol className="stepper" aria-label={t(status)}>
      {CUSTOMER_STEPS.map((s, i) => {
        const state = i + 1 < step ? "done" : i + 1 === step ? "current" : "todo";
        return (
          <li key={s} className={`step step-${state}`} aria-current={state === "current" ? "step" : undefined}>
            <span className="dot" aria-hidden="true">{state === "done" ? "✓" : i + 1}</span>
            <span>{t(s)}</span>
          </li>
        );
      })}
    </ol>
  );
}
