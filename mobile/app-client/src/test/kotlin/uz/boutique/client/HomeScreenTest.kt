package uz.boutique.client

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollToNode
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import uz.boutique.core.api.MemoryStore
import uz.boutique.core.model.Role
import uz.boutique.ui.AppGraph
import uz.boutique.ui.BoutiqueTheme
import uz.boutique.ui.LocalGraph

@RunWith(AndroidJUnit4::class)
class HomeScreenTest {
    @get:Rule val rule = createComposeRule()
    private val server = MockWebServer()

    @After fun stop() = server.shutdown()

    private fun json(body: String) = MockResponse().setHeader("Content-Type", "application/json").setBody(body)

    private fun serve(products: String, stores: String = "[]", categories: String = "[]") {
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                val path = request.path.orEmpty()
                return when {
                    path.startsWith("/v1/products") -> json("""{"data":$products,"next_cursor":""}""")
                    path.startsWith("/v1/categories") -> json("""{"data":$categories}""")
                    path.startsWith("/v1/stores") -> json("""{"data":$stores}""")
                    path.startsWith("/v1/promotions") -> json("""{"data":[]}""")
                    else -> MockResponse().setResponseCode(404)
                }
            }
        }
    }

    private fun show(tryon: Boolean, clicks: MutableList<String>) {
        val g = AppGraph(
            ApplicationProvider.getApplicationContext(), Role.CLIENT, server.url("/").toString(), false, "https://example.uz", "test", MemoryStore(),
        )
        rule.setContent {
            CompositionLocalProvider(LocalGraph provides g, LocalShop provides ShopState(g.api)) {
                BoutiqueTheme(dark = false) {
                    HomeScreen(
                        onCatalog = { sort, cat -> clicks += "catalog:$sort:$cat" },
                        onProduct = { clicks += "product:$it" },
                        onStores = { clicks += "stores" },
                        onStore = { clicks += "store:${it.slug}" },
                        onFitting = { clicks += "fitting" },
                        onFavorites = { clicks += "favorites" },
                        onNeedLogin = {},
                        tryon = tryon,
                    )
                }
            }
        }
        rule.waitUntil(5_000) { rule.onAllNodes(hasTestTag("home")).fetchSemanticsNodes().isNotEmpty() }
    }

    /** Прокрутить главную до элемента и нажать: на маленьком экране теста он может быть ниже края. */
    private fun tap(tag: String) {
        rule.onNodeWithTag("home").performScrollToNode(hasTestTag(tag))
        rule.onNodeWithTag(tag).performClick()
    }

    /** Пустой каталог (как сейчас в бою): кнопки всё равно на месте и ведут куда надо. */
    @Test fun emptyCatalogStillShowsSiteButtons() {
        serve(products = "[]")
        val clicks = mutableListOf<String>()
        show(tryon = false, clicks = clicks)
        rule.onNodeWithText("Бутики Ташкента в одном каталоге").assertExists()
        tap("home_btn_primary")
        tap("home_btn_secondary")
        tap("home_promo_new")
        tap("home_promo_stores")
        rule.onNodeWithTag("open_favorites").performClick()
        assertEquals(listOf("catalog:null:null", "stores", "catalog:new:null", "stores", "favorites"), clicks)
        rule.onNodeWithTag("home").performScrollToNode(hasTestTag("home_empty"))
        rule.onNodeWithText("Скоро здесь появятся товары.").assertExists()
    }

    @Test fun tryonButtonsLeadToFittingRoom() {
        serve(products = """[{"id":"p1","title":"Платье миди","price":45000000,"in_stock":true}]""")
        val clicks = mutableListOf<String>()
        show(tryon = true, clicks = clicks)
        rule.onNodeWithText("Примерьте образ до покупки").assertExists()
        tap("home_btn_primary")
        tap("home_btn_secondary")
        tap("home_cta")
        assertEquals(listOf("fitting", "catalog:null:null", "fitting"), clicks)
    }

    @Test fun sectionsOpenCatalogCategoryAndStore() {
        serve(
            products = """[{"id":"p1","title":"Платье миди","price":45000000,"in_stock":true}]""",
            stores = """[{"id":"s1","slug":"chic","name":"Chic","product_count":3}]""",
            categories = """[{"id":"c1","slug":"dresses","zone":"top","name_ru":"Платья","name_uz":"Ko‘ylaklar"}]""",
        )
        val clicks = mutableListOf<String>()
        show(tryon = false, clicks = clicks)
        tap("home_cat_dresses")
        tap("home_all_popular")
        tap("store_chic")
        tap("home_all_stores")
        assertEquals(listOf("catalog:null:dresses", "catalog:popular:null", "store:chic", "stores"), clicks)
    }
}
