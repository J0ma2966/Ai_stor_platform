package uz.boutique.core.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonObject

// Модели повторяют JSON API (docs/API.md, web/src/lib/*.ts). Все поля с умолчаниями:
// новое или отсутствующее поле в ответе не роняет приложение.

@Serializable
enum class Role {
    @SerialName("client") CLIENT,
    @SerialName("store") STORE,
    @SerialName("admin") ADMIN,
}

@Serializable
data class SessionUser(val id: String = "", val role: Role = Role.CLIENT)

@Serializable
data class SessionResp(
    @SerialName("access_token") val accessToken: String,
    @SerialName("expires_at") val expiresAt: String = "",
    @SerialName("is_new_user") val isNewUser: Boolean = false,
    val user: SessionUser = SessionUser(),
)

@Serializable
data class Me(
    val id: String = "",
    val role: Role = Role.CLIENT,
    val phone: String? = null,
    val login: String? = null,
    @SerialName("store_id") val storeId: String? = null,
    val gender: String? = null,
    val lang: String = "ru",
    @SerialName("token_balance") val tokenBalance: Long = 0,
    @SerialName("registration_complete") val registrationComplete: Boolean = false,
    @SerialName("has_password") val hasPassword: Boolean = false,
)

@Serializable
data class Category(
    val id: String,
    val slug: String = "",
    val zone: String = "",
    @SerialName("name_ru") val nameRu: String = "",
    @SerialName("name_uz") val nameUz: String = "",
) {
    fun name(lang: String) = if (lang == "uz" && nameUz.isNotBlank()) nameUz else nameRu
}

@Serializable
data class ProductSize(
    val size: String,
    @SerialName("in_stock") val inStock: Boolean = false,
    val low: Boolean = false,
    val stock: Int? = null, // только кабинет магазина
)

@Serializable
data class Product(
    val id: String,
    @SerialName("store_id") val storeId: String = "",
    @SerialName("store_name") val storeName: String = "",
    @SerialName("store_slug") val storeSlug: String? = null,
    @SerialName("category_id") val categoryId: String = "",
    val zone: String? = null,
    val sku: String? = null,
    val title: String = "",
    val description: String? = null,
    val brand: String = "",
    val price: Long = 0,
    @SerialName("in_stock") val inStock: Boolean = false,
    val sizes: List<ProductSize> = emptyList(),
    @SerialName("has_sizes") val hasSizes: Boolean = false,
    val status: String? = null,
    @SerialName("moderation_reason") val moderationReason: String? = null,
    val images: Map<String, String> = emptyMap(),
    val thumbs: Map<String, String> = emptyMap(),
    val views: Long? = null,
    @SerialName("fitting_adds") val fittingAdds: Long? = null,
    @SerialName("rating_avg") val ratingAvg: Double = 0.0,
    @SerialName("rating_count") val ratingCount: Int = 0,
    @SerialName("rating_dist") val ratingDist: List<Int> = emptyList(),
    @SerialName("created_at") val createdAt: String = "",
) {
    /** Фото в порядке ракурсов: спереди, сбоку, сзади. */
    val gallery: List<String> get() = ANGLES.mapNotNull { images[it] }
    /** Миниатюра для карточек (или большое фото, если копий ещё нет). */
    val cardImage: String? get() = ANGLES.firstNotNullOfOrNull { thumbs[it] } ?: gallery.firstOrNull()
    val withSizes: Boolean get() = hasSizes || sizes.isNotEmpty()

    companion object {
        val ANGLES = listOf("front", "side", "back")
    }
}

@Serializable
data class StoreContacts(
    val telegram: String? = null,
    val instagram: String? = null,
    val website: String? = null,
    val email: String? = null,
    val hours: String? = null,
)

@Serializable
data class Store(
    val id: String = "",
    val slug: String = "",
    val name: String = "",
    val description: String = "",
    @SerialName("logo_url") val logoUrl: String? = null,
    @SerialName("cover_url") val coverUrl: String? = null,
    val phone: String? = null,
    val contacts: StoreContacts = StoreContacts(),
    val address: String? = null,
    @SerialName("product_count") val productCount: Int = 0,
    val status: String? = null,
)

