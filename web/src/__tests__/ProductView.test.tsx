import { afterEach, describe, expect, it, vi } from "vitest";
import { render } from "@testing-library/react";
import { ProductView } from "@/components/ProductView";

describe("ProductView", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    sessionStorage.clear();
  });
  it("marks a view once per tab session", () => {
    const f = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(null, { status: 204 }));
    const id = "3f1c2a9e-1b2c-4d5e-8f90-123456789abc";
    render(<ProductView id={id} />).unmount();
    render(<ProductView id={id} />).unmount();
    expect(f).toHaveBeenCalledTimes(1);
    expect(String(f.mock.calls[0][0])).toMatch(new RegExp(`/v1/products/${id}/view$`));
    expect(f.mock.calls[0][1]).toMatchObject({ method: "POST" });
  });
});
