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
vi.mock("@/components/EmailAuthForm", () => ({ EmailAuthForm: () => null }));

import ClientLogin from "@/app/login/page";

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
const session = () => json(200, { data: { access_token: "a", user: { id: "u", role: "client" } } });
const me = (over: object) => json(200, { data: { id: "u", role: "client", registration_complete: true, has_password: true, ...over } });

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

  it("по умолчанию входит по паролю без SMS", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(session()).mockResolvedValueOnce(me({}));
    vi.stubGlobal("fetch", fetchMock);
    wrap();
    const user = userEvent.setup();
    await user.clear(screen.getByLabelText("Номер телефона"));
    await user.type(screen.getByLabelText("Номер телефона"), "+998901234567");
    await user.type(screen.getByLabelText("Пароль"), "my-password-1");
    await user.click(screen.getByRole("button", { name: "Войти" }));
    await waitFor(() => expect(replace).toHaveBeenCalledWith("/checkout"));
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain("/v1/auth/password/login");
    expect(JSON.parse(init.body)).toEqual({ phone: "+998901234567", password: "my-password-1" });
    expect(fetchMock.mock.calls.some(([u]) => String(u).includes("/otp/"))).toBe(false);
  });

  it("после входа по SMS без пароля ведёт на создание пароля", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(json(202, { data: { sent: true } }))
      .mockResolvedValueOnce(session())
      .mockResolvedValueOnce(me({ has_password: false }));
    vi.stubGlobal("fetch", fetchMock);
    wrap();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Регистрация / вход по SMS" }));
    await user.click(screen.getByRole("button", { name: "Получить код" }));
    await user.type(await screen.findByLabelText("Код из SMS"), "123456");
    await user.click(screen.getByRole("button", { name: "Войти" }));
    await waitFor(() => expect(replace).toHaveBeenCalledWith("/account/password?next=%2Fcheckout"));
    expect(fetchMock.mock.calls[1][0]).toContain("/v1/auth/otp/verify");
  });

  it("«Забыли пароль?»: код из SMS и новый пароль", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(json(202, { data: { sent: true } }))
      .mockResolvedValueOnce(session())
      .mockResolvedValueOnce(me({}));
    vi.stubGlobal("fetch", fetchMock);
    wrap();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Забыли пароль?" }));
    await user.click(screen.getByRole("button", { name: "Получить код" }));
    await user.type(await screen.findByLabelText("Код из SMS"), "654321");
    await user.type(screen.getByLabelText("Новый пароль"), "new-password-1");
    await user.click(screen.getByRole("button", { name: "Сохранить пароль и войти" }));
    await waitFor(() => expect(replace).toHaveBeenCalledWith("/checkout"));
    const [url, init] = fetchMock.mock.calls[1];
    expect(url).toContain("/v1/auth/password/reset");
    expect(JSON.parse(init.body)).toEqual({ phone: "+998", code: "654321", password: "new-password-1" });
  });
});
