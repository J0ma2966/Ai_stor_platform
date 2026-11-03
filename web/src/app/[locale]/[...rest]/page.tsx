import { notFound } from "next/navigation";

// Любой неизвестный адрес внутри языка — локализованная страница 404.
export default function CatchAll() {
  notFound();
}
