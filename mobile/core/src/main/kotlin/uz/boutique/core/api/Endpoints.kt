package uz.boutique.core.api

import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.builtins.serializer
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonArray
import uz.boutique.core.model.AuthMethods
import uz.boutique.core.model.Cart
import uz.boutique.core.model.CartItem
import uz.boutique.core.model.Category
import uz.boutique.core.model.CheckoutInput
import uz.boutique.core.model.CheckoutResult
import uz.boutique.core.model.CreatedProduct
import uz.boutique.core.model.Me
import uz.boutique.core.model.MyReview
import uz.boutique.core.model.Notification
import uz.boutique.core.model.OrderDetail
import uz.boutique.core.model.OrderStatus
import uz.boutique.core.model.OrderSummary
import uz.boutique.core.model.Page
import uz.boutique.core.model.PlatformSettings
import uz.boutique.core.model.Product
import uz.boutique.core.model.Review
import uz.boutique.core.model.Role
import uz.boutique.core.model.SessionResp
import uz.boutique.core.model.Store
import uz.boutique.core.model.TelegramStart
import uz.boutique.core.model.StoreStats
import uz.boutique.core.model.UnreadCount
import uz.boutique.core.model.UploadResult
import java.net.URLEncoder

internal fun enc(s: String): String = URLEncoder.encode(s, "UTF-8").replace("+", "%20")

internal fun query(vararg pairs: Pair<String, String?>): String {
    val parts = pairs.filter { !it.second.isNullOrEmpty() }.map { "${it.first}=${enc(it.second!!)}" }
    return if (parts.isEmpty()) "" else "?" + parts.joinToString("&")
}

/** Вход и аккаунт. [appRole] — роль, для которой сделано приложение. */
class AuthApi(private val c: ApiClient, private val appRole: Role) {
    /** Какие способы входа показывать (Telegram, Google, почта; SMS — если включён на сервере). */
    suspend fun methods(): AuthMethods = c.get("/v1/auth/methods", AuthMethods.serializer())

    /** Код на почту: purpose = register | reset. */
    suspend fun sendEmailCode(email: String, purpose: String, lang: String) =
        c.exec("POST", "/v1/auth/email/code", buildJsonObject { put("email", email); put("purpose", purpose); put("lang", lang) })

    suspend fun emailRegister(email: String, code: String, password: String): SessionResp =
        guard(c.startSession("/v1/auth/email/register", buildJsonObject { put("email", email); put("code", code); put("password", password) }))

    suspend fun emailLogin(email: String, password: String): SessionResp =
        guard(c.startSession("/v1/auth/email/login", buildJsonObject { put("email", email); put("password", password) }))

    suspend fun emailReset(email: String, code: String, password: String): SessionResp =
        guard(c.startSession("/v1/auth/email/reset", buildJsonObject { put("email", email); put("code", code); put("password", password) }))

    /** ID-токен Google из Credential Manager (serverClientId = веб-клиент из [methods]). */
    suspend fun google(idToken: String): SessionResp =
        guard(c.startSession("/v1/auth/google/", buildJsonObject { put("id_token", idToken) }))

    suspend fun telegramStart(): TelegramStart = c.send("POST", "/v1/auth/telegram/start", null, TelegramStart.serializer())

    /**
     * null — вход ещё не подтверждён в боте. ApiException telegram_login_expired —
     * попытка истекла, нужно начать заново.
     */
    suspend fun telegramPoll(t: TelegramStart): SessionResp? {
        val r = c.send("POST", "/v1/auth/telegram/poll", buildJsonObject {
            put("login_id", t.loginId); put("poll_token", t.pollToken)
        }, JsonObject.serializer())
        if (r["access_token"] == null) return null
        val s = ApiJson.decodeFromJsonElement(SessionResp.serializer(), r)
        c.session.set(s)
        return guard(s)
    }

    suspend fun requestOtp(phone: String) = c.exec("POST", "/v1/auth/otp/request", buildJsonObject { put("phone", phone) })

