import type { Product } from "@/lib/catalog";
import { ProductCard } from "./ProductCard";

export function ProductGrid({ items, priority = 0 }: { items: Product[]; priority?: number }) {
  return (
    <ul className="pgrid plain">
      {items.map((p, i) => (
        <li key={p.id}>
          <ProductCard p={p} priority={i < priority} />
        </li>
      ))}
    </ul>
  );
}
