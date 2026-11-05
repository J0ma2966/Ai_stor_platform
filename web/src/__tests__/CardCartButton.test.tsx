import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NextIntlClientProvider } from "next-intl";
import messages from "../../messages/ru.json";

const push = vi.fn();
const setCartQty = vi.fn();
let me: { role: string } | null = null;
vi.mock("@/i18n/navigation", () => ({ useRouter: () => ({ push }), usePathname: () => "/catalog" }));
vi.mock("@/lib/session", () => ({ useSession: () => ({ me, setCartQty }) }));
const putCart = vi.fn();
vi.mock("@/lib/shop", () => ({ setCartQty: (...a: unknown[]) => putCart(...a) }));

import { CardCartButton } from "@/components/CardCartButton";

const wrap = (inStock = true) =>
  render(
    <NextIntlClientProvider locale="ru" messages={messages}>
      <CardCartButton productId="p1" inStock={inStock} />
    </NextIntlClientProvider>,
  );

describe("кнопка «В корзину» в карточке", () => {
  afterEach(() => {
    push.mockReset();
    setCartQty.mockReset();
    putCart.mockReset();
    me = null;
  });

  it("гостя отправляет на вход и возвращает обратно", async () => {
    wrap();
    await userEvent.setup().click(screen.getByRole("button", { name: "В корзину" }));
    expect(push).toHaveBeenCalledWith("/login?next=%2Fcatalog");
    expect(putCart).not.toHaveBeenCalled();
  });

  it("клиент добавляет товар, счётчик обновляется, второй клик ведёт в корзину", async () => {
    me = { role: "client" };
    putCart.mockResolvedValue({ total_qty: 3 });
    wrap();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "В корзину" }));
    await waitFor(() => expect(setCartQty).toHaveBeenCalledWith(3));
    expect(putCart).toHaveBeenCalledWith("p1", 1);
    await user.click(screen.getByRole("button", { name: messages.product.inCart }));
    expect(push).toHaveBeenCalledWith("/cart");
  });

  it("товара нет в наличии — кнопка неактивна", () => {
    me = { role: "client" };
    wrap(false);
    expect(screen.getByRole("button", { name: messages.product.outOfStock })).toBeDisabled();
  });

  it("магазину и админу кнопку не показывает", () => {
    me = { role: "store" };
    const { container } = wrap();
    expect(container.querySelector("button")).toBeNull();
  });

  it("товар с размерами ведёт в карточку выбрать размер", async () => {
    me = { role: "client" };
    render(
      <NextIntlClientProvider locale="ru" messages={messages}>
        <CardCartButton productId="p1" inStock hasSizes />
      </NextIntlClientProvider>,
    );
    await userEvent.setup().click(screen.getByRole("button", { name: "Выберите размер" }));
    expect(push).toHaveBeenCalledWith("/product/p1");
    expect(putCart).not.toHaveBeenCalled();
  });
});
