import { describe, expect, it } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import ru from "../../messages/ru.json";
import uz from "../../messages/uz.json";
import { FLAWS, PhotoGuide } from "@/components/PhotoGuide";

const renderIt = (open = true, locale: "ru" | "uz" = "ru") =>
  render(
    <NextIntlClientProvider locale={locale} messages={locale === "ru" ? ru : uz} timeZone="Asia/Tashkent">
      <PhotoGuide defaultOpen={open} />
    </NextIntlClientProvider>,
  );

describe("PhotoGuide", () => {
  it("shows three good angles and every bad example with a caption", () => {
    const { container } = renderIt();
    expect(screen.getByText("Так — подходит")).toBeInTheDocument();
    for (const a of ["Спереди", "Сбоку", "Сзади"]) expect(screen.getByText(a)).toBeInTheDocument();
    expect(screen.getByText("Обрезаны ноги или голова")).toBeInTheDocument();
    expect(container.querySelectorAll(".pg-mark-bad")).toHaveLength(FLAWS.length);
    expect(container.querySelectorAll(".pg-mark-ok")).toHaveLength(3);
    // clipPath id уникальны, иначе фигуры на странице обрежутся чужой маской
    const ids = [...container.querySelectorAll("clipPath")].map((c) => c.id);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it("can start collapsed and be opened", () => {
    const { container } = renderIt(false);
    const details = container.querySelector("details")!;
    expect(details.open).toBe(false);
    fireEvent.click(screen.getByText("Примеры фото"));
    details.open = true;
    fireEvent(details, new Event("toggle"));
    expect(details.open).toBe(true);
  });

  it("has every flaw translated in uz", () => {
    renderIt(true, "uz");
    for (const f of FLAWS) expect(Object.keys(uz.tryon.photoGuide.flaw)).toContain(f);
    expect(screen.getByText("Bunday — mos emas")).toBeInTheDocument();
  });
});
