import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NextIntlClientProvider } from "next-intl";
import messages from "../../messages/ru.json";
import { setSession } from "@/lib/api";

const replace = vi.fn();
vi.mock("@/i18n/navigation", () => ({ useRouter: () => ({ replace }) }));
vi.mock("next/navigation", () => ({ useSearchParams: () => new URLSearchParams("next=/checkout") }));
vi.mock("@/lib/session", () => ({ useSession: () => ({ reload: vi.fn().mockResolvedValue(undefined) }) }));

import ClientLogin from "@/app/login/page";

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
const session = () => json(200, { data: { access_token: "a", user: { id: "u", role: "client" } } });
const me = (over: object = {}) =>
  json(200, { data: { id: "u", role: "client", registration_complete: true, has_password: false, needs_password: false, ...over } });

type Methods = { email: boolean; sms: boolean; google: null | { client_id: string }; telegram: null | { bot_username: string } };
const prodMethods: Methods = { email: true, sms: false, google: null, telegram: { bot_username: "shop_bot" } };

// fetch-заглушка по адресам: ответы для каждого пути по очереди.
function mockApi(routes: Record<string, Array<() => Response>>, methods: Methods = prodMethods) {
  const calls: Array<{ url: string; body: unknown }> = [];
  const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
    calls.push({ url, body: init?.body ? JSON.parse(String(init.body)) : undefined });
    if (url.endsWith("/v1/auth/methods")) return json(200, { data: methods });
    const key = Object.keys(routes).find((k) => url.endsWith(k));
    const next = key ? routes[key].shift() : undefined;
    if (!next) throw new Error("unexpected fetch " + url);
    return next();
  });
  vi.stubGlobal("fetch", fetchMock);
  return calls;
}

const wrap = () =>
  render(
    <NextIntlClientProvider locale="ru" messages={messages}>
      <ClientLogin />
    </NextIntlClientProvider>,
  );

describe("вход клиента", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    replace.mockReset();
    setSession(null);
  });

  it("показывает Telegram и почту, без входа по SMS", async () => {
    mockApi({});
    wrap();
    expect(await screen.findByRole("button", { name: /Войти через Telegram/ })).toBeTruthy();
    expect(screen.getByLabelText("Электронная почта")).toBeTruthy();
    expect(screen.queryByLabelText("Номер телефона")).toBeNull();
    expect(screen.queryByRole("tab", { name: "Телефон" })).toBeNull();
  });

  it("входит по почте и паролю", async () => {
    const calls = mockApi({ "/v1/auth/email/login": [session], "/v1/me": [me] });
    wrap();
    const user = userEvent.setup();
    await user.type(await screen.findByLabelText("Электронная почта"), "Anna@Test.uz");
    await user.type(screen.getByLabelText("Пароль"), "my-password-1");
    await user.click(screen.getByRole("button", { name: "Войти" }));
    await waitFor(() => expect(replace).toHaveBeenCalledWith("/checkout"));
    expect(calls.find((c) => c.url.endsWith("/email/login"))?.body).toEqual({ email: "anna@test.uz", password: "my-password-1" });
  });

  it("регистрирует по коду из письма и ведёт на завершение регистрации", async () => {
    const calls = mockApi({
      "/v1/auth/email/code": [() => json(202, { data: { sent: true } })],
      "/v1/auth/email/register": [session],
      "/v1/me": [() => me({ registration_complete: false })],
    });
    wrap();
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Нет аккаунта? Зарегистрироваться" }));
    await user.type(screen.getByLabelText("Электронная почта"), "new@test.uz");
    await user.click(screen.getByRole("button", { name: "Получить код" }));
    expect(await screen.findByText(/Отправили код на new@test.uz/)).toBeTruthy();
    await user.type(screen.getByLabelText("Код из письма"), "12a3456");
    await user.type(screen.getByLabelText("Пароль"), "long-password-1");
    await user.type(screen.getByLabelText("Повторите пароль"), "long-password-1");
    await user.click(screen.getByRole("button", { name: "Зарегистрироваться" }));
    await waitFor(() => expect(replace).toHaveBeenCalledWith("/account/welcome?next=%2Fcheckout"));
    expect(calls.find((c) => c.url.endsWith("/email/code"))?.body).toEqual({ email: "new@test.uz", purpose: "register", lang: "ru" });
    expect(calls.find((c) => c.url.endsWith("/email/register"))?.body).toEqual({ email: "new@test.uz", code: "123456", password: "long-password-1" });
  });

  it("вход через Telegram: открывает бота и ждёт подтверждения", async () => {
    const tab = { opener: {}, location: { href: "" }, close: vi.fn() };
    vi.stubGlobal("open", vi.fn(() => tab));
    const calls = mockApi({
      "/v1/auth/telegram/start": [
        () => json(200, { data: { login_id: "L1", poll_token: "P1", url: "https://t.me/shop_bot?start=login_L1", expires_at: "" } }),
      ],
      "/v1/auth/telegram/poll": [() => json(200, { data: { status: "pending" } }), session],
      "/v1/me": [me],
    });
    wrap();
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: /Войти через Telegram/ }));
    expect(tab.location.href).toBe("https://t.me/shop_bot?start=login_L1");
    expect(tab.opener).toBeNull();
    expect(await screen.findByText("Подтвердите вход в Telegram")).toBeTruthy();
    await waitFor(() => expect(replace).toHaveBeenCalledWith("/checkout"), { timeout: 6000 });
    expect(calls.filter((c) => c.url.endsWith("/telegram/poll"))[0].body).toEqual({ login_id: "L1", poll_token: "P1" });
  }, 10000);

  it("вход по номеру и паролю появляется, только если SMS включён на сервере", async () => {
    const calls = mockApi({ "/v1/auth/password/login": [session], "/v1/me": [me] }, { ...prodMethods, sms: true });
    wrap();
    const user = userEvent.setup();
    await user.click(await screen.findByRole("tab", { name: "Телефон" }));
    await user.clear(screen.getByLabelText("Номер телефона"));
    await user.type(screen.getByLabelText("Номер телефона"), "+998901234567");
    await user.type(screen.getByLabelText("Пароль"), "my-password-1");
    await user.click(screen.getByRole("button", { name: "Войти" }));
    await waitFor(() => expect(replace).toHaveBeenCalledWith("/checkout"));
    expect(calls.find((c) => c.url.endsWith("/password/login"))?.body).toEqual({ phone: "+998901234567", password: "my-password-1" });
  });
});
