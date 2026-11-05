package uz.boutique.core.api

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import uz.boutique.core.model.Role
import uz.boutique.core.model.SessionResp

sealed interface AuthState {
    /** Приложение только запустилось и ещё не проверило сохранённую сессию. */
    data object Unknown : AuthState
    data object LoggedOut : AuthState
    data class LoggedIn(val userId: String, val role: Role) : AuthState
}

/**
 * Access-токен живёт только в памяти (15 минут). Долгая сессия — refresh-cookie `rt`
 * в зашифрованном хранилище (см. PersistentCookieJar), как httpOnly-cookie на сайте.
 */
class SessionManager {
    @Volatile
    var accessToken: String? = null
        private set

    private val _state = MutableStateFlow<AuthState>(AuthState.Unknown)
    val state: StateFlow<AuthState> = _state.asStateFlow()

    /** Сессия потеряна не по воле пользователя (отозвана, истекла) — показать экран входа. */
    private val _lostEvents = MutableStateFlow(0)
    val lostEvents: StateFlow<Int> = _lostEvents.asStateFlow()

    fun set(resp: SessionResp) {
        accessToken = resp.accessToken
        _state.value = AuthState.LoggedIn(resp.user.id, resp.user.role)
    }

    fun clear() {
        accessToken = null
        _state.value = AuthState.LoggedOut
    }

    fun lost() {
        val was = _state.value is AuthState.LoggedIn
        clear()
        if (was) _lostEvents.value += 1
    }
}
