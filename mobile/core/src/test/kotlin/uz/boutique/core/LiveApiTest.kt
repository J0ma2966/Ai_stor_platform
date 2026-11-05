package uz.boutique.core

import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonObject
import org.junit.Assume.assumeTrue
import uz.boutique.core.api.Api
import uz.boutique.core.api.ApiClient
import uz.boutique.core.api.ApiException
import uz.boutique.core.api.AuthState
import uz.boutique.core.api.CatalogFilters
import uz.boutique.core.api.MemoryStore
import uz.boutique.core.api.PersistentCookieJar
import uz.boutique.core.api.RefreshOutcome
import uz.boutique.core.api.SessionManager
import uz.boutique.core.model.CheckoutInput
import uz.boutique.core.model.CheckoutItem
import uz.boutique.core.model.DeliveryInput
import uz.boutique.core.model.OrderStatus
import uz.boutique.core.model.Role
import java.io.File
import java.security.SecureRandom
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertTrue

/**
 * Сквозной сценарий против настоящего бэкенда (не в обычном прогоне).
 *
 * Нужен запущенный API в dev-режиме с SMS_PROVIDER=log и активный магазин:
 *   BOUTIQUE_IT_API=http://localhost:8080
 *   BOUTIQUE_IT_OTP_LOG=<файл с логом API, откуда берётся DEV OTP>
 *   BOUTIQUE_IT_STORE_LOGIN / BOUTIQUE_IT_STORE_PASSWORD
 *   cd mobile/core && ../gradlew test --tests '*LiveApiTest*'
 */
class LiveApiTest {
    private val base = System.getenv("BOUTIQUE_IT_API").orEmpty()

    private fun app(role: Role) =
        Api(ApiClient(base, SessionManager(), PersistentCookieJar(MemoryStore()), "it"), role)

    private fun lastOtp(): String {
        val log = File(System.getenv("BOUTIQUE_IT_OTP_LOG"))
        return Regex("\"code\":\"(\\d{6})\"").findAll(log.readText()).last().groupValues[1]
    }

    private fun hex(n: Int) = ByteArray(n).also { SecureRandom().nextBytes(it) }.joinToString("") { "%02x".format(it) }

