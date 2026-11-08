// Каждая внутренняя ссылка в коде (href, router.push/replace, redirect) должна вести на
// существующую страницу в src/app. Ловит опечатки вроде /products/<id> вместо /product/<id>.
import fs from "node:fs";
import path from "node:path";

const SRC = path.resolve(__dirname, "..");
const APP = path.join(SRC, "app");

function walk(dir: string, out: string[] = []): string[] {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name);
    if (e.isDirectory()) {
      if (e.name !== "__tests__" && e.name !== "node_modules") walk(p, out);
    } else if (/\.(tsx?|jsx?)$/.test(e.name)) out.push(p);
  }
  return out;
}

// Маршруты: путь к page.tsx/route.ts без групп (panel); [x] → любой сегмент.
const routes = walk(APP)
  .filter((f) => /[\\/](page|route)\.tsx?$/.test(f))
  .map((f) => {
    const segs = path.relative(APP, path.dirname(f)).split(path.sep).filter((s) => s && !/^\(.*\)$/.test(s));
    return new RegExp("^/" + segs.map((s) => (/^\[.+\]$/.test(s) ? "[^/]+" : s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"))).join("/") + "$");
  });

const resolves = (p: string) => routes.some((r) => r.test(p === "/" ? "/" : p.replace(/\/$/, "")));

// Литералы '/x', "/x", `/x/${id}` после href=, href:, push(, replace(, redirect(
const LINK = /(?:href\s*[=:]\s*\{?\s*|(?:push|replace|redirect)\(\s*)(["'`])(\/[^"'`\s]*)\1/g;

function collect() {
  const found: { file: string; link: string; path: string }[] = [];
  for (const f of walk(SRC)) {
    if (f.startsWith(path.join(SRC, "__tests__"))) continue;
    const text = fs.readFileSync(f, "utf8");
    for (const m of text.matchAll(LINK)) {
      const link = m[2];
      if (link.startsWith("//") || link.startsWith("/api/") || link.startsWith("/_next")) continue;
      // ${...} в шаблоне — один сегмент; query/hash отбрасываем
      const p = link.replace(/\$\{[^}]*\}/g, "x").split(/[?#]/)[0] || "/";
      found.push({ file: path.relative(SRC, f), link, path: p });
    }
  }
  return found;
}

describe("internal links", () => {
  it("finds the app routes and links", () => {
    expect(routes.length).toBeGreaterThan(40);
    expect(collect().length).toBeGreaterThan(50);
  });

  it("every internal link points to an existing page", () => {
    const broken = collect().filter((l) => !resolves(l.path));
    expect(broken).toEqual([]);
  });
});
