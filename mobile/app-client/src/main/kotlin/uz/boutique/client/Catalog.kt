package uz.boutique.client

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.GridItemSpan
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.itemsIndexed
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Favorite
import androidx.compose.material.icons.filled.FavoriteBorder
import androidx.compose.material.icons.filled.Checkroom
import androidx.compose.material.icons.filled.FilterList
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.IconButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import uz.boutique.core.api.ApiException
import uz.boutique.core.api.AuthState
import uz.boutique.core.api.CatalogFilters
import uz.boutique.core.model.Product
import uz.boutique.ui.AppScaffold
import uz.boutique.ui.Brand
import uz.boutique.ui.CenterLoading
import uz.boutique.ui.EmptyState
import uz.boutique.ui.ErrorState
import uz.boutique.ui.LocalGraph
import uz.boutique.ui.PagedFooter
import uz.boutique.ui.ProductImage
import uz.boutique.ui.RatingRow
import uz.boutique.ui.currentLang
import uz.boutique.ui.errorRes
import uz.boutique.ui.money
import uz.boutique.ui.rememberLoader
import uz.boutique.ui.rememberPaged
import uz.boutique.ui.R as UiR

private val SORT_LABELS = mapOf(
    "popular" to UiR.string.sort_popular,
    "rating" to UiR.string.sort_rating,
    "new" to UiR.string.sort_new,
    "price_asc" to UiR.string.sort_price_asc,
    "price_desc" to UiR.string.sort_price_desc,
)

