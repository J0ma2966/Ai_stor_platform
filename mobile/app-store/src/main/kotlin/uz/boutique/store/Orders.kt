package uz.boutique.store

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.widget.Toast
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.PrimaryTabRow
import androidx.compose.material3.Tab
import androidx.compose.material3.Text
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.delay
import uz.boutique.core.api.ApiException
import uz.boutique.core.model.OrderDetail
import uz.boutique.core.model.OrderStatus
import uz.boutique.core.model.OrderSummary
import uz.boutique.core.util.Dates
import uz.boutique.core.util.Orders
import uz.boutique.core.util.Phone
import uz.boutique.ui.AppScaffold
import uz.boutique.ui.Brand
import uz.boutique.ui.ConfirmDialog
import uz.boutique.ui.ErrorBanner
import uz.boutique.ui.HistoryCard
import uz.boutique.ui.InfoBanner
import uz.boutique.ui.KeyValue
import uz.boutique.ui.LoadContent
import uz.boutique.ui.LocalGraph
import uz.boutique.ui.OrderItemsCard
import uz.boutique.ui.OrderSummaryCard
import uz.boutique.ui.PagedFooter
import uz.boutique.ui.PagedState
import uz.boutique.ui.PrimaryButton
import uz.boutique.ui.ProductImage
import uz.boutique.ui.ReasonDialog
import uz.boutique.ui.ScreenPadding
import uz.boutique.ui.SecondaryButton
import uz.boutique.ui.SectionCard
import uz.boutique.ui.SectionTitle
import uz.boutique.ui.StatusChip
import uz.boutique.ui.actionLabel
import uz.boutique.ui.addressLine
import uz.boutique.ui.rememberAction
import uz.boutique.ui.rememberLoader
import uz.boutique.ui.rememberPaged
import uz.boutique.ui.R as UiR

private val TAB_LABELS = mapOf(
    Orders.StoreTab.NEW to UiR.string.store_tab_new,
    Orders.StoreTab.ACTIVE to UiR.string.store_tab_active,
    Orders.StoreTab.DONE to UiR.string.store_tab_done,
)

