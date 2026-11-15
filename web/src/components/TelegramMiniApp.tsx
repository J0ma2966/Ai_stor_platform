"use client";
// Магазин внутри Telegram (Mini App в нашем боте): включает вид приложения,
// кнопку «Назад» Telegram, цвета шапки Telegram под тему сайта и автоматический вход
// по данным Telegram, если этот аккаунт уже входил через бота.
import { useEffect, useRef, useState } from "react";
import { usePathname, useRouter } from "@/i18n/navigation";
import { loginTelegramWebApp } from "@/lib/api";
import { useSession } from "@/lib/session";
import { isTelegramApp, loadTelegramWebApp, TG_COOKIE, type TelegramWebApp } from "@/lib/telegram";
import { isTabRoot } from "./TabBar";

const TRIED = "tg-login-tried";

function cssVar(name: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

export function TelegramMiniApp() {
  const [tg, setTg] = useState<TelegramWebApp | null>(null);
  const pathname = usePathname();
  const router = useRouter();
  const { ready, me, reload } = useSession();
  const tried = useRef(false);

  // 1. Запуск: скрипт Telegram, развернуть на весь экран, не закрывать свайпом вниз при прокрутке.
  useEffect(() => {
    if (!isTelegramApp()) return;
    const root = document.documentElement;
    root.dataset.tg = "1";
    document.cookie = `${TG_COOKIE}=1; path=/; samesite=lax`;
    let alive = true;
    loadTelegramWebApp().then((w) => {
      if (!alive || !w) return;
      w.ready();
      w.expand();
      if (w.isVersionAtLeast("7.7")) w.disableVerticalSwipes?.();
      // тема «как в системе» внутри Telegram — как в Telegram
      if (!document.cookie.split("; ").some((c) => c.startsWith("theme=") && c.length > 6)) {
        root.dataset.theme = w.colorScheme === "dark" ? "dark" : "light";
      }
      setTg(w);
    });
    return () => {
      alive = false;
    };
  }, []);

  // 2. Цвета шапки и низа Telegram — как фон сайта и нижняя панель (и после смены темы).
  useEffect(() => {
    if (!tg) return;
    const paint = () => {
      if (!tg.isVersionAtLeast("6.1")) return;
      const bg = cssVar("--bg");
      const surface = cssVar("--surface");
      if (/^#[0-9a-f]{6}$/i.test(bg)) {
        tg.setHeaderColor?.(bg);
        tg.setBackgroundColor?.(bg);
      }
      if (/^#[0-9a-f]{6}$/i.test(surface) && tg.isVersionAtLeast("7.10")) tg.setBottomBarColor?.(surface);
    };
    paint();
    const obs = new MutationObserver(paint);
    obs.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme"] });
    return () => obs.disconnect();
  }, [tg]);

  // 3. Кнопка «Назад» Telegram — на всех экранах, кроме вкладок нижней панели.
  useEffect(() => {
    if (!tg || !tg.isVersionAtLeast("6.1")) return;
    const back = () => {
      if (window.history.length > 1) router.back();
      else router.replace("/");
    };
    if (isTabRoot(pathname)) {
      tg.BackButton.hide();
      return;
    }
    tg.BackButton.show();
    tg.BackButton.onClick(back);
    return () => tg.BackButton.offClick(back);
  }, [tg, pathname, router]);

  // 4. Автовход: Telegram уже привязан к покупателю — входим без кнопок (один раз за сессию).
  useEffect(() => {
    if (!tg || !ready || me || tried.current || !tg.initData) return;
    tried.current = true;
    try {
      if (sessionStorage.getItem(TRIED) === tg.initData.slice(-64)) return;
      sessionStorage.setItem(TRIED, tg.initData.slice(-64));
    } catch {}
    loginTelegramWebApp(tg.initData).then(() => reload(), () => {});
  }, [tg, ready, me, reload]);

  return null;
}
