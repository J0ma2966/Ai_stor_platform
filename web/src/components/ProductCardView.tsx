import { Link } from "@/i18n/navigation";
import { frontImage, type Product } from "@/lib/catalog";
import { formatSum } from "@/lib/money";
import { CardCartButton } from "./CardCartButton";
import { FavoriteButton } from "./FavoriteButton";

// Общая разметка карточки: используется и на сервере, и в клиентском «Показать ещё».
export function ProductCardView({ p, locale, outOfStock, priority = false }: { p: Product; locale: string; outOfStock: string; priority?: boolean }) {
  const img = frontImage(p);
  const seller = p.brand && p.brand !== p.store_name ? `${p.brand} · ${p.store_name}` : p.store_name;
  return (
    <article className="pcard">
      <Link href={`/product/${p.id}`} className="pcard-link">
        <div className="pcard-img">
          {img ? <img src={img} alt={p.title} loading={priority ? "eager" : "lazy"} width={300} height={400} /> : <div className="img-ph" aria-hidden="true" />}
          {!p.in_stock && <span className="pcard-flag">{outOfStock}</span>}
        </div>
        <div className="pcard-body">
          <span className="price">{formatSum(p.price, locale)}</span>
          <span className="pcard-title">{p.title}</span>
          <span className="pcard-store">{seller}</span>
        </div>
      </Link>
      <FavoriteButton productId={p.id} className="pcard-fav" />
      <CardCartButton productId={p.id} inStock={p.in_stock} />
    </article>
  );
}
