package uz.boutique.core.tryon

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import uz.boutique.core.api.ApiClient
import uz.boutique.core.api.ApiJson
import uz.boutique.core.api.enc
import uz.boutique.core.api.query
import uz.boutique.core.model.Page

// AI-примерка: JSON как в backend/internal/tryon/handlers.go и web/src/lib/tryon.ts.
// Биометрия (фото тела, результаты) хранится на сервере в Узбекистане; приложение
// держит её только в памяти экрана и показывает по подписанным ссылкам.

object Zones {
    const val HEADWEAR = "headwear"
    const val TOP = "top"
    const val BOTTOM = "bottom"
    const val SHOES = "shoes"
    val ALL = listOf(HEADWEAR, TOP, BOTTOM, SHOES)
}

object Angles {
    val ALL = listOf("front", "side", "back")
}

@Serializable
data class TransferInfo(val required: Boolean = false, val recipient: String = "", val consent: Boolean = false)

@Serializable
data class RetentionInfo(
    @SerialName("photo_hours") val photoHours: Int = 24,
    @SerialName("saved_photo_days") val savedPhotoDays: Int = 180,
    @SerialName("result_days") val resultDays: Int = 30,
    @SerialName("saved_look_days") val savedLookDays: Int = 365,
)

@Serializable
data class Quote(
    val cost: Long = 0,
    val balance: Long = 0,
    val enough: Boolean = false,
    val consent: Boolean = false,
    val registered: Boolean = false,
    val gender: String? = null,
    val active: Int = 0,
    @SerialName("max_active") val maxActive: Int = 1,
    @SerialName("supported_zones") val supportedZones: List<String> = Zones.ALL,
    val transfer: TransferInfo = TransferInfo(),
    val retention: RetentionInfo = RetentionInfo(),
) {
    /** Все нужные согласия уже даны (общее и, если нужно, на передачу внешнему провайдеру). */
    val consentsGiven: Boolean get() = consent && (!transfer.required || transfer.consent)
}

@Serializable
data class SavedPhoto(
    val id: String = "",
    val angle: String,
    val key: String,
    val url: String = "",
    @SerialName("expires_at") val expiresAt: String = "",
)

@Serializable
data class GenPhoto(val angle: String = "", val key: String = "", val url: String = "")

@Serializable
data class GenProduct(
    val id: String,
    val title: String = "",
    val brand: String = "",
    val price: Long = 0,
    val zone: String = "",
    @SerialName("is_set") val isSet: Boolean = false,
    @SerialName("image_url") val imageUrl: String = "",
    @SerialName("store_name") val storeName: String = "",
    @SerialName("store_slug") val storeSlug: String = "",
    val available: Boolean = false,
)

@Serializable
data class BodyInput(
    @SerialName("height_cm") val heightCm: Int? = null,
    @SerialName("weight_kg") val weightKg: Double? = null,
    val params: Map<String, Double>? = null,
) {
    val isEmpty: Boolean get() = heightCm == null && weightKg == null && params.isNullOrEmpty()
}

@Serializable
enum class GenStatus {
    @SerialName("queued") QUEUED,
    @SerialName("running") RUNNING,
    @SerialName("succeeded") SUCCEEDED,
    @SerialName("failed") FAILED;

    val active: Boolean get() = this == QUEUED || this == RUNNING
}

@Serializable
data class Generation(
    val id: String,
    val status: GenStatus = GenStatus.QUEUED,
    val cost: Long = 0,
    @SerialName("error_code") val errorCode: String? = null,
    val refunded: Boolean = false,
    val saved: Boolean = false,
    @SerialName("result_url") val resultUrl: String? = null,
    @SerialName("download_url") val downloadUrl: String? = null,
    @SerialName("result_expires_at") val resultExpiresAt: String? = null,
    val photos: List<GenPhoto> = emptyList(),
    val products: List<GenProduct> = emptyList(),
    val body: BodyInput = BodyInput(),
    @SerialName("created_at") val createdAt: String = "",
    @SerialName("finished_at") val finishedAt: String? = null,
)

@Serializable
data class Photos(val front: String, val side: String, val back: String)

@Serializable
data class CreateGeneration(
    @SerialName("product_ids") val productIds: List<String>,
    val photos: Photos,
    @SerialName("save_photos") val savePhotos: Boolean,
    val body: BodyInput? = null,
)

/** /v1/tryon/… — только для покупателя. При TRYON_PROVIDER=off сервер отвечает 503 tryon_disabled. */
class TryonApi(private val c: ApiClient) {
    suspend fun quote(): Quote = c.get("/v1/tryon/quote", Quote.serializer())
    suspend fun giveConsent() = c.exec("POST", "/v1/tryon/consent")
    suspend fun giveTransferConsent() = c.exec("POST", "/v1/tryon/consent/transfer")
    /** Отзыв согласия: сервер удаляет все фото и результаты примерок клиента. */
    suspend fun revokeConsent() = c.exec("DELETE", "/v1/tryon/consent")

    suspend fun photos(): List<SavedPhoto> = c.list("/v1/tryon/photos", SavedPhoto.serializer()).items
    suspend fun deletePhoto(angle: String) = c.exec("DELETE", "/v1/tryon/photos/${enc(angle)}")

    /** Ключ идемпотентности переиспользуется при повторе — токены не спишутся дважды. */
    suspend fun create(input: CreateGeneration, idempotencyKey: String): Generation =
        c.send("POST", "/v1/tryon/generations", ApiJson.encodeToJsonElement(CreateGeneration.serializer(), input),
            Generation.serializer(), mapOf("Idempotency-Key" to idempotencyKey))

    suspend fun get(id: String): Generation = c.get("/v1/tryon/generations/${enc(id)}", Generation.serializer())

    suspend fun list(saved: Boolean, cursor: String? = null): Page<Generation> =
        c.list("/v1/tryon/generations" + query("limit" to "12", "saved" to if (saved) "true" else null, "cursor" to cursor), Generation.serializer())

    suspend fun setSaved(id: String, saved: Boolean): Generation =
        c.send(if (saved) "POST" else "DELETE", "/v1/tryon/generations/${enc(id)}/save", null, Generation.serializer())

    suspend fun delete(id: String) = c.exec("DELETE", "/v1/tryon/generations/${enc(id)}")

    /** Счётчик «В примерочную» для статистики магазина; ошибки пользователю не показываются. */
    suspend fun countFittingAdd(productId: String) {
        runCatching { c.exec("POST", "/v1/fitting/adds", buildJsonObject { put("product_id", productId) }) }
    }
}
