package uz.boutique.client

import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.spring
import androidx.compose.foundation.LocalIndication
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsPressedAsState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowForward
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.Checkroom
import androidx.compose.material.icons.filled.FavoriteBorder
import androidx.compose.material.icons.filled.LocalShipping
import androidx.compose.material.icons.filled.Payments
import androidx.compose.material.icons.filled.Storefront
import androidx.compose.material3.Badge
import androidx.compose.material3.BadgedBox
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LocalContentColor
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.lerp
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import coil.compose.AsyncImage
import kotlinx.coroutines.async
import kotlinx.coroutines.coroutineScope
import uz.boutique.core.api.Api
import uz.boutique.core.api.CatalogFilters
import uz.boutique.core.model.Category
import uz.boutique.core.model.Product
import uz.boutique.core.model.Store
import uz.boutique.ui.AppScaffold
import uz.boutique.ui.Brand
import uz.boutique.ui.EyebrowStyle
import uz.boutique.ui.LoadContent
import uz.boutique.ui.LocalGraph
import uz.boutique.ui.R as UiR
import uz.boutique.ui.Serif
import uz.boutique.ui.ThemeToggleButton
import uz.boutique.ui.currentLang
import uz.boutique.ui.rememberLoader
import uz.boutique.ui.surfaceDepth

private val HeroShape = RoundedCornerShape(22.dp)
private val PanelShape = RoundedCornerShape(16.dp)

/** Всё, что показывает главная (как главная сайта). */
data class HomeData(
    val categories: List<Category> = emptyList(),
    val popular: List<Product> = emptyList(),
    val fresh: List<Product> = emptyList(),
    val stores: List<Store> = emptyList(),
    val promoted: List<Product> = emptyList(),
)

/**
 * Грузит главную параллельно. Без популярных товаров экран не имеет смысла, поэтому их
 * ошибка (например, нет интернета) показывается с кнопкой «Повторить»; остальные блоки
 * при ошибке просто не показываются — как на сайте.
 */
suspend fun loadHome(api: Api): HomeData = coroutineScope {
    val cats = async { runCatching { api.catalog.categories() }.getOrDefault(emptyList()) }
    val popular = async { api.catalog.products(CatalogFilters(sort = "popular"), limit = 10).items }
    val fresh = async { runCatching { api.catalog.products(CatalogFilters(sort = "new"), limit = 10).items }.getOrDefault(emptyList()) }
    val stores = async { runCatching { api.catalog.stores() }.getOrDefault(emptyList()) }
    val promoted = async { runCatching { api.catalog.promoted() }.getOrDefault(emptyList()) }
    HomeData(cats.await(), popular.await(), fresh.await(), stores.await(), promoted.await())
}

