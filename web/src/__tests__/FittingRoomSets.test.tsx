import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import messages from "../../messages/ru.json";

vi.mock("@/i18n/navigation", () => ({
  Link: (p: { href: string; children: React.ReactNode }) => <a href={p.href}>{p.children}</a>,
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
}));
const apiList = vi.fn();
vi.mock("@/lib/api", async (orig) => ({
  ...(await orig<typeof import("@/lib/api")>()),
  currentRole: () => null,
  refreshSession: async () => false,
  api: vi.fn(async () => [
    { id: "c-top", zone: "top", name_ru: "Верх", name_uz: "Ustki" },
    { id: "c-bottom", zone: "bottom", name_ru: "Низ", name_uz: "Pastki" },
  ]),
  apiList: (url: string) => apiList(url),
}));

import { FittingRoom } from "@/components/FittingRoom";

const renderIt = () =>
  render(
    <NextIntlClientProvider locale="ru" messages={messages} timeZone="Asia/Tashkent">
      <FittingRoom />
    </NextIntlClientProvider>,
  );

describe("FittingRoom: комплект верх+низ", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    vi.stubGlobal("fetch", vi.fn(async () => new Response(null, { status: 204 })));
    apiList.mockImplementation(async (url: string) => ({
      items: url.includes("c-top")
        ? [
            { id: "suit", title: "Костюм", brand: "A", price: 100, store_name: "S", is_set: true, images: {} },
            { id: "shirt", title: "Рубашка", brand: "A", price: 100, store_name: "S", images: {} },
          ]
        : [{ id: "jeans", title: "Джинсы", brand: "A", price: 100, store_name: "S", images: {} }],
      next: "",
    }));
  });

  it("adapts to the pick: a set hides the bottom, a regular top brings it back", async () => {
    localStorage.setItem("fitting_v1", JSON.stringify({ bottom: { id: "pants", title: "Брюки", brand: "A", price: 100, image_url: "", zone: "bottom" } }));
    renderIt();
    const tabs = screen.getByRole("tablist");
    expect(within(tabs).getAllByRole("tab").map((x) => x.textContent)).toEqual(["Головной убор", "Верх", "Низ", "Обувь"]);
    expect(screen.getByText("Брюки")).toBeInTheDocument();
    // верх — одним списком, без отдельного раздела для комплектов
    await waitFor(() => expect(apiList).toHaveBeenCalledWith("/v1/products?limit=12&category=c-top"));

    const pick = async (title: string) => {
      const tile = (await screen.findByText(title, { selector: ".product-tile div" })).closest("li")!;
      fireEvent.click(within(tile).getByRole("button"));
    };
    await pick("Костюм");
    await waitFor(() => expect(within(tabs).queryByRole("tab", { name: "Низ" })).toBeNull());
    expect(screen.queryByText("Брюки")).toBeNull(); // низ уже в комплекте
    expect(JSON.parse(localStorage.getItem("fitting_v1")!)).toEqual({ top: expect.objectContaining({ id: "suit", is_set: true }) });

    await pick("Рубашка");
    await waitFor(() => expect(within(tabs).getByRole("tab", { name: "Низ" })).toBeInTheDocument());
  });
});