/** Окно заказов магазина: новые, в работе, завершённые. Новые обновляются сами раз в 30 с. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun StoreOrdersScreen(onOrder: (String) -> Unit) {
    val api = LocalGraph.current.api
    var tab by rememberSaveable { mutableStateOf(Orders.StoreTab.NEW) }
    val paged = rememberPaged<OrderSummary>(tab) { c -> api.orders.list(tab.statuses, c) }
    LaunchedEffect(tab) {
        while (true) {
            delay(30_000)
            if (!paged.loading) paged.refresh()
        }
    }
    AppScaffold(title = stringResource(UiR.string.nav_store_orders)) { pad ->
        Column(Modifier.padding(pad).fillMaxSize()) {
            PrimaryTabRow(selectedTabIndex = tab.ordinal, containerColor = MaterialTheme.colorScheme.background) {
                Orders.StoreTab.entries.forEach { t ->
                    Tab(
                        selected = tab == t,
                        onClick = { tab = t },
                        text = { Text(stringResource(TAB_LABELS.getValue(t))) },
                        modifier = Modifier.testTag("orders_tab_${t.name.lowercase()}"),
                    )
                }
            }
            PullToRefreshBox(isRefreshing = paged.loading && paged.loadedOnce, onRefresh = { paged.refresh() }, modifier = Modifier.fillMaxSize()) {
                PagedState(paged, stringResource(UiR.string.store_orders_empty)) {
                    LazyColumn(contentPadding = ScreenPadding, verticalArrangement = Arrangement.spacedBy(10.dp)) {
                        items(paged.items, key = { it.id }) { o -> OrderSummaryCard(o, showStore = false, onClick = { onOrder(o.id) }) }
                        item { PagedFooter(paged) }
                    }
                }
            }
        }
    }
}

@Composable
fun StoreOrderScreen(id: String, onBack: () -> Unit) {
    val api = LocalGraph.current.api
    val context = LocalContext.current
    val loader = rememberLoader(id) { api.orders.get(id) }
    val action = rememberAction()
    val copiedText = stringResource(UiR.string.window_copied)
    val title = loader.value?.let { stringResource(UiR.string.order_number, it.number.toString()) } ?: ""
    AppScaffold(title = title, onBack = onBack) { pad ->
        LoadContent(loader, Modifier.padding(pad)) { o ->
            StoreOrderContent(
                o = o,
                busy = action.busy,
                error = action.error,
                modifier = Modifier.padding(pad),
                onStatus = { to, reason ->
                    action.run {
                        try {
                            loader.set(api.orders.setStatus(o.id, to, o.status, reason))
                        } catch (e: ApiException) {
                            // статус уже поменяли (другой сотрудник, клиент отменил) — показываем свежие данные
                            if (e.status == 409) loader.reload()
                            throw e
                        }
                    }
                },
                onCopy = { text ->
                    val cm = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
                    cm.setPrimaryClip(ClipData.newPlainText("order", text))
                    Toast.makeText(context, copiedText, Toast.LENGTH_SHORT).show()
                },
                onOpen = { uri -> runCatching { context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(uri))) } },
                onCall = { phone -> runCatching { context.startActivity(Intent(Intent.ACTION_DIAL, Uri.parse("tel:$phone"))) } },
            )
        }
    }
}

/** Карточка заказа для магазина (без сети — удобно тестировать). */
@Composable
fun StoreOrderContent(
    o: OrderDetail,
    busy: Boolean,
    error: Throwable?,
    modifier: Modifier = Modifier,
    onStatus: (OrderStatus, String) -> Unit,
    onCopy: (String) -> Unit,
    onOpen: (String) -> Unit,
    onCall: (String) -> Unit,
) {
    var pending by rememberSaveable { mutableStateOf<OrderStatus?>(null) }
    Column(
        modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        SectionCard {
            androidx.compose.foundation.layout.Row {
                Text(Dates.dateTime(o.createdAt), color = Brand.colors.muted, modifier = Modifier.weight(1f))
                StatusChip(o.status)
            }
            if (!o.rejectionReason.isNullOrBlank()) InfoBanner(stringResource(UiR.string.order_reason, o.rejectionReason!!), color = Brand.colors.bad)
            if (o.nextStatuses.isEmpty() || o.status.isTerminal) {
                Text(stringResource(UiR.string.window_finished), color = Brand.colors.muted)
            } else {
                Orders.sortActions(o.nextStatuses).forEach { to ->
                    if (Orders.reasonRequired(to)) {
                        SecondaryButton(actionLabel(to), color = Brand.colors.bad, enabled = !busy, onClick = { pending = to }, modifier = Modifier.testTag("action_${to.wire}"))
                    } else {
                        PrimaryButton(actionLabel(to), busy = busy, onClick = { pending = to }, modifier = Modifier.testTag("action_${to.wire}"))
                    }
                }
            }
            ErrorBanner(error)
        }
        SectionCard {
            SectionTitle(stringResource(UiR.string.window_recipient))
            Text(o.customerName.orEmpty(), style = MaterialTheme.typography.titleMedium)
            if (!o.customerPhone.isNullOrBlank()) {
                Text(
                    Phone.display(o.customerPhone) + " · " + stringResource(UiR.string.window_call),
                    color = MaterialTheme.colorScheme.primary,
                    fontWeight = FontWeight.Bold,
                    modifier = Modifier.clickable { onCall(o.customerPhone!!) }.testTag("call"),
                )
            }
        }
        SectionCard {
            SectionTitle(stringResource(UiR.string.order_address))
            Text(addressLine(o.delivery), style = MaterialTheme.typography.bodyLarge)
            KeyValue(stringResource(UiR.string.checkout_address_comment), o.delivery.addressComment)
            if (!o.delivery.mapsUrl.isNullOrBlank()) {
                SecondaryButton(stringResource(UiR.string.window_open_map), onClick = { onOpen(o.delivery.mapsUrl!!) })
            } else {
                Text(stringResource(UiR.string.window_no_location), style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted)
            }
            if (!o.delivery.photoUrl.isNullOrBlank()) {
                Text(stringResource(UiR.string.window_photo), style = MaterialTheme.typography.labelMedium, color = Brand.colors.muted)
                ProductImage(o.delivery.photoUrl, stringResource(UiR.string.window_photo), Modifier.fillMaxWidth().height(200.dp).clickable { onOpen(o.delivery.photoUrl!!) })
            }
            KeyValue(stringResource(UiR.string.order_comment), o.customerComment)
            KeyValue(stringResource(UiR.string.order_time_comment), o.deliveryTimeComment)
            if (!o.copyText.isNullOrBlank()) {
                SecondaryButton(stringResource(UiR.string.window_copy), onClick = { onCopy(o.copyText!!) }, modifier = Modifier.testTag("copy"))
            }
        }
        OrderItemsCard(o)
        HistoryCard(o)
    }
    val to = pending
    if (to != null) {
        if (Orders.reasonRequired(to)) {
            ReasonDialog(
                title = actionLabel(to),
                label = stringResource(UiR.string.window_reason_label),
                placeholder = stringResource(UiR.string.window_reason_ph),
                confirmLabel = actionLabel(to),
                required = true,
                onConfirm = { reason -> pending = null; onStatus(to, reason) },
                onDismiss = { pending = null },
            )
        } else {
            ConfirmDialog(
                text = stringResource(UiR.string.window_confirm_action, actionLabel(to)),
                confirmLabel = actionLabel(to),
                onConfirm = { pending = null; onStatus(to, "") },
                onDismiss = { pending = null },
            )
        }
    }
}
