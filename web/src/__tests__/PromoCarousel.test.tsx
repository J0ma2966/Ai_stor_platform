import { act } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import messages from "../../messages/ru.json";
import type { Product } from "@/lib/catalog";
import { addDays, dateToEnd, dateToStart, isoToDate } from "@/lib/promotions";

vi.mock("@/i18n/navigation", () => ({
  Link: ({ href, children, ...rest }: { href: string; children: React.ReactNode }) => <a href={href} {...rest}>{children}</a>,
}));

import { PromoCarousel, ROTATE_MS } from "@/components/PromoCarousel";

let reduced = false;
let wide = false;
beforeEach(() => {
  vi.useFakeTimers();
  window.matchMedia = ((q: string) => ({
    matches: q.includes("reduced-motion") ? reduced : wide,
    media: q,
    addEventListener: () => {},
    removeEventListener: () => {},
  })) as unknown as typeof window.matchMedia;
});
afterEach(() => {
  vi.useRealTimers();
  reduced = false;
  wide = false;
});

const prod = (i: number): Product =>
  ({ id: `p${i}`, title: `Платье ${i}`, brand: "", store_name: `Бутик ${i}`, price: 10000000 * i, in_stock: true, images: {}, thumbs: {} }) as unknown as Product;

const wrap = (n: number) =>
  render(
    <NextIntlClientProvider locale="ru" messages={messages}>
      <PromoCarousel items={Array.from({ length: n }, (_, i) => prod(i + 1))} />
    </NextIntlClientProvider>,
  );

const current = () => screen.getAllByRole("group").filter((g) => g.getAttribute("aria-hidden") === "false").map((g) => g.getAttribute("aria-label"));

describe("карусель «Реклама»", () => {
  it("без рекламы ничего не рисует", () => {
    const { container } = wrap(0);
    expect(container.innerHTML).toBe("");
  });

  it("каждый слайд помечен «Реклама», один товар не крутится", () => {
    wrap(1);
    expect(screen.getAllByText("Реклама").length).toBeGreaterThanOrEqual(2); // заголовок + слайд
    expect(screen.queryByRole("button", { name: "Следующий товар" })).toBeNull();
  });

  it("сама переходит к следующему товару и по кругу возвращается к первому", () => {
    wrap(3);
    expect(current()).toEqual(["1 из 3"]);
    act(() => vi.advanceTimersByTime(ROTATE_MS));
    expect(current()).toEqual(["2 из 3"]);
    act(() => vi.advanceTimersByTime(ROTATE_MS));
    act(() => vi.advanceTimersByTime(ROTATE_MS));
    expect(current()).toEqual(["1 из 3"]);
  });

  it("кнопки «назад/вперёд», пауза останавливает прокрутку", () => {
    wrap(3);
    fireEvent.click(screen.getByRole("button", { name: "Предыдущий товар" }));
    expect(current()).toEqual(["3 из 3"]);
    fireEvent.click(screen.getByRole("button", { name: "Остановить прокрутку" }));
    act(() => vi.advanceTimersByTime(ROTATE_MS * 3));
    expect(current()).toEqual(["3 из 3"]);
    fireEvent.click(screen.getByRole("button", { name: "Запустить прокрутку" }));
    act(() => vi.advanceTimersByTime(ROTATE_MS));
    expect(current()).toEqual(["1 из 3"]);
  });

  it("стоит, пока курсор над каруселью", () => {
    wrap(3);
    const root = screen.getByRole("region");
    fireEvent.pointerEnter(root);
    act(() => vi.advanceTimersByTime(ROTATE_MS * 2));
    expect(current()).toEqual(["1 из 3"]);
    fireEvent.pointerLeave(root);
    act(() => vi.advanceTimersByTime(ROTATE_MS));
    expect(current()).toEqual(["2 из 3"]);
  });

  it("при «уменьшении движения» не крутится сама", () => {
    reduced = true;
    wrap(3);
    act(() => vi.advanceTimersByTime(ROTATE_MS * 3));
    expect(current()).toEqual(["1 из 3"]);
    fireEvent.click(screen.getByRole("button", { name: "Следующий товар" }));
    expect(current()).toEqual(["2 из 3"]);
  });

  it("на широком экране показывает два товара и двигается по одному", () => {
    wide = true;
    wrap(3);
    expect(current()).toEqual(["1 из 3", "2 из 3"]);
    act(() => vi.advanceTimersByTime(ROTATE_MS));
    expect(current()).toEqual(["2 из 3", "3 из 3"]);
    act(() => vi.advanceTimersByTime(ROTATE_MS));
    expect(current()).toEqual(["1 из 3", "2 из 3"]);
  });
});

describe("даты рекламы (Ташкент, UTC+5)", () => {
  it("начало — с начала дня, конец — до конца дня", () => {
    expect(dateToStart("2026-11-10")).toBe("2026-11-09T19:00:00.000Z");
    expect(dateToEnd("2026-11-10")).toBe("2026-11-10T18:59:59.000Z");
    expect(dateToStart("")).toBeNull();
    expect(isoToDate("2026-11-09T19:30:00Z")).toBe("2026-11-10");
    expect(isoToDate(addDays("2026-11-10T00:00:00Z", 6))).toBe("2026-11-16");
  });
});
