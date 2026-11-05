import { afterEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import messages from "../../messages/ru.json";

const push = vi.fn();
vi.mock("@/i18n/navigation", () => ({
  Link: (p: { href: string; children: React.ReactNode }) => <a href={p.href}>{p.children}</a>,
  useRouter: () => ({ push }),
  usePathname: () => "/product/p1",
}));
vi.mock("@/lib/session", () => ({ useSession: () => ({ me: { role: "client" }, setCartQty: vi.fn() }) }));
vi.mock("@/components/FavoriteButton", () => ({ FavoriteButton: () => null }));
const setCartQty = vi.fn(async () => ({ items: [], total_qty: 1, total: 0, can_checkout: true, has_unavailable: false }));
vi.mock("@/lib/shop", () => ({ setCartQty: (...a: unknown[]) => setCartQty(...(a as [])) }));
const cancelOrder = vi.fn();
const getOrder = vi.fn();
vi.mock("@/lib/orders", async (orig) => ({
  ...(await orig<typeof import("@/lib/orders")>()),
  cancelOrder: (...a: unknown[]) => cancelOrder(...a),
  getOrder: (...a: unknown[]) => getOrder(...a),
}));

import { ProductActions } from "@/components/ProductActions";
import { ClientOrderView } from "@/components/ClientOrderView";
import type { Product } from "@/lib/catalog";

const wrap = (ui: React.ReactNode) =>
  render(<NextIntlClientProvider locale="ru" messages={messages} timeZone="Asia/Tashkent">{ui}</NextIntlClientProvider>);

const product: Product = {
  id: "p1", store_id: "s", store_name: "Бутик", category_id: "c", title: "Платье", brand: "Acme", price: 100000, in_stock: true,
  images: {}, created_at: "2026-11-01T00:00:00Z",
  sizes: [{ size: "S", in_stock: false }, { size: "M", in_stock: true, low: true }, { size: "L", in_stock: true }],
};

describe("ProductActions with sizes", () => {
  afterEach(() => vi.clearAllMocks());

  it("asks for a size, disables sold-out sizes and sends the chosen one", async () => {
    wrap(<ProductActions product={product} />);
    expect(screen.getByRole("radio", { name: "S" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "В корзину" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Выберите размер");
    expect(setCartQty).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("radio", { name: /M/ }));
    expect(screen.getByText("осталось мало")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "В корзину" }));
    await waitFor(() => expect(setCartQty).toHaveBeenCalledWith("p1", 1, "M"));
  });
});

const order = {
  id: "o1", number: 100042, checkout_id: "c", store_id: "s", store_name: "Бутик", status: "pending", step: 0, items_total: 100000, items_count: 1,
  created_at: "2026-11-05T10:00:00Z", updated_at: "2026-11-05T10:00:00Z", customer_phone: "+998901112233", customer_comment: null,
  delivery_time_comment: null, rejection_reason: null, can_cancel: true,
  delivery: { street: "Навои", house: "5", has_photo: false },
  items: [{ product_id: "p1", title: "Платье", brand: "Acme", unit_price: 100000, qty: 1, size: "M" }],
  history: [],
};

describe("ClientOrderView cancel", () => {
  afterEach(() => vi.clearAllMocks());

  it("shows the size and cancels after confirmation", async () => {
    getOrder.mockResolvedValue(order);
    cancelOrder.mockResolvedValue({ ...order, status: "cancelled", can_cancel: false, rejection_reason: "передумал" });
    wrap(<ClientOrderView id="o1" />);
    expect(await screen.findByText(/размер M/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Отменить заказ" }));
    fireEvent.change(screen.getByLabelText("Причина (необязательно)"), { target: { value: "передумал" } });
    fireEvent.click(screen.getByRole("button", { name: "Да, отменить" }));
    await waitFor(() => expect(cancelOrder).toHaveBeenCalledWith("o1", "передумал"));
    expect(await screen.findByText("Причина: передумал")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Отменить заказ" })).toBeNull();
  });
});