    suspend fun verifyOtp(phone: String, code: String): SessionResp =
        guard(c.startSession("/v1/auth/otp/verify", buildJsonObject { put("phone", phone); put("code", code) }))

    suspend fun passwordLogin(phone: String, password: String): SessionResp =
        guard(c.startSession("/v1/auth/password/login", buildJsonObject { put("phone", phone); put("password", password) }))

    suspend fun resetPassword(phone: String, code: String, password: String): SessionResp =
        guard(c.startSession("/v1/auth/password/reset", buildJsonObject { put("phone", phone); put("code", code); put("password", password) }))

    /** Первый пароль клиента или смена (тогда нужен текущий). Сервер выдаёт новую сессию. */
    suspend fun setPassword(password: String, current: String? = null): SessionResp =
        c.startSession("/v1/auth/password", buildJsonObject {
            put("password", password)
            if (!current.isNullOrEmpty()) put("current_password", current)
        })

    /** Вход магазина (и админа) по логину и паролю. */
    suspend fun staffLogin(login: String, password: String): SessionResp =
        guard(c.startSession("/v1/auth/login", buildJsonObject { put("login", login); put("password", password) }))

    suspend fun logout() {
        try {
            c.exec("POST", "/v1/auth/logout")
        } catch (_: ApiException) {
            // сервер недоступен — всё равно выходим локально
        } finally {
            c.cookies.clear()
            c.session.clear()
        }
    }

    suspend fun me(): Me = c.get("/v1/me", Me.serializer())

    suspend fun completeRegistration(gender: String, lang: String): Me {
        c.exec("POST", "/v1/me/complete-registration", buildJsonObject {
            put("gender", gender); put("pd_consent", true); put("lang", lang)
        })
        return me()
    }

    suspend fun setLang(lang: String) = c.exec("PATCH", "/v1/me", buildJsonObject { put("lang", lang) })

    suspend fun deleteAccount() {
        c.exec("DELETE", "/v1/me", buildJsonObject { put("confirm", true) })
        c.cookies.clear()
        c.session.clear()
    }

    /** Магазин не должен войти в приложение покупателя и наоборот. */
    private suspend fun guard(s: SessionResp): SessionResp {
        if (s.user.role != appRole) {
            logout()
            throw ApiException(403, ApiException.WRONG_APP, "account role ${s.user.role} is not allowed in this app")
        }
        return s
    }
}

data class CatalogFilters(
    val q: String? = null,
    val category: String? = null,
    val store: String? = null,
    val brand: String? = null,
    val priceMinSum: Long? = null,
    val priceMaxSum: Long? = null,
    val inStock: Boolean = false,
    val sort: String? = null,
) {
    fun toQuery(cursor: String?, limit: Int): String = query(
        "q" to q?.trim(),
        "category" to category,
        "store" to store,
        "brand" to brand,
        "price_min" to priceMinSum?.let { (it * 100).toString() }, // в API — тийины
        "price_max" to priceMaxSum?.let { (it * 100).toString() },
        "in_stock" to if (inStock) "true" else null,
        "sort" to sort,
        "cursor" to cursor,
        "limit" to limit.toString(),
    )

    companion object {
        val SORTS = listOf("popular", "rating", "new", "price_asc", "price_desc")
    }
}

