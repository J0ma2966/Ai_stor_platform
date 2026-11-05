package uz.boutique.store

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import uz.boutique.core.model.Review
import uz.boutique.core.util.Dates
import uz.boutique.ui.AppScaffold
import uz.boutique.ui.Brand
import uz.boutique.ui.ErrorBanner
import uz.boutique.ui.FormField
import uz.boutique.ui.LocalGraph
import uz.boutique.ui.PagedFooter
import uz.boutique.ui.PagedState
import uz.boutique.ui.ProductImage
import uz.boutique.ui.SectionCard
import uz.boutique.ui.rememberAction
import uz.boutique.ui.rememberPaged
import uz.boutique.ui.R as UiR

/** Отзывы о товарах магазина: ответ виден всем покупателям под отзывом. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun StoreReviewsScreen() {
    val api = LocalGraph.current.api
    var unanswered by rememberSaveable { mutableStateOf(true) }
    val paged = rememberPaged<Review>(unanswered) { c -> api.reviews.storeList(unanswered = unanswered, cursor = c) }
    var replyTo by rememberSaveable { mutableStateOf<String?>(null) }
    val action = rememberAction()
    AppScaffold(title = stringResource(UiR.string.nav_store_reviews)) { pad ->
        Column(Modifier.padding(pad).fillMaxSize()) {
            Row(Modifier.padding(horizontal = 16.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                FilterChip(selected = unanswered, onClick = { unanswered = true }, label = { Text(stringResource(UiR.string.store_reviews_unanswered)) })
                FilterChip(selected = !unanswered, onClick = { unanswered = false }, label = { Text(stringResource(UiR.string.store_reviews_all)) })
            }
            ErrorBanner(action.error, Modifier.padding(horizontal = 16.dp))
            PullToRefreshBox(isRefreshing = paged.loading && paged.loadedOnce, onRefresh = { paged.refresh() }, modifier = Modifier.fillMaxSize()) {
                PagedState(paged, stringResource(UiR.string.store_reviews_empty)) {
                    LazyColumn(contentPadding = PaddingValues(16.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
                        items(paged.items, key = { it.id }) { r ->
                            StoreReviewCard(
                                r,
                                onReply = { replyTo = r.id },
                                onDeleteReply = { action.run { api.reviews.deleteReply(r.id); paged.refresh() } },
                            )
                        }
                        item { PagedFooter(paged) }
                    }
                }
            }
        }
    }
    val target = paged.items.firstOrNull { it.id == replyTo }
    if (target != null) {
        ReplyDialog(
            initial = target.reply.orEmpty(),
            onDismiss = { replyTo = null },
            onSave = { text ->
                replyTo = null
                action.run { api.reviews.reply(target.id, text); paged.refresh() }
            },
        )
    }
}

@Composable
private fun StoreReviewCard(r: Review, onReply: () -> Unit, onDeleteReply: () -> Unit) {
    SectionCard(Modifier.testTag("review_${r.id}")) {
        Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            ProductImage(r.imageUrl, r.productTitle, Modifier.size(48.dp))
            Column(Modifier.weight(1f)) {
                Text(r.productTitle.orEmpty(), style = MaterialTheme.typography.titleSmall)
                Text("★".repeat(r.rating.coerceIn(0, 5)) + "☆".repeat((5 - r.rating).coerceIn(0, 5)), color = Brand.colors.star)
            }
        }
        Text(
            r.authorName.ifBlank { stringResource(UiR.string.reviews_anonymous) } + " · " + Dates.date(r.createdAt),
            style = MaterialTheme.typography.labelMedium,
            color = Brand.colors.muted,
        )
        if (r.body.isNotBlank()) Text(r.body)
        if (!r.reply.isNullOrBlank()) {
            Text(stringResource(UiR.string.reviews_store_reply), style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.primary)
            Text(r.reply!!)
            Row {
                TextButton(onClick = onReply) { Text(stringResource(UiR.string.store_reply_edit)) }
                TextButton(onClick = onDeleteReply) { Text(stringResource(UiR.string.store_reply_delete), color = Brand.colors.bad) }
            }
        } else {
            TextButton(onClick = onReply, modifier = Modifier.testTag("reply_${r.id}")) { Text(stringResource(UiR.string.store_reply)) }
        }
    }
}

@Composable
private fun ReplyDialog(initial: String, onDismiss: () -> Unit, onSave: (String) -> Unit) {
    var text by rememberSaveable { mutableStateOf(initial) }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(UiR.string.store_reply_label)) },
        text = { FormField(text, { text = it }, stringResource(UiR.string.store_reply_label), singleLine = false, minLines = 3, maxLength = 2000) },
        confirmButton = { TextButton(onClick = { onSave(text.trim()) }, enabled = text.isNotBlank()) { Text(stringResource(UiR.string.app_save)) } },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(UiR.string.app_cancel)) } },
    )
}