/**
 * Главная приложения — те же блоки и кнопки, что на главной сайта:
 * большая карточка с двумя кнопками, «Новинки недели» и «Все бутики города», реклама,
 * категории, популярное, как работает примерка, новинки, бутики, «Почему у нас» и призыв примерить.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HomeScreen(
    onCatalog: (sort: String?, category: String?) -> Unit,
    onProduct: (String) -> Unit,
    onStores: () -> Unit,
    onStore: (Store) -> Unit,
    onFitting: () -> Unit,
    onFavorites: () -> Unit,
    onNeedLogin: () -> Unit,
    tryon: Boolean = tryonEnabled,
) {
    val api = LocalGraph.current.api
    val shop = LocalShop.current
    val lang = currentLang()
    val home = rememberLoader("home") { loadHome(api) }
    AppScaffold(
        title = stringResource(UiR.string.nav_home),
        actions = {
            ThemeToggleButton()
            FavoritesAction(shop.favIds.size, onFavorites)
            if (tryon) FittingAction(shop.fittingItems.size, onFitting)
        },
    ) { pad ->
        LoadContent(home, Modifier.padding(pad)) { d ->
            PullToRefreshBox(isRefreshing = home.refreshing, onRefresh = { home.reload() }, modifier = Modifier.padding(pad).fillMaxSize()) {
                LazyColumn(
                    contentPadding = PaddingValues(start = 16.dp, end = 16.dp, top = 4.dp, bottom = 24.dp),
                    verticalArrangement = Arrangement.spacedBy(24.dp),
                    modifier = Modifier.fillMaxSize().testTag("home"),
                ) {
                    item(key = "hero") {
                        HeroCard(
                            tryon = tryon,
                            image = (d.popular + d.fresh).firstNotNullOfOrNull { it.cardImage },
                            onPrimary = if (tryon) onFitting else ({ onCatalog(null, null) }),
                            onSecondary = if (tryon) ({ onCatalog(null, null) }) else onStores,
                        )
                    }
                    item(key = "promos") {
                        Row(Modifier.fillMaxWidth().height(IntrinsicSize.Min), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                            PromoPanel(
                                eyebrow = stringResource(UiR.string.home_promo_eyebrow),
                                title = stringResource(UiR.string.home_promo_new),
                                bright = true,
                                onClick = { onCatalog("new", null) },
                                modifier = Modifier.weight(1f).fillMaxHeight().testTag("home_promo_new"),
                            )
                            PromoPanel(
                                eyebrow = null,
                                title = stringResource(UiR.string.home_promo_stores),
                                bright = false,
                                onClick = onStores,
                                modifier = Modifier.weight(1f).fillMaxHeight().testTag("home_promo_stores"),
                            )
                        }
                    }
                    if (d.promoted.isNotEmpty()) {
                        item(key = "ads") { PromoCarousel(d.promoted, onProduct = onProduct, edge = 0.dp) }
                    }
                    if (d.categories.isNotEmpty()) {
                        item(key = "cats") {
                            Section(stringResource(UiR.string.home_categories)) {
                                d.categories.chunked(3).forEach { row ->
                                    Row(Modifier.fillMaxWidth().height(IntrinsicSize.Min), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                                        row.forEach { c ->
                                            CategoryTile(c.name(lang), onClick = { onCatalog(null, c.slug) }, modifier = Modifier.weight(1f).fillMaxHeight().testTag("home_cat_${c.slug}"))
                                        }
                                        repeat(3 - row.size) { Spacer(Modifier.weight(1f)) }
                                    }
                                }
                            }
                        }
                    }
                    item(key = "popular") {
                        Section(stringResource(UiR.string.home_popular), onSeeAll = { onCatalog("popular", null) }, seeAllTag = "home_all_popular") {
                            if (d.popular.isEmpty()) {
                                Text(stringResource(UiR.string.home_empty), color = Brand.colors.muted, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.testTag("home_empty"))
                            } else {
                                ProductRow(d.popular, onProduct, onNeedLogin)
                            }
                        }
                    }
                    if (tryon) {
                        item(key = "how") {
                            Section(stringResource(UiR.string.home_how_title)) {
                                listOf(
                                    UiR.string.home_how1 to UiR.string.home_how1t,
                                    UiR.string.home_how2 to UiR.string.home_how2t,
                                    UiR.string.home_how3 to UiR.string.home_how3t,
                                ).forEachIndexed { i, (title, text) -> HowStep(i + 1, stringResource(title), stringResource(text)) }
                            }
                        }
                    }
                    if (d.fresh.isNotEmpty()) {
                        item(key = "fresh") {
                            Section(stringResource(UiR.string.home_new), onSeeAll = { onCatalog("new", null) }, seeAllTag = "home_all_new") {
                                ProductRow(d.fresh, onProduct, onNeedLogin)
                            }
                        }
                    }
                    if (d.stores.isNotEmpty()) {
                        item(key = "stores") {
                            Section(stringResource(UiR.string.home_stores), onSeeAll = onStores, seeAllText = stringResource(UiR.string.home_btn_stores), seeAllTag = "home_all_stores") {
                                LazyRow(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                                    items(d.stores.take(8), key = { it.id }) { s -> StoreTile(s, onClick = { onStore(s) }, modifier = Modifier.width(230.dp)) }
                                }
                            }
                        }
                    }
                    item(key = "adv") {
                        Section(stringResource(UiR.string.home_adv_title)) {
                            val adv = if (tryon) listOf(1, 2, 3, 4) else listOf(1, 3, 5)
                            adv.forEach { i -> Benefit(i) }
                        }
                    }
                    if (tryon) {
                        item(key = "cta") { CtaCard(onFitting) }
                    }
                }
            }
        }
    }
}

/** Сердечко в верхней панели (как на сайте в шапке) — ведёт в избранное. */
@Composable
fun FavoritesAction(count: Int, onClick: () -> Unit) {
    IconButton(onClick = onClick, modifier = Modifier.testTag("open_favorites")) {
        val label = stringResource(UiR.string.nav_favorites)
        if (count > 0) {
            BadgedBox(badge = { Badge { Text(count.coerceAtMost(99).toString()) } }) {
                Icon(Icons.Filled.FavoriteBorder, contentDescription = label)
            }
        } else {
            Icon(Icons.Filled.FavoriteBorder, contentDescription = label)
        }
    }
}

