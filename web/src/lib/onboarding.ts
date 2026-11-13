import type { Me } from "./session";

// Куда вести клиента после входа: сначала обязательные шаги регистрации
// (пол и согласие; пароль — только тем, кто вошёл по SMS), потом — туда, куда он шёл.
export function clientNextStep(me: Pick<Me, "role" | "registration_complete" | "needs_password">, next: string): string {
  if (me.role !== "client") return next;
  const q = `?next=${encodeURIComponent(next)}`;
  if (!me.registration_complete) return `/account/welcome${q}`;
  if (me.needs_password) return `/account/password${q}`;
  return next;
}
