"use client";
import { useCallback, useEffect, useRef, useState } from "react";

type Page<T> = { items: T[]; nextCursor: string };

// Список с курсорной пагинацией: перезагружается при смене фильтров (key),
// «Показать ещё» дописывает следующую страницу. Ответы устаревших запросов отбрасываются.
export function useCursorList<T>(fetchPage: (cursor: string) => Promise<Page<T>>, key: string) {
  const [items, setItems] = useState<T[]>([]);
  const [cursor, setCursor] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);
  const gen = useRef(0);
  const fetchRef = useRef(fetchPage);
  fetchRef.current = fetchPage;

  const load = useCallback(async (from = "") => {
    const my = ++gen.current;
    setLoading(true);
    setError(null);
    try {
      const r = await fetchRef.current(from);
      if (my !== gen.current) return;
      setItems((prev) => (from ? [...prev, ...r.items] : r.items));
      setCursor(r.nextCursor);
    } catch (e) {
      if (my === gen.current) setError(e);
    } finally {
      if (my === gen.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    load("");
  }, [key, load]);

  return {
    items,
    setItems,
    loading,
    error,
    hasMore: cursor !== "",
    more: () => load(cursor),
    reload: () => load(""),
  };
}
