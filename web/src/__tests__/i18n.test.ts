// @vitest-environment node
import { describe, expect, it } from "vitest";
import { NextRequest } from "next/server";
import ru from "../../messages/ru.json";
import uz from "../../messages/uz.json";
import adminRu from "../../messages/admin.ru.json";
import adminUz from "../../messages/admin.uz.json";
import { localeFromPath, localizePath, prefixLocale, stripLocale } from "@/i18n/routing";
import middleware from "@/middleware";
import { alternates } from "@/lib/site";

describe("адреса языков", () => {
  it("добавляет и убирает префикс /uz", () => {
    expect(localizePath("/catalog?q=1#top", "uz")).toBe("/uz/catalog?q=1#top");
    expect(localizePath("/", "uz")).toBe("/uz");
    expect(localizePath("/?q=1", "uz")).toBe("/uz?q=1");
    expect(localizePath("/uz/catalog", "ru")).toBe("/catalog");
    expect(localizePath("/uz", "ru")).toBe("/");
    expect(localizePath("/uz/catalog", "uz")).toBe("/uz/catalog");
    expect(localizePath("https://example.com/x", "uz")).toBe("https://example.com/x");
    expect(localizePath("//evil.com", "uz")).toBe("//evil.com");
    expect(localizePath("#main", "uz")).toBe("#main");
  });

  it("не путает похожие сегменты с языком", () => {
    expect(prefixLocale("/uzbek")).toBeNull();
    expect(stripLocale("/uzbek")).toBe("/uzbek");
    expect(localeFromPath("/uz/stores")).toBe("uz");
    expect(localeFromPath("/stores")).toBe("ru");
  });

  it("canonical и hreflang указывают на обе версии", () => {
    const a = alternates("uz", "/catalog");
    expect(a.canonical).toBe("http://localhost:3000/uz/catalog");
    expect(a.languages).toEqual({
      ru: "http://localhost:3000/catalog",
      uz: "http://localhost:3000/uz/catalog",
      "x-default": "http://localhost:3000/catalog",
    });
  });
});

function req(path: string, headers: Record<string, string> = {}) {
  return new NextRequest(new URL(path, "http://localhost:3000"), { headers });
}

describe("middleware", () => {
  it("старые адреса /ru/... навсегда ведут на адрес без префикса", () => {
    const r = middleware(req("/ru/catalog?q=1"));
    expect(r.status).toBe(308);
    expect(r.headers.get("location")).toBe("http://localhost:3000/catalog?q=1");
  });

  it("/uz/... показывает ту же страницу на узбекском", () => {
    const r = middleware(req("/uz/product/1"));
    expect(r.headers.get("x-middleware-rewrite")).toBe("http://localhost:3000/product/1");
    expect(r.headers.get("x-middleware-request-x-locale")).toBe("uz");
    expect(r.headers.get("content-security-policy")).toContain("nonce-");
  });

  it("робот без cookie видит русскую страницу без редиректа", () => {
    const r = middleware(req("/catalog", { "accept-language": "en-US,en;q=0.9" }));
    expect(r.status).toBe(200);
    expect(r.headers.get("x-middleware-request-x-locale")).toBe("ru");
  });

  it("возвращает человека на выбранный язык", () => {
    const r = middleware(req("/catalog?q=1", { cookie: "NEXT_LOCALE=uz" }));
    expect(r.status).toBe(307);
    expect(r.headers.get("location")).toBe("http://localhost:3000/uz/catalog?q=1");
  });

  it("при первом заходе учитывает узбекский в браузере", () => {
    expect(middleware(req("/", { "accept-language": "uz-UZ,uz;q=0.9,ru;q=0.8" })).headers.get("location")).toBe("http://localhost:3000/uz");
    expect(middleware(req("/", { "accept-language": "ru-RU,ru;q=0.9,uz;q=0.8" })).status).toBe(200);
  });

  it("догрузки роутера идут на языке вкладки из cookie", () => {
    const r = middleware(req("/uz/catalog", { cookie: "NEXT_LOCALE=ru", "sec-fetch-dest": "empty" }));
    expect(r.headers.get("x-middleware-request-x-locale")).toBe("ru");
    const r2 = middleware(req("/catalog", { cookie: "NEXT_LOCALE=uz", "sec-fetch-dest": "empty" }));
    expect(r2.status).toBe(200);
    expect(r2.headers.get("x-middleware-request-x-locale")).toBe("uz");
  });

  it("открытая ссылка /uz/... важнее cookie", () => {
    const r = middleware(req("/uz/catalog", { cookie: "NEXT_LOCALE=ru", "sec-fetch-dest": "document" }));
    expect(r.headers.get("x-middleware-request-x-locale")).toBe("uz");
  });
});

type Tree = { [k: string]: string | Tree };
function flat(t: Tree, p = ""): Record<string, string> {
  return Object.entries(t).reduce<Record<string, string>>((o, [k, v]) => {
    if (typeof v === "string") o[p + k] = v;
    else Object.assign(o, flat(v, `${p}${k}.`));
    return o;
  }, {});
}
const vars = (s: string) => [...s.matchAll(/\{(\w+)\s*[,}]/g)].map((m) => m[1]).sort();

describe("переводы", () => {
  for (const [name, a, b] of [["messages", ru, uz], ["admin", adminRu, adminUz]] as const) {
    const r = flat(a as Tree);
    const u = flat(b as Tree);
    it(`${name}: одинаковые ключи в ru и uz`, () => {
      expect(Object.keys(u).sort()).toEqual(Object.keys(r).sort());
    });
    it(`${name}: одинаковые подстановки в ru и uz`, () => {
      const bad = Object.keys(r).filter((k) => u[k] !== undefined && JSON.stringify([...new Set(vars(r[k]))]) !== JSON.stringify([...new Set(vars(u[k]))]));
      expect(bad).toEqual([]);
    });
    it(`${name}: в узбекских текстах нет кириллицы`, () => {
      const bad = Object.entries(u).filter(([k, v]) => k !== "app.lang" && /[Ѐ-ӿ]/.test(v));
      expect(bad).toEqual([]);
    });
  }
});