@Serializable
data class PlatformSettings(
    @SerialName("support_phone") val supportPhone: String? = null,
    @SerialName("support_email") val supportEmail: String? = null,
    @SerialName("support_telegram") val supportTelegram: String? = null,
)

// ---- корзина и избранное ----

@Serializable
data class CartItem(
    @SerialName("product_id") val productId: String,
    val title: String = "",
    val brand: String = "",
    val price: Long = 0,
    @SerialName("store_id") val storeId: String = "",
    @SerialName("store_name") val storeName: String = "",
    @SerialName("image_url") val imageUrl: String? = null,
    @SerialName("in_stock") val inStock: Boolean = false,
    @SerialName("has_sizes") val hasSizes: Boolean = false,
    val size: String? = null,
    @SerialName("size_stock") val sizeStock: Int? = null,
    val available: Boolean = true,
    @SerialName("unavailable_reason") val unavailableReason: String? = null,
    val qty: Int = 1,
) {
    val key: String get() = if (size.isNullOrEmpty()) productId else "$productId:$size"
}

@Serializable
data class Cart(
    val items: List<CartItem> = emptyList(),
    @SerialName("total_qty") val totalQty: Int = 0,
    val total: Long = 0,
    @SerialName("can_checkout") val canCheckout: Boolean = false,
    @SerialName("has_unavailable") val hasUnavailable: Boolean = false,
)

// ---- заказы ----

@Serializable
enum class OrderStatus {
    @SerialName("pending") PENDING,
    @SerialName("confirmed") CONFIRMED,
    @SerialName("rejected") REJECTED,
    @SerialName("assembling") ASSEMBLING,
    @SerialName("in_delivery") IN_DELIVERY,
    @SerialName("delivered") DELIVERED,
    @SerialName("cancelled") CANCELLED;

    /** Значение в API. */
    val wire: String get() = name.lowercase()
    val isTerminal: Boolean get() = this == REJECTED || this == DELIVERED || this == CANCELLED
}

@Serializable
data class OrderSummary(
    val id: String,
    val number: Long = 0,
    @SerialName("checkout_id") val checkoutId: String = "",
    @SerialName("store_id") val storeId: String = "",
    @SerialName("store_name") val storeName: String = "",
    val status: OrderStatus = OrderStatus.PENDING,
    @SerialName("items_total") val itemsTotal: Long = 0,
    @SerialName("items_count") val itemsCount: Int = 0,
    @SerialName("customer_name") val customerName: String? = null,
    @SerialName("short_address") val shortAddress: String? = null,
    @SerialName("created_at") val createdAt: String = "",
    @SerialName("updated_at") val updatedAt: String = "",
)

@Serializable
data class Delivery(
    val street: String = "",
    val house: String = "",
    val entrance: String? = null,
    val floor: String? = null,
    val apartment: String? = null,
    val intercom: String? = null,
    @SerialName("address_comment") val addressComment: String? = null,
    val lat: Double? = null,
    val lng: Double? = null,
    @SerialName("maps_url") val mapsUrl: String? = null,
    @SerialName("photo_url") val photoUrl: String? = null,
    @SerialName("has_photo") val hasPhoto: Boolean = false,
)

@Serializable
data class OrderItem(
    @SerialName("product_id") val productId: String? = null,
    val title: String = "",
    val brand: String = "",
    @SerialName("image_url") val imageUrl: String? = null,
    @SerialName("unit_price") val unitPrice: Long = 0,
    val qty: Int = 1,
    val size: String? = null,
)

@Serializable
data class HistoryEntry(
    @SerialName("from_status") val fromStatus: OrderStatus? = null,
    @SerialName("to_status") val toStatus: OrderStatus,
    val reason: String? = null,
    @SerialName("actor_role") val actorRole: String? = null,
    @SerialName("created_at") val createdAt: String = "",
)

