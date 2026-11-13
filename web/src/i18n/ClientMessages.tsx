"use client";
// Переводы для клиентских компонентов.
//
// Раньше корневой layout передавал весь словарь (около 48 КБ) свойством провайдера, и он
// попадал в HTML каждой страницы: сервер сериализовал его на каждый запрос, а браузер
// скачивал заново с каждой страницей. Теперь словарь лежит в JS-файле своего языка
// (messages.ru.ts / messages.uz.ts): он скачивается один раз и кэшируется браузером,
// а HTML страниц стал легче почти вдвое. Файл другого языка браузер не скачивает,
// пока человек не переключит язык.
//
// Язык, часовой пояс и форматы по-прежнему приходят от серверного <NextIntlClientProvider>
// в корневом layout; здесь к ним добавляются только сообщения выбранного языка. При смене
// языка (см. switch.tsx) у этого же компонента меняется язык, поэтому состояние страницы
// сохраняется, как и раньше; пока словарь нового языка догружается, React держит на экране
// прежнюю страницу (обновление идёт внутри перехода).
//
// next/dynamic здесь не подходит: его предзагрузка идёт без nonce и блокируется нашей CSP.
import { use, type ReactNode } from "react";
import { NextIntlClientProvider, useLocale, type AbstractIntlMessages } from "next-intl";
import { defaultLocale, isLocale, type Locale } from "./routing";

const IMPORTS: Record<Locale, () => Promise<{ default: unknown }>> = {
  ru: () => import("./messages.ru"),
  uz: () => import("./messages.uz"),
};

const loaded = new Map<Locale, AbstractIntlMessages>();
const loading = new Map<Locale, Promise<AbstractIntlMessages>>();

/** Загружает словарь языка (один раз); повторные вызовы возвращают тот же промис. */
export function loadMessages(locale: Locale): Promise<AbstractIntlMessages> {
  let p = loading.get(locale);
  if (!p) {
    p = IMPORTS[locale]().then(
      (m) => {
        const messages = m.default as AbstractIntlMessages;
        loaded.set(locale, messages);
        return messages;
      },
      (err) => {
        loading.delete(locale); // сеть моргнула: следующая попытка загрузит заново
        throw err;
      },
    );
    loading.set(locale, p);
  }
  return p;
}

// В браузере начинаем качать словарь сразу при загрузке этого файла, не дожидаясь
// гидратации: язык страницы уже записан сервером в <html lang>.
if (typeof document !== "undefined") {
  const lang = document.documentElement.lang;
  void loadMessages(isLocale(lang) ? lang : defaultLocale).catch(() => {});
}

export function ClientMessages({ children }: { children: ReactNode }) {
  const current = useLocale();
  const locale = isLocale(current) ? current : defaultLocale;
  const messages = loaded.get(locale) ?? use(loadMessages(locale));
  return (
    <NextIntlClientProvider locale={locale} messages={messages}>
      {children}
    </NextIntlClientProvider>
  );
}
