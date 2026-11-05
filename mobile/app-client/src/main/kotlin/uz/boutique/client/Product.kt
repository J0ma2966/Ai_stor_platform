package uz.boutique.client

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.pager.HorizontalPager
import androidx.compose.foundation.pager.rememberPagerState
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import uz.boutique.core.api.AuthState
import uz.boutique.core.model.Product
import uz.boutique.core.model.Review
import uz.boutique.core.util.Dates
import uz.boutique.ui.AppScaffold
import uz.boutique.ui.Brand
import uz.boutique.ui.ConfirmDialog
import uz.boutique.ui.ErrorBanner
import uz.boutique.ui.FormField
import uz.boutique.ui.InfoBanner
import uz.boutique.ui.LoadContent
import uz.boutique.ui.LocalGraph
import uz.boutique.ui.PagedFooter
import uz.boutique.ui.Pill
import uz.boutique.ui.PrimaryButton
import uz.boutique.ui.ProductImage
import uz.boutique.ui.RatingRow
import uz.boutique.ui.SecondaryButton
import uz.boutique.ui.SectionCard
import uz.boutique.ui.SectionTitle
import uz.boutique.ui.StarsInput
import uz.boutique.ui.money
import uz.boutique.ui.rememberAction
import uz.boutique.ui.rememberLoader
import uz.boutique.ui.rememberPaged
import uz.boutique.ui.R as UiR