@Composable
fun FittingAction(count: Int, onClick: () -> Unit) {
    IconButton(onClick = onClick, modifier = Modifier.testTag("open_fitting")) {
        val label = stringResource(UiR.string.tryon_room)
        if (count > 0) {
            BadgedBox(badge = { Badge { Text(count.toString()) } }) { Icon(Icons.Filled.Checkroom, contentDescription = label) }
        } else {
            Icon(Icons.Filled.Checkroom, contentDescription = label)
        }
    }
}

/** Пружинящее нажатие: карточка чуть сжимается под пальцем. */
@Composable
private fun Modifier.pressable(shape: Shape, onClick: () -> Unit, depth: Modifier = Modifier): Modifier {
    val source = remember { MutableInteractionSource() }
    val pressed by source.collectIsPressedAsState()
    val scale by animateFloatAsState(
        if (pressed) 0.97f else 1f,
        spring(dampingRatio = Spring.DampingRatioMediumBouncy, stiffness = Spring.StiffnessMedium),
        label = "press",
    )
    return this
        .graphicsLayer { scaleX = scale; scaleY = scale }
        .then(depth) // тень и фон до обрезки по форме, иначе тень срежется
        .clip(shape)
        .clickable(interactionSource = source, indication = LocalIndication.current, role = Role.Button, onClick = onClick)
}

@Composable
private fun Section(
    title: String,
    onSeeAll: (() -> Unit)? = null,
    seeAllText: String? = null,
    seeAllTag: String = "",
    content: @Composable ColumnScope.() -> Unit,
) {
    Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Text(title, fontFamily = Serif, fontSize = 22.sp, lineHeight = 26.sp, modifier = Modifier.weight(1f))
            if (onSeeAll != null) {
                Row(
                    Modifier.clip(CircleShape).clickable(role = Role.Button, onClick = onSeeAll).heightIn(min = 40.dp).padding(horizontal = 8.dp).testTag(seeAllTag),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(4.dp),
                ) {
                    Text(seeAllText ?: stringResource(UiR.string.home_see_all), color = MaterialTheme.colorScheme.primary, style = MaterialTheme.typography.labelLarge)
                    Icon(Icons.AutoMirrored.Filled.ArrowForward, contentDescription = null, tint = MaterialTheme.colorScheme.primary, modifier = Modifier.size(16.dp))
                }
            }
        }
        content()
    }
}

