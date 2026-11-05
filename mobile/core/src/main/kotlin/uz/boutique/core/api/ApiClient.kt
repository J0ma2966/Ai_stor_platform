package uz.boutique.core.api

import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlinx.serialization.DeserializationStrategy
import kotlinx.serialization.KSerializer
import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.Call
import okhttp3.Callback
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import uz.boutique.core.model.Page
import uz.boutique.core.model.SessionResp
import java.io.IOException
import java.util.UUID
import java.util.concurrent.TimeUnit
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException

val ApiJson = Json {
    ignoreUnknownKeys = true
    explicitNulls = false // null-поля запроса не отправляются
    encodeDefaults = true
    coerceInputValues = true // неизвестный статус/enum → значение по умолчанию, а не падение
    isLenient = false
}

enum class RefreshOutcome { OK, NO_SESSION, FAILED }

/**
 * Единый HTTP-клиент API (как web/src/lib/api.ts):
 * - Authorization: Bearer <access> из памяти;
 * - на 401 token_invalid — один refresh (общий для параллельных запросов) и повтор;
 * - на token_revoked / refresh_invalid — выход и экран входа.
 */
class ApiClient(
    baseUrl: String,
    val session: SessionManager,
    val cookies: PersistentCookieJar,
    userAgent: String = "BoutiqueApp",
    httpClient: OkHttpClient? = null,
    private val io: CoroutineDispatcher = Dispatchers.IO,
) {
    val baseUrl: String = baseUrl.trimEnd('/')

    private val http: OkHttpClient = (httpClient?.newBuilder() ?: OkHttpClient.Builder()
        .connectTimeout(15, TimeUnit.SECONDS)
        .readTimeout(30, TimeUnit.SECONDS)
        .writeTimeout(60, TimeUnit.SECONDS)) // загрузка фото по мобильному интернету
        .cookieJar(cookies)
        .addInterceptor { chain ->
            chain.proceed(chain.request().newBuilder().header("User-Agent", userAgent).build())
        }
        .build()

    private val refreshMutex = Mutex()

    class Raw(val code: Int, val body: String, val retryAfter: Long?)

    // ---------- публичные помощники ----------

    suspend fun <T> get(path: String, de: DeserializationStrategy<T>): T = data(send("GET", path), de)

    suspend fun <T> send(method: String, path: String, body: JsonElement?, de: DeserializationStrategy<T>, headers: Map<String, String> = emptyMap()): T =
        data(send(method, path, body?.let { jsonBody(it) }, headers), de)

    /** Запрос без полезного ответа (204 или ответ не нужен). */
    suspend fun exec(method: String, path: String, body: JsonElement? = null, headers: Map<String, String> = emptyMap()) {
        send(method, path, body?.let { jsonBody(it) } ?: emptyBodyFor(method), headers)
    }

    suspend fun <T> list(path: String, item: KSerializer<T>): Page<T> {
        val raw = send("GET", path)
        val obj = parseObject(raw)
        val items = obj["data"]?.takeUnless { it is JsonNull }?.let { ApiJson.decodeFromJsonElement(ListSerializer(item), it) } ?: emptyList()
        val next = obj["next_cursor"]?.takeUnless { it is JsonNull }?.jsonPrimitive?.contentOrNull ?: ""
        return Page(items, next)
    }

    suspend fun <T> upload(path: String, bytes: ByteArray, contentType: String, de: DeserializationStrategy<T>): T =
        data(send("POST", path, bytes.toRequestBody(contentType.toMediaType())), de)

    /** Ответ /auth/… с новой сессией: запоминаем access-токен. */
    suspend fun startSession(path: String, body: JsonElement): SessionResp {
        val s = send("POST", path, body, SessionResp.serializer())
        session.set(s)
        return s
    }

    /**
     * Обновить access-токен по refresh-cookie. Вызывается при старте приложения
     * и автоматически на 401 token_invalid. Временный сбой (нет сети, 429, 5xx) — FAILED:
     * сессия при этом не сбрасывается.
     */
    suspend fun refresh(): RefreshOutcome = refreshInternal(tokenUsed = session.accessToken, force = true).first

    // ---------- внутреннее ----------

    private fun <T> data(raw: Raw, de: DeserializationStrategy<T>): T {
        val obj = parseObject(raw)
        val d = obj["data"] ?: throw ApiException(raw.code, "bad_response", "no data")
        return ApiJson.decodeFromJsonElement(de, d)
    }

    private fun parseObject(raw: Raw): JsonObject = try {
        ApiJson.parseToJsonElement(raw.body).jsonObject
    } catch (e: Exception) {
        throw ApiException(raw.code, "bad_response", "invalid json")
    }

    private fun jsonBody(e: JsonElement): RequestBody = ApiJson.encodeToString(JsonElement.serializer(), e).toRequestBody(JSON)

    private fun emptyBodyFor(method: String): RequestBody? =
        if (method == "POST" || method == "PUT" || method == "PATCH") ByteArray(0).toRequestBody(null) else null

    private suspend fun send(method: String, path: String, body: RequestBody? = null, headers: Map<String, String> = emptyMap()): Raw =
        sendInternal(method, path, body ?: emptyBodyFor(method), headers, retried = false)

    private suspend fun sendInternal(method: String, path: String, body: RequestBody?, headers: Map<String, String>, retried: Boolean): Raw {
        val tokenUsed = session.accessToken
        val raw = execute(method, path, body, headers, tokenUsed)
        if (raw.code in 200..299) return raw
        val err = parseError(raw)
        if (raw.code == 401 && !retried &&
            (err.code == "token_invalid" || (err.code == "unauthorized" && tokenUsed == null && cookies.hasCookies()))
        ) {
            val (outcome, _) = refreshInternal(tokenUsed, force = false)
            if (outcome == RefreshOutcome.OK) return sendInternal(method, path, body, headers, retried = true)
        }
        if (err.code == "token_revoked" || err.code == "refresh_invalid" || (raw.code == 401 && retried)) {
            session.lost()
        }
        throw err
    }

    private suspend fun refreshInternal(tokenUsed: String?, force: Boolean): Pair<RefreshOutcome, ApiException?> =
        refreshMutex.withLock {
            // Пока ждали блокировку, другой запрос уже обновил токен — второй refresh не нужен
            // (refresh-токен одноразовый: повтор старого отзывает всю сессию).
            val current = session.accessToken
            if (!force && current != null && current != tokenUsed) return@withLock RefreshOutcome.OK to null
            if (!cookies.hasCookies()) {
                if (current == null) session.clear()
                return@withLock RefreshOutcome.NO_SESSION to null
            }
            val raw = try {
                execute("POST", "/v1/auth/refresh", emptyBodyFor("POST"), emptyMap(), token = null)
            } catch (e: ApiException) {
                return@withLock RefreshOutcome.FAILED to e
            }
            if (raw.code in 200..299) {
                val resp = ApiJson.decodeFromJsonElement(SessionResp.serializer(), parseObject(raw)["data"]!!)
                session.set(resp)
                RefreshOutcome.OK to null
            } else if (raw.code == 401 || raw.code == 403) {
                val wasIn = session.state.value is AuthState.LoggedIn
                cookies.clear()
                if (wasIn) session.lost() else session.clear()
                RefreshOutcome.NO_SESSION to null
            } else {
                RefreshOutcome.FAILED to parseError(raw)
            }
        }

    private suspend fun execute(method: String, path: String, body: RequestBody?, headers: Map<String, String>, token: String?): Raw =
        withContext(io) {
            val rb = Request.Builder().url(baseUrl + path).method(method, body).header("Accept", "application/json")
            headers.forEach { (k, v) -> rb.header(k, v) }
            if (token != null) rb.header("Authorization", "Bearer $token")
            val resp = try {
                http.newCall(rb.build()).await()
            } catch (e: IOException) {
                throw ApiException.network(e)
            }
            resp.use { r ->
                val text = try { r.body?.string() ?: "" } catch (e: IOException) { throw ApiException.network(e) }
                Raw(r.code, text, r.header("Retry-After")?.toLongOrNull())
            }
        }

    companion object {
        private val JSON = "application/json; charset=utf-8".toMediaType()

        fun newIdempotencyKey(): String = UUID.randomUUID().toString()

        fun parseError(raw: Raw): ApiException {
            var code = when (raw.code) {
                401 -> "unauthorized"
                404 -> "not_found"
                429 -> "rate_limited"
                in 500..599 -> "internal"
                else -> "http_${raw.code}"
            }
            var message = "HTTP ${raw.code}"
            var fields = emptyMap<String, String>()
            var requestId = ""
            runCatching {
                val obj = ApiJson.parseToJsonElement(raw.body).jsonObject
                val e = obj["error"]?.jsonObject
                e?.get("code")?.jsonPrimitive?.contentOrNull?.let { code = it }
                e?.get("message")?.jsonPrimitive?.contentOrNull?.let { message = it }
                e?.get("fields")?.takeUnless { it is JsonNull }?.jsonObject?.let { f ->
                    fields = f.mapValues { (_, v) -> v.jsonPrimitive.contentOrNull ?: "" }
                }
                obj["request_id"]?.jsonPrimitive?.contentOrNull?.let { requestId = it }
            }
            return ApiException(raw.code, code, message, fields, requestId, raw.retryAfter)
        }
    }
}

private suspend fun Call.await(): Response = suspendCancellableCoroutine { cont ->
    enqueue(object : Callback {
        override fun onResponse(call: Call, response: Response) = cont.resume(response)
        override fun onFailure(call: Call, e: IOException) {
            if (!cont.isCancelled) cont.resumeWithException(e)
        }
    })
    cont.invokeOnCancellation { runCatching { cancel() } }
}
