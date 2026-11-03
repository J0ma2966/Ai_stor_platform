import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import messages from "../../messages/ru.json";
import { ApiError } from "@/lib/api";

const m = {
  getBalance: vi.fn(),
  listTariffs: vi.fn(),
  listPayments: vi.fn(),
  getPayment: vi.fn(),
  createPayment: vi.fn(),
};
vi.mock("@/lib/payments", async (orig) => {
  const real = await orig<typeof import("@/lib/payments")>();
  return {
    ...real,
    getBalance: () => m.getBalance(),
    listTariffs: () => m.listTariffs(),
    listPayments: () => m.listPayments(),
    getPayment: (id: string) => m.getPayment(id),
    createPayment: (...a: unknown[]) => m.createPayment(...a),
  };
});

import { BalancePage } from "@/components/BalancePage";
import { safeCheckoutUrl } from "@/lib/payments";

const tariff = { id: "t1", name_ru: "Старт", name_uz: "Start", description: "", tokens: 50, price: 1500000, highlighted: true };
const pending = { id: "p1", number: 1, provider: "click", amount: 1500000, tokens: 50, status: "pending", paid_at: null, created_at: "2026-11-02T10:00:00Z" };

const renderIt = (pid?: string) =>
  render(
    <NextIntlClientProvider locale="ru" messages={messages} timeZone="Asia/Tashkent">
      <BalancePage returnedPaymentId={pid} />
    </NextIntlClientProvider>,
  );

describe("BalancePage", () => {
  const assign = vi.fn();
  beforeEach(() => {
    m.getBalance.mockResolvedValue(20);
    m.listTariffs.mockResolvedValue([tariff]);
    m.listPayments.mockResolvedValue({ items: [], nextCursor: "" });
    Object.defineProperty(window, "location", { value: { ...window.location, assign }, writable: true });
  });
  afterEach(() => vi.clearAllMocks());

  it("shows balance and tariffs, buys via Click reusing the idempotency key on retry", async () => {
    m.createPayment
      .mockRejectedValueOnce(new ApiError(0, "network", "x"))
      .mockResolvedValueOnce({ ...pending, checkout_url: "https://my.click.uz/services/pay?service_id=1" });
    renderIt();
    expect(await screen.findByTestId("balance")).toHaveTextContent("20 токенов");
    expect(screen.getByText("Популярный")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Click" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Нет соединения");
    fireEvent.click(screen.getByRole("button", { name: "Click" }));
    await waitFor(() => expect(assign).toHaveBeenCalledWith("https://my.click.uz/services/pay?service_id=1"));
    const [[t1, p1, k1], [, , k2]] = m.createPayment.mock.calls;
    expect([t1, p1]).toEqual(["t1", "click"]);
    expect(k1).toBe(k2);
  });

  it("refuses to redirect to a foreign checkout host", async () => {
    m.createPayment.mockResolvedValueOnce({ ...pending, provider: "payme", checkout_url: "https://evil.example/pay" });
    renderIt();
    fireEvent.click(await screen.findByRole("button", { name: "Payme" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Не удалось открыть страницу оплаты");
    expect(assign).not.toHaveBeenCalled();
  });

  it("after return polls the payment until paid and refreshes the balance", async () => {
    m.getPayment.mockResolvedValueOnce(pending).mockResolvedValueOnce({ ...pending, status: "paid" });
    m.getBalance.mockResolvedValueOnce(20).mockResolvedValueOnce(70);
    renderIt("p1");
    expect(await screen.findByText("Ждём подтверждения оплаты…")).toBeInTheDocument();
    expect(await screen.findByText("Оплата прошла, зачислено 50 токенов.", {}, { timeout: 5000 })).toBeInTheDocument();
    await waitFor(() => expect(screen.getByTestId("balance")).toHaveTextContent("70 токенов"));
  });
});

describe("safeCheckoutUrl", () => {
  it("allows only provider hosts over https", () => {
    expect(safeCheckoutUrl("https://checkout.paycom.uz/bT0xMjM=")).toBeTruthy();
    expect(safeCheckoutUrl("https://checkout.test.paycom.uz/abc")).toBeTruthy();
    expect(safeCheckoutUrl("http://my.click.uz/services/pay")).toBeNull();
    expect(safeCheckoutUrl("https://my.click.uz.evil.com/")).toBeNull();
    expect(safeCheckoutUrl("javascript:alert(1)")).toBeNull();
    expect(safeCheckoutUrl(undefined)).toBeNull();
  });
});
