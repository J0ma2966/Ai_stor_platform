// Telegram Mini App: магазин, открытый кнопкой в нашем боте.
// Официальный скрипт Telegram грузим только внутри Telegram (в обычном браузере он не нужен).
// CSP: script-src с 'strict-dynamic' разрешает скрипт, добавленный нашим же JS с nonce.

export const TG_SCRIPT = "https://telegram.org/js/telegram-web-app.js";
const FLAG = "tg-app";
/** cookie на время сессии: сервер сразу рисует страницу в виде Mini App (без подвала), без мигания */
export const TG_COOKIE = "tgapp";

type BackButton = { show(): void; hide(): void; onClick(cb: () => void): void; offClick(cb: () => void): void };
export type TelegramWebApp = {
  initData: string;
  version: string;
  platform: string;
  colorScheme: "light" | "dark";
  ready(): void;
  expand(): void;
  isVersionAtLeast(v: string): boolean;
  disableVerticalSwipes?: () => void;
  setHeaderColor?: (c: string) => void;
  setBackgroundColor?: (c: string) => void;
  setBottomBarColor?: (c: string) => void;
  openTelegramLink(url: string): void;
  openLink(url: string): void;
  onEvent(e: string, cb: () => void): void;
  offEvent(e: string, cb: () => void): void;
  BackButton: BackButton;
};

declare global {
  interface Window {
    Telegram?: { WebApp?: TelegramWebApp };
  }
}

/** Открыта ли страница внутри Telegram. Telegram передаёт данные запуска в #tgWebAppData=… */
export function isTelegramApp(): boolean {
  if (typeof window === "undefined") return false;
  try {
    if (/[#&]tgWebAppData=/.test(window.location.hash)) {
      sessionStorage.setItem(FLAG, "1");
      return true;
    }
    return sessionStorage.getItem(FLAG) === "1";
  } catch {
    return false;
  }
}

/** Уже загруженный объект Telegram.WebApp (или null вне Telegram / до загрузки). */
export function telegramWebApp(): TelegramWebApp | null {
  if (typeof window === "undefined") return null;
  const w = window.Telegram?.WebApp;
  return w && w.platform && w.platform !== "unknown" ? w : null;
}

let loading: Promise<TelegramWebApp | null> | null = null;

export function loadTelegramWebApp(): Promise<TelegramWebApp | null> {
  if (!isTelegramApp()) return Promise.resolve(null);
  const ready = telegramWebApp();
  if (ready) return Promise.resolve(ready);
  loading ??= new Promise((resolve) => {
    const s = document.createElement("script");
    s.src = TG_SCRIPT;
    s.async = true;
    s.onload = () => resolve(telegramWebApp());
    s.onerror = () => {
      loading = null;
      resolve(null);
    };
    document.head.appendChild(s);
  });
  return loading;
}
