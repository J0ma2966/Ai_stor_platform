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

// Дизайн по шаблону VESTA (выбран 2026-11-08): тёмная тема «Black Luxury» (графит и золото),
// светлая «Fashion Social» (пудровый и малиновый). Заголовки с засечками, панели с тонкой рамкой,
// кнопки-«таблетки». Пары «текст на фоне» проверяет PaletteContrastTest (не ниже 4,5:1, иконки 3:1).

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
    /** Тонкая рамка панелей и карточек. */
    val line: Color,
    /** Большая карточка с фото (примерочная на главной): тёмная подложка, светлый текст, надпись-«бровь». */
    val hero: Color,
    val onHero: Color,
    val heroEyebrow: Color,
    /** Кнопка на карточке с фото: заметна на тёмной подложке. */
    val heroButton: Color,
    val onHeroButton: Color,
    /** Карточка кошелька токенов: градиент и текст на нём. */
    val walletStart: Color,
    val walletEnd: Color,
    val walletEyebrow: Color,
    val walletText: Color,
    /** Объём (как на сайте): градиент акцентной кнопки сверху вниз, цвет её свечения. */
    val accentTop: Color,
    val accentBottom: Color,
    val glow: Color,
    /** Градиент панелей и вторичных кнопок, фон полей ввода, блик по верхнему краю, цвет тени. */
    val surfaceTop: Color,
    val surfaceBottom: Color,
    val field: Color,
    val highlight: Color,
    val shadow: Color,
)

// Светлая: Fashion Social. Малиновый темнее, чем в шаблоне (#d45d78 → #B03F5C), чтобы белый текст на нём читался.
private val LightBrand = BrandColors(
    accent2 = Color(0xFFB03F5C), promo = Color(0xFFF5C6D2), promoText = Color(0xFF3A1420),
    star = Color(0xFFE8A400), ok = Color(0xFF2F7A45), warn = Color(0xFF8A5F00), bad = Color(0xFFB23B2E),
    info = Color(0xFF2C5D8A), muted = Color(0xFF74626A), accentSoft = Color(0xFFF9E5E8), surface2 = Color(0xFFF9E5E8),
    line = Color(0xFFF0DCE0),
    hero = Color(0xFF24201D), onHero = Color(0xFFFAF6F2), heroEyebrow = Color(0xFFF5C6D2),
    heroButton = Color(0xFFF5C6D2), onHeroButton = Color(0xFF3A1420),
    walletStart = Color(0xFF2B2022), walletEnd = Color(0xFF7A3346), walletEyebrow = Color(0xFFF5C6D2), walletText = Color(0xFFF3E3E7),
    accentTop = Color(0xFFC24E6B), accentBottom = Color(0xFF983350), glow = Color(0xFFB03F5C),
    surfaceTop = Color(0xFFFFFFFF), surfaceBottom = Color(0xFFFFF7F4), field = Color(0xFFFFFDFC),
    highlight = Color(0xF2FFFFFF), shadow = Color(0xFF7A3346),
)

// Тёмная: Black Luxury. Фон как в шаблоне (#111111), текст чуть мягче (#EDE6DA), чтобы не резал глаз.
private val DarkBrand = BrandColors(
    accent2 = Color(0xFFC6A56B), promo = Color(0xFF5A4733), promoText = Color(0xFFF5EBDD),
    star = Color(0xFFFFC531), ok = Color(0xFF7CC792), warn = Color(0xFFE2B84F), bad = Color(0xFFEC8A7E),
    info = Color(0xFF8DB8E0), muted = Color(0xFFAAA39A), accentSoft = Color(0xFF29251F), surface2 = Color(0xFF29251F),
    line = Color(0xFF39352E),
    hero = Color(0xFF24201D), onHero = Color(0xFFFAF6F2), heroEyebrow = Color(0xFFE5C99E),
    heroButton = Color(0xFFC6A56B), onHeroButton = Color(0xFF111111),
    walletStart = Color(0xFF29251F), walletEnd = Color(0xFF5A4733), walletEyebrow = Color(0xFFE4C99F), walletText = Color(0xFFE1D8CD),
    accentTop = Color(0xFFE6CB98), accentBottom = Color(0xFFA4824A), glow = Color(0xFFC6A56B),
    surfaceTop = Color(0xFF262524), surfaceBottom = Color(0xFF1C1C1C), field = Color(0xFF151515),
    highlight = Color(0x12FFFFFF), shadow = Color(0xFF000000),
)

