"use client";
import type { ReactNode } from "react";
import { AuthGate } from "@/components/AuthGate";
import { AdminNav } from "@/components/admin/AdminNav";

// Все страницы панели (кроме входа) — только для роли admin.
export default function AdminPanelLayout({ children }: { children: ReactNode }) {
  return (
    <AuthGate role="admin">
      <div className="admin-shell">
        <AdminNav />
        <div className="admin-main stack">{children}</div>
      </div>
    </AuthGate>
  );
}
