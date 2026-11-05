package uz.boutique.ui

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.size
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import uz.boutique.core.model.Delivery
import uz.boutique.core.model.OrderDetail
import uz.boutique.core.model.OrderSummary
import uz.boutique.core.util.Dates

/** Карточка заказа в списке (покупатель и магазин). */
@Composable
fun OrderSummaryCard(o: OrderSummary, showStore: Boolean, onClick: () -> Unit) {
    SectionCard(Modifier.clickable(onClick = onClick).testTag("order_${o.number}")) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(stringResource(R.string.order_number, o.number.toString()), style = MaterialTheme.typography.titleMedium, modifier = Modifier.weight(1f))
            StatusChip(o.status)
        }
        if (showStore && o.storeName.isNotBlank()) Text(o.storeName, color = Brand.colors.muted)
        if (!o.customerName.isNullOrBlank()) Text(o.customerName!!, fontWeight = FontWeight.Bold)
        if (!o.shortAddress.isNullOrBlank()) Text(o.shortAddress!!, style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted)
        Row {
            Text(stringResource(R.string.order_items_count, o.itemsCount), modifier = Modifier.weight(1f), color = Brand.colors.muted)
            Text(money(o.itemsTotal), fontWeight = FontWeight.ExtraBold)
        }
        Text(Dates.dateTime(o.createdAt), style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted)
    }
}

@Composable
fun OrderItemsCard(o: OrderDetail, onProduct: ((String) -> Unit)? = null) {
    SectionCard {
        SectionTitle(stringResource(R.string.order_items))
        o.items.forEach { item ->
            Row(
                Modifier.fillMaxWidth().clickable(enabled = onProduct != null && item.productId != null) { onProduct?.invoke(item.productId!!) },
                horizontalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                ProductImage(item.imageUrl, item.title, Modifier.size(width = 60.dp, height = 80.dp))
                Column(Modifier.weight(1f)) {
                    Text(item.title, style = MaterialTheme.typography.titleSmall)
                    val meta = listOfNotNull(
                        item.brand.ifBlank { null },
                        item.size?.ifBlank { null }?.let { s -> stringResource(R.string.order_size, s) },
                    ).joinToString(" · ")
                    if (meta.isNotEmpty()) Text(meta, style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted)
                    Text(stringResource(R.string.order_qty_price, item.qty, money(item.unitPrice)), style = MaterialTheme.typography.bodySmall)
                }
                Text(money(item.unitPrice * item.qty), fontWeight = FontWeight.Bold)
            }
        }
        Row {
            Text(stringResource(R.string.order_total), modifier = Modifier.weight(1f), fontWeight = FontWeight.ExtraBold)
            Text(money(o.itemsTotal), fontWeight = FontWeight.ExtraBold, modifier = Modifier.testTag("order_total"))
        }
        Text(stringResource(R.string.order_pay_on_delivery), style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted)
    }
}

/** «ул. Навои, 5, подъезд 2, этаж 3, кв. 12, домофон 12К» */
@Composable
fun addressLine(d: Delivery): String {
    val parts = mutableListOf(listOf(d.street, d.house).filter { it.isNotBlank() }.joinToString(", "))
    d.entrance?.ifBlank { null }?.let { parts += stringResource(R.string.window_entrance, it) }
    d.floor?.ifBlank { null }?.let { parts += stringResource(R.string.window_floor, it) }
    d.apartment?.ifBlank { null }?.let { parts += stringResource(R.string.window_apartment, it) }
    d.intercom?.ifBlank { null }?.let { parts += stringResource(R.string.window_intercom, it) }
    return parts.filter { it.isNotBlank() }.joinToString(", ")
}

@Composable
fun HistoryCard(o: OrderDetail) {
    if (o.history.isEmpty()) return
    SectionCard {
        SectionTitle(stringResource(R.string.order_history))
        o.history.forEach { h ->
            Column {
                Text(statusLabel(h.toStatus), fontWeight = FontWeight.Bold)
                Text(Dates.dateTime(h.createdAt), style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted)
                if (!h.reason.isNullOrBlank()) Text(stringResource(R.string.order_reason, h.reason!!), style = MaterialTheme.typography.bodySmall)
            }
        }
    }
}