internal val LightScheme = lightColorScheme(
    primary = Color(0xFFB03F5C), onPrimary = Color.White,
    primaryContainer = Color(0xFFB03F5C), onPrimaryContainer = Color.White,
    secondary = Color(0xFFB03F5C), onSecondary = Color.White,
    secondaryContainer = Color(0xFFF9E5E8), onSecondaryContainer = Color(0xFF7A3346),
    tertiary = Color(0xFF7A3346), onTertiary = Color.White,
    tertiaryContainer = Color(0xFFF5C6D2), onTertiaryContainer = Color(0xFF3A1420),
    background = Color(0xFFFFF4F0), onBackground = Color(0xFF2B2022),
    surface = Color(0xFFFFFAF8), onSurface = Color(0xFF2B2022),
    surfaceVariant = Color(0xFFF9E5E8), onSurfaceVariant = Color(0xFF74626A),
    surfaceContainer = Color(0xFFFFFAF8), surfaceContainerLow = Color(0xFFFFF7F4), surfaceContainerHigh = Color(0xFFFCEFEC),
    surfaceContainerHighest = Color(0xFFF9E5E8), surfaceContainerLowest = Color(0xFFFFFAF8),
    inverseSurface = Color(0xFF2B2022), inverseOnSurface = Color(0xFFFFF4F0), inversePrimary = Color(0xFFF5C6D2),
    outline = Color(0xFFDDBFC6), outlineVariant = Color(0xFFF0DCE0),
    error = Color(0xFFB23B2E), onError = Color.White,
)
internal val DarkScheme = darkColorScheme(
    primary = Color(0xFFC6A56B), onPrimary = Color(0xFF111111),
    primaryContainer = Color(0xFFC6A56B), onPrimaryContainer = Color(0xFF111111),
    secondary = Color(0xFFC6A56B), onSecondary = Color(0xFF111111),
    secondaryContainer = Color(0xFF29251F), onSecondaryContainer = Color(0xFFE4C99F),
    tertiary = Color(0xFFD9BE8C), onTertiary = Color(0xFF111111),
    tertiaryContainer = Color(0xFF5A4733), onTertiaryContainer = Color(0xFFF5EBDD),
    background = Color(0xFF111111), onBackground = Color(0xFFEDE6DA),
    surface = Color(0xFF1D1D1D), onSurface = Color(0xFFEDE6DA),
    surfaceVariant = Color(0xFF29251F), onSurfaceVariant = Color(0xFFAAA39A),
    surfaceContainer = Color(0xFF1D1D1D), surfaceContainerLow = Color(0xFF171717), surfaceContainerHigh = Color(0xFF232220),
    surfaceContainerHighest = Color(0xFF29251F), surfaceContainerLowest = Color(0xFF111111),
    inverseSurface = Color(0xFFEDE6DA), inverseOnSurface = Color(0xFF1D1D1D), inversePrimary = Color(0xFF8C6332),
    outline = Color(0xFF4A453C), outlineVariant = Color(0xFF39352E),
    error = Color(0xFFEC8A7E), onError = Color(0xFF111111),
)

internal fun brandColors(dark: Boolean) = if (dark) DarkBrand else LightBrand
internal fun colorScheme(dark: Boolean) = if (dark) DarkScheme else LightScheme

// Вариативный шрифт: Font(resId, weight) по умолчанию выставляет ось wght под нужную насыщенность.
private fun rubik(weight: Int) = Font(R.font.rubik, weight = FontWeight(weight))

val Rubik = FontFamily(rubik(400), rubik(500), rubik(600), rubik(700), rubik(800))

/** Заголовки с засечками, как Georgia в шаблоне VESTA. Системный шрифт: файл не нужен. */
val Serif: FontFamily = FontFamily.Serif

private fun style(size: Int, weight: Int, line: Int, family: FontFamily = Rubik) =
    TextStyle(fontFamily = family, fontSize = size.sp, fontWeight = FontWeight(weight), lineHeight = line.sp)

private val AppTypography = Typography(
    displaySmall = style(32, 400, 36, Serif),
    headlineMedium = style(28, 400, 32, Serif),
    headlineSmall = style(24, 400, 28, Serif),
    titleLarge = style(21, 400, 26, Serif),
    titleMedium = style(16, 700, 22),
    titleSmall = style(14, 700, 20),
    bodyLarge = style(16, 500, 23),
    bodyMedium = style(14, 500, 20),
    bodySmall = style(12, 500, 16),
    labelLarge = style(15, 700, 20),
    labelMedium = style(13, 600, 18),
    labelSmall = style(11, 600, 14),
)

/** Надпись-«бровь» над заголовком: мелкие заглавные с разрядкой, как в шаблоне. */
val EyebrowStyle = TextStyle(fontFamily = Rubik, fontSize = 10.sp, fontWeight = FontWeight.ExtraBold, letterSpacing = 2.sp, lineHeight = 14.sp)

private val AppShapes = Shapes(
    extraSmall = RoundedCornerShape(8.dp),
    small = RoundedCornerShape(12.dp),
    medium = RoundedCornerShape(14.dp),
    large = RoundedCornerShape(18.dp),
    extraLarge = RoundedCornerShape(22.dp),
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
