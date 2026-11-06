package uz.boutique.core

import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import uz.boutique.core.api.Api
import uz.boutique.core.api.ApiClient
import uz.boutique.core.api.ApiException
import uz.boutique.core.api.MemoryStore
import uz.boutique.core.api.PersistentCookieJar
import uz.boutique.core.api.SessionManager
import uz.boutique.core.model.Me
import uz.boutique.core.model.Product
import uz.boutique.core.model.Role
import uz.boutique.core.model.SessionResp
import uz.boutique.core.tryon.CreateGeneration
import uz.boutique.core.tryon.FittingItem
import uz.boutique.core.tryon.FittingSelection
import uz.boutique.core.tryon.GenStatus
import uz.boutique.core.tryon.Photos
import uz.boutique.core.tryon.TryonRules
import uz.boutique.core.tryon.Zones
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class TryonTest {
    private val server = MockWebServer()

    @After fun stop() = server.shutdown()

    private fun api(): Api {
        val s = SessionManager()
        s.set(SessionResp("t"))
        return Api(ApiClient(server.url("/").toString(), s, PersistentCookieJar(MemoryStore()), "test"), Role.CLIENT)
    }

    private fun json(body: String, code: Int = 200) = MockResponse().setResponseCode(code)
        .setHeader("Content-Type", "application/json").setBody(body)

    @Test fun fittingKeepsOneItemPerZoneAndPersists() {
        val store = MemoryStore()
        val f = FittingSelection(store)
        assertTrue(f.add(FittingItem("a", "Футболка", zone = Zones.TOP)))
        assertFalse(f.add(FittingItem("a", "Футболка", zone = Zones.TOP))) // повтор — не новая
        assertTrue(f.add(FittingItem("b", "Рубашка", zone = Zones.TOP))) // замена в той же зоне
        f.add(FittingItem("c", "Кепка", zone = Zones.HEADWEAR))
        assertEquals(listOf("c", "b"), f.all.map { it.id }) // порядок зон: голова, верх, низ, обувь
        assertFalse(f.contains("a"))

        val again = FittingSelection(store)
        assertEquals(2, again.count)
        again.remove(Zones.TOP)
        assertEquals(listOf("c"), FittingSelection(store).all.map { it.id })
        assertFailsWith<IllegalArgumentException> { f.add(FittingItem("x", "?", zone = "bag")) }
    }

    @Test fun twoPieceSetTakesTheBottomZone() {
        val store = MemoryStore()
        val f = FittingSelection(store)
        f.add(FittingItem("pants", "Брюки", zone = Zones.BOTTOM))
        f.add(FittingItem("hat", "Кепка", zone = Zones.HEADWEAR))
        f.add(FittingItem("suit", "Костюм-двойка", zone = Zones.TOP, isSet = true))
        assertTrue(f.hasSet)
        assertEquals(listOf("hat", "suit"), f.all.map { it.id }) // выбранный низ убран
        assertEquals(listOf(Zones.HEADWEAR, Zones.TOP, Zones.SHOES), f.visibleZones)
        assertTrue(FittingSelection(store).hasSet) // переживает перезапуск
        // отдельный низ вытесняет двойку
        f.add(FittingItem("jeans", "Джинсы", zone = Zones.BOTTOM))
        assertFalse(f.hasSet)
        assertEquals(listOf("hat", "jeans"), f.all.map { it.id })
        assertEquals(Zones.ALL, f.visibleZones)
        // флаг двойки вне зоны «верх» не учитывается
        f.add(FittingItem("skirt", "Юбка", zone = Zones.BOTTOM, isSet = true))
        assertFalse(f.hasSet)
        // из сохранённого состояния с двойкой и низом низ отбрасывается
        store.put(FittingSelection.KEY, """[{"id":"s","title":"S","zone":"top","isSet":true},{"id":"b","title":"B","zone":"bottom"}]""")
        assertEquals(listOf("s"), FittingSelection(store).all.map { it.id })
    }

    @Test fun fittingItemFromProductKeepsSetFlagOnlyForTops() {
        val p = Product(id = "p", title = "Костюм", zone = Zones.TOP, isSet = true)
        assertTrue(FittingItem.of(p)!!.isSet)
        assertFalse(FittingItem.of(p.copy(zone = Zones.BOTTOM))!!.isSet)
        val json = """{"id":"p","title":"T","zone":"top","is_set":true,"in_stock":true}"""
        assertTrue(uz.boutique.core.api.ApiJson.decodeFromString(Product.serializer(), json).isSet)
    }

    @Test fun fittingPickerOffersOnlyCartItems() {
        val json = """{"items":[
            {"product_id":"jeans","title":"Джинсы","zone":"bottom","qty":1},
            {"product_id":"suit","title":"Костюм","zone":"top","is_set":true,"qty":1},
            {"product_id":"shirt","title":"Рубашка","zone":"top","size":"M","qty":1},
            {"product_id":"shirt","title":"Рубашка","zone":"top","size":"L","qty":1},
            {"product_id":"bag","title":"Сумка","zone":"","qty":1}
        ],"total_qty":5}"""
        val cart = uz.boutique.core.api.ApiJson.decodeFromString(uz.boutique.core.model.Cart.serializer(), json)
        val offer = FittingItem.fromCart(cart.items)
        assertEquals(listOf("suit", "shirt", "jeans"), offer.map { it.id }) // по зонам, без дублей размеров и без вещей вне зон
        assertTrue(offer.first().isSet)
        // выбран комплект — низ из корзины не предлагается
        assertEquals(listOf(Zones.HEADWEAR, Zones.TOP, Zones.SHOES), FittingSelection.visibleZones(listOf(offer.first())))
    }

    @Test fun brokenStoredFittingIsIgnored() {
        val store = MemoryStore()
        store.put(FittingSelection.KEY, "{oops")
        assertEquals(0, FittingSelection(store).count)
    }

    @Test fun bodyFromForm() {
        assertNull(TryonRules.body("", " ", emptyMap()))
        val b = TryonRules.body("172", "64,5", mapOf("waist_cm" to "70", "chest_cm" to "abc"))!!
        assertEquals(172, b.heightCm)
        assertEquals(64.5, b.weightKg)
        assertEquals(mapOf("waist_cm" to 70.0), b.params)
    }

    @Test fun meReadsBodyParams() {
        val me = uz.boutique.core.api.ApiJson.decodeFromString(Me.serializer(),
            """{"id":"u","role":"client","height_cm":170,"weight_kg":60.5,"body_params":{"waist_cm":70}}""")
        assertEquals(170, me.heightCm)
        assertEquals(mapOf("waist_cm" to 70.0), me.bodyParams)
        val empty = uz.boutique.core.api.ApiJson.decodeFromString(Me.serializer(), """{"id":"u","body_params":null}""")
        assertEquals(emptyMap(), empty.bodyParams)
    }

    @Test fun quoteAndCreateWithIdempotencyKey() = runBlocking {
        server.enqueue(json("""{"data":{"cost":5,"balance":3,"enough":false,"consent":true,"registered":true,
            "supported_zones":["top","bottom"],"transfer":{"required":true,"recipient":"OpenAI","consent":false}}}"""))
        server.enqueue(json("""{"data":{"id":"g1","status":"queued","cost":5,"products":[{"id":"p1","zone":"top"}]}}""", 202))
        val a = api()
        val q = a.tryon.quote()
        assertFalse(q.consentsGiven) // нет согласия на передачу
        assertEquals(listOf(FittingItem("p2", "Кепка", zone = Zones.HEADWEAR)),
            TryonRules.unsupported(listOf(FittingItem("p1", "Футболка", zone = Zones.TOP), FittingItem("p2", "Кепка", zone = Zones.HEADWEAR)), q))

        val g = a.tryon.create(CreateGeneration(listOf("p1"), Photos("f", "s", "b"), true), "key-12345")
        assertEquals(GenStatus.QUEUED, g.status)
        assertTrue(g.status.active)
        server.takeRequest()
        val r = server.takeRequest()
        assertEquals("/v1/tryon/generations", r.path)
        assertEquals("key-12345", r.getHeader("Idempotency-Key"))
        assertEquals("""{"product_ids":["p1"],"photos":{"front":"f","side":"s","back":"b"},"save_photos":true}""", r.body.readUtf8())
    }

    @Test fun disabledTryonIsAnApiError() = runBlocking {
        server.enqueue(json("""{"error":{"code":"tryon_disabled","message":"off"}}""", 503))
        val e = assertFailsWith<ApiException> { api().tryon.quote() }
        assertEquals("tryon_disabled", e.code)
    }

    @Test fun listAndSave() = runBlocking {
        server.enqueue(json("""{"data":[{"id":"g1","status":"succeeded","saved":true,"result_url":"http://x/r.jpg"}],"next_cursor":"c2"}"""))
        server.enqueue(json("""{"data":{"id":"g1","status":"succeeded","saved":false}}"""))
        val a = api()
        val page = a.tryon.list(saved = true)
        assertEquals("c2", page.nextCursor)
        assertEquals("/v1/tryon/generations?limit=12&saved=true", server.takeRequest().path)
        assertFalse(a.tryon.setSaved("g1", false).saved)
        assertEquals("DELETE", server.takeRequest().method)
    }
}
