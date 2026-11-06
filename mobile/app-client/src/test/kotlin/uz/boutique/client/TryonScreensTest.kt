package uz.boutique.client

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import uz.boutique.core.api.MemoryStore
import uz.boutique.core.model.Role
import uz.boutique.core.model.SessionResp
import uz.boutique.core.tryon.FittingItem
import uz.boutique.core.tryon.GenStatus
import uz.boutique.core.tryon.Generation
import uz.boutique.core.tryon.Zones
import uz.boutique.ui.AppGraph
import uz.boutique.ui.BoutiqueTheme
import uz.boutique.ui.LocalGraph
import java.util.concurrent.ConcurrentLinkedQueue

@RunWith(AndroidJUnit4::class)
class TryonScreensTest {
    @get:Rule val rule = createComposeRule()
    private val server = MockWebServer()
    private val requests = ConcurrentLinkedQueue<RecordedRequest>()

    @After fun stop() = server.shutdown()

    private fun graph(loggedIn: Boolean) = AppGraph(
        ApplicationProvider.getApplicationContext(), Role.CLIENT, server.url("/").toString(), false, "https://example.uz", "test", MemoryStore(),
    ).also { if (loggedIn) it.api.session.set(SessionResp("t", user = uz.boutique.core.model.SessionUser("u", Role.CLIENT))) }

    private fun json(body: String, code: Int = 200) = MockResponse().setResponseCode(code)
        .setHeader("Content-Type", "application/json").setBody(body)

    private val shirt = FittingItem("p1", "Футболка", "Brand", 15_000_000, "", Zones.TOP)

    private fun show(g: AppGraph, shop: ShopState, content: @androidx.compose.runtime.Composable () -> Unit) {
        rule.setContent {
            CompositionLocalProvider(LocalGraph provides g, LocalShop provides shop) {
                BoutiqueTheme(dark = false) { content() }
            }
        }
    }

    @Test fun fittingRoomKeepsOneItemPerZoneAndRemoves() {
        val g = graph(loggedIn = false)
        val shop = ShopState(g.api, MemoryStore())
        shop.setFitting(listOf(shirt))
        show(g, shop) { FittingRoomScreen(onBack = {}, onProduct = {}, onStart = {}, onHistory = {}, onCatalog = {}) }
        rule.onNodeWithText("Футболка").assertExists()
        rule.onNodeWithTag("tryon_generate").assertExists()
        rule.onNodeWithTag("remove_top").performClick()
        rule.onNodeWithText("Футболка").assertDoesNotExist()
        rule.onNodeWithTag("tryon_generate").assertDoesNotExist()
        assertTrue(shop.fittingItems.isEmpty())
    }

    @Test fun twoPieceSetHidesTheBottomZone() {
        val g = graph(loggedIn = false)
        val shop = ShopState(g.api, MemoryStore())
        shop.setFitting(listOf(FittingItem("p2", "Брюки", "Brand", 9_000_000, "", Zones.BOTTOM)))
        show(g, shop) { FittingRoomScreen(onBack = {}, onProduct = {}, onStart = {}, onHistory = {}, onCatalog = {}) }
        rule.onNodeWithTag("zone_bottom").assertExists()
        shop.setFitting(listOf(FittingItem("p3", "Костюм-двойка", "Brand", 30_000_000, "", Zones.TOP, isSet = true)))
        rule.waitForIdle()
        rule.onNodeWithTag("zone_bottom").assertDoesNotExist()
        rule.onNodeWithText("Двойка (верх + низ)").assertExists()
        rule.onNodeWithTag("set_hint").assertExists()
        // выбранный отдельно низ вытесняет двойку и возвращает зону «низ»
        shop.setFitting(listOf(FittingItem("p3", "Костюм-двойка", "Brand", 30_000_000, "", Zones.TOP, isSet = true), FittingItem("p2", "Брюки", "Brand", 9_000_000, "", Zones.BOTTOM)))
        rule.waitForIdle()
        rule.onNodeWithTag("zone_bottom").assertExists()
        rule.onNodeWithText("Костюм-двойка").assertDoesNotExist()
    }

