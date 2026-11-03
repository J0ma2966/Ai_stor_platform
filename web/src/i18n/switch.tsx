"use client";
// Смена языка на месте: без новой страницы и без потери состояния.
//
// 1. Запоминаем выбор в cookie (middleware вернёт человека на его язык при следующем заходе).
// 2. Мягко меняем адрес на /uz/... или /... (router.replace). Дерево страниц то же самое —
//    сегмента [locale] в app/ нет, — поэтому React ничего не пересоздаёт: сервер присылает
//    переведённую страницу, а введённый текст, прокрутка, открытые меню и вход сохраняются.
//    Cookie действует и на RSC-запросы роутера (см. middleware), поэтому все догрузки после
//    смены языка приходят на новом языке.
// 3. Корневой layout (шапка, подвал, сообщения next-intl) при обычном переходе не перерисовывается,
//    поэтому <LocaleSync> видит, что язык адреса и язык текста разошлись, и делает router.refresh().
//    Та же синхронизация срабатывает на «назад/вперёд» по истории между языками.
import { useCallback, useEffect, useTransition } from "react";
import { usePathname, useRouter } from "next/navigation";
import { useLocale } from "next-intl";
import { LOCALE_COOKIE, localeFromPath, localizePath, type Locale } from "./routing";

export function rememberLocale(l: Locale) {
  document.cookie = `${LOCALE_COOKIE}=${l}; path=/; max-age=31536000; samesite=lax`;
}

/** Возвращает функцию, переключающую язык текущей страницы. */
export function useSwitchLocale(): (l: Locale) => void {
  const router = useRouter();
  return useCallback(
    (l: Locale) => {
      rememberLocale(l);
      const { pathname, search, hash } = window.location;
      const target = localizePath(pathname, l) + search + hash;
      if (target !== pathname + search + hash) router.replace(target, { scroll: false });
    },
    [router],
  );
}

/** true, пока страница догружает перевод после смены языка. */
export function useLocalePending(): boolean {
  const locale = useLocale();
  const pathname = usePathname() ?? "/";
  return localeFromPath(pathname) !== locale;
}

export function LocaleSync() {
  const locale = useLocale();
  const pathname = usePathname() ?? "/";
  const router = useRouter();
  const [, start] = useTransition();
  const want = localeFromPath(pathname);

  useEffect(() => {
    // язык адреса и текста разошлись (смена языка или «назад» на страницу другого языка)
    if (want !== locale) {
      rememberLocale(want);
      start(() => router.refresh());
    }
  }, [want, locale, router]);

  useEffect(() => {
    // cookie — язык активной вкладки: при возврате во вкладку восстанавливаем его,
    // если в другой вкладке выбрали другой язык
    const sync = () => {
      if (document.visibilityState === "visible") rememberLocale(localeFromPath(window.location.pathname));
    };
    sync();
    document.addEventListener("visibilitychange", sync);
    window.addEventListener("focus", sync);
    return () => {
      document.removeEventListener("visibilitychange", sync);
      window.removeEventListener("focus", sync);
    };
  }, []);

  useEffect(() => {
    document.documentElement.lang = locale;
  }, [locale]);

  return null;
}
