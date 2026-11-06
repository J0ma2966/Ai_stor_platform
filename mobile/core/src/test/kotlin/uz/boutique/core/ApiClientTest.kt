package uz.boutique.core

import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import org.junit.After
import org.junit.Before
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
import java.util.concurrent.ConcurrentLinkedQueue
import java.util.concurrent.atomic.AtomicInteger
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertIs
import kotlin.test.assertNull
import kotlin.test.assertTrue

class ApiClientTest {
    private lateinit var server: MockWebServer
    private val store = MemoryStore()
    private val requests = ConcurrentLinkedQueue<RecordedRequest>()
    private var handler: (RecordedRequest) -> MockResponse = { MockResponse().setResponseCode(404) }

    private fun session(token: String, role: String = "client", cookie: String = "rt=$token-refresh") = MockResponse()
        .setHeader("Content-Type", "application/json")
        .addHeader("Set-Cookie", "$cookie; Path=/v1/auth; Max-Age=3600; HttpOnly; SameSite=Lax")
        .setBody("""{"data":{"access_token":"$token","expires_at":"2026-11-05T10:00:00Z","is_new_user":false,"user":{"id":"u1","role":"$role"}}}""")

    private fun json(body: String, code: Int = 200) = MockResponse().setResponseCode(code).setHeader("Content-Type", "application/json").setBody(body)
    private fun err(code: Int, c: String) = json("""{"error":{"code":"$c","message":"m"},"request_id":"rid"}""", code)

