package uz.boutique.client

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextClearance
import androidx.compose.ui.test.performTextInput
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import uz.boutique.core.api.MemoryStore
import uz.boutique.core.model.Product
import uz.boutique.core.model.Role
import uz.boutique.ui.AppGraph
import uz.boutique.ui.BoutiqueTheme
import uz.boutique.ui.LocalGraph

@RunWith(AndroidJUnit4::class)
class ClientScreensTest {
    @get:Rule val rule = createComposeRule()
    private val server = MockWebServer()

    @After fun stop() = server.shutdown()

    private fun graph() = AppGraph(
        ApplicationProvider.getApplicationContext(), Role.CLIENT, server.url("/").toString(), false, "https://example.uz", "test", MemoryStore(),
    )

    private fun json(body: String) = MockResponse().setHeader("Content-Type", "application/json").setBody(body)

    @Test fun productCardShowsPriceAndRating() {
        val g = graph()
        rule.setContent {
            CompositionLocalProvider(LocalGraph provides g, LocalShop provides ShopState(g.api)) {
                BoutiqueTheme(dark = false) {
                    ProductCard(
                        Product(id = "p1", title = "Платье миди", brand = "Zara", price = 45_000_000, inStock = true, ratingAvg = 4.5, ratingCount = 12),
                        onClick = {}, onNeedLogin = {},
                    )
                }
            }
        }
        rule.onNodeWithText("450 000 сум").assertIsDisplayed()
        rule.onNodeWithText("4,5 (12)").assertIsDisplayed()
    }

    @Test fun favoriteAsGuestAsksToLogIn() {
        val g = graph()
        var asked = false
        rule.setContent {
            CompositionLocalProvider(LocalGraph provides g, LocalShop provides ShopState(g.api)) {
                BoutiqueTheme(dark = false) { FavoriteButton("p1", onNeedLogin = { asked = true }) }
            }
        }
        rule.onNodeWithTag("fav_p1").performClick()
        assertEquals(true, asked)
    }

    @Test fun loginValidatesPhoneAndLogsIn() {
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse = when (request.path) {
                "/v1/auth/methods" -> json(SMS_METHODS)
                "/v1/auth/password/login" -> json("""{"data":{"access_token":"t","user":{"id":"u","role":"client"}}}""")
                    .addHeader("Set-Cookie", "rt=r; Path=/v1/auth; Max-Age=600; HttpOnly")
                "/v1/me" -> json("""{"data":{"id":"u","role":"client","registration_complete":true,"has_password":true,"lang":"ru"}}""")
                else -> MockResponse().setResponseCode(404)
            }
        }
        val g = graph()
        var done = false
        var next: String? = "unset"
        rule.setContent {
            CompositionLocalProvider(LocalGraph provides g, LocalShop provides ShopState(g.api)) {
                BoutiqueTheme(dark = false) {
                    LoginScreen(onBack = {}, onSms = {}, onForgot = {}, onEmail = {}, onDone = { done = true; next = it }, onRestart = {})
                }
            }
        }
        rule.waitUntil(5_000) { rule.onAllNodes(hasTestTag("phone")).fetchSemanticsNodes().isNotEmpty() }
        rule.onNodeWithTag("phone").performScrollTo().performTextClearance()
        rule.onNodeWithTag("phone").performScrollTo().performTextInput("12345")
        rule.onNodeWithTag("password").performScrollTo().performTextInput("secret-password")
        rule.onNodeWithTag("sign_in").performScrollTo().performClick()
        rule.onNodeWithText("Введите номер Узбекистана: +998 XX XXX XX XX").assertExists()