class CatalogApi(private val c: ApiClient) {
    suspend fun categories(): List<Category> = c.get("/v1/categories", ListSerializer(Category.serializer()))
    suspend fun products(f: CatalogFilters, cursor: String? = null, limit: Int = 24): Page<Product> =
        c.list("/v1/products" + f.toQuery(cursor, limit), Product.serializer())
    suspend fun product(id: String): Product = c.get("/v1/products/${enc(id)}", Product.serializer())
    /** Товары с идущей рекламой для карусели «Реклама» на главной (одобряет суперадмин). */
    suspend fun promoted(): List<Product> = c.list("/v1/promotions", Product.serializer()).items
    /** Открытие карточки товара (статистика магазина). Ошибки не важны. */
    suspend fun markViewed(id: String) = runCatching { c.exec("POST", "/v1/products/${enc(id)}/view") }
    suspend fun stores(q: String? = null): List<Store> = c.get("/v1/stores" + query("q" to q), ListSerializer(Store.serializer()))
    suspend fun store(slug: String): Store = c.get("/v1/stores/${enc(slug)}", Store.serializer())
    suspend fun platform(): PlatformSettings = c.get("/v1/platform", PlatformSettings.serializer())
    suspend fun reviews(productId: String, cursor: String? = null): Page<Review> =
        c.list("/v1/products/${enc(productId)}/reviews" + query("limit" to "10", "cursor" to cursor), Review.serializer())
}

class ShopApi(private val c: ApiClient) {
    private fun cartPath(productId: String, size: String?) =
        "/v1/me/cart/${enc(productId)}" + query("size" to size)

    suspend fun cart(): Cart = c.get("/v1/me/cart", Cart.serializer())
    suspend fun setQty(productId: String, qty: Int, size: String? = null): Cart =
        c.send("PUT", cartPath(productId, size), buildJsonObject { put("qty", qty) }, Cart.serializer())
    suspend fun remove(productId: String, size: String? = null): Cart =
        c.send("DELETE", cartPath(productId, size), null, Cart.serializer())

    suspend fun favorites(cursor: String? = null): Page<CartItem> =
        c.list("/v1/me/favorites" + query("cursor" to cursor), CartItem.serializer())
    suspend fun favoriteIds(): Set<String> = c.get("/v1/me/favorites/ids", ListSerializer(String.serializer())).toSet()
    suspend fun addFavorite(id: String) = c.exec("PUT", "/v1/me/favorites/${enc(id)}")
    suspend fun removeFavorite(id: String) = c.exec("DELETE", "/v1/me/favorites/${enc(id)}")
}

enum class OrderScope(val base: String) { CLIENT("/v1/orders"), STORE("/v1/store/orders") }

class OrdersApi(private val c: ApiClient, private val scope: OrderScope) {
    suspend fun list(statuses: List<OrderStatus> = emptyList(), cursor: String? = null): Page<OrderSummary> =
        c.list(scope.base + query("status" to statuses.joinToString(",") { it.wire }.ifEmpty { null }, "cursor" to cursor), OrderSummary.serializer())

    suspend fun get(id: String): OrderDetail = c.get("${scope.base}/${enc(id)}", OrderDetail.serializer())

    /** Ключ идемпотентности создаётся при открытии формы и переиспользуется при повторе. */
    suspend fun checkout(input: CheckoutInput, idempotencyKey: String): CheckoutResult {
        check(scope == OrderScope.CLIENT)
        return c.send("POST", "/v1/orders", ApiJson.encodeToJsonElement(CheckoutInput.serializer(), input),
            CheckoutResult.serializer(), mapOf("Idempotency-Key" to idempotencyKey))
    }

    suspend fun cancel(id: String, reason: String = ""): OrderDetail =
        c.send("POST", "/v1/orders/${enc(id)}/cancel", buildJsonObject { put("reason", reason) }, OrderDetail.serializer())

    /** Смена статуса магазином. [from] — статус, который видел сотрудник (защита от гонки, иначе 409). */
    suspend fun setStatus(id: String, to: OrderStatus, from: OrderStatus, reason: String = ""): OrderDetail {
        check(scope == OrderScope.STORE)
        return c.send("POST", "${scope.base}/${enc(id)}/status", buildJsonObject {
            put("status", to.wire); put("from", from.wire); put("reason", reason)
        }, OrderDetail.serializer())
    }
}

