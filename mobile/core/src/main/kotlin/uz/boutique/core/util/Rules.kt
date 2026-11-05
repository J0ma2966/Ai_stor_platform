package uz.boutique.core.util

import uz.boutique.core.model.CheckoutInput
import uz.boutique.core.model.Notification
import uz.boutique.core.model.OrderStatus
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonPrimitive

object Passwords {
    const val MIN = 10
    const val MAX = 128

    /** null — пароль подходит, иначе код ошибки (как у сервера). */
    fun check(password: String): String? = when {
        password.length < MIN -> "too_short"
        password.length > MAX -> "too_long"
        else -> null
    }
}

object Orders {
    /** Шаги, которые видит покупатель (как CUSTOMER_STEPS на сайте). */
    val CUSTOMER_STEPS = listOf(OrderStatus.CONFIRMED, OrderStatus.ASSEMBLING, OrderStatus.IN_DELIVERY, OrderStatus.DELIVERED)

    /** Сколько шагов пройдено: 0 — ждёт подтверждения магазином. */
    fun customerStep(status: OrderStatus): Int = CUSTOMER_STEPS.indexOf(status) + 1

    /** Отклонение и отмена магазином требуют причину, её увидит покупатель. */
    fun reasonRequired(to: OrderStatus): Boolean = to == OrderStatus.REJECTED || to == OrderStatus.CANCELLED

    /** Вкладки окна заказов магазина. */
    enum class StoreTab(val statuses: List<OrderStatus>) {
        NEW(listOf(OrderStatus.PENDING)),
        ACTIVE(listOf(OrderStatus.CONFIRMED, OrderStatus.ASSEMBLING, OrderStatus.IN_DELIVERY)),
        DONE(listOf(OrderStatus.DELIVERED, OrderStatus.REJECTED, OrderStatus.CANCELLED)),
    }

    /** Порядок кнопок: сначала движение вперёд, отказ и отмена — последними. */
    fun sortActions(next: List<OrderStatus>): List<OrderStatus> = next.sortedBy { if (reasonRequired(it)) 1 else 0 }
}

/** Лимиты совпадают с backend/internal/orders/validate.go. */
object CheckoutRules {
    const val MAX_NAME = 120
    const val MAX_STREET = 200
    const val MAX_HOUSE = 30
    const val MAX_ENTRANCE = 20
    const val MAX_FLOOR = 10
    const val MAX_APARTMENT = 20
    const val MAX_INTERCOM = 30
    const val MAX_COMMENT = 1000
    const val MAX_TIME_COMMENT = 500

    /** Ошибки по полям в тех же ключах и кодах, что у сервера ("required", "max", "e164"). */
    fun validate(input: CheckoutInput): Map<String, String> {
        val f = LinkedHashMap<String, String>()
        fun text(name: String, v: String?, max: Int, required: Boolean) {
            val n = v?.trim()?.length ?: 0
            if (required && n == 0) f[name] = "required" else if (n > max) f[name] = "max"
        }
        val d = input.delivery
        text("customer_name", input.customerName, MAX_NAME, true)
        text("delivery.street", d.street, MAX_STREET, true)
        text("delivery.house", d.house, MAX_HOUSE, true)
        text("delivery.entrance", d.entrance, MAX_ENTRANCE, false)
        text("delivery.floor", d.floor, MAX_FLOOR, false)
        text("delivery.apartment", d.apartment, MAX_APARTMENT, false)
        text("delivery.intercom", d.intercom, MAX_INTERCOM, false)
        text("delivery.address_comment", d.addressComment, MAX_COMMENT, false)
        text("customer_comment", input.customerComment, MAX_COMMENT, false)
        text("delivery_time_comment", input.deliveryTimeComment, MAX_TIME_COMMENT, false)
        val phone = input.customerPhone
        if (!phone.isNullOrBlank() && Phone.normalizeUz(phone) == null) f["customer_phone"] = "e164"
        if (input.items.isEmpty()) f["items"] = "required"
        return f
    }
}

/** Куда ведёт уведомление (как notificationHref на сайте). */
sealed interface NotificationTarget {
    data class ClientOrder(val id: String?) : NotificationTarget
    data class StoreOrder(val id: String?) : NotificationTarget
    data class StoreProduct(val id: String?) : NotificationTarget
    data object StoreReviews : NotificationTarget
    data class ProductPage(val id: String) : NotificationTarget
    data object None : NotificationTarget

    companion object {
        fun of(n: Notification): NotificationTarget {
            fun s(k: String) = runCatching { n.data[k]?.jsonPrimitive?.contentOrNull }.getOrNull()
            return when (n.kind) {
                "order_created", "order_cancelled" -> StoreOrder(s("order_id"))
                "order_status" -> ClientOrder(s("order_id"))
                "product_moderated" -> StoreProduct(s("product_id"))
                "review_created" -> StoreReviews
                "review_reply", "review_hidden" -> s("product_id")?.let { ProductPage(it) } ?: None
                else -> None
            }
        }
    }
}
