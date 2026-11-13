import { describe, expect, it, vi, afterEach } from "vitest";
import { buildCsp, newNonce } from "@/lib/csp";

describe("csp", () => {
  afterEach(() => vi.unstubAllEnvs());
  it("uses nonce + strict-dynamic, no unsafe-inline for scripts", () => {
    const csp = buildCsp("abc", false);
    const script = csp.split("; ").find((d) => d.startsWith("script-src"))!;
    expect(script).toBe("script-src 'self' 'nonce-abc' 'strict-dynamic'");
    expect(csp).toContain("object-src 'none'");
  });
  it("accepts a list of media origins and drops junk", () => {
    vi.stubEnv("NEXT_PUBLIC_MEDIA_ORIGIN", "https://cdn.example.com, https://media.example.uz; script-src *");
    const img = buildCsp("n", false).split("; ").find((d) => d.startsWith("img-src"))!;
    expect(img).toBe("img-src 'self' data: blob: https://cdn.example.com https://media.example.uz");
  });
  it("allows only Google Identity Services for the Google button", () => {
    const csp = buildCsp("n", false);
    expect(csp).toContain("frame-src https://accounts.google.com/gsi/");
    expect(csp).toContain("https://accounts.google.com/gsi/style");
    expect(csp).not.toContain("frame-src *");
  });
  it("generates unique nonces", () => {
    expect(newNonce()).not.toBe(newNonce());
  });
});
