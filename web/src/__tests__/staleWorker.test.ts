import { afterEach, describe, expect, it, vi } from "vitest";
import { STALE_WORKER_SCRIPT } from "@/lib/staleWorker";

type Reg = { unregister: ReturnType<typeof vi.fn> };

function run(regs: Reg[], controlled: boolean) {
  const reload = vi.fn();
  const del = vi.fn().mockResolvedValue(true);
  const sw = { controller: controlled ? {} : null, getRegistrations: vi.fn().mockResolvedValue(regs) };
  vi.stubGlobal("navigator", { serviceWorker: sw });
  vi.stubGlobal("caches", { keys: vi.fn().mockResolvedValue(["workbox-precache", "next-static"]), delete: del });
  vi.stubGlobal("location", { reload });
  new Function(STALE_WORKER_SCRIPT)();
  return { reload, del, sw };
}

const flush = () => new Promise((r) => setTimeout(r, 0));

describe("stale service worker guard", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("does nothing when no service worker is registered", async () => {
    const { reload, del } = run([], false);
    await flush();
    expect(del).not.toHaveBeenCalled();
    expect(reload).not.toHaveBeenCalled();
  });

  it("unregisters a foreign worker, clears its caches and reloads a controlled page once", async () => {
    const reg = { unregister: vi.fn().mockResolvedValue(true) };
    const { reload, del } = run([reg], true);
    await flush();
    expect(reg.unregister).toHaveBeenCalledOnce();
    expect(del).toHaveBeenCalledTimes(2);
    expect(reload).toHaveBeenCalledOnce();
  });

  it("does not reload a page the worker did not control", async () => {
    const reg = { unregister: vi.fn().mockResolvedValue(true) };
    const { reload } = run([reg], false);
    await flush();
    expect(reg.unregister).toHaveBeenCalledOnce();
    expect(reload).not.toHaveBeenCalled();
  });

  it("is a no-op where service workers are unsupported", () => {
    vi.stubGlobal("navigator", {});
    expect(() => new Function(STALE_WORKER_SCRIPT)()).not.toThrow();
  });
});

describe("stale worker script text", () => {
  it("has nothing React would escape inside <script>", () => {
    expect(STALE_WORKER_SCRIPT).not.toMatch(/[<>&"']/);
  });
});
