import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import messages from "../../messages/ru.json";

vi.mock("@/i18n/navigation", () => ({
  Link: (p: { href: string; children: React.ReactNode }) => <a href={p.href}>{p.children}</a>,
}));

import { TryOnGate } from "@/components/TryOnGate";

const wrap = (enabled: boolean) =>
  render(
    <NextIntlClientProvider locale="ru" messages={messages}>
      <TryOnGate enabled={enabled}><p>room</p></TryOnGate>
    </NextIntlClientProvider>,
  );

describe("TryOnGate", () => {
  it("shows the room when try-on is on", () => {
    wrap(true);
    expect(screen.getByText("room")).toBeTruthy();
  });
  it("shows a stub instead of the room when try-on is off", () => {
    wrap(false);
    expect(screen.queryByText("room")).toBeNull();
    expect(screen.getByTestId("tryon-disabled").textContent).toContain("Примерка пока недоступна");
  });
});

describe("addToFitting when try-on is off", () => {
  it("does nothing", async () => {
    vi.resetModules();
    vi.stubEnv("NEXT_PUBLIC_TRYON_ENABLED", "false");
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response("{}"));
    const f = await import("@/lib/fitting");
    f.addToFitting({ id: "p1", title: "T", brand: "B", price: 1, image_url: "", zone: "top" });
    expect(f.TRYON_ENABLED).toBe(false);
    expect(f.getFitting()).toEqual({});
    expect(fetchSpy).not.toHaveBeenCalled();
    vi.unstubAllEnvs();
    fetchSpy.mockRestore();
  });
});
