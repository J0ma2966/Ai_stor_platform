import { afterEach, describe, expect, it, vi } from "vitest";
import { itemsParam, parseItemsParam } from "@/lib/orders";
import { safeNext } from "@/lib/safeNext";
import { formatSum } from "@/lib/money";
import { api, ApiError, currentRole, setOnAuthLost, setSession, withRefreshLock } from "@/lib/api";

const ID = "3f1c2a9e-1b2c-4d5e-8f90-123456789abc";

describe("parseItemsParam", () => {
  it("accepts valid ids and qty, drops junk and duplicates", () => {
    expect(parseItemsParam(`${ID}:2,not-a-uuid:1,${ID}:3,${ID.replace("3f", "4f")}:0`)).toEqual([{ product_id: ID, qty: 2 }]);
    expect(parseItemsParam(null)).toEqual([]);
  });
  it("keeps size per item: same product in two sizes, round-trips through the URL", () => {
    const items = [{ product_id: ID, qty: 1, size: "M" }, { product_id: ID, qty: 2, size: "38/40" }, { product_id: ID, qty: 1, size: "M" }];
    expect(parseItemsParam(itemsParam(items))).toEqual(items.slice(0, 2));
    expect(parseItemsParam(`${ID}:1:%E0%A4%A`)).toEqual([]);
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
    expect(formatSum(150000000, "ru")).toBe("1\u00a0500\u00a0000\u00a0сум");
    expect(formatSum(150000000, "uz")).toBe("1\u00a0500\u00a0000\u00a0so‘m");
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

  it("serializes refresh across tabs with the Web Locks API", async () => {
    // очередь блокировки, общая для «вкладок»
    let tail = Promise.resolve();
    const request = vi.fn((_name: string, cb: () => Promise<unknown>) => {
      const run = tail.then(cb);
      tail = run.then(() => undefined, () => undefined);
      return run;
    });
    vi.stubGlobal("navigator", { locks: { request } });
    let active = 0;
    let maxActive = 0;
    const order: string[] = [];
    const tab = (name: string) =>
      withRefreshLock(async () => {
        active++;
        maxActive = Math.max(maxActive, active);
        order.push(`${name}:start`);
        await new Promise((r) => setTimeout(r, 5));
        order.push(`${name}:end`);
        active--;
      });
    await Promise.all([tab("a"), tab("b")]);
    expect(request).toHaveBeenCalledTimes(2);
    expect(request.mock.calls[0][0]).toBe("boutique-auth-refresh");
    expect(maxActive).toBe(1);
    expect(order).toEqual(["a:start", "a:end", "b:start", "b:end"]);
    vi.unstubAllGlobals();
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

import { clientNextStep } from "@/lib/onboarding";

describe("clientNextStep", () => {
  const base = { role: "client" as const, registration_complete: true, needs_password: false };
  it("ведёт нового клиента на завершение регистрации, затем на пароль (только после SMS)", () => {
    expect(clientNextStep({ ...base, registration_complete: false, needs_password: true }, "/checkout")).toBe("/account/welcome?next=%2Fcheckout");
    expect(clientNextStep({ ...base, needs_password: true }, "/checkout")).toBe("/account/password?next=%2Fcheckout");
    expect(clientNextStep(base, "/checkout")).toBe("/checkout");
  });
  it("не трогает сотрудников", () => {
    expect(clientNextStep({ ...base, role: "store", needs_password: true }, "/store")).toBe("/store");
  });
});
