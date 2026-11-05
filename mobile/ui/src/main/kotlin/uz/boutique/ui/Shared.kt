package uz.boutique.ui

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonPrimitive
import uz.boutique.core.model.Notification
import uz.boutique.core.model.OrderStatus
import uz.boutique.core.util.Dates
import uz.boutique.core.util.NotificationTarget

/** Текст уведомления по виду и данным (без ПД), как на сайте. */
@Composable
fun notificationText(n: Notification): String {
    fun s(k: String) = runCatching { n.data[k]?.jsonPrimitive?.contentOrNull }.getOrNull() ?: ""
    val number = s("number")
    val title = s("title")
    return when (n.kind) {
        "order_created" -> stringResource(R.string.notif_order_created, number)
        "order_cancelled" -> stringResource(R.string.notif_order_cancelled, number)
        "order_status" -> {
            val st = runCatching { OrderStatus.valueOf(s("status").uppercase()) }.getOrNull()
            stringResource(R.string.notif_order_status, number, st?.let { statusLabel(it) } ?: s("status"))
        }
        "product_moderated" -> {
            val st = when (s("status")) {
                "unpublished" -> stringResource(R.string.mod_unpublished)
                "blocked" -> stringResource(R.string.mod_blocked)
                "draft" -> stringResource(R.string.mod_draft)
                else -> s("status")
            }
            val base = stringResource(R.string.notif_product_moderated, title, st)
            val reason = s("reason")
            if (reason.isNotBlank()) base + ". " + stringResource(R.string.order_reason, reason) else base
        }
        "review_created" -> stringResource(R.string.notif_review_created, title)
        "review_reply" -> stringResource(R.string.notif_review_reply, title)
        "review_hidden" -> stringResource(R.string.notif_review_hidden, title)
        "payment_paid" -> stringResource(R.string.notif_payment_paid)
        "payment_failed" -> stringResource(R.string.notif_payment_failed)
        "generation_succeeded" -> stringResource(R.string.notif_generation_succeeded)
        "generation_failed" -> stringResource(R.string.notif_generation_failed)
        else -> stringResource(R.string.notif_system)
    }
}

/** Экран уведомлений — общий для обоих приложений. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun NotificationsScreen(onBack: (() -> Unit)?, onOpen: (NotificationTarget) -> Unit) {
    val api = LocalGraph.current.api
    val scope = rememberCoroutineScope()
    val paged = rememberPaged<Notification> { c -> api.notifications.list(c) }
    AppScaffold(
        title = stringResource(R.string.app_notifications),
        onBack = onBack,
        actions = {
            if (paged.items.any { !it.read }) {
                TextButton(onClick = {
                    scope.launch {
                        runCatching { api.notifications.markAllRead() }.onSuccess {
                            paged.update { list -> for (i in list.indices) list[i] = list[i].copy(read = true) }
                        }
                    }
                }) { Text(stringResource(R.string.notif_mark_all)) }
            }
        },
    ) { pad ->
        PullToRefreshBox(isRefreshing = paged.loading && paged.loadedOnce, onRefresh = { paged.refresh() }, modifier = Modifier.padding(pad).fillMaxSize()) {
            PagedState(paged, stringResource(R.string.notif_empty)) {
                LazyColumn(contentPadding = ScreenPadding, verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    items(paged.items, key = { it.id }) { n ->
                        SectionCard(Modifier.clickable {
                            if (!n.read) scope.launch {
                                runCatching { api.notifications.markRead(listOf(n.id)) }
                                paged.update { list -> for (i in list.indices) if (list[i].id == n.id) list[i] = list[i].copy(read = true) }
                            }
                            onOpen(NotificationTarget.of(n))
                        }) {
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                Text(
                                    notificationText(n),
                                    modifier = Modifier.weight(1f),
                                    fontWeight = if (n.read) FontWeight.Medium else FontWeight.ExtraBold,
                                )
                                if (!n.read) {
                                    Spacer(Modifier.width(8.dp))
                                    Pill(stringResource(R.string.notif_new), Brand.colors.accent2)
                                }
                            }
                            Text(Dates.dateTime(n.createdAt), style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted)
                        }
                    }
                    item { PagedFooter(paged) }
                }
            }
        }
    }
}

/** Выбор языка интерфейса. [onChanged] — например, сохранить язык в профиле на сервере. */
@Composable
fun LanguagePicker(onChanged: (String) -> Unit = {}) {
    val current = currentLang()
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text(stringResource(R.string.app_lang), style = MaterialTheme.typography.labelMedium, color = Brand.colors.muted)
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            for ((code, label) in listOf("ru" to R.string.lang_ru, "uz" to R.string.lang_uz)) {
                FilterChip(
                    selected = current == code,
                    onClick = {
                        if (current != code) {
                            onChanged(code)
                            AppLang.set(code)
                        }
                    },
                    label = { Text(stringResource(label)) },
                    modifier = Modifier.testTag("lang_$code"),
                )
            }
        }
    }
}

/** Debug-сборка: адрес API (телефон в одной Wi-Fi-сети с компьютером разработчика). */
@Composable
fun ServerDialog(onDismiss: () -> Unit, onSaved: () -> Unit) {
    val graph = LocalGraph.current
    var url by rememberSaveable { mutableStateOf(graph.baseUrl) }
    var error by rememberSaveable { mutableStateOf(false) }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.app_server)) },
        text = {
            Column {
                FormField(
                    url, { url = it; error = false }, stringResource(R.string.app_server),
                    keyboardType = KeyboardType.Uri,
                    error = if (error) stringResource(R.string.app_server_invalid) else null,
                    hint = stringResource(R.string.app_server_hint),
                )
            }
        },
        confirmButton = {
            TextButton(onClick = {
                val u = url.trim()
                if (!(u.startsWith("http://") || u.startsWith("https://"))) {
                    error = true
                } else {
                    graph.setServer(u)
                    onSaved()
                }
            }) { Text(stringResource(R.string.app_save)) }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.app_cancel)) } },
    )
}
