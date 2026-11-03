import { getRequestConfig } from "next-intl/server";
import { headers } from "next/headers";
import { defaultLocale, isLocale, LOCALE_HEADER } from "./routing";
import { mergeMessages } from "./messages";

// Язык запроса определяет middleware (префикс /uz или cookie) и передаёт его заголовком.
export default getRequestConfig(async () => {
  const h = (await headers()).get(LOCALE_HEADER);
  const locale = isLocale(h) ? h : defaultLocale;
  const [base, admin] = await Promise.all([
    import(`../../messages/${locale}.json`),
    import(`../../messages/admin.${locale}.json`),
  ]);
  return {
    locale,
    messages: mergeMessages(base.default, admin.default),
  };
});
