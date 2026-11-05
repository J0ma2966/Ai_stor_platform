import { afterEach, describe, expect, it, vi } from "vitest";
import { cardImage, serverHeaders } from "@/lib/catalog";

describe("cardImage", () => {
  it("prefers the 800 px copy and falls back to the large photo", () => {
    expect(cardImage({ images: { front: "L" }, thumbs: { front: "T" } })).toBe("T");
    expect(cardImage({ images: { side: "L" } })).toBe("L");
    expect(cardImage({ images: {}, thumbs: {} })).toBe("");
  });
});

describe("serverHeaders", () => {
  afterEach(() => vi.unstubAllEnvs());
  it("never sends the internal token from the browser", () => {
    vi.stubEnv("INTERNAL_API_TOKEN", "secret-secret-secret-secret-secret-1");
    // тесты идут в jsdom: window есть, как в браузере
    expect(serverHeaders()).toEqual({ Accept: "application/json" });
  });
});