/** Каталог: поиск, категории, сортировка, сетка с подгрузкой. [storeSlug] — товары одного бутика. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun CatalogScreen(
    storeSlug: String?,
    storeName: String?,
    onProduct: (String) -> Unit,
    onBack: (() -> Unit)?,
    onNeedLogin: () -> Unit,
    onFitting: () -> Unit = {},
) {
    val api = LocalGraph.current.api
    val lang = currentLang()
    val fittingCount = LocalShop.current.fittingItems.size
    var query by rememberSaveable { mutableStateOf("") }
    var applied by rememberSaveable { mutableStateOf("") }
    var category by rememberSaveable { mutableStateOf<String?>(null) }
    var sort by rememberSaveable { mutableStateOf("popular") }
    var inStock by rememberSaveable { mutableStateOf(false) }
    var sortMenu by remember { mutableStateOf(false) }
    LaunchedEffect(query) {
        delay(400) // не дёргаем API на каждую букву
        applied = query
    }
    val categories = rememberLoader("categories") { api.catalog.categories() }
    val filters = CatalogFilters(q = applied.ifBlank { null }, category = category, store = storeSlug, inStock = inStock, sort = sort)
    val paged = rememberPaged(filters) { c -> api.catalog.products(filters, c) }
    // Карусель «Реклама» — только на главной (весь каталог без поиска и фильтров).
    // Ошибка загрузки рекламы не мешает каталогу: карусель просто не показывается.
    val promos = rememberLoader("promoted") {
        try {
            api.catalog.promoted()
        } catch (e: ApiException) {
            emptyList()
        }
    }
    val showPromos = storeSlug == null && applied.isBlank() && category == null && !inStock

    AppScaffold(
        title = storeName?.takeIf { it.isNotBlank() } ?: stringResource(UiR.string.nav_catalog),
        onBack = onBack,
        actions = {
            if (tryonEnabled) {
                IconButton(onClick = onFitting, modifier = Modifier.testTag("open_fitting")) {
                    if (fittingCount > 0) {
                        androidx.compose.material3.BadgedBox(badge = { androidx.compose.material3.Badge { Text(fittingCount.toString()) } }) {
                            Icon(Icons.Filled.Checkroom, contentDescription = stringResource(UiR.string.tryon_room))
                        }
                    } else {
                        Icon(Icons.Filled.Checkroom, contentDescription = stringResource(UiR.string.tryon_room))
                    }
                }
            }
            Box {
                IconButton(onClick = { sortMenu = true }, modifier = Modifier.testTag("sort")) {
                    Icon(Icons.Filled.FilterList, contentDescription = stringResource(UiR.string.catalog_sort))
                }
                DropdownMenu(expanded = sortMenu, onDismissRequest = { sortMenu = false }) {
                    CatalogFilters.SORTS.forEach { s ->
                        DropdownMenuItem(
                            text = {
                                Text(stringResource(SORT_LABELS.getValue(s)), fontWeight = if (s == sort) FontWeight.ExtraBold else FontWeight.Medium)
                            },
                            onClick = { sort = s; sortMenu = false },
                        )
                    }
                }
            }
        },
    ) { pad ->
        Column(Modifier.padding(pad).fillMaxSize()) {
            OutlinedTextField(
                value = query,
                onValueChange = { query = it.take(100) },
                placeholder = { Text(stringResource(UiR.string.catalog_search)) },
                leadingIcon = { Icon(Icons.Filled.Search, contentDescription = null) },
                trailingIcon = {
                    if (query.isNotEmpty()) IconButton(onClick = { query = ""; applied = "" }) {
                        Icon(Icons.Filled.Close, contentDescription = stringResource(UiR.string.app_close))
                    }
                },
                singleLine = true,
                shape = CircleShape,
                modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp).testTag("search"),
            )
            LazyRow(contentPadding = PaddingValues(horizontal = 16.dp, vertical = 8.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                item {
                    FilterChip(selected = inStock, onClick = { inStock = !inStock }, label = { Text(stringResource(UiR.string.catalog_in_stock)) })
                }
                item {
                    FilterChip(selected = category == null, onClick = { category = null }, label = { Text(stringResource(UiR.string.catalog_all)) })
                }
                items(categories.value.orEmpty(), key = { it.id }) { c ->
                    FilterChip(
                        selected = category == c.slug,
                        onClick = { category = if (category == c.slug) null else c.slug },
                        label = { Text(c.name(lang)) },
                    )
                }
            }
            PullToRefreshBox(
                isRefreshing = paged.loading && paged.loadedOnce,
                onRefresh = { paged.refresh(); if (showPromos) promos.reload() },
                modifier = Modifier.fillMaxSize(),
            ) {
                when {
                    !paged.loadedOnce && paged.error != null -> ErrorState(paged.error!!, onRetry = { paged.refresh() })
                    !paged.loadedOnce -> CenterLoading()
                    paged.items.isEmpty() -> EmptyState(stringResource(UiR.string.catalog_empty))
                    else -> LazyVerticalGrid(
                        columns = GridCells.Adaptive(160.dp),
                        contentPadding = PaddingValues(16.dp),
                        horizontalArrangement = Arrangement.spacedBy(12.dp),
                        verticalArrangement = Arrangement.spacedBy(16.dp),
                        modifier = Modifier.fillMaxSize().testTag("catalog_grid"),
                    ) {
                        val promoItems = promos.value.orEmpty()
                        if (showPromos && promoItems.isNotEmpty()) {
                            item(span = { GridItemSpan(maxLineSpan) }, key = "promo_carousel") {
                                // сетка уже даёт отступ 16 dp по краям — свой отступ карусели не нужен
                                PromoCarousel(promoItems, onProduct = onProduct, edge = 0.dp, modifier = Modifier.padding(bottom = 8.dp))
                            }
                        }
                        itemsIndexed(paged.items, key = { _, p -> p.id }) { i, p ->
                            if (i >= paged.items.size - 6) LaunchedEffect(paged.items.size) { paged.loadMore() }
                            ProductCard(p, onClick = { onProduct(p.id) }, onNeedLogin = onNeedLogin)
                        }
                        item(span = { GridItemSpan(maxLineSpan) }) { PagedFooter(paged) }
                    }
                }
            }
        }
    }
}

@Composable
fun FavoriteButton(productId: String, onNeedLogin: () -> Unit, modifier: Modifier = Modifier) {
    val api = LocalGraph.current.api
    val shop = LocalShop.current
    val scope = rememberCoroutineScope()
    val context = LocalContext.current
    val fav = productId in shop.favIds
    IconButton(
        onClick = {
            if (api.session.state.value !is AuthState.LoggedIn) {
                onNeedLogin()
            } else {
                scope.launch {
                    try {
                        shop.toggleFavorite(productId)
                    } catch (e: ApiException) {
                        val res = errorRes(e.code, uz.boutique.core.model.Role.CLIENT) ?: UiR.string.err_internal
                        android.widget.Toast.makeText(context, context.getString(res), android.widget.Toast.LENGTH_SHORT).show()
                    }
                }
            }
        },
        colors = IconButtonDefaults.iconButtonColors(containerColor = MaterialTheme.colorScheme.surface.copy(alpha = 0.9f)),
        modifier = modifier.testTag("fav_$productId"),
    ) {
        Icon(
            if (fav) Icons.Filled.Favorite else Icons.Filled.FavoriteBorder,
            contentDescription = stringResource(if (fav) UiR.string.product_in_fav else UiR.string.product_to_fav),
            tint = if (fav) Brand.colors.accent2 else MaterialTheme.colorScheme.onSurface,
        )
    }
}

@Composable
fun ProductCard(p: Product, onClick: () -> Unit, onNeedLogin: () -> Unit, modifier: Modifier = Modifier) {
    Column(modifier.clickable(onClick = onClick).testTag("product_${p.id}"), verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Box {
            ProductImage(p.cardImage, contentDescription = p.title, modifier = Modifier.fillMaxWidth().aspectRatio(3f / 4f))
            FavoriteButton(p.id, onNeedLogin, Modifier.align(Alignment.TopEnd).padding(6.dp))
        }
        Text(money(p.price), style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.ExtraBold)
        if (p.brand.isNotBlank()) Text(p.brand, style = MaterialTheme.typography.labelMedium, color = Brand.colors.muted, maxLines = 1)
        Text(p.title, style = MaterialTheme.typography.bodyMedium, maxLines = 2, overflow = TextOverflow.Ellipsis)
        RatingRow(p.ratingAvg, p.ratingCount)
        if (!p.inStock) Text(stringResource(UiR.string.product_out_of_stock), style = MaterialTheme.typography.labelSmall, color = Brand.colors.bad)
    }
}
