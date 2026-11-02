import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import messages from "../../messages/ru.json";
import type { OrderDetail } from "@/lib/orders";

vi.mock("@/i18n/navigation", () => ({ Link: (p: { href: string; children: React.ReactNode }) => <a href={p.href}>{p.children}</a> }));
const getOrder = vi.fn();
const setOrderStatus = vi.fn();
const copyToClipboard = vi.fn(async (_: string) => true);
vi.mock("@/lib/orders", async (orig) => ({
  ...(await orig<typeof import("@/lib/orders")>()),
  getOrder: (...a: unknown[]) => getOrder(...a),
  setOrderStatus: (...a: unknown[]) => setOrderStatus(...a),
  copyToClipboard: (t: string) => copyToClipboard(t),
}));

import { DeliveryWindow } from "@/components/DeliveryWindow";

const order: OrderDetail = {
  id: "o1", number: 100001, checkout_id: "c", store_id: "s", store_name: "Бутик", status: "pending", step: 0,
  items_total: 10000, items_count: 1, customer_name: "<img src=x onerror=alert(1)>", created_at: "2026-11-02T10:00:00Z",
  updated_at: "2026-11-02T10:00:00Z", customer_phone: "+998901234567", customer_comment: "Позвоните",
  delivery_time_comment: "после 18:00", rejection_reason: null,
  delivery: { street: "Навои", house: "5", entrance: "2", floor: "7", has_photo: true, photo_url: "https://cdn/x.jpg", lat: 41.3, lng: 69.2, maps_url: "https://maps.google.com/?q=41.3,69.2" },
  items: [{ product_id: "p", title: "Платье", brand: "Acme", unit_price: 10000, qty: 1 }],
  history: [{ from_status: null, to_status: "pending", created_at: "2026-11-02T10:00:00Z" }],
  next_statuses: ["confirmed", "rejected", "cancelled"],
  copy_text: "Заказ: №100001\nАдрес: Навои, дом 5\nПодъезд: 2",
};

function renderIt() {
  return render(
    <NextIntlClientProvider locale="ru" messages={messages} timeZone="Asia/Tashkent">
      <DeliveryWindow id="o1" />
    </NextIntlClientProvider>,
  );
}

describe("DeliveryWindow", () => {
  afterEach(() => vi.clearAllMocks());

  it("shows delivery data as text and copies courier text", async () => {
    getOrder.mockResolvedValue(order);
    const { container } = renderIt();
    await screen.findByText("Доставка заказа №100001");
    expect(container.querySelector("img[src='x']")).toBeNull(); // имя клиента — текст, не HTML
    expect(screen.getByText(/<img src=x/)).toBeInTheDocument();
    expect(screen.getByText("после 18:00")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Открыть на карте/ })).toHaveAttribute("rel", "noopener noreferrer");
    fireEvent.click(screen.getByRole("button", { name: "Скопировать данные доставки" }));
    await waitFor(() => expect(copyToClipboard).toHaveBeenCalledWith(order.copy_text));
    expect(await screen.findByText(/Скопировано/)).toBeInTheDocument();
  });

  it("requires a reason to reject and sends expected current status", async () => {
    getOrder.mockResolvedValue(order);
    setOrderStatus.mockResolvedValue({ ...order, status: "rejected", next_statuses: [] });
    renderIt();
    fireEvent.click(await screen.findByRole("button", { name: "Отклонить" }));
    const submit = screen.getAllByRole("button", { name: "Отклонить" }).at(-1)!;
    expect(submit).toBeDisabled();
    fireEvent.change(screen.getByLabelText(/Причина/), { target: { value: "Нет размера" } });
    fireEvent.click(submit);
    await waitFor(() => expect(setOrderStatus).toHaveBeenCalledWith("store", "o1", "rejected", "pending", "Нет размера"));
    expect(await screen.findByText(/статус больше не меняется/)).toBeInTheDocument();
  });
});
