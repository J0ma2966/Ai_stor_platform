import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import messages from "../../messages/ru.json";
import { ApiError } from "@/lib/api";

vi.mock("@/i18n/navigation", () => ({ Link: (p: { href: string; children: React.ReactNode }) => <a href={p.href}>{p.children}</a> }));
const checkout = vi.fn();
vi.mock("@/lib/orders", async (orig) => ({
  ...(await orig<typeof import("@/lib/orders")>()),
  checkout: (...a: unknown[]) => checkout(...a),
}));
vi.mock("@/lib/api", async (orig) => ({
  ...(await orig<typeof import("@/lib/api")>()),
  api: vi.fn(async () => ({ id: "p1", title: "Платье", brand: "Acme", price: 100000, in_stock: true, store_name: "Бутик", images: {} })),
}));

import { CheckoutForm } from "@/components/CheckoutForm";

const items = [{ product_id: "p1", qty: 2 }];
const renderIt = () =>
  render(
    <NextIntlClientProvider locale="ru" messages={messages} timeZone="Asia/Tashkent">
      <CheckoutForm items={items} />
    </NextIntlClientProvider>,
  );

describe("CheckoutForm", () => {
  afterEach(() => vi.clearAllMocks());

  it("validates required address fields locally", async () => {
    renderIt();
    fireEvent.click(screen.getByRole("button", { name: "Оформить заказ" }));
    expect(await screen.findAllByText("Обязательное поле")).toHaveLength(3);
    expect(checkout).not.toHaveBeenCalled();
  });

  it("sends address, comments and reuses idempotency key on retry", async () => {
    checkout.mockRejectedValueOnce(new ApiError(0, "network", "x")).mockResolvedValueOnce({
      checkout_id: "c", orders: [{ id: "o1", number: 100001, store_id: "s", store_name: "Бутик", status: "pending", items_total: 200000 }],
    });
    renderIt();
    const type = (label: RegExp, v: string) => fireEvent.change(screen.getByLabelText(label), { target: { value: v } });
    type(/Имя получателя/, "Азиз");
    type(/^Улица/, "Навои");
    type(/^Дом( \*)?$/, "5");
    type(/Подъезд/, "2");
    type(/Этаж/, "7");
    type(/Комментарий к заказу/, "Позвоните");
    type(/ко времени доставки/, "после 18:00");
    fireEvent.click(screen.getByRole("button", { name: "Оформить заказ" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Нет соединения");
    fireEvent.click(screen.getByRole("button", { name: "Оформить заказ" }));
    expect(await screen.findByText("Заказ оформлен")).toBeInTheDocument();
    expect(checkout).toHaveBeenCalledTimes(2);
    const [input, key1] = checkout.mock.calls[0];
    const [, key2] = checkout.mock.calls[1];
    expect(key1).toBe(key2);
    expect(input).toMatchObject({
      items, customer_name: "Азиз", customer_comment: "Позвоните", delivery_time_comment: "после 18:00",
      delivery: { street: "Навои", house: "5", entrance: "2", floor: "7" },
    });
  });
});
