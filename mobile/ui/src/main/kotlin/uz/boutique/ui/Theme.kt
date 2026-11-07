package uz.boutique.ui

import android.content.Context
import androidx.appcompat.app.AppCompatDelegate
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

// Дизайн «Бенто», палитра «Лаванда и мята» (выбрана 2026-11-07).
// Фон ни белый, ни чёрный; основной текст около 13:1 к фону; в тёмной теме акценты приглушены.
// Пары «текст на фоне» проверяет PaletteContrastTest (не ниже 4,5:1, для иконок 3:1).

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
    /** Лавандовая плитка (примерочная, главная кнопка) и тёмный текст на ней. */
    val lavender: Color,
    val onLavender: Color,
    /** Мятная плитка. */
    val mint: Color,
    val onMint: Color,
    /** Персиковая плитка. */
    val peach: Color,
    val onPeach: Color,
)

private val LightBrand = BrandColors(
    accent2 = Color(0xFFC94F6D), promo = Color(0xFFF5BFA3), promoText = Color(0xFF3A1E10),
    star = Color(0xFFE8A400), ok = Color(0xFF2F7A45), warn = Color(0xFF8A5F00), bad = Color(0xFFB23B2E),
    info = Color(0xFF2C5D8A), muted = Color(0xFF5E6378), accentSoft = Color(0xFFE1DDFB), surface2 = Color(0xFFE6E8F2),
    lavender = Color(0xFF9387F2), onLavender = Color(0xFF16142E),
    mint = Color(0xFF93D3C3), onMint = Color(0xFF11302A),
    peach = Color(0xFFF5BFA3), onPeach = Color(0xFF3A1E10),
)
private val DarkBrand = BrandColors(
    accent2 = Color(0xFFF29BB0), promo = Color(0xFFDDA388), promoText = Color(0xFF2E170C),
    star = Color(0xFFFFC531), ok = Color(0xFF7CC792), warn = Color(0xFFE2B84F), bad = Color(0xFFEC8A7E),
    info = Color(0xFF8DB8E0), muted = Color(0xFFA3A8BD), accentSoft = Color(0xFF3A3566), surface2 = Color(0xFF2E3243),
    lavender = Color(0xFF9D92F2), onLavender = Color(0xFF16142E),
    mint = Color(0xFF6FB5A4), onMint = Color(0xFF0E2420),
    peach = Color(0xFFDDA388), onPeach = Color(0xFF2E170C),
)

internal val LightScheme = lightColorScheme(
    primary = Color(0xFF5A4CC9), onPrimary = Color.White,
    primaryContainer = Color(0xFF9387F2), onPrimaryContainer = Color(0xFF16142E),
    secondary = Color(0xFF5A4CC9), onSecondary = Color.White,
    secondaryContainer = Color(0xFFE1DDFB), onSecondaryContainer = Color(0xFF2B2470),
    tertiary = Color(0xFF2F7A69), onTertiary = Color.White,
    tertiaryContainer = Color(0xFF93D3C3), onTertiaryContainer = Color(0xFF11302A),
    background = Color(0xFFF1F2F7), onBackground = Color(0xFF22253A),
    surface = Color(0xFFFCFCFE), onSurface = Color(0xFF22253A),
    surfaceVariant = Color(0xFFE6E8F2), onSurfaceVariant = Color(0xFF5E6378),
    surfaceContainer = Color(0xFFFCFCFE), surfaceContainerLow = Color(0xFFF7F7FB), surfaceContainerHigh = Color(0xFFF7F7FB),
    surfaceContainerHighest = Color(0xFFE6E8F2), surfaceContainerLowest = Color(0xFFFCFCFE),
    inverseSurface = Color(0xFF2E3243), inverseOnSurface = Color(0xFFE4E6F0), inversePrimary = Color(0xFFB3AAF7),
    outline = Color(0xFFD5D8E5), outlineVariant = Color(0xFFE2E4EE),
    error = Color(0xFFB23B2E), onError = Color.White,
)
internal val DarkScheme = darkColorScheme(
    primary = Color(0xFFB3AAF7), onPrimary = Color(0xFF16142E),
    primaryContainer = Color(0xFF9D92F2), onPrimaryContainer = Color(0xFF16142E),
    secondary = Color(0xFFB3AAF7), onSecondary = Color(0xFF16142E),
    secondaryContainer = Color(0xFF3A3566), onSecondaryContainer = Color(0xFFDCD8FC),
    tertiary = Color(0xFF8FD1C0), onTertiary = Color(0xFF0E2420),
    tertiaryContainer = Color(0xFF6FB5A4), onTertiaryContainer = Color(0xFF0E2420),
    background = Color(0xFF1A1C26), onBackground = Color(0xFFE4E6F0),
    surface = Color(0xFF242735), onSurface = Color(0xFFE4E6F0),
    surfaceVariant = Color(0xFF2E3243), onSurfaceVariant = Color(0xFFA3A8BD),
    surfaceContainer = Color(0xFF242735), surfaceContainerLow = Color(0xFF1F222E), surfaceContainerHigh = Color(0xFF2A2D3C),
    surfaceContainerHighest = Color(0xFF2E3243), surfaceContainerLowest = Color(0xFF1A1C26),
    inverseSurface = Color(0xFFE4E6F0), inverseOnSurface = Color(0xFF22253A), inversePrimary = Color(0xFF5A4CC9),
    outline = Color(0xFF3A3F52), outlineVariant = Color(0xFF323646),
    error = Color(0xFFEC8A7E), onError = Color(0xFF1A1C26),
)

