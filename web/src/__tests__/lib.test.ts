import { afterEach, describe, expect, it, vi } from "vitest";
import { parseItemsParam } from "@/lib/orders";
import { safeNext } from "@/lib/safeNext";
import { formatSum } from "@/lib/money";
import { api, ApiError, currentRole, setOnAuthLost, setSession } from "@/lib/api";

const ID = "3f1c2a9e-1b2c-4d5e-8f90-123456789abc";

describe("parseItemsParam", () => {
  it("accepts valid ids and qty, drops junk and duplicates", () => {
    expect(parseItemsParam(`${ID}:2,not-a-uuid:1,${ID}:3,${ID.replace("3f", "4f")}:0`)).toEqual([{ product_id: ID, qty: 2 }]);
    expect(parseItemsParam(null)).toEqual([]);
  });
});

describe("safeNext", () => {
  it("allows only internal paths", () => {
    expect(safeNext("/account/orders/1", "/x")).toBe("/account/orders/1");
    for (const bad of ["https://evil.com", "//evil.com", "/\\evil.com", "javascript:alert(1)", "/a\nb"]) {
      expect(safeNext(bad, "/x")).toBe("/x");
    }
  });
});

describe("formatSum", () => {
  it("converts tiyin to sum", () => {
    expect(formatSum(150000000, "ru")).toBe("1 500 000 сум");
    expect(formatSum(150000000, "uz")).toBe("1 500 000 so‘m");
  });
});

function jsonRes(status: number, body: unknown) {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

describe("api client", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    setSession(null);
    setOnAuthLost(null);
  });

  it("refreshes once on token_invalid and retries", async () => {
    setSession("old", "client");
    const calls: string[] = [];
    vi.stubGlobal("fetch", vi.fn(async (url: string, init: RequestInit) => {
      calls.push(`${url} ${(init.headers as Record<string, string>)?.Authorization ?? ""}`);
      if (url.endsWith("/v1/auth/refresh")) return jsonRes(200, { data: { access_token: "new", user: { id: "u", role: "client" } } });
      if ((init.headers as Record<string, string>).Authorization === "Bearer old") return jsonRes(401, { error: { code: "token_invalid" } });
      return jsonRes(200, { data: { ok: true } });
    }));
    await expect(api("/v1/orders")).resolves.toEqual({ ok: true });
    expect(calls).toHaveLength(3);
    expect(calls[2]).toContain("Bearer new");
    expect(currentRole()).toBe("client");
  });

  it("logs out on token_revoked and exposes request_id", async () => {
    setSession("t", "store");
    const lost = vi.fn();
    setOnAuthLost(lost);
    vi.stubGlobal("fetch", vi.fn(async () => jsonRes(401, { error: { code: "token_revoked", message: "x" }, request_id: "rid-1" })));
    const err = (await api("/v1/store/orders").catch((e: unknown) => e)) as ApiError;
    expect(err).toBeInstanceOf(ApiError);
    expect(err.requestId).toBe("rid-1");
    expect(lost).toHaveBeenCalled();
    expect(currentRole()).toBeNull();
  });
});
