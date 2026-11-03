import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { NextIntlClientProvider } from "next-intl";
import base from "../../messages/ru.json";
import adminRu from "../../messages/admin.ru.json";
import adminUz from "../../messages/admin.uz.json";
import baseUz from "../../messages/uz.json";
import { mergeMessages } from "@/i18n/messages";
import { ApiError } from "@/lib/api";
import { generatePassword, qs, sumToTiyin, userTitle, LOGIN_RE, SLUG_RE } from "@/lib/admin";

const createStore = vi.fn();
const setStoreStatus = vi.fn();
vi.mock("@/lib/admin", async (orig) => {
  const m = await orig<typeof import("@/lib/admin")>();
  return {
    ...m,
    adminApi: { ...m.adminApi, createStore: (...a: unknown[]) => createStore(...a), setStoreStatus: (...a: unknown[]) => setStoreStatus(...a) },
  };
});

import { CreateStoreForm } from "@/components/admin/CreateStoreForm";
import { ConfirmAction } from "@/components/admin/ConfirmAction";

const messages = mergeMessages(base, adminRu);
const wrap = (ui: React.ReactNode) =>
  render(
    <NextIntlClientProvider locale="ru" messages={messages} timeZone="Asia/Tashkent">
      {ui}
    </NextIntlClientProvider>,
  );

function keys(o: object, p = ""): string[] {
  return Object.entries(o).flatMap(([k, v]) => (v && typeof v === "object" ? keys(v, `${p}${k}.`) : [`${p}${k}`]));
}

describe("admin messages", () => {
  it("ru and uz have the same keys", () => {
    expect(keys(adminUz).sort()).toEqual(keys(adminRu).sort());
  });
  it("merge keeps base keys and adds admin ones", () => {
    const m = mergeMessages(baseUz, adminUz) as Record<string, Record<string, string>>;
    expect(m.errors!.network).toBe((baseUz as unknown as Record<string, Record<string, string>>).errors!.network);
    expect(m.errors!.slug_taken).toBeTruthy();
    expect(m.admin).toBeTruthy();
  });
});

describe("admin helpers", () => {
  it("converts sums to tiyin", () => {
    expect(sumToTiyin("150 000")).toBe(15000000);
    expect(sumToTiyin("1,5")).toBe(150);
    expect(sumToTiyin("0")).toBeNull();
    expect(sumToTiyin("abc")).toBeNull();
  });
  it("generates passwords that pass server rules", () => {
    const p = generatePassword();
    expect(p).toHaveLength(14);
    expect(generatePassword()).not.toBe(p);
  });
  it("builds query strings without empty params", () => {
    expect(qs({ a: "", b: "x y", c: undefined })).toBe("?b=x+y");
    expect(qs({})).toBe("");
  });
  it("validates slug and login like the DB", () => {
    expect(SLUG_RE.test("butik-lola")).toBe(true);
    expect(SLUG_RE.test("-bad")).toBe(false);
    expect(LOGIN_RE.test("Lola")).toBe(false);
    expect(LOGIN_RE.test("lola.shop")).toBe(true);
  });
  it("titles users by store, phone or login", () => {
    expect(userTitle({ phone: null, login: "lola", store_name: "Lola" })).toBe("Lola (lola)");
    expect(userTitle({ phone: "+998901112233", login: null, store_name: null })).toBe("+998901112233");
  });
});

