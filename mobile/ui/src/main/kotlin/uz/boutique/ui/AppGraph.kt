package uz.boutique.ui

import android.content.Context
import android.os.Build
import android.util.Base64
import androidx.appcompat.app.AppCompatDelegate
import androidx.compose.runtime.Composable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.platform.LocalConfiguration
import androidx.core.content.edit
import androidx.core.os.LocaleListCompat
import uz.boutique.core.api.Api
import uz.boutique.core.api.ApiClient
import uz.boutique.core.api.KeyValueStore
import uz.boutique.core.api.PersistentCookieJar
import uz.boutique.core.api.SessionManager
import uz.boutique.core.model.Role
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

/**
 * Зашифрованное хранилище: SharedPreferences + AES-256-GCM, ключ в Android Keystore
 * (не извлекается из устройства). Здесь лежит refresh-cookie сессии.
 */
class SecurePrefs(context: Context, name: String) : KeyValueStore {
    private val prefs = context.getSharedPreferences(name, Context.MODE_PRIVATE)
    private val alias = "boutique_$name"

    private fun key(): SecretKey {
        val ks = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        (ks.getKey(alias, null) as? SecretKey)?.let { return it }
        val gen = KeyGenerator.getInstance(android.security.keystore.KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore")
        gen.init(
            android.security.keystore.KeyGenParameterSpec.Builder(
                alias,
                android.security.keystore.KeyProperties.PURPOSE_ENCRYPT or android.security.keystore.KeyProperties.PURPOSE_DECRYPT,
            )
                .setBlockModes(android.security.keystore.KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(android.security.keystore.KeyProperties.ENCRYPTION_PADDING_NONE)
                .setKeySize(256)
                .build(),
        )
        return gen.generateKey()
    }

    @Synchronized
    override fun get(key: String): String? {
        val raw = prefs.getString(key, null) ?: return null
        return try {
            val parts = raw.split(':')
            val iv = Base64.decode(parts[0], Base64.NO_WRAP)
            val ct = Base64.decode(parts[1], Base64.NO_WRAP)
            val c = Cipher.getInstance("AES/GCM/NoPadding")
            c.init(Cipher.DECRYPT_MODE, key(), GCMParameterSpec(128, iv))
            String(c.doFinal(ct), Charsets.UTF_8)
        } catch (e: Exception) {
            // ключ потерян (переустановка, сброс блокировки) — данные не расшифровать, войти заново
            prefs.edit { remove(key) }
            null
        }
    }

    @Synchronized
    override fun put(key: String, value: String?) {
        if (value == null) {
            prefs.edit { remove(key) }
            return
        }
        val c = Cipher.getInstance("AES/GCM/NoPadding")
        c.init(Cipher.ENCRYPT_MODE, key())
        val ct = c.doFinal(value.toByteArray(Charsets.UTF_8))
        val enc = Base64.encodeToString(c.iv, Base64.NO_WRAP) + ":" + Base64.encodeToString(ct, Base64.NO_WRAP)
        prefs.edit { putString(key, enc) }
    }
}

/**
 * Зависимости приложения. [allowServerOverride] — только в debug: адрес сервера можно
 * поменять на экране входа (тест на телефоне против компьютера в локальной сети).
 */
class AppGraph(
    private val context: Context,
    val role: Role,
    private val defaultBaseUrl: String,
    val allowServerOverride: Boolean,
    val siteUrl: String,
    private val versionName: String,
    private val store: KeyValueStore = SecurePrefs(context.applicationContext, "session"),
) {
    private val prefs = context.applicationContext.getSharedPreferences("app", Context.MODE_PRIVATE)

    var baseUrl: String = defaultBaseUrl
        private set
    lateinit var api: Api
        private set

    init {
        if (allowServerOverride) prefs.getString(KEY_SERVER, null)?.let { baseUrl = it }
        api = build()
    }

    private fun build(): Api {
        val ua = "BoutiqueAndroid/${role.name.lowercase()}/$versionName (Android ${Build.VERSION.RELEASE})"
        return Api(ApiClient(baseUrl, SessionManager(), PersistentCookieJar(store), ua), role)
    }

    /** Сменить сервер (debug). Сессия старого сервера забывается. */
    fun setServer(url: String) {
        require(allowServerOverride)
        val clean = url.trim().trimEnd('/')
        prefs.edit { putString(KEY_SERVER, clean) }
        api.client.cookies.clear()
        baseUrl = clean
        api = build()
    }

    fun resetServer() {
        prefs.edit { remove(KEY_SERVER) }
        api.client.cookies.clear()
        baseUrl = defaultBaseUrl
        api = build()
    }

    val isDefaultServer: Boolean get() = baseUrl == defaultBaseUrl

    private companion object {
        const val KEY_SERVER = "server_url"
    }
}

val LocalGraph = staticCompositionLocalOf<AppGraph> { error("AppGraph is not provided") }

object AppLang {
    val SUPPORTED = listOf("ru", "uz")

    /** Язык интерфейса: выбранный в приложении, иначе системный (если это uz), иначе ru. */
    fun current(): String {
        val chosen = AppCompatDelegate.getApplicationLocales()[0]?.language
        if (chosen in SUPPORTED) return chosen!!
        return if (java.util.Locale.getDefault().language == "uz") "uz" else "ru"
    }

    fun set(lang: String) {
        AppCompatDelegate.setApplicationLocales(LocaleListCompat.forLanguageTags(lang))
    }
}

/** Язык текущей конфигурации — для форматирования сумм («сум» / «so‘m»). */
@Composable
fun currentLang(): String {
    val l = LocalConfiguration.current.locales[0]?.language
    return if (l == "uz") "uz" else "ru"
}
