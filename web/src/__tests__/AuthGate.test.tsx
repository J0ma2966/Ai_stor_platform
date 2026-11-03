import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NextIntlClientProvider } from "next-intl";
import messages from "../../messages/ru.json";

const replace = vi.fn();
vi.mock("@/i18n/navigation", () => ({
  useRouter: () => ({ replace }),
  usePathname: () => "/store/orders",
}));

import { AuthGate } from "@/components/AuthGate";
import { setSession } from "@/lib/api";

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
const ok = () => json(200, { data: { access_token: "a", user: { id: "u", role: "store" } } });

const wrap = () =>
  render(
    <NextIntlClientProvider locale="ru" messages={messages}>
      <AuthGate role="store"><p>cabinet</p></AuthGate>
    </NextIntlClientProvider>,
  );

describe("AuthGate after a page reload", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    replace.mockReset();
    setSession(null);
  });

  it("keeps the user on the page when refresh is rate limited, and retries", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(json(429, { error: { code: "rate_limited", message: "x" } }))
      .mockResolvedValueOnce(ok());
    vi.stubGlobal("fetch", fetchMock);
    wrap();
    expect(await screen.findByText(messages.errors.rate_limited)).toBeTruthy();
    expect(replace).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole("button", { name: messages.app.retry }));
    expect(await screen.findByText("cabinet")).toBeTruthy();
    expect(replace).not.toHaveBeenCalled();
  });

  it("does not log out when the API is unreachable", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new TypeError("Failed to fetch")));
    wrap();
    expect(await screen.findByText(messages.errors.network)).toBeTruthy();
    expect(replace).not.toHaveBeenCalled();
  });

  it("sends to login when the refresh cookie is invalid", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json(401, { error: { code: "refresh_invalid", message: "x" } })));
    wrap();
    await waitFor(() => expect(replace).toHaveBeenCalledWith("/store/login?next=%2Fstore%2Forders"));
    expect(screen.queryByText("cabinet")).toBeNull();
  });
});