    @Test fun wizardAsksConsentsThenStartsOnce() {
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                requests += request
                return when (request.path) {
                    "/v1/tryon/quote" -> json("""{"data":{"cost":5,"balance":10,"enough":true,"consent":false,"registered":true,
                        "supported_zones":["headwear","top","bottom","shoes"],"transfer":{"required":true,"recipient":"OpenAI","consent":false}}}""")
                    "/v1/tryon/photos" -> json("""{"data":[{"angle":"front","key":"kf","url":""},{"angle":"side","key":"ks","url":""},{"angle":"back","key":"kb","url":""}],"next_cursor":""}""")
                    "/v1/me" -> json("""{"data":{"id":"u","role":"client","height_cm":170}}""")
                    "/v1/tryon/consent", "/v1/tryon/consent/transfer" -> MockResponse().setResponseCode(204)
                    "/v1/tryon/generations" -> json("""{"data":{"id":"g1","status":"queued","cost":5}}""", 202)
                    else -> MockResponse().setResponseCode(404)
                }
            }
        }
        val g = graph(loggedIn = true)
        val shop = ShopState(g.api, MemoryStore())
        shop.setFitting(listOf(shirt))
        var started: String? = null
        show(g, shop) { TryonWizardScreen(onBack = {}, onStarted = { started = it }, onCatalog = {}) }

        rule.waitUntil(5_000) { rule.onAllNodes(hasTestTag("next_photos")).fetchSemanticsNodes().isNotEmpty() }
        rule.onNodeWithTag("next_photos").performScrollTo().assertIsNotEnabled() // нет согласий
        rule.onNodeWithTag("consent").performScrollTo().performClick()
        rule.onNodeWithTag("next_photos").assertIsNotEnabled() // нет согласия на передачу в OpenAI
        rule.onNodeWithTag("transfer_consent").performScrollTo().performClick()
        rule.onNodeWithTag("next_photos").assertIsEnabled().performClick()

        rule.waitUntil(5_000) { rule.onAllNodes(hasTestTag("next_body")).fetchSemanticsNodes().isNotEmpty() }
        rule.onNodeWithText("170").assertExists() // рост из профиля
        rule.onNodeWithTag("next_body").performClick()
        rule.onNodeWithText("Стоимость: 5 токенов. Ваш баланс: 10.").assertExists()
        rule.onNodeWithTag("run").performClick()
        rule.waitUntil(5_000) { started != null }
        assertEquals("g1", started)

        val paths = requests.map { it.method + " " + it.path }
        assertTrue(paths.contains("POST /v1/tryon/consent"))
        assertTrue(paths.contains("POST /v1/tryon/consent/transfer"))
        val create = requests.single { it.path == "/v1/tryon/generations" }
        assertTrue(create.getHeader("Idempotency-Key").orEmpty().length >= 8)
        val body = create.body.readUtf8()
        assertTrue(body, body.contains(""""product_ids":["p1"]""") && body.contains(""""front":"kf"""") && body.contains(""""save_photos":false"""))
        assertTrue(body, body.contains(""""height_cm":170"""))
    }

    @Test fun disabledServerShowsStub() {
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest) = json("""{"error":{"code":"tryon_disabled","message":"off"}}""", 503)
        }
        val g = graph(loggedIn = true)
        val shop = ShopState(g.api, MemoryStore())
        shop.setFitting(listOf(shirt))
        show(g, shop) { TryonWizardScreen(onBack = {}, onStarted = {}, onCatalog = {}) }
        rule.waitUntil(5_000) { rule.onAllNodes(hasTestTag("tryon_disabled")).fetchSemanticsNodes().isNotEmpty() }
    }

    @Test fun failedGenerationExplainsAndRefunds() {
        val g = graph(loggedIn = true)
        val shop = ShopState(g.api, MemoryStore())
        val gen = Generation("g1", GenStatus.FAILED, cost = 5, errorCode = "photo_rejected", refunded = true)
        show(g, shop) {
            GenerationContent(gen, androidx.compose.ui.Modifier, onChanged = {}, onDeleted = {}, onProduct = {}, onFitting = {}, onRepeat = {})
        }
        rule.onNodeWithText("AI не смог распознать человека на фото. Загрузите фото в полный рост по подсказкам.").assertExists()
        rule.onNodeWithText("5 токенов возвращены на баланс.").assertExists()
        rule.onNodeWithTag("repeat").assertExists()
    }
}
