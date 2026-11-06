import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import messages from "../../messages/ru.json";

vi.mock("@/i18n/navigation", () => ({
  Link: (p: { href: string; children: React.ReactNode }) => <a href={p.href}>{p.children}</a>,
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
}));
let role: string | null = "client";
vi.mock("@/lib/api", async (orig) => ({
  ...(await orig<typeof import("@/lib/api")>()),
  currentRole: () => role,
  refreshSession: async () => false,
}));
vi.mock("@/lib/tryon", async (orig) => ({ ...(await orig<typeof import("@/lib/tryon")>()), getQuote: async () => null }));
const getCart = vi.fn();
vi.mock("@/lib/shop", async (orig) => ({ ...(await orig<typeof import("@/lib/shop")>()), getCart: () => getCart() }));

import { FittingRoom } from "@/components/FittingRoom";

const renderIt = () =>
  render(
    <NextIntlClientProvider locale="ru" messages={messages} timeZone="Asia/Tashkent">
      <FittingRoom />
    </NextIntlClientProvider>,
  );

const ci = (id: string, title: string, zone: string, extra: Record<string, unknown> = {}) => ({
  product_id: id, title, brand: "A", price: 100, store_id: "s", store_name: "S", category_id: "c", zone,
  in_stock: true, has_sizes: false, available: true, qty: 1, added_at: "2026-11-06T00:00:00Z", ...extra,
});

describe("FittingRoom: вещи из корзины", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    role = "client";
    vi.stubGlobal("fetch", vi.fn(async () => new Response(null, { status: 204 })));
    getCart.mockResolvedValue({
      items: [
        ci("suit", "Костюм", "top", { is_set: true }),
        ci("shirt", "Рубашка", "top", { size: "M" }),
        ci("shirt", "Рубашка", "top", { size: "L" }), // тот же товар в другом размере — один раз
        ci("jeans", "Джинсы", "bottom"),
      ],
      total_qty: 4, total: 400, can_checkout: true, has_unavailable: false,
    });
  });

  it("lists only cart items and adapts to a set", async () => {
    renderIt();
    const tabs = await screen.findByRole("tablist");
    await waitFor(() => expect(within(tabs).getByRole("tab", { name: "Верх · 2" })).toBeInTheDocument());
    expect(within(tabs).getByRole("tab", { name: "Низ · 1" })).toBeInTheDocument();
    expect(screen.getAllByText("Рубашка", { selector: ".product-tile div" })).toHaveLength(1);

    const pick = (title: string) => {
      const tile = screen.getByText(title, { selector: ".product-tile div" }).closest("li")!;
      fireEvent.click(within(tile).getByRole("button"));
    };
    fireEvent.click(within(tabs).getByRole("tab", { name: "Низ · 1" }));
    pick("Джинсы");
    fireEvent.click(within(tabs).getByRole("tab", { name: "Верх · 2" }));
    pick("Костюм");
    await waitFor(() => expect(within(tabs).queryByRole("tab", { name: /Низ/ })).toBeNull());
    expect(JSON.parse(localStorage.getItem("fitting_v1")!)).toEqual({ top: expect.objectContaining({ id: "suit", is_set: true }) });

    pick("Рубашка");
    await waitFor(() => expect(within(tabs).getByRole("tab", { name: "Низ · 1" })).toBeInTheDocument());
  });

  it("asks to sign in instead of showing the catalog", async () => {
    role = null;
    renderIt();
    expect(await screen.findByText("Примерить можно вещи из корзины.")).toBeInTheDocument();
    expect(getCart).not.toHaveBeenCalled();
  });

  it("explains an empty cart", async () => {
    getCart.mockResolvedValue({ items: [], total_qty: 0, total: 0, can_checkout: false, has_unavailable: false });
    renderIt();
    expect(await screen.findByText(/Корзина пуста/)).toBeInTheDocument();
  });
});
