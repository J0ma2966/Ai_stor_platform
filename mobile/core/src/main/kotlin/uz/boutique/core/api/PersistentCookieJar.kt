package uz.boutique.core.api

import kotlinx.serialization.Serializable
import kotlinx.serialization.builtins.ListSerializer
import okhttp3.Cookie
import okhttp3.CookieJar
import okhttp3.HttpUrl

/** Хранилище строк. На Android — SharedPreferences, зашифрованные ключом из Android Keystore. */
interface KeyValueStore {
    fun get(key: String): String?
    fun put(key: String, value: String?)
}

class MemoryStore : KeyValueStore {
    private val map = HashMap<String, String>()
    @Synchronized override fun get(key: String) = map[key]
    @Synchronized override fun put(key: String, value: String?) {
        if (value == null) map.remove(key) else map[key] = value
    }
}

/**
 * Cookie API (refresh-токен `rt`, Path=/v1/auth) переживают перезапуск приложения.
 * OkHttp сам проверяет домен, путь и Secure: cookie уходит только на /v1/auth/… своего сервера.
 */
class PersistentCookieJar(
    private val store: KeyValueStore,
    private val now: () -> Long = System::currentTimeMillis,
) : CookieJar {
    private val lock = Any()
    private var cache: MutableList<Cookie>? = null

    @Serializable
    private data class Stored(
        val name: String,
        val value: String,
        val expiresAt: Long,
        val domain: String,
        val path: String,
        val secure: Boolean,
        val httpOnly: Boolean,
        val hostOnly: Boolean,
    )

    private fun load(): MutableList<Cookie> {
        cache?.let { return it }
        val raw = store.get(KEY)
        val list = runCatching {
            if (raw == null) emptyList() else ApiJson.decodeFromString(ListSerializer(Stored.serializer()), raw)
        }.getOrDefault(emptyList()).map { s ->
            Cookie.Builder().name(s.name).value(s.value).expiresAt(s.expiresAt).path(s.path).apply {
                if (s.hostOnly) hostOnlyDomain(s.domain) else domain(s.domain)
                if (s.secure) secure()
                if (s.httpOnly) httpOnly()
            }.build()
        }
        return list.toMutableList().also { cache = it }
    }

    private fun persist(list: List<Cookie>) {
        val stored = list.filter { it.persistent }.map {
            Stored(it.name, it.value, it.expiresAt, it.domain, it.path, it.secure, it.httpOnly, it.hostOnly)
        }
        store.put(KEY, if (stored.isEmpty()) null else ApiJson.encodeToString(ListSerializer(Stored.serializer()), stored))
    }

    override fun saveFromResponse(url: HttpUrl, cookies: List<Cookie>) {
        synchronized(lock) {
            val list = load()
            for (c in cookies) {
                list.removeAll { it.name == c.name && it.domain == c.domain && it.path == c.path }
                if (c.expiresAt > now()) list.add(c) // Max-Age=-1 (выход) просто удаляет cookie
            }
            persist(list)
        }
    }

    override fun loadForRequest(url: HttpUrl): List<Cookie> = synchronized(lock) {
        val list = load()
        val t = now()
        if (list.removeAll { it.expiresAt <= t }) persist(list)
        list.filter { it.matches(url) }
    }

    fun hasCookies(): Boolean = synchronized(lock) { load().any { it.expiresAt > now() } }

    fun clear() = synchronized(lock) {
        load().clear()
        persist(emptyList())
    }

    private companion object {
        const val KEY = "cookies_v1"
    }
}
