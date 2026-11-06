package uz.boutique.core.tryon

import kotlinx.serialization.Serializable
import kotlinx.serialization.builtins.ListSerializer
import uz.boutique.core.api.ApiJson
import uz.boutique.core.api.KeyValueStore
import uz.boutique.core.model.CartItem
import uz.boutique.core.model.Product

/** Вещь в примерочной: снимок карточки, чтобы примерочная открывалась без сети. */
@Serializable
data class FittingItem(
    val id: String,
    val title: String,
    val brand: String = "",
    val price: Long = 0,
    val imageUrl: String = "",
    val zone: String,
    val storeName: String = "",
    /** «Двойка»: верх и низ на одном фото; бывает только в зоне «верх» и занимает и «низ». */
    val isSet: Boolean = false,
) {
    companion object {
        fun of(p: Product): FittingItem? {
            val zone = p.zone ?: return null
            if (zone !in Zones.ALL) return null
            return FittingItem(p.id, p.title, p.brand, p.price, p.cardImage.orEmpty(), zone, p.storeName, p.isSet && zone == Zones.TOP)
        }

        fun of(c: CartItem): FittingItem? {
            if (c.zone !in Zones.ALL) return null
            return FittingItem(c.productId, c.title, c.brand, c.price, c.imageUrl.orEmpty(), c.zone, c.storeName, c.isSet && c.zone == Zones.TOP)
        }

        /**
         * Вещи для выбора в примерочной — только из корзины (не весь каталог), как на сайте.
         * Один товар в разных размерах — один раз; порядок — по зонам манекена.
         */
        fun fromCart(items: List<CartItem>): List<FittingItem> {
            val list = items.mapNotNull { of(it) }.distinctBy { it.id }
            return Zones.ALL.flatMap { z -> list.filter { it.zone == z } }
        }
    }
}

/**
 * Примерочная (как на сайте, web/src/lib/fitting.ts): по одной вещи на зону манекена,
 * новая вещь той же зоны заменяет старую. Хранится на телефоне и работает без входа.
 * «Двойка» лежит в зоне «верх» и занимает также «низ»: выбранный низ убирается, а
 * отдельный низ, добавленный позже, вытесняет двойку.
 */
class FittingSelection(private val store: KeyValueStore) {
    private var items: Map<String, FittingItem> = load()

    val all: List<FittingItem> get() = Zones.ALL.mapNotNull { items[it] }
    val count: Int get() = items.size

    /** Выбрана «двойка» — зона «низ» занята ею. */
    val hasSet: Boolean get() = items[Zones.TOP]?.isSet == true

    /** Зоны, которые показывает примерочная (без «низа» при двойке). */
    val visibleZones: List<String> get() = visibleZones(all)
    operator fun get(zone: String): FittingItem? = items[zone]
    fun contains(productId: String): Boolean = items.values.any { it.id == productId }

    /** @return true, если вещь новая (для счётчика «В примерочную»). */
    fun add(item: FittingItem): Boolean {
        require(item.zone in Zones.ALL) { "unknown zone ${item.zone}" }
        val x = if (item.isSet && item.zone != Zones.TOP) item.copy(isSet = false) else item
        val isNew = items[x.zone]?.id != x.id
        var next = items + (x.zone to x)
        if (x.isSet) next = next - Zones.BOTTOM // двойка уже включает низ
        if (x.zone == Zones.BOTTOM && next[Zones.TOP]?.isSet == true) next = next - Zones.TOP
        items = next
        save()
        return isNew
    }

    fun remove(zone: String) {
        items = items - zone
        save()
    }

    fun clear() {
        items = emptyMap()
        save()
    }

    private fun load(): Map<String, FittingItem> = try {
        store.get(KEY)?.let { raw ->
            ApiJson.decodeFromString(ListSerializer(FittingItem.serializer()), raw)
                .filter { it.zone in Zones.ALL }
                .associateBy { it.zone }
                .let { m -> if (m[Zones.TOP]?.isSet == true) m - Zones.BOTTOM else m }
        } ?: emptyMap()
    } catch (_: Exception) {
        emptyMap()
    }

    private fun save() = store.put(KEY, ApiJson.encodeToString(ListSerializer(FittingItem.serializer()), all))

    companion object {
        const val KEY = "fitting_v1"

        /** Зоны примерочной для выбранных вещей: при «двойке» без «низа». */
        fun visibleZones(items: List<FittingItem>): List<String> =
            if (items.any { it.zone == Zones.TOP && it.isSet }) Zones.ALL - Zones.BOTTOM else Zones.ALL
    }
}

/** Проверки мастера примерки (как в web/src/components/TryOnWizard.tsx). */
object TryonRules {
    val BODY_PARAMS = listOf("chest_cm", "waist_cm", "hips_cm")

    /** Число из поля ввода: «72,5» → 72.5; пусто или мусор → null. */
    fun number(s: String): Double? = s.trim().replace(',', '.').toDoubleOrNull()?.takeIf { it > 0 }

    /** Параметры тела из полей формы; null, если ничего не заполнено. */
    fun body(height: String, weight: String, params: Map<String, String>): BodyInput? {
        val p = BODY_PARAMS.mapNotNull { k -> number(params[k].orEmpty())?.let { k to it } }.toMap()
        val b = BodyInput(number(height)?.toInt(), number(weight), p.ifEmpty { null })
        return if (b.isEmpty) null else b
    }

    /** Вещи из зон, которые текущая AI-модель не переодевает (останутся как на фото). */
    fun unsupported(items: List<FittingItem>, quote: Quote?): List<FittingItem> =
        if (quote == null) emptyList() else items.filter { it.zone !in quote.supportedZones }
}