@Composable
private fun HeroCard(tryon: Boolean, image: String?, onPrimary: () -> Unit, onSecondary: () -> Unit) {
    val b = Brand.colors
    Box(
        Modifier
            .fillMaxWidth()
            .heightIn(min = 380.dp)
            .shadow(16.dp, HeroShape, ambientColor = b.shadow.copy(alpha = 0.45f), spotColor = b.shadow.copy(alpha = 0.6f))
            .clip(HeroShape)
            .background(b.hero)
            .testTag("home_hero"),
    ) {
        if (image != null) {
            AsyncImage(model = image, contentDescription = null, contentScale = ContentScale.Crop, alpha = 0.85f, modifier = Modifier.matchParentSize())
        }
        // Затемнение снизу: светлый текст читается на любом фото.
        Box(Modifier.matchParentSize().background(Brush.verticalGradient(0.15f to Color.Transparent, 0.75f to b.hero.copy(alpha = 0.9f), 1f to b.hero)))
        Column(Modifier.align(Alignment.BottomStart).fillMaxWidth().padding(20.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(stringResource(UiR.string.home_hero_tag).uppercase(), style = EyebrowStyle, color = b.heroEyebrow)
            Text(
                stringResource(if (tryon) UiR.string.home_hero_title_tryon else UiR.string.home_hero_title),
                fontFamily = Serif, fontSize = 29.sp, lineHeight = 33.sp, color = b.onHero,
            )
            Text(
                stringResource(if (tryon) UiR.string.home_hero_text_tryon else UiR.string.home_hero_text),
                style = MaterialTheme.typography.bodyMedium, color = b.onHero.copy(alpha = 0.88f),
            )
            Spacer(Modifier.height(6.dp))
            HeroButton(
                stringResource(if (tryon) UiR.string.home_btn_fitting else UiR.string.home_btn_catalog),
                filled = true, onClick = onPrimary, modifier = Modifier.testTag("home_btn_primary"),
            )
            HeroButton(
                stringResource(if (tryon) UiR.string.home_btn_catalog else UiR.string.home_btn_stores),
                filled = false, onClick = onSecondary, modifier = Modifier.testTag("home_btn_secondary"),
            )
        }
    }
}

/** Кнопки на тёмной карточке: залитая «золотая/малиновая» и прозрачная с рамкой. */
@Composable
private fun HeroButton(text: String, filled: Boolean, onClick: () -> Unit, modifier: Modifier = Modifier) {
    val b = Brand.colors
    val depth = if (filled) {
        Modifier
            .shadow(8.dp, CircleShape, ambientColor = Color.Black.copy(alpha = 0.5f), spotColor = Color.Black.copy(alpha = 0.6f))
            .background(Brush.verticalGradient(listOf(lerp(b.heroButton, Color.White, 0.25f), b.heroButton, lerp(b.heroButton, Color.Black, 0.12f))), CircleShape)
            .border(1.dp, Brush.verticalGradient(listOf(Color.White.copy(alpha = 0.55f), Color.Transparent)), CircleShape)
    } else {
        Modifier
            .background(Color.White.copy(alpha = 0.08f), CircleShape)
            .border(1.dp, b.onHero.copy(alpha = 0.55f), CircleShape)
    }
    Row(
        modifier.fillMaxWidth().heightIn(min = 50.dp).pressable(CircleShape, onClick, depth).padding(horizontal = 18.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.CenterHorizontally),
    ) {
        val fg = if (filled) b.onHeroButton else b.onHero
        Text(text, color = fg, style = MaterialTheme.typography.labelLarge, fontWeight = FontWeight.Bold)
        if (filled) Icon(Icons.AutoMirrored.Filled.ArrowForward, contentDescription = null, tint = fg, modifier = Modifier.size(18.dp))
    }
}

@Composable
private fun PromoPanel(eyebrow: String?, title: String, bright: Boolean, onClick: () -> Unit, modifier: Modifier = Modifier) {
    val b = Brand.colors
    val depth = if (bright) {
        Modifier
            .shadow(6.dp, PanelShape, ambientColor = b.shadow.copy(alpha = 0.35f), spotColor = b.shadow.copy(alpha = 0.45f))
            .background(Brush.verticalGradient(listOf(b.accentTop, b.accentBottom)), PanelShape)
    } else {
        Modifier.surfaceDepth(b, PanelShape)
    }
    val fg = if (bright) MaterialTheme.colorScheme.onPrimary else MaterialTheme.colorScheme.onSurface
    Column(
        modifier.heightIn(min = 128.dp).pressable(PanelShape, onClick, depth).padding(14.dp),
        verticalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        if (eyebrow != null) Text(eyebrow.uppercase(), style = EyebrowStyle, color = fg.copy(alpha = 0.85f))
        Text(title, fontFamily = Serif, fontSize = 19.sp, lineHeight = 23.sp, color = fg, modifier = Modifier.weight(1f))
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(stringResource(UiR.string.home_promo_go), color = if (bright) fg else MaterialTheme.colorScheme.primary, style = MaterialTheme.typography.labelLarge)
            Icon(Icons.AutoMirrored.Filled.ArrowForward, contentDescription = null, tint = if (bright) fg else MaterialTheme.colorScheme.primary, modifier = Modifier.size(16.dp))
        }
    }
}

@Composable
private fun CategoryTile(name: String, onClick: () -> Unit, modifier: Modifier = Modifier) {
    Column(
        modifier.heightIn(min = 96.dp).pressable(PanelShape, onClick, Modifier.surfaceDepth(Brand.colors, PanelShape, elevation = 4.dp)).padding(10.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(8.dp, Alignment.CenterVertically),
    ) {
        Box(Modifier.size(40.dp).background(Brand.colors.accentSoft, CircleShape), contentAlignment = Alignment.Center) {
            Icon(Icons.Filled.Checkroom, contentDescription = null, tint = MaterialTheme.colorScheme.primary, modifier = Modifier.size(22.dp))
        }
        Text(name, style = MaterialTheme.typography.labelMedium, textAlign = TextAlign.Center, maxLines = 2, overflow = TextOverflow.Ellipsis, color = MaterialTheme.colorScheme.onSurface)
    }
}

@Composable
private fun ProductRow(items: List<Product>, onProduct: (String) -> Unit, onNeedLogin: () -> Unit) {
    LazyRow(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
        items(items, key = { it.id }) { p -> ProductCard(p, onClick = { onProduct(p.id) }, onNeedLogin = onNeedLogin, modifier = Modifier.width(160.dp)) }
    }
}

@Composable
private fun HowStep(n: Int, title: String, text: String) {
    Row(
        Modifier.fillMaxWidth().surfaceDepth(Brand.colors, PanelShape, elevation = 4.dp).padding(14.dp),
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Box(Modifier.size(36.dp).background(MaterialTheme.colorScheme.primary, CircleShape), contentAlignment = Alignment.Center) {
            Text(n.toString(), color = MaterialTheme.colorScheme.onPrimary, fontFamily = Serif, fontSize = 18.sp)
        }
        Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Text(title, style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.Bold)
            Text(text, style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted)
        }
    }
}

