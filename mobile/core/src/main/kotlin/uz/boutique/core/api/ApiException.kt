package uz.boutique.core.api

/**
 * Ошибка API в формате {"error":{"code","message","fields"},"request_id"}.
 * status = 0 и code = "network" — нет связи с сервером.
 */
class ApiException(
    val status: Int,
    val code: String,
    message: String,
    val fields: Map<String, String> = emptyMap(),
    val requestId: String = "",
    val retryAfterSeconds: Long? = null,
) : Exception(message) {
    val isNetwork: Boolean get() = status == 0
    val isUnauthorized: Boolean get() = status == 401

    companion object {
        const val NETWORK = "network"
        /** Аккаунт другой роли вошёл не в своё приложение (магазин — в клиентское и наоборот). */
        const val WRONG_APP = "wrong_app"
        fun network(cause: Throwable? = null) = ApiException(0, NETWORK, cause?.message ?: "network error").also {
            if (cause != null) it.initCause(cause)
        }
    }
}