internal fun brandColors(dark: Boolean) = if (dark) DarkBrand else LightBrand
internal fun colorScheme(dark: Boolean) = if (dark) DarkScheme else LightScheme

// Вариативный шрифт: Font(resId, weight) по умолчанию выставляет ось wght под нужную насыщенность.
private fun rubik(weight: Int) = Font(R.font.rubik, weight = FontWeight(weight))

val Rubik = FontFamily(rubik(400), rubik(500), rubik(600), rubik(700), rubik(800))

/** Широкий акцентный шрифт для коротких надписей на плитках («ПРИМЕРЬ НА СЕБЕ», «−30%»). Не для длинного текста. */
val RubikMono = FontFamily(Font(R.font.rubik_mono_one, weight = FontWeight.Normal))

private fun style(size: Int, weight: Int, line: Int) =
    TextStyle(fontFamily = Rubik, fontSize = size.sp, fontWeight = FontWeight(weight), lineHeight = line.sp)

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
    extraLarge = RoundedCornerShape(26.dp),
)

val LocalBrand = staticCompositionLocalOf { LightBrand }

object Brand {
    val colors: BrandColors @Composable get() = LocalBrand.current
}

@Composable
fun BoutiqueTheme(dark: Boolean = isSystemInDarkTheme(), content: @Composable () -> Unit) {
    CompositionLocalProvider(LocalBrand provides brandColors(dark)) {
        MaterialTheme(
            colorScheme = colorScheme(dark),
            typography = AppTypography,
            shapes = AppShapes,
            content = content,
        )
    }
}

/**
 * Тема приложения: как в системе, светлая или тёмная. Выбор хранится на телефоне и применяется
 * через AppCompat, поэтому меняются сразу и окно, и системные панели, и Compose (isSystemInDarkTheme).
 */
object AppTheme {
    const val SYSTEM = "system"
    const val LIGHT = "light"
    const val DARK = "dark"
    val MODES = listOf(SYSTEM, LIGHT, DARK)

    private const val PREFS = "ui"
    private const val KEY = "theme"

    fun nightMode(mode: String): Int = when (mode) {
        LIGHT -> AppCompatDelegate.MODE_NIGHT_NO
        DARK -> AppCompatDelegate.MODE_NIGHT_YES
        else -> AppCompatDelegate.MODE_NIGHT_FOLLOW_SYSTEM
    }

    private fun prefs(context: Context) = context.applicationContext.getSharedPreferences(PREFS, Context.MODE_PRIVATE)

    fun current(context: Context): String = prefs(context).getString(KEY, null)?.takeIf { it in MODES } ?: SYSTEM

    fun set(context: Context, mode: String) {
        require(mode in MODES) { "unknown theme $mode" }
        prefs(context).edit().putString(KEY, mode).apply()
        AppCompatDelegate.setDefaultNightMode(nightMode(mode))
    }

    /** Вызывать в Application.onCreate, до первой Activity. */
    fun apply(context: Context) = AppCompatDelegate.setDefaultNightMode(nightMode(current(context)))
}
