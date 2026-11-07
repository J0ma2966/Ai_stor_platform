package uz.boutique.ui

import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.luminance
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Палитра «Лаванда и мята»: текст читается в обеих темах (WCAG AA 4,5:1, иконки 3:1),
 * но без режущего глаза чисто белого на чёрном (фон ни #FFF, ни #000, основной текст не выше 16:1).
 */
class PaletteContrastTest {
    private fun contrast(a: Color, b: Color): Double {
        val l1 = a.luminance().toDouble()
        val l2 = b.luminance().toDouble()
        return (maxOf(l1, l2) + 0.05) / (minOf(l1, l2) + 0.05)
    }

    private fun check(name: String, fg: Color, bg: Color, min: Double) {
        val c = contrast(fg, bg)
        assertTrue("$name: контраст %.2f, нужно не меньше $min".format(c), c >= min)
    }

    private fun bothThemes(block: (String, androidx.compose.material3.ColorScheme, BrandColors) -> Unit) {
        block("светлая", colorScheme(false), brandColors(false))
        block("тёмная", colorScheme(true), brandColors(true))
    }

    @Test fun mainTextIsComfortable() = bothThemes { t, s, _ ->
        val c = contrast(s.onBackground, s.background)
        assertTrue("$t: основной текст %.2f".format(c), c in 12.0..16.0)
        assertTrue("$t: фон не чисто белый и не чёрный", s.background != Color.White && s.background != Color.Black)
        assertTrue("$t: плитки не чисто белые и не чёрные", s.surface != Color.White && s.surface != Color.Black)
    }

    @Test fun textPairsPassAA() = bothThemes { t, s, b ->
        val bgs = listOf("фон" to s.background, "плитка" to s.surface, "плитка 2" to b.surface2)
        for ((n, bg) in bgs) {
            check("$t: текст на «$n»", s.onSurface, bg, 4.5)
            check("$t: приглушённый текст на «$n»", b.muted, bg, 4.5)
            check("$t: акцентный текст на «$n»", s.primary, bg, 4.5)
        }
        for ((n, fg) in listOf("ok" to b.ok, "warn" to b.warn, "bad" to b.bad, "info" to b.info)) {
            check("$t: статус $n на фоне", fg, s.background, 4.5)
            check("$t: статус $n на плитке", fg, s.surface, 4.5)
        }
        check("$t: кнопка primary", s.onPrimary, s.primary, 4.5)
        check("$t: главная кнопка (лаванда)", s.onPrimaryContainer, s.primaryContainer, 4.5)
        check("$t: выбранный чип", s.onSecondaryContainer, s.secondaryContainer, 4.5)
        check("$t: мятный контейнер", s.onTertiaryContainer, s.tertiaryContainer, 4.5)
        check("$t: ошибка", s.onError, s.error, 4.5)
    }

    @Test fun pastelTilesUseDarkText() = bothThemes { t, _, b ->
        check("$t: лавандовая плитка", b.onLavender, b.lavender, 4.5)
        check("$t: мятная плитка", b.onMint, b.mint, 4.5)
        check("$t: персиковая плитка", b.onPeach, b.peach, 4.5)
        check("$t: промо", b.promoText, b.promo, 4.5)
    }

    @Test fun iconsAreVisible() = bothThemes { t, s, b ->
        check("$t: сердечко избранного", b.accent2, s.surface, 3.0)
        check("$t: иконка на лавандовой плитке", b.lavender, b.onLavender, 3.0)
    }

    @Test fun darkAccentsAreMuted() {
        // В тёмной теме пастельные плитки темнее светлых, чтобы не «светились».
        assertTrue(brandColors(true).mint.luminance() < brandColors(false).mint.luminance())
        assertTrue(brandColors(true).peach.luminance() < brandColors(false).peach.luminance())
    }
}
