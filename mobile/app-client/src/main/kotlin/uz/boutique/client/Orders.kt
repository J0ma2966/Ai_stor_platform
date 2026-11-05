package uz.boutique.client

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import uz.boutique.core.model.OrderDetail
import uz.boutique.core.model.OrderStatus
import uz.boutique.core.model.OrderSummary
import uz.boutique.core.util.Dates
import uz.boutique.core.util.Orders
import uz.boutique.ui.AppScaffold
import uz.boutique.ui.Brand
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
import uz.boutique.ui.ReasonDialog
import uz.boutique.ui.ScreenPadding
import uz.boutique.ui.SecondaryButton
import uz.boutique.ui.SectionCard
import uz.boutique.ui.SectionTitle
import uz.boutique.ui.StatusChip
import uz.boutique.ui.addressLine
import uz.boutique.ui.rememberAction
import uz.boutique.ui.rememberLoader
import uz.boutique.ui.rememberPaged
import uz.boutique.ui.statusLabel
import uz.boutique.ui.R as UiR

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun OrdersScreen(onOrder: (String) -> Unit) {
    val api = LocalGraph.current.api
    val paged = rememberPaged<OrderSummary>("client-orders") { c -> api.orders.list(cursor = c) }
    AppScaffold(title = stringResource(UiR.string.orders_title)) { pad ->
        PullToRefreshBox(isRefreshing = paged.loading && paged.loadedOnce, onRefresh = { paged.refresh() }, modifier = Modifier.padding(pad).fillMaxSize()) {
            PagedState(paged, stringResource(UiR.string.order_empty)) {
                LazyColumn(contentPadding = ScreenPadding, verticalArrangement = Arrangement.spacedBy(10.dp)) {
                    items(paged.items, key = { it.id }) { o -> OrderSummaryCard(o, showStore = true, onClick = { onOrder(o.id) }) }
                    item { PagedFooter(paged) }
                }
            }
        }
    }
}

@Composable
fun OrderDetailScreen(id: String, onBack: () -> Unit, onProduct: (String) -> Unit) {
    val api = LocalGraph.current.api
    val loader = rememberLoader(id) { api.orders.get(id) }
    val cancel = rememberAction()
    var askCancel by rememberSaveable { mutableStateOf(false) }
    val title = loader.value?.let { stringResource(UiR.string.order_number, it.number.toString()) } ?: ""
    AppScaffold(title = title, onBack = onBack) { pad ->
        LoadContent(loader, Modifier.padding(pad)) { o ->
            Column(
                Modifier.padding(pad).fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                SectionCard {
                    Row {
                        Text(o.storeName, style = MaterialTheme.typography.titleMedium, modifier = Modifier.weight(1f))
                        StatusChip(o.status)
                    }
                    Text(stringResource(UiR.string.order_created_at, Dates.dateTime(o.createdAt)), style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted)
                    OrderProgress(o)
                    when {
                        o.status == OrderStatus.PENDING -> Text(stringResource(UiR.string.order_pending_hint), color = Brand.colors.muted)
                        !o.rejectionReason.isNullOrBlank() -> InfoBanner(stringResource(UiR.string.order_reason, o.rejectionReason!!), color = Brand.colors.bad)
                    }
                }
                OrderItemsCard(o, onProduct)
                SectionCard {
                    SectionTitle(stringResource(UiR.string.order_address))
                    Text(addressLine(o.delivery))
                    KeyValue(stringResource(UiR.string.checkout_address_comment), o.delivery.addressComment)
                    KeyValue(stringResource(UiR.string.order_comment), o.customerComment)
                    KeyValue(stringResource(UiR.string.order_time_comment), o.deliveryTimeComment)
                }
                HistoryCard(o)
                if (o.canCancel) {
                    SecondaryButton(stringResource(UiR.string.order_cancel), color = Brand.colors.bad, enabled = !cancel.busy, onClick = { askCancel = true }, modifier = Modifier.testTag("cancel_order"))
                    Text(stringResource(UiR.string.order_cancel_hint), style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted)
                }
                ErrorBanner(cancel.error)
            }
            if (askCancel) {
                ReasonDialog(
                    title = stringResource(UiR.string.order_cancel_confirm, o.number.toString()),
                    label = stringResource(UiR.string.order_cancel_reason),
                    confirmLabel = stringResource(UiR.string.order_cancel_yes),
                    dismissLabel = stringResource(UiR.string.order_cancel_no),
                    required = false,
                    onConfirm = { reason ->
                        askCancel = false
                        cancel.run { loader.set(api.orders.cancel(o.id, reason)) }
                    },
                    onDismiss = { askCancel = false },
                )
            }
        }
    }
}

/** Шаги «подтверждён → собирается → в доставке → доставлен» (как на сайте). */
@Composable
fun OrderProgress(o: OrderDetail) {
    if (o.status == OrderStatus.REJECTED || o.status == OrderStatus.CANCELLED) return
    val step = Orders.customerStep(o.status)
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
            Orders.CUSTOMER_STEPS.forEachIndexed { i, _ ->
                Box(
                    Modifier.weight(1f).height(6.dp).clip(RoundedCornerShape(3.dp))
                        .background(if (i < step) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.outline),
                )
            }
        }
        Row(Modifier.fillMaxWidth()) {
            Orders.CUSTOMER_STEPS.forEach { s ->
                Text(statusLabel(s), style = MaterialTheme.typography.labelSmall, color = Brand.colors.muted, modifier = Modifier.weight(1f))
            }
        }
    }
}
