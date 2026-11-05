package uz.boutique.ui

import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.Stable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.launch
import uz.boutique.core.model.Page

sealed interface Load<out T> {
    data object Loading : Load<Nothing>
    data class Ok<T>(val value: T) : Load<T>
    data class Failed(val error: Throwable) : Load<Nothing>
}

/** Загрузка данных экрана: первая загрузка, повтор после ошибки, тихое обновление. */
@Stable
class Loader<T>(private val scope: CoroutineScope, private val fetch: suspend () -> T) {
    var state by mutableStateOf<Load<T>>(Load.Loading)
        private set
    var refreshing by mutableStateOf(false)
        private set
    /** Ошибка тихого обновления: данные на экране остались, показать сообщение. */
    var refreshError by mutableStateOf<Throwable?>(null)

    private var job: Job? = null

    val value: T? get() = (state as? Load.Ok<T>)?.value

    fun reload() {
        job?.cancel()
        job = scope.launch {
            val hadData = state is Load.Ok
            if (!hadData) state = Load.Loading
            refreshing = hadData
            try {
                state = Load.Ok(fetch())
                refreshError = null
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                if (hadData) refreshError = e else state = Load.Failed(e)
            } finally {
                refreshing = false
            }
        }
    }

    /** Подменить данные ответом сервера (например, после смены статуса). */
    fun set(value: T) {
        job?.cancel()
        state = Load.Ok(value)
    }
}

@Composable
fun <T> rememberLoader(vararg keys: Any?, fetch: suspend () -> T): Loader<T> {
    val scope = rememberCoroutineScope()
    val latest by rememberUpdatedState(fetch)
    val loader = remember(*keys) { Loader(scope) { latest() } }
    LaunchedEffect(loader) { loader.reload() }
    return loader
}

/** Список с курсорной пагинацией: refresh() — сначала, loadMore() — следующая страница. */
@Stable
class Paged<T>(private val scope: CoroutineScope, private val fetch: suspend (cursor: String?) -> Page<T>) {
    val items = mutableStateListOf<T>()
    var loading by mutableStateOf(false)
        private set
    var loadedOnce by mutableStateOf(false)
        private set
    var end by mutableStateOf(false)
        private set
    var error by mutableStateOf<Throwable?>(null)
        private set
    private var cursor: String? = null
    private var job: Job? = null

    fun refresh() {
        job?.cancel()
        job = scope.launch { load(reset = true) }
    }

    fun loadMore() {
        if (loading || end || !loadedOnce) return
        job = scope.launch { load(reset = false) }
    }

    fun update(transform: (MutableList<T>) -> Unit) = transform(items)

    private suspend fun load(reset: Boolean) {
        loading = true
        try {
            val page = fetch(if (reset) null else cursor)
            if (reset) items.clear()
            items.addAll(page.items)
            cursor = page.nextCursor.ifEmpty { null }
            end = cursor == null
            error = null
            loadedOnce = true
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            error = e
        } finally {
            loading = false
        }
    }
}

@Composable
fun <T> rememberPaged(vararg keys: Any?, fetch: suspend (cursor: String?) -> Page<T>): Paged<T> {
    val scope = rememberCoroutineScope()
    val latest by rememberUpdatedState(fetch)
    val paged = remember(*keys) { Paged(scope) { c -> latest(c) } }
    LaunchedEffect(paged) { paged.refresh() }
    return paged
}

/** Выполнить действие с индикатором и ошибкой: busy/err для кнопок и форм. */
@Stable
class Action(private val scope: CoroutineScope) {
    var busy by mutableStateOf(false)
        private set
    var error by mutableStateOf<Throwable?>(null)

    fun run(block: suspend () -> Unit) {
        if (busy) return
        busy = true
        error = null
        scope.launch {
            try {
                block()
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                error = e
            } finally {
                busy = false
            }
        }
    }
}

@Composable
fun rememberAction(): Action {
    val scope = rememberCoroutineScope()
    return remember { Action(scope) }
}
