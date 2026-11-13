import { describe, expect, it } from "vitest";
import { applyTheme, parseTheme, themeAttr, themeCookie } from "@/lib/theme";

describe("theme", () => {
  it("parses only known values, everything else is system", () => {
    expect(parseTheme("dark")).toBe("dark");
    expect(parseTheme("light")).toBe("light");
    expect(parseTheme(undefined)).toBe("system");
    expect(parseTheme("<script>")).toBe("system");
  });
  it("system has no data-theme and clears the cookie", () => {
    expect(themeAttr("system")).toBeUndefined();
    expect(themeCookie("system")).toContain("max-age=0");
    expect(themeCookie("dark")).toMatch(/^theme=dark; path=\/; max-age=31536000/);
  });
  it("applies to <html>", () => {
    applyTheme("dark");
    expect(document.documentElement.dataset.theme).toBe("dark");
    applyTheme("system");
    expect(document.documentElement.dataset.theme).toBeUndefined();
  });
});