@Composable
fun ProductScreen(
    id: String,
    onBack: () -> Unit,
    onNeedLogin: () -> Unit,
    onCart: () -> Unit,
    onStore: (slug: String, name: String) -> Unit,
) {
    val api = LocalGraph.current.api
    val loader = rememberLoader(id) { api.catalog.product(id) }
    LaunchedEffect(id) { api.catalog.markViewed(id) }
    AppScaffold(
        title = loader.value?.brand?.ifBlank { null } ?: "",
        onBack = onBack,
        actions = { FavoriteButton(id, onNeedLogin) },
    ) { pad ->
        LoadContent(loader, Modifier.padding(pad)) { p ->
            ProductContent(p, Modifier.padding(pad), onNeedLogin, onCart, onStore)
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
fun ProductContent(
    p: Product,
    modifier: Modifier,
    onNeedLogin: () -> Unit,
    onCart: () -> Unit,
    onStore: (slug: String, name: String) -> Unit,
) {
    val api = LocalGraph.current.api
    val shop = LocalShop.current
    val auth by api.session.state.collectAsStateWithLifecycle()
    var size by rememberSaveable(p.id) { mutableStateOf<String?>(null) }
    var added by rememberSaveable(p.id) { mutableStateOf(false) }
    var sizeMissing by rememberSaveable(p.id) { mutableStateOf(false) }
    val add = rememberAction()
    val reviews = rememberPaged(p.id) { c -> api.catalog.reviews(p.id, c) }

    LazyColumn(modifier.fillMaxSize(), contentPadding = androidx.compose.foundation.layout.PaddingValues(bottom = 24.dp)) {
        item { Gallery(p) }
        item {
            Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
                Text(p.title, style = MaterialTheme.typography.headlineSmall)
                Text(money(p.price), style = MaterialTheme.typography.headlineMedium, modifier = Modifier.testTag("price"))
                RatingRow(p.ratingAvg, p.ratingCount)
                if (p.storeName.isNotBlank()) {
                    Text(
                        stringResource(UiR.string.product_seller, p.storeName),
                        color = MaterialTheme.colorScheme.primary,
                        fontWeight = FontWeight.Bold,
                        modifier = Modifier.clickable(enabled = !p.storeSlug.isNullOrBlank()) { onStore(p.storeSlug!!, p.storeName) },
                    )
                }
                if (p.withSizes) {
                    Text(stringResource(UiR.string.product_size), style = MaterialTheme.typography.titleSmall)
                    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        p.sizes.forEach { s ->
                            FilterChip(
                                selected = size == s.size,
                                enabled = s.inStock,
                                onClick = { size = s.size; sizeMissing = false; added = false },
                                label = {
                                    Text(
                                        when {
                                            !s.inStock -> s.size + " · " + stringResource(UiR.string.product_size_out)
                                            s.low -> s.size + " · " + stringResource(UiR.string.product_size_low)
                                            else -> s.size
                                        },
                                    )
                                },
                                modifier = Modifier.testTag("size_${s.size}"),
                            )
                        }
                    }
                    if (sizeMissing) Text(stringResource(UiR.string.product_choose_size), color = Brand.colors.bad)
                }
                if (!p.inStock) {
                    Pill(stringResource(UiR.string.product_out_of_stock), Brand.colors.bad)
                } else if (added) {
                    SecondaryButton(stringResource(UiR.string.product_in_cart), onClick = onCart, modifier = Modifier.testTag("go_cart"))
                } else {
                    PrimaryButton(stringResource(UiR.string.product_to_cart), busy = add.busy, modifier = Modifier.testTag("add_to_cart"), onClick = {
                        when {
                            auth !is AuthState.LoggedIn -> onNeedLogin()
                            p.withSizes && size == null -> sizeMissing = true
                            else -> add.run {
                                // Добавляем ещё одну штуку к тому, что уже лежит в корзине.
                                val cart = api.shop.cart()
                                val have = cart.items.firstOrNull { it.productId == p.id && (it.size ?: "") == (size ?: "") }?.qty ?: 0
                                val updated = api.shop.setQty(p.id, have + 1, size)
                                shop.cartCount = updated.totalQty
                                added = true
                            }
                        }
                    })
                }
                ErrorBanner(add.error)
                Text(stringResource(UiR.string.product_perks), style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted)
                if (!p.description.isNullOrBlank()) {
                    SectionTitle(stringResource(UiR.string.product_description))
                    Text(p.description!!)
                }
                HorizontalDivider(Modifier.padding(vertical = 8.dp))
                SectionTitle(stringResource(UiR.string.reviews_title) + if (p.ratingCount > 0) " · ${p.ratingCount}" else "")
                if (auth is AuthState.LoggedIn) MyReviewBlock(p.id, onSaved = { reviews.refresh() })
            }
        }
        if (reviews.loadedOnce && reviews.items.isEmpty()) {
            item { Text(stringResource(UiR.string.reviews_none), color = Brand.colors.muted, modifier = Modifier.padding(horizontal = 16.dp)) }
        }
        items(reviews.items, key = { it.id }) { r -> ReviewItem(r, Modifier.padding(horizontal = 16.dp, vertical = 6.dp)) }
        item { PagedFooter(reviews) }
    }
}

@OptIn(ExperimentalFoundationApi::class)
@Composable
private fun Gallery(p: Product) {
    val photos = p.gallery
    if (photos.isEmpty()) {
        ProductImage(null, null, Modifier.fillMaxWidth().aspectRatio(3f / 4f))
        return
    }
    val pager = rememberPagerState(pageCount = { photos.size })
    Box {
        HorizontalPager(state = pager, modifier = Modifier.fillMaxWidth().aspectRatio(3f / 4f)) { i ->
            ProductImage(photos[i], contentDescription = p.title, modifier = Modifier.fillMaxSize())
        }
        if (photos.size > 1) {
            Pill(
                "${pager.currentPage + 1} / ${photos.size}",
                MaterialTheme.colorScheme.onSurface,
                Modifier.align(Alignment.BottomCenter).padding(12.dp),
            )
        }
    }
}

@Composable
fun ReviewItem(r: Review, modifier: Modifier = Modifier) {
    SectionCard(modifier) {
        Text("★".repeat(r.rating.coerceIn(0, 5)) + "☆".repeat((5 - r.rating).coerceIn(0, 5)), color = Brand.colors.star)
        Text(
            (r.authorName.ifBlank { stringResource(UiR.string.reviews_anonymous) }) + " · " + Dates.date(r.createdAt),
            style = MaterialTheme.typography.labelMedium,
            color = Brand.colors.muted,
        )
        if (r.body.isNotBlank()) Text(r.body)
        if (!r.reply.isNullOrBlank()) {
            Column(Modifier.fillMaxWidth().padding(start = 12.dp)) {
                Text(stringResource(UiR.string.reviews_store_reply), style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.primary)
                Text(r.reply!!)
            }
        }
    }
}

/** Свой отзыв: только после доставки заказа с этим товаром. */
@Composable
private fun MyReviewBlock(productId: String, onSaved: () -> Unit) {
    val api = LocalGraph.current.api
    val mine = rememberLoader(productId, "my-review") { api.reviews.mine(productId) }
    val my = mine.value ?: return
    var editing by rememberSaveable(productId) { mutableStateOf(false) }
    var rating by rememberSaveable(productId) { mutableStateOf(0) }
    var body by rememberSaveable(productId) { mutableStateOf("") }
    var confirmDelete by rememberSaveable { mutableStateOf(false) }
    var ratingMissing by rememberSaveable { mutableStateOf(false) }
    val save = rememberAction()
    val existing = my.review

    when {
        editing -> SectionCard {
            Text(stringResource(UiR.string.reviews_form_title), style = MaterialTheme.typography.titleSmall)
            StarsInput(rating, { rating = it; ratingMissing = false })
            if (ratingMissing) Text(stringResource(UiR.string.reviews_rating_required), color = Brand.colors.bad)
            FormField(body, { body = it }, stringResource(UiR.string.reviews_body_label), singleLine = false, minLines = 3, maxLength = 2000)
            ErrorBanner(save.error)
            PrimaryButton(stringResource(UiR.string.reviews_submit), busy = save.busy, onClick = {
                if (rating !in 1..5) ratingMissing = true else save.run {
                    api.reviews.save(productId, rating, body.trim())
                    editing = false
                    mine.reload()
                    onSaved()
                }
            })
            TextButton(onClick = { editing = false }) { Text(stringResource(UiR.string.app_cancel)) }
        }
        existing != null -> SectionCard {
            Text(stringResource(UiR.string.reviews_your), style = MaterialTheme.typography.titleSmall)
            Text("★".repeat(existing.rating.coerceIn(0, 5)), color = Brand.colors.star)
            if (existing.body.isNotBlank()) Text(existing.body)
            if (existing.status == "hidden") InfoBanner(stringResource(UiR.string.reviews_hidden), color = Brand.colors.warn)
            if (existing.status != "hidden") {
                TextButton(onClick = { rating = existing.rating; body = existing.body; editing = true }) { Text(stringResource(UiR.string.reviews_edit)) }
            }
            TextButton(onClick = { confirmDelete = true }) { Text(stringResource(UiR.string.reviews_delete), color = Brand.colors.bad) }
            ErrorBanner(save.error)
        }
        my.canReview -> SecondaryButton(stringResource(UiR.string.reviews_rate_item), onClick = { rating = 0; body = ""; editing = true })
        else -> Text(stringResource(UiR.string.reviews_only_buyers), style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted)
    }
    if (confirmDelete) {
        ConfirmDialog(
            text = stringResource(UiR.string.reviews_confirm_delete),
            confirmLabel = stringResource(UiR.string.app_delete),
            onConfirm = {
                confirmDelete = false
                save.run {
                    api.reviews.delete(productId)
                    mine.reload()
                    onSaved()
                }
            },
            onDismiss = { confirmDelete = false },
        )
    }
}
