"use client";
// Замена next/link и next/navigation с учётом языка: ссылки и переходы пишутся без префикса
// (href="/catalog"), а префикс /uz добавляется сам по текущему языку.
import NextLink from "next/link";
import { usePathname as useNextPathname, useRouter as useNextRouter } from "next/navigation";
import { useLocale } from "next-intl";
import { createElement, forwardRef, useMemo, type ComponentPropsWithoutRef } from "react";
import { localizePath, stripLocale, type Locale } from "./routing";

type LinkProps = Omit<ComponentPropsWithoutRef<typeof NextLink>, "href" | "locale"> & {
  href: string;
  /** Ссылка на ту же страницу на другом языке. */
  locale?: Locale;
};

export const Link = forwardRef<HTMLAnchorElement, LinkProps>(function Link({ href, locale, ...rest }, ref) {
  const current = useLocale() as Locale;
  return createElement(NextLink, { ref, href: localizePath(href, locale ?? current), ...rest });
});

/** Текущий путь без языкового префикса: на /uz/catalog вернёт "/catalog". */
export function usePathname(): string {
  return stripLocale(useNextPathname() ?? "/");
}

type NavOpts = { scroll?: boolean; locale?: Locale };

export function useRouter() {
  const router = useNextRouter();
  const current = useLocale() as Locale;
  return useMemo(
    () => ({
      push: (href: string, o: NavOpts = {}) => router.push(localizePath(href, o.locale ?? current), { scroll: o.scroll }),
      replace: (href: string, o: NavOpts = {}) => router.replace(localizePath(href, o.locale ?? current), { scroll: o.scroll }),
      prefetch: (href: string) => router.prefetch(localizePath(href, current)),
      back: () => router.back(),
      forward: () => router.forward(),
      refresh: () => router.refresh(),
    }),
    [router, current],
  );
}
