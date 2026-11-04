// ВРЕМЕННО: тест формы входа по почте; удаляется вместе с EmailAuthForm.
import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NextIntlClientProvider } from "next-intl";
import messages from "../../messages/ru.json";
import { EmailAuthForm } from "@/components/EmailAuthForm";
import { setSession } from "@/lib/api";

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });

const wrap = (onSignedIn = vi.fn().mockResolvedValue(undefined)) => {
  render(
    <NextIntlClientProvider locale="ru" messages={messages}>
      <EmailAuthForm onSignedIn={onSignedIn} />
    </NextIntlClientProvider>,
  );
  return onSignedIn;
};

describe("EmailAuthForm", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    setSession(null);
  });

  it("is hidden when the server has it switched off", async () => {
    const fetchMock = vi.fn().mockResolvedValue(json(200, { data: { enabled: false } }));
    vi.stubGlobal("fetch", fetchMock);
    wrap();
    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    expect(screen.queryByLabelText("Электронная почта")).toBeNull();
  });

  it("registers with a normalized email and continues the sign-in flow", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(json(200, { data: { enabled: true } }))
      .mockResolvedValueOnce(json(200, { data: { access_token: "a", user: { id: "u", role: "client" }, is_new_user: true } }));
    vi.stubGlobal("fetch", fetchMock);
    const onSignedIn = wrap();
    const user = userEvent.setup();
    await user.click(await screen.findByText("Нет аккаунта? Зарегистрироваться"));
    await user.type(screen.getByLabelText("Электронная почта"), " Test@Mail.UZ ");
    await user.type(screen.getByLabelText("Пароль"), "long-enough-pw");
    await user.click(screen.getByRole("button", { name: "Зарегистрироваться" }));
    await waitFor(() => expect(onSignedIn).toHaveBeenCalled());
    const [url, init] = fetchMock.mock.calls[1];
    expect(url).toContain("/v1/auth/email/register");
    expect(JSON.parse(init.body)).toEqual({ email: "test@mail.uz", password: "long-enough-pw" });
  });

  it("shows a readable error when the email is taken", async () => {
    vi.stubGlobal("fetch", vi.fn()
      .mockResolvedValueOnce(json(200, { data: { enabled: true } }))
      .mockResolvedValueOnce(json(409, { error: { code: "email_taken", message: "x" } })));
    wrap();
    const user = userEvent.setup();
    await user.click(await screen.findByText("Нет аккаунта? Зарегистрироваться"));
    await user.type(screen.getByLabelText("Электронная почта"), "a@b.uz");
    await user.type(screen.getByLabelText("Пароль"), "long-enough-pw");
    await user.click(screen.getByRole("button", { name: "Зарегистрироваться" }));
    expect(await screen.findByText("Эта почта уже зарегистрирована. Войдите.")).toBeTruthy();
  });
});
