import { describe, expect, it } from "vitest";
import { act, render, screen } from "@testing-library/react";
import { Suspense } from "react";
import { NextIntlClientProvider, useTranslations } from "next-intl";
import ru from "../../messages/ru.json";
import uz from "../../messages/uz.json";
import adminRu from "../../messages/admin.ru.json";
import adminUz from "../../messages/admin.uz.json";
import { ClientMessages, loadMessages } from "@/i18n/ClientMessages";
import { mergeMessages } from "@/i18n/messages";

function Probe() {
  const t = useTranslations();
  return <p>{t("nav.searchPh")} | {t("admin.loginTitle")}</p>;
}

// Как в корневом layout: серверный провайдер даёт только язык, словарь — ClientMessages.
function tree(locale: string) {
  return (
    <NextIntlClientProvider locale={locale} messages={null} timeZone="Asia/Tashkent">
      <Suspense fallback={<p>loading</p>}>
        <ClientMessages>
          <Probe />
        </ClientMessages>
      </Suspense>
    </NextIntlClientProvider>
  );
}

describe("словари клиентских компонентов", () => {
  it("состав словаря тот же, что у серверных компонентов (общие + админка)", async () => {
    expect(await loadMessages("ru")).toEqual(mergeMessages(ru, adminRu));
    expect(await loadMessages("uz")).toEqual(mergeMessages(uz, adminUz));
  });

  it("повторная загрузка не создаёт новый запрос", () => {
    expect(loadMessages("ru")).toBe(loadMessages("ru"));
  });

  it("отдаёт переводы выбранного языка и меняет их при смене языка без пересоздания детей", async () => {
    const { rerender } = render(tree("ru"));
    expect(await screen.findByText(`${ru.nav.searchPh} | ${adminRu.admin.loginTitle}`)).toBeInTheDocument();
    await act(async () => {
      rerender(tree("uz"));
      await loadMessages("uz");
    });
    expect(await screen.findByText(`${uz.nav.searchPh} | ${adminUz.admin.loginTitle}`)).toBeInTheDocument();
  });
});
