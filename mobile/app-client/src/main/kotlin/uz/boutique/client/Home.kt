package uz.boutique.client

import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.spring
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.LocalIndication
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsPressedAsState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowForward
import androidx.compose.material.icons.filled.AutoAwesome
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
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import uz.boutique.ui.Brand
import uz.boutique.ui.RubikMono
import uz.boutique.ui.R as UiR

private val TileShape = RoundedCornerShape(24.dp)

/**
 * Главная в стиле «бенто»: плитки разного размера над каталогом.
 * Самая большая плитка ведёт в примерочную (если примерка включена в сборке).
 */
@Composable
fun HomeBento(
    showFitting: Boolean,
    fittingCount: Int,
    favCount: Int,
    onFitting: () -> Unit,
    onNew: () -> Unit,
    onFavorites: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val b = Brand.colors
    Column(modifier.fillMaxWidth().testTag("home_bento"), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        if (showFitting) FittingTile(fittingCount, onFitting)
        Row(Modifier.fillMaxWidth().height(IntrinsicSize.Min), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            BentoTile(
                bg = MaterialTheme.colorScheme.surface, fg = MaterialTheme.colorScheme.onSurface,
                onClick = onNew, modifier = Modifier.weight(1f).fillMaxHeight().testTag("home_new"),
            ) {
                Text(stringResource(UiR.string.home_new), style = MaterialTheme.typography.titleMedium)
                Icon(Icons.Filled.AutoAwesome, contentDescription = null, tint = b.lavender, modifier = Modifier.size(34.dp))
                Text(stringResource(UiR.string.home_new_hint), style = MaterialTheme.typography.bodySmall, color = b.muted)
            }
            BentoTile(
                bg = b.mint, fg = b.onMint,
                onClick = onFavorites, modifier = Modifier.weight(1f).fillMaxHeight().testTag("home_favorites"),
            ) {
                Text(stringResource(UiR.string.nav_favorites), style = MaterialTheme.typography.titleMedium)
                Text(favCount.toString(), fontFamily = RubikMono, fontSize = 32.sp, lineHeight = 34.sp)
                Text(stringResource(UiR.string.home_fav_hint, favCount), style = MaterialTheme.typography.bodySmall, fontWeight = FontWeight.SemiBold)
            }
        }
    }
}

/** Пружинящее нажатие, как в макете: плитка чуть сжимается под пальцем. */
@Composable
private fun Modifier.pressable(onClick: () -> Unit): Modifier {
    val source = remember { MutableInteractionSource() }
    val pressed by source.collectIsPressedAsState()
    val scale by animateFloatAsState(
        if (pressed) 0.97f else 1f,
        spring(dampingRatio = Spring.DampingRatioMediumBouncy, stiffness = Spring.StiffnessMedium),
        label = "press",
    )
    return this
        .graphicsLayer { scaleX = scale; scaleY = scale }
        .clip(TileShape)
        .clickable(interactionSource = source, indication = LocalIndication.current, role = Role.Button, onClick = onClick)
}

@Composable
private fun BentoTile(bg: Color, fg: Color, onClick: () -> Unit, modifier: Modifier = Modifier, content: @Composable ColumnScope.() -> Unit) {
    Column(
        modifier
            .heightIn(min = 150.dp)
            .pressable(onClick)
            .background(bg)
            .padding(14.dp),
        verticalArrangement = Arrangement.SpaceBetween,
    ) {
        CompositionLocalProvider(LocalContentColor provides fg) { content() }
    }
}

@Composable
private fun FittingTile(count: Int, onClick: () -> Unit) {
    val b = Brand.colors
    val ink = b.onLavender
    Box(
        Modifier
            .fillMaxWidth()
            .heightIn(min = 196.dp)
            .pressable(onClick)
            .background(b.lavender)
            .testTag("home_fitting"),
    ) {
        Figure(ink, MaterialTheme.colorScheme.surface, Modifier.align(Alignment.BottomEnd).padding(end = 6.dp).size(width = 120.dp, height = 170.dp))
        Column(Modifier.padding(18.dp).fillMaxWidth(0.66f), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text(
                stringResource(UiR.string.home_tryon_badge).uppercase(),
                color = b.lavender,
                style = MaterialTheme.typography.labelSmall,
                fontWeight = FontWeight.ExtraBold,
                modifier = Modifier.clip(RoundedCornerShape(50)).background(ink).padding(horizontal = 10.dp, vertical = 4.dp),
            )
            Text(
                stringResource(UiR.string.home_tryon_title).uppercase(),
                color = ink, fontFamily = RubikMono, fontSize = 20.sp, lineHeight = 23.sp,
            )
            if (count > 0) {
                Text(stringResource(UiR.string.home_tryon_count, count), color = ink, style = MaterialTheme.typography.bodyMedium, fontWeight = FontWeight.SemiBold)
            }
            Row(
                Modifier.clip(RoundedCornerShape(14.dp)).background(ink).heightIn(min = 44.dp).padding(horizontal = 14.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                Text(stringResource(UiR.string.home_tryon_open), color = b.lavender, style = MaterialTheme.typography.labelLarge)
                Icon(Icons.AutoMirrored.Filled.ArrowForward, contentDescription = null, tint = b.lavender, modifier = Modifier.size(18.dp))
            }
        }
    }
}

/** Силуэт в жакете и брюках (как на макете), рисуется кодом — без картинок. */
@Composable
private fun Figure(ink: Color, pants: Color, modifier: Modifier) {
    Canvas(modifier) {
        val sx = size.width / 120f
        val sy = size.height / 170f
        fun o(x: Float, y: Float) = Offset(x * sx, y * sy)
        drawCircle(Color(0xFFEAC8B0), radius = 13f * sx, center = o(60f, 22f))
        val arm = 9f * sx
        drawLine(ink, o(36f, 46f), o(24f, 92f), strokeWidth = arm, cap = StrokeCap.Round)
        drawLine(ink, o(84f, 46f), o(96f, 92f), strokeWidth = arm, cap = StrokeCap.Round)
        drawPath(
            Path().apply {
                moveTo(36f * sx, 42f * sy); lineTo(52f * sx, 36f * sy); quadraticTo(60f * sx, 42f * sy, 68f * sx, 36f * sy)
                lineTo(84f * sx, 42f * sy); lineTo(88f * sx, 96f * sy); lineTo(32f * sx, 96f * sy); close()
            },
            ink,
        )
        drawPath(
            Path().apply {
                moveTo(32f * sx, 96f * sy); lineTo(88f * sx, 96f * sy); lineTo(85f * sx, 170f * sy); lineTo(66f * sx, 170f * sy)
                lineTo(60f * sx, 118f * sy); lineTo(54f * sx, 170f * sy); lineTo(35f * sx, 170f * sy); close()
            },
            pants,
        )
    }
}
