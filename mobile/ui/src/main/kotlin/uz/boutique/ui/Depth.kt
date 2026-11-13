package uz.boutique.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.RoundRect
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.graphics.drawscope.clipPath
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp

// Объём, как на сайте (2026-11-13): градиентные кнопки с бликом и цветным свечением,
// панели с мягкой многослойной тенью, поля ввода «утоплены» внутрь. Цвета — BrandColors.

/** Акцентная поверхность: малиновый (светлая) или «металлическое» золото (тёмная). */
fun Modifier.accentDepth(b: BrandColors, shape: Shape, elevation: Dp = 8.dp, enabled: Boolean = true): Modifier =
    this
        .shadow(if (enabled) elevation else 0.dp, shape, clip = false, ambientColor = b.glow.copy(alpha = 0.5f), spotColor = b.glow.copy(alpha = 0.6f))
        .background(Brush.verticalGradient(listOf(b.accentTop, b.accentBottom)), shape)
        .border(1.dp, Brush.verticalGradient(listOf(Color.White.copy(alpha = 0.38f), Color.Transparent, Color.Black.copy(alpha = 0.12f))), shape)

/** Панель или вторичная кнопка: светлый верх, мягкая тень, блик по верхнему краю. */
fun Modifier.surfaceDepth(b: BrandColors, shape: Shape, elevation: Dp = 6.dp): Modifier =
    this
        .shadow(elevation, shape, clip = false, ambientColor = b.shadow.copy(alpha = 0.25f), spotColor = b.shadow.copy(alpha = 0.3f))
        .background(Brush.verticalGradient(listOf(b.surfaceTop, b.surfaceBottom)), shape)
        .border(1.dp, Brush.verticalGradient(listOf(b.highlight, b.line)), shape)

/**
 * Внутренняя тень сверху: поле ввода выглядит утопленным. Рисуется поверх фона поля, внутри его
 * скруглённой рамки. [top] — отступ рамки OutlinedTextField сверху (место под плавающую подпись).
 */
fun Modifier.insetDepth(b: BrandColors, corner: Dp = 14.dp, top: Dp = 8.dp, depth: Dp = 7.dp): Modifier = drawWithContent {
    drawContent()
    val t = top.toPx()
    val h = depth.toPx()
    val r = corner.toPx()
    val clip = Path().apply {
        addRoundRect(RoundRect(0f, t, size.width, size.height, CornerRadius(r, r)))
    }
    clipPath(clip) {
        drawRect(
            Brush.verticalGradient(listOf(b.shadow.copy(alpha = 0.13f), Color.Transparent), startY = t, endY = t + h),
            topLeft = Offset(0f, t),
            size = Size(size.width, h),
        )
    }
}
