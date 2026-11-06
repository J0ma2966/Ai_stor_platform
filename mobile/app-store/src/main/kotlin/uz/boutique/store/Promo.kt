package uz.boutique.store

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import uz.boutique.ui.Brand
import uz.boutique.ui.ErrorBanner
import uz.boutique.ui.LocalGraph
import uz.boutique.ui.rememberAction
import uz.boutique.ui.R as UiR

/** Сроки показа на выбор — как на сайте (web/src/lib/promotions.ts DAY_PRESETS). */
val PROMO_DAYS = listOf(3, 7, 14, 30)
const val PROMO_NOTE_MAX = 500

/** Заявка на рекламу опубликованного товара. Решение принимает суперадмин; о нём придёт уведомление. */
@Composable
fun PromotionRequestDialog(productId: String, onDismiss: () -> Unit) {
    val api = LocalGraph.current.api
    val context = LocalContext.current
    val send = rememberAction()
    var days by rememberSaveable { mutableIntStateOf(7) }
    var note by rememberSaveable { mutableStateOf("") }
    val sent = stringResource(UiR.string.promo_sent)
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(UiR.string.promo_request)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                Text(stringResource(UiR.string.promo_request_hint), style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted)
                Text(stringResource(UiR.string.promo_days), style = MaterialTheme.typography.labelLarge)
                FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    PROMO_DAYS.forEach { d ->
                        FilterChip(
                            selected = days == d,
                            onClick = { days = d },
                            label = { Text(stringResource(UiR.string.promo_days_n, d)) },
                            modifier = Modifier.testTag("promo_days_$d"),
                        )
                    }
                }
                OutlinedTextField(
                    value = note,
                    onValueChange = { note = it.take(PROMO_NOTE_MAX) },
                    label = { Text(stringResource(UiR.string.promo_note)) },
                    minLines = 2,
                    modifier = Modifier.fillMaxWidth().testTag("promo_note"),
                )
                ErrorBanner(send.error)
            }
        },
        confirmButton = {
            TextButton(
                enabled = !send.busy,
                modifier = Modifier.testTag("promo_send"),
                onClick = {
                    send.run {
                        api.store.requestPromotion(productId, days, note.trim())
                        android.widget.Toast.makeText(context, sent, android.widget.Toast.LENGTH_LONG).show()
                        onDismiss()
                    }
                },
            ) { Text(stringResource(UiR.string.promo_send)) }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(UiR.string.app_cancel)) } },
    )
}
