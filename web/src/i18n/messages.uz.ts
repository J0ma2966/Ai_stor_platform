// Словарь языка «uz» для клиентских компонентов. Подключается только через import()
// в ClientMessages.tsx, поэтому попадает в отдельный JS-файл.
import base from "../../messages/uz.json";
import admin from "../../messages/admin.uz.json";
import { mergeMessages } from "./messages";

// Тот же состав, что отдаёт серверным компонентам src/i18n/request.ts.
export default mergeMessages(base, admin);
