package uz.boutique.core

import okhttp3.Cookie
import okhttp3.HttpUrl.Companion.toHttpUrl
import uz.boutique.core.api.MemoryStore
import uz.boutique.core.api.PersistentCookieJar
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class CookieJarTest {
    private val url = "https://api.example.uz/v1/auth/refresh".toHttpUrl()

    @Test fun persistsAndMatchesPath() {
        val store = MemoryStore()
        var now = 1_000_000L
        val jar = PersistentCookieJar(store) { now }
        val c = Cookie.parse(url, "rt=abc; Path=/v1/auth; Domain=api.example.uz; Secure; HttpOnly; Max-Age=60")!!
        jar.saveFromResponse(url, listOf(c))

        val again = PersistentCookieJar(store) { now }
        assertEquals("abc", again.loadForRequest(url).single().value)
        assertTrue(again.loadForRequest("https://api.example.uz/v1/me".toHttpUrl()).isEmpty())
        assertTrue(again.loadForRequest("http://api.example.uz/v1/auth/refresh".toHttpUrl()).isEmpty(), "Secure cookie over http")
        assertTrue(again.loadForRequest("https://evil.example.com/v1/auth/refresh".toHttpUrl()).isEmpty())
    }

    @Test fun deleteAndExpire() {
        val store = MemoryStore()
        val jar = PersistentCookieJar(store)
        jar.saveFromResponse(url, listOf(Cookie.parse(url, "rt=abc; Path=/v1/auth; Max-Age=60")!!))
        assertTrue(jar.hasCookies())
        jar.saveFromResponse(url, listOf(Cookie.parse(url, "rt=; Path=/v1/auth; Max-Age=-1")!!))
        assertFalse(jar.hasCookies())
        assertFalse(PersistentCookieJar(store).hasCookies())
    }

    @Test fun corruptedStorageIsIgnored() {
        val store = MemoryStore()
        store.put("cookies_v1", "{not json")
        assertFalse(PersistentCookieJar(store).hasCookies())
    }
}