class ReviewsApi(private val c: ApiClient) {
    suspend fun mine(productId: String): MyReview = c.get("/v1/reviews/products/${enc(productId)}", MyReview.serializer())
    suspend fun save(productId: String, rating: Int, body: String): Review =
        c.send("PUT", "/v1/reviews/products/${enc(productId)}", buildJsonObject { put("rating", rating); put("body", body) }, Review.serializer())
    suspend fun delete(productId: String) = c.exec("DELETE", "/v1/reviews/products/${enc(productId)}")

    // магазин
    suspend fun storeList(unanswered: Boolean = false, rating: Int? = null, cursor: String? = null): Page<Review> =
        c.list("/v1/store/reviews" + query("unanswered" to if (unanswered) "true" else null, "rating" to rating?.toString(), "cursor" to cursor), Review.serializer())
    suspend fun reply(id: String, text: String) = c.exec("PUT", "/v1/store/reviews/${enc(id)}/reply", buildJsonObject { put("text", text) })
    suspend fun deleteReply(id: String) = c.exec("DELETE", "/v1/store/reviews/${enc(id)}/reply")
}

class NotificationsApi(private val c: ApiClient) {
    suspend fun list(cursor: String? = null): Page<Notification> =
        c.list("/v1/me/notifications" + query("cursor" to cursor), Notification.serializer())
    suspend fun unread(): Int = c.get("/v1/me/notifications/unread-count", UnreadCount.serializer()).unread
    suspend fun markAllRead() = c.exec("POST", "/v1/me/notifications/read", buildJsonObject { put("all", true) })
    suspend fun markRead(ids: List<Long>) = c.exec("POST", "/v1/me/notifications/read", buildJsonObject {
        putJsonArray("ids") { ids.forEach { add(kotlinx.serialization.json.JsonPrimitive(it)) } }
    })
}

/** Загрузка фото: тело запроса — сам файл (JPEG/PNG/WebP до 10 МБ). */
class UploadsApi(private val c: ApiClient) {
    suspend fun upload(purpose: String, bytes: ByteArray, contentType: String): UploadResult =
        c.upload("/v1/uploads?purpose=${enc(purpose)}", bytes, contentType, UploadResult.serializer())
}

class StoreApi(private val c: ApiClient) {
    suspend fun products(): List<Product> = c.list("/v1/store/products", Product.serializer()).items
    suspend fun product(id: String): Product = c.get("/v1/store/products/${enc(id)}", Product.serializer())
    suspend fun create(body: JsonObject): CreatedProduct = c.send("POST", "/v1/store/products", body, CreatedProduct.serializer())
    suspend fun update(id: String, patch: JsonObject): Product = c.send("PATCH", "/v1/store/products/${enc(id)}", patch, Product.serializer())
    suspend fun delete(id: String) = c.exec("DELETE", "/v1/store/products/${enc(id)}")
    suspend fun setStatus(id: String, status: String) =
        c.exec("PATCH", "/v1/store/products/${enc(id)}/status", buildJsonObject { put("status", status) })
    suspend fun profile(): Store = c.get("/v1/store/profile", Store.serializer())
    suspend fun stats(from: String, to: String): StoreStats = c.get("/v1/store/stats" + query("from" to from, "to" to to), StoreStats.serializer())
    /** Заявка на рекламу товара: решение принимает суперадмин (docs/PROMOTIONS.md). */
    suspend fun requestPromotion(productId: String, days: Int, note: String) =
        c.exec("POST", "/v1/store/promotions", buildJsonObject { put("product_id", productId); put("days", days); put("note", note) })
}

/** Все API одного приложения. */
class Api(val client: ApiClient, appRole: Role) {
    val auth = AuthApi(client, appRole)
    val catalog = CatalogApi(client)
    val shop = ShopApi(client)
    val orders = OrdersApi(client, if (appRole == Role.STORE) OrderScope.STORE else OrderScope.CLIENT)
    val reviews = ReviewsApi(client)
    val notifications = NotificationsApi(client)
    val uploads = UploadsApi(client)
    val store = StoreApi(client)
    val tryon = uz.boutique.core.tryon.TryonApi(client)
    val session: SessionManager get() = client.session
}

