package uz.boutique.ui

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Shapes
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

// Дизайн «мягкий маркетплейс» (вариант B): те же токены, что в web/src/app/globals.css.

@Immutable
data class BrandColors(
    val accent2: Color,
    val promo: Color,
    val promoText: Color,
    val star: Color,
    val ok: Color,
    val warn: Color,
    val bad: Color,
    val info: Color,
    val muted: Color,
    val accentSoft: Color,
    val surface2: Color,
)

private val LightBrand = BrandColors(
    accent2 = Color(0xFFE8484E), promo = Color(0xFFFFD25E), promoText = Color(0xFF2A1A00),
    star = Color(0xFFE8A400), ok = Color(0xFF2F7A45), warn = Color(0xFF8A5F00), bad = Color(0xFFB23B2E),
    info = Color(0xFF2C5D8A), muted = Color(0xFF5F5A6B), accentSoft = Color(0xFFEFE9F8), surface2 = Color(0xFFF7F6FB),
)
private val DarkBrand = BrandColors(
    accent2 = Color(0xFFFF7A7E), promo = Color(0xFFFFD25E), promoText = Color(0xFF2A1A00),
    star = Color(0xFFFFC531), ok = Color(0xFF7CC792), warn = Color(0xFFE2B84F), bad = Color(0xFFEC8A7E),
    info = Color(0xFF8DB8E0), muted = Color(0xFFAAA3B7), accentSoft = Color(0xFF2B2338), surface2 = Color(0xFF24202B),
)

private val LightScheme = lightColorScheme(
    primary = Color(0xFF3F1F6E), onPrimary = Color.White,
    primaryContainer = Color(0xFFEFE9F8), onPrimaryContainer = Color(0xFF2D1452),
    secondary = Color(0xFFE8484E), onSecondary = Color.White,
    secondaryContainer = Color(0xFFEFE9F8), onSecondaryContainer = Color(0xFF3F1F6E),
    tertiary = Color(0xFFFFD25E), onTertiary = Color(0xFF2A1A00),
    background = Color(0xFFF3F2F7), onBackground = Color(0xFF17151C),
    surface = Color.White, onSurface = Color(0xFF17151C),
    surfaceVariant = Color(0xFFF7F6FB), onSurfaceVariant = Color(0xFF5F5A6B),
    surfaceContainer = Color.White, surfaceContainerLow = Color(0xFFF7F6FB), surfaceContainerHigh = Color(0xFFF7F6FB),
    surfaceContainerHighest = Color(0xFFEFE9F8), surfaceContainerLowest = Color.White,
    outline = Color(0xFFE6E3EE), outlineVariant = Color(0xFFE6E3EE),
    error = Color(0xFFB23B2E), onError = Color.White,
)
private val DarkScheme = darkColorScheme(
    primary = Color(0xFFB99CF2), onPrimary = Color(0xFF1A1022),
    primaryContainer = Color(0xFF2B2338), onPrimaryContainer = Color(0xFFCDB6F7),
    secondary = Color(0xFFFF7A7E), onSecondary = Color(0xFF1A1022),
    secondaryContainer = Color(0xFF2B2338), onSecondaryContainer = Color(0xFFCDB6F7),
    tertiary = Color(0xFFFFD25E), onTertiary = Color(0xFF2A1A00),
    background = Color(0xFF121016), onBackground = Color(0xFFF1EEF6),
    surface = Color(0xFF1C1922), onSurface = Color(0xFFF1EEF6),
    surfaceVariant = Color(0xFF24202B), onSurfaceVariant = Color(0xFFAAA3B7),
    surfaceContainer = Color(0xFF1C1922), surfaceContainerLow = Color(0xFF24202B), surfaceContainerHigh = Color(0xFF24202B),
    surfaceContainerHighest = Color(0xFF2B2338), surfaceContainerLowest = Color(0xFF121016),
    outline = Color(0xFF312C3A), outlineVariant = Color(0xFF312C3A),
    error = Color(0xFFEC8A7E), onError = Color(0xFF1A1022),
)

// Вариативный шрифт: Font(resId, weight) по умолчанию выставляет ось wght под нужную насыщенность.
private fun manrope(weight: Int) = Font(R.font.manrope, weight = FontWeight(weight))

val Manrope = FontFamily(manrope(400), manrope(500), manrope(600), manrope(700), manrope(800))

private fun style(size: Int, weight: Int, line: Int) =
    TextStyle(fontFamily = Manrope, fontSize = size.sp, fontWeight = FontWeight(weight), lineHeight = line.sp)

private val AppTypography = Typography(
    displaySmall = style(30, 800, 36),
    headlineMedium = style(26, 800, 32),
    headlineSmall = style(22, 800, 28),
    titleLarge = style(20, 700, 26),
    titleMedium = style(16, 700, 22),
    titleSmall = style(14, 700, 20),
    bodyLarge = style(16, 500, 23),
    bodyMedium = style(14, 500, 20),
    bodySmall = style(12, 500, 16),
    labelLarge = style(15, 700, 20),
    labelMedium = style(13, 600, 18),
    labelSmall = style(11, 600, 14),
)

private val AppShapes = Shapes(
    extraSmall = RoundedCornerShape(8.dp),
    small = RoundedCornerShape(12.dp),
    medium = RoundedCornerShape(16.dp),
    large = RoundedCornerShape(20.dp),
    extraLarge = RoundedCornerShape(28.dp),
)

val LocalBrand = staticCompositionLocalOf { LightBrand }

object Brand {
    val colors: BrandColors @Composable get() = LocalBrand.current
}

@Composable
fun BoutiqueTheme(dark: Boolean = isSystemInDarkTheme(), content: @Composable () -> Unit) {
    CompositionLocalProvider(LocalBrand provides if (dark) DarkBrand else LightBrand) {
        MaterialTheme(
            colorScheme = if (dark) DarkScheme else LightScheme,
            typography = AppTypography,
            shapes = AppShapes,
            content = content,
        )
    }
}
