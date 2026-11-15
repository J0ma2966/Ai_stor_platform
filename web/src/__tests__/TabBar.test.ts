import { describe, expect, it } from "vitest";
import { isTabRoot } from "@/components/TabBar";

describe("TabBar", () => {
  it("knows which screens are tabs (no Telegram back button there)", () => {
    for (const p of ["/", "/catalog", "/cart", "/account", "/account/orders", "/catalog/"]) expect(isTabRoot(p)).toBe(true);
    for (const p of ["/stores", "/product/x", "/account/orders/123", "/checkout", "/login"]) expect(isTabRoot(p)).toBe(false);
  });
});
