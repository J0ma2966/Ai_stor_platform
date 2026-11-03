"use client";
import { useSearchParams } from "next/navigation";
import { AuthGate } from "@/components/AuthGate";
import { BalancePage } from "@/components/BalancePage";

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export default function Balance() {
  const sp = useSearchParams();
  const pid = sp.get("payment") ?? "";
  return (
    <AuthGate role="client">
      <BalancePage returnedPaymentId={UUID.test(pid) ? pid : undefined} />
    </AuthGate>
  );
}
