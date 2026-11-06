package uz.boutique.client

import android.provider.Settings
import androidx.compose.foundation.background
import androidx.compose.foundation.interaction.collectIsDraggedAsState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.pager.HorizontalPager
import androidx.compose.foundation.pager.rememberPagerState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Campaign
import androidx.compose.material.icons.filled.Pause
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.compose.currentStateAsState
import kotlinx.coroutines.delay
import uz.boutique.core.model.Product
import uz.boutique.ui.Brand
import uz.boutique.ui.ProductImage
import uz.boutique.ui.RatingRow
import uz.boutique.ui.money
import uz.boutique.ui.R as UiR

/** Сколько показывается каждый товар, прежде чем карусель перейдёт к следующему. */
const val PROMO_ROTATE_MS = 5_000L

/**
 * Карусель «Реклама» на главной: товары, продвижение которых одобрил суперадмин
 * (GET /v1/promotions). Каждый слайд помечен «Реклама».
 *
 * Автопрокрутка: по кругу раз в [PROMO_ROTATE_MS]; стоит, пока пользователь листает,
 * пока приложение свёрнуто, после кнопки «пауза» и при
 * выключенной анимации в системе (Специальные возможности → «Удалить анимацию»).
 */
@Composable
fun PromoCarousel(items: List<Product>, onProduct: (String) -> Unit, modifier: Modifier = Modifier, edge: Dp = 16.dp) {
    if (items.isEmpty()) return
    val n = items.size
    val pager = rememberPagerState { n }
    var paused by rememberSaveable { mutableStateOf(false) }
    val dragged by pager.interactionSource.collectIsDraggedAsState()
    val lifecycle by LocalLifecycleOwner.current.lifecycle.currentStateAsState()
    val reduced = rememberAnimationsOff()
    val auto = n > 1 && !paused && !dragged && !reduced && lifecycle.isAtLeast(Lifecycle.State.RESUMED)

    // один таймер: перезапускается после каждого перехода (и ручного тоже)
    LaunchedEffect(auto, pager.settledPage, n) {
        if (!auto) return@LaunchedEffect
        delay(PROMO_ROTATE_MS)
        pager.animateScrollToPage((pager.settledPage + 1) % n)
    }

    Column(modifier.fillMaxWidth().testTag("promo_carousel"), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Row(
            Modifier.fillMaxWidth().padding(start = edge, end = (edge - 12.dp).coerceAtLeast(0.dp)),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            AdChip(icon = true)
            Text(
                stringResource(UiR.string.promo_title),
                style = MaterialTheme.typography.titleMedium,
                fontWeight = FontWeight.ExtraBold,
                modifier = Modifier.weight(1f),
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            if (n > 1 && !reduced) {
                IconButton(onClick = { paused = !paused }, modifier = Modifier.testTag("promo_pause")) {
                    Icon(
                        if (paused) Icons.Filled.PlayArrow else Icons.Filled.Pause,
                        contentDescription = stringResource(if (paused) UiR.string.promo_play else UiR.string.promo_pause),
                    )
                }
            }
        }
        HorizontalPager(
            state = pager,
            contentPadding = PaddingValues(horizontal = edge),
            pageSpacing = 12.dp,
            key = { items[it].id },
            modifier = Modifier.fillMaxWidth().testTag("promo_pager"),
        ) { page ->
            val p = items[page]
            val position = stringResource(UiR.string.promo_position, page + 1, n)
            PromoSlide(p, onClick = { onProduct(p.id) }, modifier = Modifier.semantics { contentDescription = position })
        }
        if (n > 1) {
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.Center) {
                repeat(n) { i ->
                    val on = i == pager.currentPage
                    Box(
                        Modifier.padding(3.dp).height(8.dp).width(if (on) 22.dp else 8.dp).clip(CircleShape)
                            .background(if (on) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.outlineVariant),
                    )
                }
            }
        }
    }
}

@Composable
private fun AdChip(icon: Boolean = false) {
    Row(
        Modifier.clip(RoundedCornerShape(50)).background(Brand.colors.promo).padding(horizontal = 10.dp, vertical = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        if (icon) Icon(Icons.Filled.Campaign, contentDescription = null, tint = Brand.colors.promoText, modifier = Modifier.size(14.dp))
        Text(
            stringResource(UiR.string.promo_label).uppercase(),
            style = MaterialTheme.typography.labelSmall,
            fontWeight = FontWeight.ExtraBold,
            color = Brand.colors.promoText,
        )
    }
}

@Composable
private fun PromoSlide(p: Product, onClick: () -> Unit, modifier: Modifier = Modifier) {
    Card(
        onClick = onClick,
        shape = RoundedCornerShape(20.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        border = CardDefaults.outlinedCardBorder(),
        modifier = modifier.fillMaxWidth().testTag("promo_${p.id}"),
    ) {
        Row(Modifier.height(180.dp)) {
            ProductImage(p.cardImage, contentDescription = p.title, modifier = Modifier.aspectRatio(3f / 4f))
            Column(Modifier.weight(1f).padding(14.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                AdChip()
                Text(p.storeName, style = MaterialTheme.typography.labelMedium, color = Brand.colors.muted, maxLines = 1, overflow = TextOverflow.Ellipsis)
                Text(p.title, style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.Bold, maxLines = 2, overflow = TextOverflow.Ellipsis)
                RatingRow(p.ratingAvg, p.ratingCount)
                Spacer(Modifier.weight(1f))
                Text(money(p.price), style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.ExtraBold)
            }
        }
    }
}

/** Анимация выключена в системе («Удалить анимацию» / масштаб анимации 0) — автопрокрутку не включаем. */
@Composable
private fun rememberAnimationsOff(): Boolean {
    val context = LocalContext.current
    return remember(context) {
        runCatching {
            Settings.Global.getFloat(context.contentResolver, Settings.Global.ANIMATOR_DURATION_SCALE, 1f) == 0f
        }.getOrDefault(false)
    }
}
