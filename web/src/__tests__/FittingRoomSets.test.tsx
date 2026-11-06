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

describe("FittingRoom: двойки", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    vi.stubGlobal("fetch", vi.fn(async () => new Response(null, { status: 204 })));
    apiList.mockImplementation(async (url: string) => ({
      items: url.includes("set=true")
        ? [{ id: "suit", title: "Костюм-двойка", brand: "A", price: 100, store_name: "S", is_set: true, images: {} }]
        : [{ id: "x", title: "Вещь", brand: "A", price: 100, store_name: "S", images: {} }],
      next: "",
    }));
  });

  it("picking a set hides the bottom slot and the bottom tab", async () => {
    localStorage.setItem("fitting_v1", JSON.stringify({ bottom: { id: "pants", title: "Брюки", brand: "A", price: 100, image_url: "", zone: "bottom" } }));
    renderIt();
    const tabs = screen.getByRole("tablist");
    expect(within(tabs).getByRole("tab", { name: "Низ" })).toBeInTheDocument();
    expect(screen.getByText("Брюки")).toBeInTheDocument();

    fireEvent.click(within(tabs).getByRole("tab", { name: "Двойки" }));
    await waitFor(() => expect(apiList).toHaveBeenCalledWith(expect.stringContaining("category=c-top&set=true")));
    const tile = (await screen.findByText("Костюм-двойка")).closest("li")!;
    fireEvent.click(within(tile).getByRole("button", { name: "В примерочную" }));

    await waitFor(() => expect(within(tabs).queryByRole("tab", { name: "Низ" })).toBeNull());
    expect(screen.queryByText("Брюки")).toBeNull(); // выбранный низ убран
    expect(screen.getByText("Двойка (верх + низ)")).toBeInTheDocument();
    expect(JSON.parse(localStorage.getItem("fitting_v1")!)).toEqual({
      top: expect.objectContaining({ id: "suit", zone: "top", is_set: true }),
    });
  });

  it("the regular top tab lists only non-set tops", async () => {
    renderIt();
    await waitFor(() => expect(apiList).toHaveBeenCalledWith(expect.stringContaining("category=c-top&set=false")));
  });
});