    @Test
    fun clientAndStoreFullFlow() = runBlocking {
        assumeTrue("BOUTIQUE_IT_API не задан", base.isNotEmpty())

        // ---- магазин: вход, товар с размерами, публикация ----
        val store = app(Role.STORE)
        val ss = store.auth.staffLogin(System.getenv("BOUTIQUE_IT_STORE_LOGIN"), System.getenv("BOUTIQUE_IT_STORE_PASSWORD"))
        assertEquals(Role.STORE, ss.user.role)
        val profile = store.store.profile()
        val category = app(Role.CLIENT).catalog.categories().first { it.zone == "top" }
        // Ключи фото того же вида, что выдаёт /v1/uploads (сами файлы для создания товара не нужны).
        val created = store.store.create(buildJsonObject {
            put("category_id", category.id)
            put("title", "Футболка IT ${hex(3)}")
            put("description", "Хлопок")
            put("brand", "Boutique")
            put("price", 15_000_000)
            put("in_stock", true)
            putJsonObject("images") {
                for (a in listOf("front", "side", "back")) put(a, "pub/u/${ss.user.id}/product/${hex(16)}.jpg")
            }
            put("sizes", buildJsonArray {
                add(buildJsonObject { put("size", "M"); put("stock", 3) })
                add(buildJsonObject { put("size", "L"); put("stock", 0) })
            })
        })
        store.store.setStatus(created.id, "published")
        val mine = store.store.product(created.id)
        assertEquals("published", mine.status)
        assertTrue(store.store.products().any { it.id == created.id })

        // ---- клиент: регистрация по SMS, пароль, повторный вход по паролю ----
        val phone = "+99890" + (1_000_000..9_999_999).random()
        val client = app(Role.CLIENT)
        client.auth.requestOtp(phone)
        val s1 = client.auth.verifyOtp(phone, lastOtp())
        assertTrue(s1.isNewUser)
        client.auth.completeRegistration("female", "ru")
        client.auth.setPassword("Client-Pass-123")
        assertTrue(client.auth.me().hasPassword)
        client.auth.logout()
        assertEquals(AuthState.LoggedOut, client.session.state.value)

        val again = app(Role.CLIENT)
        again.auth.passwordLogin(phone, "Client-Pass-123")
        assertEquals(RefreshOutcome.OK, again.client.refresh()) // refresh-кука сохранена и принимается
        // клиентский вход в приложении магазина запрещён
        assertFailsWith<ApiException> { app(Role.STORE).auth.passwordLogin(phone, "Client-Pass-123") }

        // ---- каталог, карточка, избранное, корзина ----
        val page = again.catalog.products(CatalogFilters(q = mine.title))
        val p = page.items.single { it.id == created.id }
        assertEquals(15_000_000, p.price)
        val full = again.catalog.product(created.id)
        assertEquals(profile.name, full.storeName)
        assertEquals(setOf("M", "L"), full.sizes.map { it.size }.toSet())
        again.shop.addFavorite(created.id)
        assertTrue(created.id in again.shop.favoriteIds())
        val cart = again.shop.setQty(created.id, 2, "M")
        assertEquals(30_000_000, cart.total)

        // ---- оформление ----
        val input = CheckoutInput(
            items = listOf(CheckoutItem(created.id, 2, "M")),
            customerName = "Тест Клиент",
            delivery = DeliveryInput(street = "Амира Темура", house = "1", apartment = "5", lat = 41.31, lng = 69.28),
        )
        val key = ApiClient.newIdempotencyKey()
        val res = again.orders.checkout(input, key)
        val res2 = again.orders.checkout(input, key) // повтор с тем же ключом не создаёт второй заказ
        assertEquals(res.orders.map { it.id }, res2.orders.map { it.id })
        val orderId = res.orders.single().id
        assertEquals(OrderStatus.PENDING, again.orders.get(orderId).status)
        assertTrue(again.orders.list().items.any { it.id == orderId })

        // ---- окно магазина: заказ и смена статусов ----
        val incoming = store.orders.list(listOf(OrderStatus.PENDING)).items
        assertTrue(incoming.any { it.id == orderId })
        var detail = store.orders.get(orderId)
        assertTrue(OrderStatus.CONFIRMED in detail.nextStatuses)
        // отказ без причины не проходит
        assertFailsWith<ApiException> { store.orders.setStatus(orderId, OrderStatus.REJECTED, OrderStatus.PENDING) }
        for ((from, to) in listOf(
            OrderStatus.PENDING to OrderStatus.CONFIRMED,
            OrderStatus.CONFIRMED to OrderStatus.ASSEMBLING,
            OrderStatus.ASSEMBLING to OrderStatus.IN_DELIVERY,
            OrderStatus.IN_DELIVERY to OrderStatus.DELIVERED,
        )) detail = store.orders.setStatus(orderId, to, from)
        assertEquals(OrderStatus.DELIVERED, detail.status)
        // устаревший from — конфликт
        assertFailsWith<ApiException> { store.orders.setStatus(orderId, OrderStatus.CANCELLED, OrderStatus.PENDING) }

        // ---- клиент видит статус, уведомления, отзыв; магазин отвечает ----
        val seen = again.orders.get(orderId)
        assertEquals(OrderStatus.DELIVERED, seen.status)
        assertTrue(seen.history.size >= 4)
        again.notifications.list()
        again.reviews.save(created.id, 5, "Отлично сидит")
        val reviews = store.reviews.storeList(unanswered = true).items
        val r = reviews.first { it.rating == 5 }
        store.reviews.reply(r.id, "Спасибо!")
        assertEquals(1, again.catalog.product(created.id).ratingCount)

        // ---- второй заказ: отмена клиентом ----
        again.shop.setQty(created.id, 1, "M")
        val o2 = again.orders.checkout(input.copy(items = listOf(CheckoutItem(created.id, 1, "M"))), ApiClient.newIdempotencyKey())
        val cancelled = again.orders.cancel(o2.orders.single().id, "Передумал")
        assertEquals(OrderStatus.CANCELLED, cancelled.status)

        val today = java.time.LocalDate.now(java.time.ZoneId.of("Asia/Tashkent"))
        val stats = store.store.stats(today.minusDays(29).toString(), today.toString())
        assertTrue(stats.orders >= 2 && stats.ordersDelivered >= 1 && stats.revenue >= 30_000_000)
        store.notifications.unread()
        Unit
    }
}
