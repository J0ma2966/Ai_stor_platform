import { getRequestConfig } from "next-intl/server";
import { hasLocale } from "next-intl";
import { routing } from "./routing";
import { mergeMessages } from "./messages";

export default getRequestConfig(async ({ requestLocale }) => {
  const requested = await requestLocale;
  const locale = hasLocale(routing.locales, requested) ? requested : routing.defaultLocale;
  const [base, admin] = await Promise.all([
    import(`../../messages/${locale}.json`),
    import(`../../messages/admin.${locale}.json`),
  ]);
  return {
    locale,
    messages: mergeMessages(base.default, admin.default),
  };
});