    @Before fun setUp() {
        server = MockWebServer()
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                requests.add(request)
                return handler(request)
            }
        }
        server.start()
    }

    @After fun tearDown() = server.shutdown()

    private fun api(role: Role = Role.CLIENT, s: SessionManager = SessionManager()) =
        Api(ApiClient(server.url("/").toString(), s, PersistentCookieJar(store), "test"), role)

    @Test fun listParsesDataAndCursor() = runBlocking {
        handler = { json("""{"data":[{"id":"p1","title":"Платье","price":15000000,"thumbs":{"front":"t.jpg"},"images":{"front":"f.jpg","back":"b.jpg"},"unknown":1}],"next_cursor":"abc"}""") }
        val page = api().catalog.products(CatalogFilters(q = "платье мини", priceMinSum = 1000, inStock = true, sort = "new"))
        assertEquals("abc", page.nextCursor)
        assertEquals("Платье", page.items[0].title)
        assertEquals("t.jpg", page.items[0].cardImage)
        assertEquals(listOf("f.jpg", "b.jpg"), page.items[0].gallery)
        val path = requests.last().path!!
        assertTrue(path.startsWith("/v1/products?"), path)
        assertTrue("q=%D0%BF%D0%BB%D0%B0%D1%82%D1%8C%D0%B5%20%D0%BC%D0%B8%D0%BD%D0%B8" in path, path)
        assertTrue("price_min=100000" in path && "in_stock=true" in path && "sort=new" in path && "limit=24" in path, path)
    }

    @Test fun promotedCarouselAndStoreRequest() = runBlocking {
        handler = { r ->
            if (r.path == "/v1/promotions") json("""{"data":[{"id":"p1","title":"Платье","store_name":"Lola","price":15000000}]}""")
            else MockResponse().setResponseCode(201).setHeader("Content-Type", "application/json").setBody("""{"data":{"id":"x"}}""")
        }
        val promoted = api().catalog.promoted()
        assertEquals(listOf("Lola"), promoted.map { it.storeName })
        api(Role.STORE).store.requestPromotion("p1", 7, "Новинка")
        val req = requests.last()
        assertEquals("POST", req.method)
        assertEquals("/v1/store/promotions", req.path)
        assertEquals("""{"product_id":"p1","days":7,"note":"Новинка"}""", req.body.readUtf8())
    }

    @Test fun errorsAreParsed() = runBlocking {
        handler = { json("""{"error":{"code":"validation_failed","message":"bad","fields":{"phone":"e164"}},"request_id":"r-1"}""", 422) }
        val e = assertFailsWith<ApiException> { api().auth.requestOtp("x") }
        assertEquals(422, e.status)
        assertEquals("validation_failed", e.code)
        assertEquals(mapOf("phone" to "e164"), e.fields)
        assertEquals("r-1", e.requestId)
    }

    @Test fun nonJsonErrorFallsBackToStatus() = runBlocking {
        handler = { MockResponse().setResponseCode(502).setBody("<html>bad gateway</html>") }
        val e = assertFailsWith<ApiException> { api().catalog.categories() }
        assertEquals("internal", e.code)
        assertEquals(502, e.status)
    }

    @Test fun networkErrorIsReported() = runBlocking {
        val a = api()
        server.shutdown()
        val e = assertFailsWith<ApiException> { a.catalog.categories() }
        assertTrue(e.isNetwork)
    }

    @Test fun loginKeepsTokenAndRefreshCookieOnlyForAuthPaths() = runBlocking {
        handler = { r ->
            when (r.path) {
                "/v1/auth/password/login" -> session("A1")
                else -> json("""{"data":{"id":"u1","role":"client","has_password":true,"registration_complete":true,"lang":"uz"}}""")
            }
        }
        val a = api()
        a.auth.passwordLogin("+998901234567", "secret-password")
        assertEquals(AuthState.LoggedIn("u1", Role.CLIENT), a.session.state.value)
        val me = a.auth.me()
        assertEquals("uz", me.lang)
        val meReq = requests.last()
        assertEquals("Bearer A1", meReq.getHeader("Authorization"))
        assertNull(meReq.getHeader("Cookie"), "refresh cookie must not leak outside /v1/auth")
        assertEquals("test", meReq.getHeader("User-Agent"))
        // cookie пережила «перезапуск»: новый jar читает её из хранилища
        assertTrue(PersistentCookieJar(store).hasCookies())
    }

    @Test fun expiredAccessTokenIsRefreshedOnceForParallelRequests() = runBlocking {
        val refreshes = AtomicInteger()
        handler = { r ->
            when {
                r.path == "/v1/auth/password/login" -> session("OLD")
                r.path == "/v1/auth/refresh" -> {
                    refreshes.incrementAndGet()
                    assertEquals("rt=OLD-refresh", r.getHeader("Cookie"))
                    Thread.sleep(100)
                    session("NEW")
                }
                r.getHeader("Authorization") == "Bearer OLD" -> err(401, "token_invalid")
                else -> json("""{"data":{"items":[],"total":0,"total_qty":0,"can_checkout":false}}""")
            }
        }
        val a = api()
        a.auth.passwordLogin("+998901234567", "secret-password")
        val carts = (1..5).map { async(kotlinx.coroutines.Dispatchers.IO) { a.shop.cart() } }.awaitAll()
        assertEquals(5, carts.size)
        assertEquals(1, refreshes.get(), "refresh token is single-use: only one refresh allowed")
        assertEquals("Bearer NEW", requests.last().getHeader("Authorization"))
    }

    @Test fun failedRefreshLogsOut() = runBlocking {
        handler = { r ->
            when (r.path) {
                "/v1/auth/password/login" -> session("OLD")
                "/v1/auth/refresh" -> err(401, "refresh_invalid")
                else -> err(401, "token_invalid")
            }
        }
        val a = api()
        a.auth.passwordLogin("+998901234567", "secret-password")
        val e = assertFailsWith<ApiException> { a.shop.cart() }
        assertEquals("token_invalid", e.code)
        assertEquals(AuthState.LoggedOut, a.session.state.value)
        assertEquals(1, a.session.lostEvents.value)
        assertFalse(PersistentCookieJar(store).hasCookies())
    }

    @Test fun revokedTokenLogsOut() = runBlocking {
        handler = { r -> if (r.path == "/v1/auth/login") session("S", role = "store") else err(401, "token_revoked") }
        val a = api(Role.STORE)
        a.auth.staffLogin("shop", "pw")
        assertFailsWith<ApiException> { a.store.products() }
        assertEquals(AuthState.LoggedOut, a.session.state.value)
    }

    @Test fun temporaryRefreshFailureKeepsSession() = runBlocking {
        handler = { r -> if (r.path == "/v1/auth/password/login") session("A") else err(503, "unavailable") }
        val a = api()
        a.auth.passwordLogin("+998901234567", "secret-password")
        assertEquals(RefreshOutcome.FAILED, a.client.refresh())
        assertIs<AuthState.LoggedIn>(a.session.state.value)
        assertTrue(PersistentCookieJar(store).hasCookies())
    }

    @Test fun restoreSessionAtStartup() = runBlocking {
        handler = { r -> if (r.path == "/v1/auth/password/login") session("A") else session("B") }
        api().auth.passwordLogin("+998901234567", "secret-password")
        // новое «приложение»: токена в памяти нет, cookie — в хранилище
        val fresh = api()
        assertEquals(AuthState.Unknown, fresh.session.state.value)
        assertEquals(RefreshOutcome.OK, fresh.client.refresh())
        assertEquals("B", fresh.session.accessToken)
    }

    @Test fun startupWithoutCookieIsLoggedOutWithoutNetwork() = runBlocking {
        val a = api()
        assertEquals(RefreshOutcome.NO_SESSION, a.client.refresh())
        assertEquals(AuthState.LoggedOut, a.session.state.value)
        assertTrue(requests.isEmpty())
    }

    @Test fun storeAccountCannotUseClientApp() = runBlocking {
        handler = { r -> if (r.path == "/v1/auth/password/login") session("S", role = "store") else MockResponse().setResponseCode(204) }
        val a = api(Role.CLIENT)
        val e = assertFailsWith<ApiException> { a.auth.passwordLogin("+998901234567", "secret-password") }
        assertEquals(ApiException.WRONG_APP, e.code)
        assertEquals(AuthState.LoggedOut, a.session.state.value)
        assertEquals("/v1/auth/logout", requests.last().path)
        assertFalse(PersistentCookieJar(store).hasCookies())
    }

    @Test fun checkoutSendsIdempotencyKeyAndOmitsNulls() = runBlocking {
        handler = { json("""{"data":{"checkout_id":"c1","orders":[{"id":"o1","number":12,"store_name":"S","status":"pending","items_total":100}]}}""", 201) }
        val res = api().orders.checkout(
            CheckoutInput(listOf(CheckoutItem("p1", 2, "M"), CheckoutItem("p2", 1)), "Аня", delivery = DeliveryInput("Навои", "5")),
            "key-1",
        )
        assertEquals(12, res.orders.single().number)
        val r = requests.last()
        assertEquals("key-1", r.getHeader("Idempotency-Key"))
        val body = r.body.readUtf8()
        assertEquals(
            """{"items":[{"product_id":"p1","qty":2,"size":"M"},{"product_id":"p2","qty":1}],"customer_name":"Аня","delivery":{"street":"Навои","house":"5"}}""",
            body,
        )
    }

    @Test fun storeStatusChangeSendsFromStatus() = runBlocking {
        handler = { json("""{"data":{"id":"o1","status":"confirmed","next_statuses":["assembling","cancelled"],"weird_status":"x"}}""") }
        val o = api(Role.STORE).orders.setStatus("o1", OrderStatus.CONFIRMED, OrderStatus.PENDING)
        assertEquals(listOf(OrderStatus.ASSEMBLING, OrderStatus.CANCELLED), o.nextStatuses)
        val r = requests.last()
        assertEquals("/v1/store/orders/o1/status", r.path)
        assertEquals("""{"status":"confirmed","from":"pending","reason":""}""", r.body.readUtf8())
    }

    @Test fun cartPathEncodesSize() = runBlocking {
        handler = { json("""{"data":{"items":[{"product_id":"p1","size":"42 1/2","qty":2,"available":false,"unavailable_reason":"out_of_stock"}],"total":0,"total_qty":2}}""") }
        val cart = api().shop.setQty("p1", 2, "42 1/2")
        assertEquals("p1:42 1/2", cart.items[0].key)
        assertFalse(cart.items[0].available)
        assertEquals("/v1/me/cart/p1?size=42%201%2F2", requests.last().path)
        assertEquals("PUT", requests.last().method)
    }

    @Test fun unknownEnumValueDoesNotCrash() = runBlocking {
        handler = { json("""{"data":[{"id":"o1","status":"something_new","items_total":5}]}""") }
        val page = api().orders.list(listOf(OrderStatus.PENDING, OrderStatus.CONFIRMED))
        assertEquals(OrderStatus.PENDING, page.items[0].status)
        assertEquals("", page.nextCursor)
        assertEquals("/v1/orders?status=pending%2Cconfirmed", requests.last().path)
    }
}