        rule.waitUntil(5_000) { rule.onAllNodes(hasTestTag("phone")).fetchSemanticsNodes().isNotEmpty() }
        rule.onNodeWithTag("phone").performScrollTo().performTextClearance()
        rule.onNodeWithTag("phone").performScrollTo().performTextInput("90 123 45 67")
        rule.onNodeWithTag("sign_in").performScrollTo().performClick()
        rule.waitUntil(5_000) { done }
        assertNull(next)
        server.takeRequest() // /v1/auth/methods
        val login = server.takeRequest()
        assertEquals("""{"phone":"+998901234567","password":"secret-password"}""", login.body.readUtf8())
    }

    @Test fun newSmsUserGoesToWelcome() {
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse = when (request.path) {
                "/v1/auth/methods" -> json(SMS_METHODS)
                "/v1/auth/password/login" -> json("""{"data":{"access_token":"t","user":{"id":"u","role":"client"}}}""")
                "/v1/me" -> json("""{"data":{"id":"u","role":"client","registration_complete":false,"has_password":false}}""")
                else -> MockResponse().setResponseCode(404)
            }
        }
        val g = graph()
        var next: String? = "unset"
        rule.setContent {
            CompositionLocalProvider(LocalGraph provides g, LocalShop provides ShopState(g.api)) {
                BoutiqueTheme(dark = false) {
                    LoginScreen(onBack = {}, onSms = {}, onForgot = {}, onEmail = {}, onDone = { next = it }, onRestart = {})
                }
            }
        }
        rule.waitUntil(5_000) { rule.onAllNodes(hasTestTag("phone")).fetchSemanticsNodes().isNotEmpty() }
        rule.onNodeWithTag("phone").performScrollTo().performTextClearance()
        rule.onNodeWithTag("phone").performScrollTo().performTextInput("901234567")
        rule.onNodeWithTag("password").performScrollTo().performTextInput("secret-password")
        rule.onNodeWithTag("sign_in").performScrollTo().performClick()
        rule.waitUntil(5_000) { next != "unset" }
        assertEquals(Routes.WELCOME, next)
    }

    @Test fun wrongPasswordShowsServerError() {
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse = when (request.path) {
                "/v1/auth/methods" -> json(SMS_METHODS)
                else -> json("""{"error":{"code":"bad_credentials","message":"x"},"request_id":"r1"}""").setResponseCode(401)
            }
        }
        val g = graph()
        rule.setContent {
            CompositionLocalProvider(LocalGraph provides g, LocalShop provides ShopState(g.api)) {
                BoutiqueTheme(dark = false) {
                    LoginScreen(onBack = {}, onSms = {}, onForgot = {}, onEmail = {}, onDone = {}, onRestart = {})
                }
            }
        }
        rule.waitUntil(5_000) { rule.onAllNodes(hasTestTag("phone")).fetchSemanticsNodes().isNotEmpty() }
        rule.onNodeWithTag("phone").performScrollTo().performTextClearance()
        rule.onNodeWithTag("phone").performScrollTo().performTextInput("901234567")
        rule.onNodeWithTag("password").performScrollTo().performTextInput("wrong-password")
        rule.onNodeWithTag("sign_in").performScrollTo().performClick()
        rule.waitUntil(5_000) { rule.onAllNodes(hasTestTag("error_banner")).fetchSemanticsNodes().isNotEmpty() }
        rule.onNodeWithText("Неверный номер, логин или пароль.").assertExists()
    }

    @Test fun prodLoginShowsTelegramAndEmailWithoutPhone() {
        val polls = java.util.concurrent.atomic.AtomicInteger()
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse = when (request.path) {
                "/v1/auth/methods" -> json(PROD_METHODS)
                "/v1/auth/telegram/start" -> json("""{"data":{"login_id":"L1","poll_token":"P1","url":"https://t.me/shop_bot?start=login_L1","expires_at":""}}""")
                "/v1/auth/telegram/poll" ->
                    if (polls.incrementAndGet() < 2) json("""{"data":{"status":"pending"}}""")
                    else json("""{"data":{"access_token":"t","is_new_user":true,"user":{"id":"u","role":"client"}}}""")
                "/v1/me" -> json("""{"data":{"id":"u","role":"client","registration_complete":false,"has_password":false,"has_telegram":true}}""")
                else -> MockResponse().setResponseCode(404)
            }
        }
        val g = graph()
        rule.setContent {
            CompositionLocalProvider(LocalGraph provides g, LocalShop provides ShopState(g.api)) {
                BoutiqueTheme(dark = false) {
                    LoginScreen(onBack = {}, onSms = {}, onForgot = {}, onEmail = {}, onDone = {}, onRestart = {}, telegramPollMs = 50)
                }
            }
        }
        rule.waitUntil(10_000) { rule.onAllNodes(hasTestTag("telegram")).fetchSemanticsNodes().isNotEmpty() }
        rule.onNodeWithTag("email_login").assertExists()
        assertEquals(0, rule.onAllNodes(hasTestTag("phone")).fetchSemanticsNodes().size)
        rule.onNodeWithTag("telegram").performScrollTo().performClick()
        rule.waitUntil(5_000) { rule.onAllNodes(hasTestTag("telegram_wait")).fetchSemanticsNodes().isNotEmpty() }
        // Сам опрос идёт через delay, а часы Robolectric в CI не двигаются надёжно:
        // здесь проверяем запуск входа, а опрос покрыт тестами бэкенда и сайта.
        val paths = generateSequence { server.takeRequest(1, java.util.concurrent.TimeUnit.SECONDS) }.take(50).map { it.path }.toList()
        org.junit.Assert.assertTrue("telegram/start not called: $paths", "/v1/auth/telegram/start" in paths)
    }

    @Test fun emailRegistrationSendsCodeThenRegisters() {
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse = when (request.path) {
                "/v1/auth/email/code" -> MockResponse().setResponseCode(202).setHeader("Content-Type", "application/json").setBody("""{"data":{"sent":true}}""")
                "/v1/auth/email/register" -> json("""{"data":{"access_token":"t","is_new_user":true,"user":{"id":"u","role":"client"}}}""")
                "/v1/me" -> json("""{"data":{"id":"u","role":"client","registration_complete":false,"has_password":true,"email":"anna@test.uz"}}""")
                else -> MockResponse().setResponseCode(404)
            }
        }
        val g = graph()
        var next: String? = "unset"
        rule.setContent {
            CompositionLocalProvider(LocalGraph provides g, LocalShop provides ShopState(g.api)) {
                BoutiqueTheme(dark = false) { EmailScreen(initialMode = "register", onBack = {}, onDone = { next = it }) }
            }
        }
        rule.onNodeWithTag("email").performScrollTo().performTextInput("bad-address")
        rule.onNodeWithTag("send_code").performScrollTo().performClick()
        rule.onNodeWithText("Проверьте адрес почты").assertExists()
        rule.onNodeWithTag("email").performScrollTo().performTextClearance()
        rule.onNodeWithTag("email").performScrollTo().performTextInput("Anna@Test.uz")
        rule.onNodeWithTag("send_code").performScrollTo().performClick()
        rule.waitUntil(5_000) { rule.onAllNodes(hasTestTag("code")).fetchSemanticsNodes().isNotEmpty() }
        rule.onNodeWithTag("code").performScrollTo().performTextInput("123456")
        rule.onNodeWithTag("new_password").performScrollTo().performTextInput("long-password-1")
        rule.onNodeWithTag("repeat_password").performScrollTo().performTextInput("long-password-1")
        rule.onNodeWithTag("submit").performScrollTo().performClick()
        rule.waitUntil(5_000) { next != "unset" }
        assertEquals(Routes.WELCOME, next)
        val code = server.takeRequest()
        assertEquals("/v1/auth/email/code", code.path)
        assertEquals("""{"email":"anna@test.uz","purpose":"register","lang":"ru"}""", code.body.readUtf8())
        assertEquals("""{"email":"anna@test.uz","code":"123456","password":"long-password-1"}""", server.takeRequest().body.readUtf8())
    }

    private companion object {
        const val SMS_METHODS = """{"data":{"email":true,"sms":true,"google":null,"telegram":null}}"""
        const val PROD_METHODS = """{"data":{"email":true,"sms":false,"google":null,"telegram":{"bot_username":"shop_bot"}}}"""
    }
}