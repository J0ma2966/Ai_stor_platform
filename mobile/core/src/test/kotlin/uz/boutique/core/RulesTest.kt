package uz.boutique.core

import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import uz.boutique.core.model.CheckoutInput
import uz.boutique.core.model.CheckoutItem
import uz.boutique.core.model.DeliveryInput
import uz.boutique.core.model.Notification
import uz.boutique.core.model.OrderStatus
import uz.boutique.core.util.CheckoutRules
import uz.boutique.core.util.NotificationTarget
import uz.boutique.core.util.Orders
import uz.boutique.core.util.Passwords
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class RulesTest {
    @Test fun passwords() {
        assertEquals("too_short", Passwords.check("123456789"))
        assertNull(Passwords.check("1234567890"))
        assertEquals("too_long", Passwords.check("x".repeat(129)))
    }

    @Test fun customerSteps() {
        assertEquals(0, Orders.customerStep(OrderStatus.PENDING))
        assertEquals(1, Orders.customerStep(OrderStatus.CONFIRMED))
        assertEquals(4, Orders.customerStep(OrderStatus.DELIVERED))
        assertTrue(Orders.reasonRequired(OrderStatus.REJECTED))
        assertTrue(Orders.reasonRequired(OrderStatus.CANCELLED))
        assertFalse(Orders.reasonRequired(OrderStatus.CONFIRMED))
        assertEquals(
            listOf(OrderStatus.CONFIRMED, OrderStatus.REJECTED, OrderStatus.CANCELLED),
            Orders.sortActions(listOf(OrderStatus.REJECTED, OrderStatus.CONFIRMED, OrderStatus.CANCELLED)),
        )
    }

    private fun input(name: String = "Аня", street: String = "Амира Темура", house: String = "1", phone: String? = null, items: Int = 1) =
        CheckoutInput(
            items = List(items) { CheckoutItem("p$it", 1) },
            customerName = name,
            customerPhone = phone,
            delivery = DeliveryInput(street = street, house = house),
        )

    @Test fun checkoutValidation() {
        assertTrue(CheckoutRules.validate(input()).isEmpty())
        val e = CheckoutRules.validate(input(name = "  ", street = "", house = "x".repeat(31), phone = "123", items = 0))
        assertEquals("required", e["customer_name"])
        assertEquals("required", e["delivery.street"])
        assertEquals("max", e["delivery.house"])
        assertEquals("e164", e["customer_phone"])
        assertEquals("required", e["items"])
        assertTrue(CheckoutRules.validate(input(phone = "+998901234567")).isEmpty())
    }

    @Test fun notificationTargets() {
        fun n(kind: String, vararg kv: Pair<String, String>) =
            Notification(1, kind, buildJsonObject { kv.forEach { put(it.first, it.second) } })
        assertEquals(NotificationTarget.StoreOrder("o1"), NotificationTarget.of(n("order_created", "order_id" to "o1")))
        assertEquals(NotificationTarget.ClientOrder("o2"), NotificationTarget.of(n("order_status", "order_id" to "o2")))
        assertEquals(NotificationTarget.ProductPage("p1"), NotificationTarget.of(n("review_reply", "product_id" to "p1")))
        assertEquals(NotificationTarget.StoreReviews, NotificationTarget.of(n("review_created")))
        for (k in listOf("promotion_approved", "promotion_rejected", "promotion_stopped")) {
            assertEquals(NotificationTarget.StoreProduct("p9"), NotificationTarget.of(n(k, "product_id" to "p9")))
        }
        assertEquals(NotificationTarget.None, NotificationTarget.of(n("review_hidden")))
        assertEquals(NotificationTarget.None, NotificationTarget.of(Notification(2, "system", buildJsonObject { put("x", JsonPrimitive(1)) })))
    }
}
