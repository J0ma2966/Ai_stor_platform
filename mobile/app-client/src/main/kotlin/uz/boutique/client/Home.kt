package uz.boutique.client

import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.spring
import androidx.compose.foundation.BorderStroke
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
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowForward
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.FavoriteBorder
import androidx.compose.material3.Icon
import androidx.compose.material3.LocalContentColor
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import coil.compose.AsyncImage
import uz.boutique.ui.Brand
import uz.boutique.ui.EyebrowStyle
import uz.boutique.ui.R as UiR
import uz.boutique.ui.Serif

private val HeroShape = RoundedCornerShape(22.dp)
private val PanelShape = RoundedCornerShape(16.dp)

/**
 * Главная по шаблону VESTA: «бровь», крупный заголовок с засечками, большая карточка с фото
 * (ведёт в примерочную, а если примерка выключена в сборке — к новинкам) и две панели под ней.
 * [heroImage] — фото первого товара каталога; пока его нет, карточка просто тёмная.
 */
@Composable
fun HomeHero(
    showFitting: Boolean,
    fittingCount: Int,
    favCount: Int,
    heroImage: String?,
    onFitting: () -> Unit,
    onNew: () -> Unit,
    onFavorites: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val b = Brand.colors
    Column(modifier.fillMaxWidth().testTag("home_hero"), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Text(stringResource(UiR.string.home_eyebrow).uppercase(), style = EyebrowStyle, color = MaterialTheme.colorScheme.primary)
        Text(stringResource(UiR.string.home_title), style = MaterialTheme.typography.displaySmall)
        Text(
            stringResource(if (showFitting) UiR.string.home_sub_tryon else UiR.string.home_sub),
            style = MaterialTheme.typography.bodyMedium, color = b.muted,
        )
        if (showFitting) {
            HeroCard(
                image = heroImage,
                badge = stringResource(UiR.string.home_tryon_badge),
                title = stringResource(UiR.string.home_tryon_title),
                hint = stringResource(UiR.string.home_tryon_hint),
                count = if (fittingCount > 0) stringResource(UiR.string.home_tryon_count, fittingCount) else null,
                action = stringResource(UiR.string.home_tryon_open),
                onClick = onFitting,
                modifier = Modifier.testTag("home_fitting"),
            )
        } else {
            HeroCard(
                image = heroImage,
                badge = stringResource(UiR.string.home_new),
                title = stringResource(UiR.string.home_new_title),
                hint = null,
                count = null,
                action = stringResource(UiR.string.home_new_open),
                onClick = onNew,
                modifier = Modifier.testTag("home_hero_new"),
            )
        }
        Row(Modifier.fillMaxWidth().height(IntrinsicSize.Min), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            Panel(onClick = onNew, modifier = Modifier.weight(1f).fillMaxHeight().testTag("home_new")) {
                Icon(Icons.Filled.AutoAwesome, contentDescription = null, tint = MaterialTheme.colorScheme.primary, modifier = Modifier.size(24.dp))
                Text(stringResource(UiR.string.home_new), fontFamily = Serif, fontSize = 19.sp, lineHeight = 23.sp)
                Text(stringResource(UiR.string.home_new_hint), style = MaterialTheme.typography.bodySmall, color = b.muted)
            }
            Panel(onClick = onFavorites, modifier = Modifier.weight(1f).fillMaxHeight().testTag("home_favorites")) {
                Icon(Icons.Filled.FavoriteBorder, contentDescription = null, tint = b.accent2, modifier = Modifier.size(24.dp))
                Text(favCount.toString(), fontFamily = Serif, fontSize = 22.sp, lineHeight = 24.sp)
                Text(stringResource(UiR.string.home_fav_hint, favCount), style = MaterialTheme.typography.bodySmall, color = b.muted)
            }
        }
    }
}

/** Пружинящее нажатие: карточка чуть сжимается под пальцем. */
@Composable
private fun Modifier.pressable(shape: Shape, onClick: () -> Unit): Modifier {
    val source = remember { MutableInteractionSource() }
    val pressed by source.collectIsPressedAsState()
    val scale by animateFloatAsState(
        if (pressed) 0.97f else 1f,
        spring(dampingRatio = Spring.DampingRatioMediumBouncy, stiffness = Spring.StiffnessMedium),
        label = "press",
    )
    return this
        .graphicsLayer { scaleX = scale; scaleY = scale }
        .clip(shape)
        .clickable(interactionSource = source, indication = LocalIndication.current, role = Role.Button, onClick = onClick)
}

@Composable
private fun Panel(onClick: () -> Unit, modifier: Modifier = Modifier, content: @Composable ColumnScope.() -> Unit) {
    Column(
        modifier
            .heightIn(min = 112.dp)
            .pressable(PanelShape, onClick)
            .background(MaterialTheme.colorScheme.surface)
            .border(BorderStroke(1.dp, Brand.colors.line), PanelShape)
            .padding(14.dp),
        verticalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        CompositionLocalProvider(LocalContentColor provides MaterialTheme.colorScheme.onSurface) { content() }
    }
}

@Composable
private fun HeroCard(
    image: String?,
    badge: String,
    title: String,
    hint: String?,
    count: String?,
    action: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val b = Brand.colors
    Box(
        modifier
            .fillMaxWidth()
            .height(320.dp)
            .pressable(HeroShape, onClick)
            .background(b.hero),
    ) {
        if (image != null) {
            AsyncImage(model = image, contentDescription = null, contentScale = ContentScale.Crop, alpha = 0.85f, modifier = Modifier.fillMaxSize())
        }
        // Затемнение снизу: светлый текст читается на любом фото.
        Box(
            Modifier
                .fillMaxSize()
                .background(Brush.verticalGradient(0.3f to Color.Transparent, 1f to b.hero.copy(alpha = 0.94f))),
        )
        Column(
            Modifier.align(Alignment.BottomStart).padding(20.dp),
            verticalArrangement = Arrangement.spacedBy(7.dp),
        ) {
            Text(badge.uppercase(), style = EyebrowStyle, color = b.heroEyebrow)
            Text(title, fontFamily = Serif, fontSize = 27.sp, lineHeight = 31.sp, color = b.onHero)
            if (hint != null) Text(hint, style = MaterialTheme.typography.bodyMedium, color = b.onHero)
            if (count != null) Text(count, style = MaterialTheme.typography.labelMedium, color = b.heroEyebrow)
            Row(
                Modifier
                    .padding(top = 6.dp)
                    .clip(CircleShape)
                    .background(b.heroButton)
                    .heightIn(min = 44.dp)
                    .padding(horizontal = 18.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                Text(action.uppercase(), color = b.onHeroButton, style = MaterialTheme.typography.labelMedium, letterSpacing = 0.5.sp)
                Icon(Icons.AutoMirrored.Filled.ArrowForward, contentDescription = null, tint = b.onHeroButton, modifier = Modifier.size(16.dp))
            }
        }
    }
}
