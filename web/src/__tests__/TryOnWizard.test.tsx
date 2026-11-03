import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import messages from "../../messages/ru.json";
import { ApiError } from "@/lib/api";
import type { Quote } from "@/lib/tryon";

const replace = vi.fn();
vi.mock("@/i18n/navigation", () => ({
  Link: (p: { href: string; children: React.ReactNode }) => <a href={p.href}>{p.children}</a>,
  useRouter: () => ({ push: replace, replace }),
}));
vi.mock("@/lib/api", async (orig) => ({
  ...(await orig<typeof import("@/lib/api")>()),
  api: vi.fn(async () => ({ height_cm: 170, weight_kg: null, body_params: {} })),
}));
const quote: Quote = {
  cost: 20, balance: 40, enough: true, consent: false, registered: true, gender: "female", active: 0, max_active: 2,
  supported_zones: ["headwear", "top", "bottom", "shoes"], retention: { photo_hours: 24, saved_photo_days: 180, result_days: 30, saved_look_days: 365 },
};
const createGeneration = vi.fn();
const giveConsent = vi.fn(async () => undefined);
const uploadBodyPhoto = vi.fn();
vi.mock("@/lib/tryon", async (orig) => ({
  ...(await orig<typeof import("@/lib/tryon")>()),
  getQuote: async () => quote,
  listPhotos: async () => [
    { id: "1", angle: "front", key: "kf", url: "uf", expires_at: "2027-01-01T00:00:00Z" },
    { id: "2", angle: "side", key: "ks", url: "us", expires_at: "2027-01-01T00:00:00Z" },
  ],
  giveConsent: () => giveConsent(),
  uploadBodyPhoto: (f: Blob) => uploadBodyPhoto(f),
  createGeneration: (...a: unknown[]) => createGeneration(...a),
}));

import { TryOnWizard } from "@/components/TryOnWizard";

const renderIt = () =>
  render(
    <NextIntlClientProvider locale="ru" messages={messages} timeZone="Asia/Tashkent">
      <TryOnWizard />
    </NextIntlClientProvider>,
  );

describe("TryOnWizard", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.setItem("fitting_v1", JSON.stringify({ top: { id: "p1", title: "Куртка", brand: "A", price: 100, image_url: "", zone: "top" } }));
    vi.stubGlobal("fetch", vi.fn(async () => new Response(null, { status: 204 })));
  });

  it("requires all 3 photos and consent, then runs once with a stable idempotency key", async () => {
    uploadBodyPhoto.mockResolvedValue({ key: "kb", url: "ub" });
    renderIt();
    const next = await screen.findByRole("button", { name: "Далее" });
    expect(screen.getByText("Нужны все три фото: спереди, сбоку и сзади.")).toBeInTheDocument();
    expect(next).toBeDisabled();

    const file = new File([new Uint8Array([1, 2, 3])], "b.jpg", { type: "image/jpeg" });
    fireEvent.change(screen.getByLabelText("Загрузить фото: Сзади"), { target: { files: [file] } });
    await waitFor(() => expect(screen.queryByText("Нужны все три фото: спереди, сбоку и сзади.")).not.toBeInTheDocument());
    expect(next).toBeDisabled(); // нет согласия
    fireEvent.click(screen.getByLabelText(/Я согласен/));
    fireEvent.click(next);
    await waitFor(() => expect(giveConsent).toHaveBeenCalledTimes(1));

    expect(await screen.findByText(/Укажите рост, вес и параметры тела/)).toBeInTheDocument();
    expect(screen.getByLabelText("Рост, см")).toHaveValue("170");
    fireEvent.click(screen.getByRole("button", { name: "Далее" }));

    createGeneration.mockRejectedValueOnce(new ApiError(0, "network", "x")).mockResolvedValueOnce({ id: "g9" });
    const run = await screen.findByRole("button", { name: "Запустить за 20 токенов" });
    fireEvent.click(run);
    expect(await screen.findByText(/Нет соединения/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Запустить за 20 токенов" }));
    await waitFor(() => expect(replace).toHaveBeenCalledWith("/fitting/g9"));

    const [input1, key1] = createGeneration.mock.calls[0];
    const [, key2] = createGeneration.mock.calls[1];
    expect(key1).toBe(key2);
    expect(input1).toMatchObject({ product_ids: ["p1"], photos: { front: "kf", side: "ks", back: "kb" }, save_photos: true, body: { height_cm: 170 } });
  });
});
