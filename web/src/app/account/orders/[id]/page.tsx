"use client";
import { use } from "react";
import { AuthGate } from "@/components/AuthGate";
import { ClientOrderView } from "@/components/ClientOrderView";

export default function MyOrderPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  return (
    <AuthGate role="client">
      <ClientOrderView id={id} />
    </AuthGate>
  );
}