describe("CreateStoreForm", () => {
  afterEach(() => vi.clearAllMocks());
  const fill = (label: RegExp, v: string) => fireEvent.change(screen.getByLabelText(label), { target: { value: v } });

  it("validates locally before calling the API", async () => {
    wrap(<CreateStoreForm onCreated={() => {}} />);
    fill(/Адрес \(slug\)/, "Bad Slug");
    fireEvent.click(screen.getByRole("button", { name: "Создать" }));
    expect(await screen.findByText("Обязательное поле")).toBeInTheDocument();
    expect(screen.getAllByText("Неверное значение")).toHaveLength(2);
    expect(createStore).not.toHaveBeenCalled();
  });

  it("creates, activates and shows the password once", async () => {
    createStore.mockResolvedValue({ store_id: "s1", user_id: "u1", status: "pending_activation" });
    setStoreStatus.mockResolvedValue({ id: "s1", status: "active" });
    const onCreated = vi.fn();
    wrap(<CreateStoreForm onCreated={onCreated} />);
    fill(/Название/, "Бутик Лола");
    fill(/Адрес \(slug\)/, "lola");
    fill(/^Логин/, "lola");
    fill(/^Пароль/, "very-strong-pass");
    fireEvent.click(screen.getByRole("button", { name: "Создать" }));
    expect(await screen.findByText("Магазин создан")).toBeInTheDocument();
    expect(createStore).toHaveBeenCalledWith({ name: "Бутик Лола", slug: "lola", login: "lola", password: "very-strong-pass", phone: undefined });
    expect(setStoreStatus).toHaveBeenCalledWith("s1", "active");
    expect(screen.getByText(/very-strong-pass/)).toBeInTheDocument();
    expect(onCreated).toHaveBeenCalled();
  });

  it("maps slug_taken to the slug field", async () => {
    createStore.mockRejectedValue(new ApiError(409, "slug_taken", "x"));
    wrap(<CreateStoreForm onCreated={() => {}} />);
    fill(/Название/, "Lola");
    fill(/Адрес \(slug\)/, "lola");
    fill(/^Логин/, "lola");
    fireEvent.click(screen.getByRole("button", { name: "Создать" }));
    expect(await screen.findByText("Уже занято")).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent("Такой адрес магазина уже занят.");
    expect(setStoreStatus).not.toHaveBeenCalled();
  });
});

describe("ConfirmAction", () => {
  it("requires a reason before running", async () => {
    const run = vi.fn();
    wrap(<ConfirmAction label="Заблокировать" danger needReason onConfirm={run} />);
    fireEvent.click(screen.getByRole("button", { name: "Заблокировать" }));
    const confirm = screen.getByRole("button", { name: "Заблокировать" });
    expect(confirm).toBeDisabled();
    fireEvent.change(screen.getByLabelText("Причина"), { target: { value: "спам" } });
    fireEvent.click(confirm);
    await waitFor(() => expect(run).toHaveBeenCalledWith("спам"));
  });
  it("can be cancelled", () => {
    const run = vi.fn();
    wrap(<ConfirmAction label="Активировать" onConfirm={run} />);
    fireEvent.click(screen.getByRole("button", { name: "Активировать" }));
    fireEvent.click(screen.getByRole("button", { name: "Отмена" }));
    expect(run).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Активировать" })).toBeInTheDocument();
  });
});

import { AuditList } from "@/components/admin/AuditList";
vi.mock("@/i18n/navigation", () => ({ Link: (p: { href: string; children: React.ReactNode }) => <a href={p.href}>{p.children}</a> }));

describe("AuditList", () => {
  it("translates known actions, links entities and shows data as text", () => {
    wrap(
      <AuditList
        items={[
          { id: 2, actor_id: "a", actor_role: "admin", actor_login: "root", action: "store.status", entity_type: "store", entity_id: "s1", data: { status: "blocked", reason: "<b>x</b>" }, ip: null, request_id: null, created_at: "2026-11-02T10:00:00Z" },
          { id: 1, actor_id: null, actor_role: null, actor_login: null, action: "custom.thing", entity_type: "x", entity_id: null, data: {}, ip: null, request_id: null, created_at: "2026-11-02T09:00:00Z" },
        ]}
      />,
    );
    expect(screen.getByText("Изменён статус магазина")).toBeInTheDocument();
    expect(screen.getByText("custom.thing")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Открыть" })).toHaveAttribute("href", "/admin/stores/s1");
    expect(screen.getByText(/reason: <b>x<\/b>/)).toBeInTheDocument();
  });
});
