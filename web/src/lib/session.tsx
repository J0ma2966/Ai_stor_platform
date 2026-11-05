"use client";
// Общее состояние сессии для шапки и кнопок: кто вошёл, баланс, счётчики корзины,
// уведомлений, избранного. Access-токен в памяти, поэтому при загрузке страницы
// один раз пробуем восстановить сессию по refresh-cookie.
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { api, currentRole, logout as apiLogout, refreshSession, type Role } from "./api";
import { getCart, getFavoriteIds, getUnread } from "./shop";

export type Me = {
  id: string;
  role: Role;
  phone?: string;
  login?: string;
  store_id?: string;
  gender: "male" | "female" | null;
  lang: "ru" | "uz";
  token_balance: number;
  height_cm: number | null;
  weight_kg: number | null;
  body_params: Record<string, number>;
  registration_complete: boolean;
  has_password: boolean;
  pd_consent_at: string | null;
  created_at: string;
};

type SessionState = {
  ready: boolean;
  me: Me | null;
  cartQty: number;
  unread: number;
  favorites: Set<string>;
  reload: () => Promise<void>;
  setMe: (m: Me) => void;
  setCartQty: (n: number) => void;
  setUnread: (n: number) => void;
  toggleFavoriteLocal: (id: string, on: boolean) => void;
  signOut: () => Promise<void>;
};

const Ctx = createContext<SessionState | null>(null);

export function SessionProvider({ children }: { children: ReactNode }) {
  const [ready, setReady] = useState(false);
  const [me, setMe] = useState<Me | null>(null);
  const [cartQty, setCartQty] = useState(0);
  const [unread, setUnread] = useState(0);
  const [favorites, setFavorites] = useState<Set<string>>(new Set());
  const started = useRef(false);

  const loadCounters = useCallback(async (m: Me) => {
    const tasks: Promise<unknown>[] = [getUnread().then((r) => setUnread(r.unread)).catch(() => {})];
    if (m.role === "client") {
      tasks.push(getCart().then((c) => setCartQty(c.total_qty + c.items.filter((i) => !i.available).length)).catch(() => {}));
      tasks.push(getFavoriteIds().then((ids) => setFavorites(new Set(ids))).catch(() => {}));
    }
    await Promise.all(tasks);
  }, []);

  const reload = useCallback(async () => {
    try {
      if (!currentRole() && !(await refreshSession())) {
        setMe(null);
        return;
      }
      const m = await api<Me>("/v1/me");
      setMe(m);
      await loadCounters(m);
    } catch {
      setMe(null);
    } finally {
      setReady(true);
    }
  }, [loadCounters]);

  useEffect(() => {
    if (started.current) return;
    started.current = true;
    reload();
  }, [reload]);

  // уведомления: обновляем счётчик раз в минуту, пока вкладка открыта
  useEffect(() => {
    if (!me) return;
    const id = setInterval(() => {
      if (document.visibilityState === "visible") getUnread().then((r) => setUnread(r.unread)).catch(() => {});
    }, 60000);
    return () => clearInterval(id);
  }, [me]);

  const value = useMemo<SessionState>(
    () => ({
      ready,
      me,
      cartQty,
      unread,
      favorites,
      reload,
      setMe,
      setCartQty,
      setUnread,
      toggleFavoriteLocal: (id, on) =>
        setFavorites((prev) => {
          const n = new Set(prev);
          if (on) n.add(id);
          else n.delete(id);
          return n;
        }),
      signOut: async () => {
        await apiLogout().catch(() => {});
        setMe(null);
        setCartQty(0);
        setUnread(0);
        setFavorites(new Set());
      },
    }),
    [ready, me, cartQty, unread, favorites, reload],
  );
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

const EMPTY_FAVORITES: Set<string> = new Set();

// До монтирования компонента отдаём то же, что видел сервер (сессия ещё не загружена).
// Иначе компонент, который гидрируется позже (внутри Suspense), получил бы уже
// загруженную сессию и разошёлся бы с серверным HTML (React #418).
export function useSession(): SessionState {
  const v = useContext(Ctx);
  if (!v) throw new Error("useSession outside SessionProvider");
  const [mounted, setMounted] = useState(false);
  useEffect(() => setMounted(true), []);
  if (mounted) return v;
  return { ...v, ready: false, me: null, cartQty: 0, unread: 0, favorites: EMPTY_FAVORITES };
}
