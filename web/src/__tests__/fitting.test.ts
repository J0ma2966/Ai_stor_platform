import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { addToFitting, clearFitting, FITTING_EVENT, fittingItems, getFitting, removeFromFitting, type FittingItem } from "@/lib/fitting";

const item = (id: string, zone: FittingItem["zone"]): FittingItem => ({ id, title: "T" + id, brand: "B", price: 100, image_url: "", zone });

describe("fitting store", () => {
  const fetchMock = vi.fn(async () => new Response(null, { status: 204 }));
  beforeEach(() => {
    localStorage.clear();
    vi.stubGlobal("fetch", fetchMock);
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    fetchMock.mockClear();
  });

  it("keeps one item per zone and orders by mannequin zones", () => {
    addToFitting(item("shoe", "shoes"));
    addToFitting(item("t1", "top"));
    addToFitting(item("t2", "top"));
    addToFitting(item("hat", "headwear"));
    expect(fittingItems(getFitting()).map((i) => i.id)).toEqual(["hat", "t2", "shoe"]);
    removeFromFitting("shoes");
    expect(getFitting().shoes).toBeUndefined();
    clearFitting();
    expect(getFitting()).toEqual({});
  });

  it("counts an add once and notifies listeners", () => {
    const seen: unknown[] = [];
    const h = (e: Event) => seen.push((e as CustomEvent).detail);
    window.addEventListener(FITTING_EVENT, h);
    addToFitting(item("t1", "top"));
    addToFitting(item("t1", "top"));
    window.removeEventListener(FITTING_EVENT, h);
    expect(seen).toHaveLength(2);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toMatch(/\/v1\/fitting\/adds$/);
    expect(JSON.parse(init.body as string)).toEqual({ product_id: "t1" });
  });

  it("ignores corrupted or tampered storage", () => {
    localStorage.setItem("fitting_v1", "{not json");
    expect(getFitting()).toEqual({});
    localStorage.setItem("fitting_v1", JSON.stringify({ top: { id: "x", title: "y", zone: "shoes" }, evil: { id: 1 } }));
    expect(getFitting()).toEqual({});
  });
});
