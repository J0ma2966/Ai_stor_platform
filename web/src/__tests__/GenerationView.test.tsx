import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import messages from "../../messages/ru.json";
import type { Generation } from "@/lib/tryon";

const push = vi.fn();
vi.mock("@/i18n/navigation", () => ({
  Link: (p: { href: string; children: React.ReactNode }) => <a href={p.href}>{p.children}</a>,
  useRouter: () => ({ push, replace: push }),
}));
const getGeneration = vi.fn();
const setSaved = vi.fn();
const createGeneration = vi.fn();
vi.mock("@/lib/tryon", async (orig) => ({
  ...(await orig<typeof import("@/lib/tryon")>()),
  getGeneration: (...a: unknown[]) => getGeneration(...a),
  setSaved: (...a: unknown[]) => setSaved(...a),
  createGeneration: (...a: unknown[]) => createGeneration(...a),
}));

import { GenerationView } from "@/components/GenerationView";

const base: Generation = {
  id: "g1", status: "queued", cost: 20, error_code: null, refunded: false, saved: false, result_url: null, result_expires_at: null,
  photos: [
    { angle: "front", key: "k1", url: "u1" }, { angle: "side", key: "k2", url: "u2" }, { angle: "back", key: "k3", url: "u3" },
  ],
  products: [{ id: "p1", title: "Куртка", brand: "Acme", price: 100000, zone: "top", image_url: "", store_name: "Бутик", store_slug: "b", available: true }],
  body: {}, created_at: "2026-11-02T10:00:00Z", finished_at: null,
};

const renderIt = () =>
  render(
    <NextIntlClientProvider locale="ru" messages={messages} timeZone="Asia/Tashkent">
      <GenerationView id="g1" />
    </NextIntlClientProvider>,
  );

describe("GenerationView", () => {
  afterEach(() => {
    vi.clearAllMocks();
    vi.useRealTimers();
  });

  it("polls while queued and shows the composite result", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    getGeneration.mockResolvedValueOnce(base).mockResolvedValueOnce({ ...base, status: "running" })
      .mockResolvedValue({ ...base, status: "succeeded", result_url: "https://s3/r.jpg", result_expires_at: "2026-12-02T10:00:00Z" });
    renderIt();
    expect(await screen.findByText("Ожидает")).toBeInTheDocument();
    await act(async () => { await vi.advanceTimersByTimeAsync(3100); });
    expect(await screen.findByText("Выполняется")).toBeInTheDocument();
    await act(async () => { await vi.advanceTimersByTimeAsync(3100); });
    const img = await screen.findByAltText("Вы в выбранном образе: спереди, сбоку и сзади");
    expect(img).toHaveAttribute("src", "https://s3/r.jpg");
    expect(screen.getByRole("link", { name: "Открыть в полном размере" })).toHaveAttribute("href", "https://s3/r.jpg");
    expect(screen.getByRole("link", { name: "Скачать" })).toHaveAttribute("href", "https://s3/r.jpg");
    expect(getGeneration).toHaveBeenCalledTimes(3);
    // после завершения опрос прекращается
    await act(async () => { await vi.advanceTimersByTimeAsync(10000); });
    expect(getGeneration).toHaveBeenCalledTimes(3);

    setSaved.mockResolvedValue({ ...base, status: "succeeded", result_url: "https://s3/r.jpg", saved: true });
    fireEvent.click(screen.getByRole("button", { name: "Сохранить образ" }));
    expect(await screen.findByRole("button", { name: "Убрать из сохранённых" })).toBeInTheDocument();
    expect(setSaved).toHaveBeenCalledWith("g1", true);
  });

  it("explains failure and refund; repeat reuses photos", async () => {
    getGeneration.mockResolvedValue({ ...base, status: "failed", error_code: "photo_rejected", refunded: true });
    createGeneration.mockResolvedValue({ ...base, id: "g2" });
    renderIt();
    expect(await screen.findByText(/не смог распознать человека/)).toBeInTheDocument();
    expect(screen.getByText("20 токенов возвращены на баланс.")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Повторить генерацию (20 токенов)" }));
    await waitFor(() => expect(push).toHaveBeenCalledWith("/fitting/g2"));
    expect(createGeneration.mock.calls[0][0]).toMatchObject({ product_ids: ["p1"], photos: { front: "k1", side: "k2", back: "k3" } });
  });
});