@Composable
private fun StoreTile(s: Store, onClick: () -> Unit, modifier: Modifier = Modifier) {
    Column(
        modifier.pressable(PanelShape, onClick, Modifier.surfaceDepth(Brand.colors, PanelShape, elevation = 4.dp)).padding(14.dp).testTag("store_${s.slug}"),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        StoreHeader(s)
        if (s.description.isNotBlank()) {
            Text(s.description, style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted, maxLines = 2, overflow = TextOverflow.Ellipsis)
        }
    }
}

/** Логотип (или первая буква), название и число товаров бутика. */
@Composable
fun StoreHeader(s: Store) {
    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
        Box(Modifier.size(44.dp).clip(CircleShape).background(Brand.colors.accentSoft), contentAlignment = Alignment.Center) {
            Text(s.name.take(1).uppercase(), fontFamily = Serif, fontSize = 20.sp, color = MaterialTheme.colorScheme.primary)
            if (!s.logoUrl.isNullOrBlank()) {
                AsyncImage(model = s.logoUrl, contentDescription = null, contentScale = ContentScale.Crop, modifier = Modifier.matchParentSize())
            }
        }
        Column {
            Text(s.name, style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.Bold, maxLines = 1, overflow = TextOverflow.Ellipsis, color = MaterialTheme.colorScheme.onSurface)
            Text(stringResource(UiR.string.stores_products, s.productCount), style = MaterialTheme.typography.labelSmall, color = Brand.colors.muted)
        }
    }
}

private val advIcons: Map<Int, ImageVector> = mapOf(
    1 to Icons.Filled.Storefront,
    2 to Icons.Filled.AutoAwesome,
    3 to Icons.Filled.LocalShipping,
    4 to Icons.Filled.AutoAwesome,
    5 to Icons.Filled.Payments,
)
private val advText: Map<Int, Pair<Int, Int>> = mapOf(
    1 to (UiR.string.home_adv1 to UiR.string.home_adv1t),
    2 to (UiR.string.home_adv2 to UiR.string.home_adv2t),
    3 to (UiR.string.home_adv3 to UiR.string.home_adv3t),
    4 to (UiR.string.home_adv4 to UiR.string.home_adv4t),
    5 to (UiR.string.home_adv5 to UiR.string.home_adv5t),
)

@Composable
private fun Benefit(i: Int) {
    val (title, text) = advText.getValue(i)
    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(12.dp), verticalAlignment = Alignment.CenterVertically) {
        Box(Modifier.size(44.dp).background(Brand.colors.accentSoft, CircleShape), contentAlignment = Alignment.Center) {
            Icon(advIcons.getValue(i), contentDescription = null, tint = MaterialTheme.colorScheme.primary, modifier = Modifier.size(22.dp))
        }
        Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Text(stringResource(title), style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.Bold)
            Text(stringResource(text), style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted)
        }
    }
}

@Composable
private fun CtaCard(onFitting: () -> Unit) {
    Column(
        Modifier.fillMaxWidth().surfaceDepth(Brand.colors, HeroShape).padding(20.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        CompositionLocalProvider(LocalContentColor provides MaterialTheme.colorScheme.onSurface) {
            Text(stringResource(UiR.string.home_cta_title), fontFamily = Serif, fontSize = 24.sp, lineHeight = 28.sp, textAlign = TextAlign.Center)
            Text(stringResource(UiR.string.home_cta_text), style = MaterialTheme.typography.bodyMedium, color = Brand.colors.muted, textAlign = TextAlign.Center)
            uz.boutique.ui.PrimaryButton(stringResource(UiR.string.home_cta_btn), onClick = onFitting, modifier = Modifier.padding(top = 4.dp).testTag("home_cta"))
        }
    }
}