@Serializable
data class OrderDetail(
    val id: String,
    val number: Long = 0,
    @SerialName("store_id") val storeId: String = "",
    @SerialName("store_name") val storeName: String = "",
    val status: OrderStatus = OrderStatus.PENDING,
    @SerialName("items_total") val itemsTotal: Long = 0,
    @SerialName("items_count") val itemsCount: Int = 0,
    @SerialName("customer_name") val customerName: String? = null,
    @SerialName("customer_phone") val customerPhone: String? = null,
    @SerialName("customer_comment") val customerComment: String? = null,
    @SerialName("delivery_time_comment") val deliveryTimeComment: String? = null,
    @SerialName("rejection_reason") val rejectionReason: String? = null,
    val delivery: Delivery = Delivery(),
    val items: List<OrderItem> = emptyList(),
    val history: List<HistoryEntry> = emptyList(),
    @SerialName("next_statuses") val nextStatuses: List<OrderStatus> = emptyList(),
    @SerialName("copy_text") val copyText: String? = null,
    @SerialName("can_cancel") val canCancel: Boolean = false,
    @SerialName("created_at") val createdAt: String = "",
    @SerialName("updated_at") val updatedAt: String = "",
)

@Serializable
data class CheckoutItem(
    @SerialName("product_id") val productId: String,
    val qty: Int,
    val size: String? = null,
)

@Serializable
data class DeliveryInput(
    val street: String,
    val house: String,
    val entrance: String? = null,
    val floor: String? = null,
    val apartment: String? = null,
    val intercom: String? = null,
    @SerialName("address_comment") val addressComment: String? = null,
    val lat: Double? = null,
    val lng: Double? = null,
    @SerialName("photo_key") val photoKey: String? = null,
)

@Serializable
data class CheckoutInput(
    val items: List<CheckoutItem>,
    @SerialName("customer_name") val customerName: String,
    @SerialName("customer_phone") val customerPhone: String? = null,
    @SerialName("customer_comment") val customerComment: String? = null,
    @SerialName("delivery_time_comment") val deliveryTimeComment: String? = null,
    val delivery: DeliveryInput,
)

@Serializable
data class CheckoutOrder(
    val id: String,
    val number: Long = 0,
    @SerialName("store_name") val storeName: String = "",
    val status: OrderStatus = OrderStatus.PENDING,
    @SerialName("items_total") val itemsTotal: Long = 0,
)

@Serializable
data class CheckoutResult(
    @SerialName("checkout_id") val checkoutId: String = "",
    val orders: List<CheckoutOrder> = emptyList(),
)

// ---- отзывы ----

@Serializable
data class Review(
    val id: String,
    val rating: Int = 0,
    val body: String = "",
    @SerialName("author_name") val authorName: String = "",
    val reply: String? = null,
    @SerialName("reply_at") val replyAt: String? = null,
    @SerialName("created_at") val createdAt: String = "",
    @SerialName("product_id") val productId: String? = null,
    @SerialName("product_title") val productTitle: String? = null,
    @SerialName("image_url") val imageUrl: String? = null,
    val status: String? = null,
    @SerialName("moderation_reason") val moderationReason: String? = null,
)

@Serializable
data class MyReview(
    @SerialName("can_review") val canReview: Boolean = false,
    val review: Review? = null,
)

// ---- уведомления ----

@Serializable
data class Notification(
    val id: Long,
    val kind: String = "",
    val data: JsonObject = JsonObject(emptyMap()),
    val read: Boolean = false,
    @SerialName("created_at") val createdAt: String = "",
)

@Serializable
data class UnreadCount(val unread: Int = 0)

// ---- кабинет магазина ----

@Serializable
data class UploadResult(val key: String, val url: String = "")

@Serializable
data class CreatedProduct(val id: String, val sku: String = "", val status: String = "")

@Serializable
data class SizeInput(val size: String, val stock: Int)

@Serializable
data class TopProduct(
    val id: String,
    val title: String = "",
    val views: Long = 0,
    @SerialName("fitting_adds") val fittingAdds: Long = 0,
    val ordered: Long = 0,
)

@Serializable
data class StoreStats(
    val from: String = "",
    val to: String = "",
    @SerialName("products_total") val productsTotal: Int = 0,
    @SerialName("products_published") val productsPublished: Int = 0,
    val views: Long = 0,
    val orders: Long = 0,
    @SerialName("orders_delivered") val ordersDelivered: Long = 0,
    val revenue: Long = 0,
    @SerialName("top_products") val topProducts: List<TopProduct> = emptyList(),
)

/** Страница списка с курсорной пагинацией. Пустой nextCursor — последняя страница. */
data class Page<T>(val items: List<T>, val nextCursor: String)
