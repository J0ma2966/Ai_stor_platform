package uz.boutique.store

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import uz.boutique.core.api.MemoryStore
import uz.boutique.core.model.Delivery
import uz.boutique.core.model.OrderDetail
import uz.boutique.core.model.OrderItem
import uz.boutique.core.model.OrderStatus
import uz.boutique.core.model.Role
import uz.boutique.ui.AppGraph
import uz.boutique.ui.BoutiqueTheme
import uz.boutique.ui.LocalGraph

@RunWith(AndroidJUnit4::class)
class StoreOrderContentTest {
    @get:Rule val rule = createComposeRule()

    private val graph by lazy {
        AppGraph(ApplicationProvider.getApplicationContext(), Role.STORE, "http://127.0.0.1:9", false, "https://example.uz", "test", MemoryStore())
    }

    private fun order(status: OrderStatus, next: List<OrderStatus>) = OrderDetail(
        id = "o1", number = 42, status = status, itemsTotal = 150_000_000, itemsCount = 1,
        customerName = "Аня", customerPhone = "+998901234567",
        delivery = Delivery(street = "Навои", house = "5", apartment = "12"),
        items = listOf(OrderItem("p1", "Платье", "Brand", null, 150_000_000, 1, "M")),
        nextStatuses = next, copyText = "Заказ №42\nНавои, 5",
    )

    private fun show(o: OrderDetail, onStatus: (OrderStatus, String) -> Unit = { _, _ -> }, onCopy: (String) -> Unit = {}) {
        rule.setContent {
            CompositionLocalProvider(LocalGraph provides graph) {
                BoutiqueTheme(dark = false) {
                    StoreOrderContent(o, busy = false, error = null, onStatus = onStatus, onCopy = onCopy, onOpen = {}, onCall = {})
                }
            }
        }
    }

    @Test fun confirmNeedsOnlyConfirmation() {
        val calls = mutableListOf<Pair<OrderStatus, String>>()
        show(order(OrderStatus.PENDING, listOf(OrderStatus.CONFIRMED, OrderStatus.REJECTED)), onStatus = { s, r -> calls += s to r })
        rule.onNodeWithTag("action_confirmed").performClick()
        rule.onNodeWithTag("confirm").performClick()
        assertEquals(listOf(OrderStatus.CONFIRMED to ""), calls)
    }

    @Test fun rejectRequiresReason() {
        val calls = mutableListOf<Pair<OrderStatus, String>>()
        show(order(OrderStatus.PENDING, listOf(OrderStatus.CONFIRMED, OrderStatus.REJECTED)), onStatus = { s, r -> calls += s to r })
        rule.onNodeWithTag("action_rejected").performClick()
        rule.onNodeWithTag("confirm").assertIsNotEnabled()
        rule.onNodeWithTag("reason").performTextInput("Нет размера M")
        rule.onNodeWithTag("confirm").assertIsEnabled().performClick()
        assertEquals(listOf(OrderStatus.REJECTED to "Нет размера M"), calls)
    }

    @Test fun finishedOrderHasNoActions() {
        show(order(OrderStatus.DELIVERED, emptyList()))
        rule.onNodeWithText("Заказ завершён, статус больше не меняется.").assertExists()
        rule.onNodeWithTag("action_confirmed").assertDoesNotExist()
        rule.onNodeWithTag("status_delivered").assertExists()
    }

    @Test fun copyForCourierAndTotal() {
        var copied = ""
        show(order(OrderStatus.CONFIRMED, listOf(OrderStatus.ASSEMBLING, OrderStatus.CANCELLED)), onCopy = { copied = it })
        rule.onNodeWithTag("copy").performScrollTo().performClick()
        assertTrue(copied.startsWith("Заказ №42"))
        rule.onNodeWithText("Навои, 5, кв. 12").assertExists()
        rule.onNodeWithTag("order_total").performScrollTo().assertExists()
        rule.onNodeWithText("1 500 000 сум", useUnmergedTree = true).assertExists()
    }
}
