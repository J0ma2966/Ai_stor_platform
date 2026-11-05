package uz.boutique.core.util

import java.time.OffsetDateTime
import java.time.ZoneId
import java.time.format.DateTimeFormatter

object Money {
    private const val NBSP = ' '

    /** Деньги приходят в тийинах (1 сум = 100 тийин). Показываем в сумах без копеек, разряды — пробелом. */
    fun format(tiyin: Long, lang: String): String {
        val sum = Math.round(tiyin / 100.0)
        val digits = kotlin.math.abs(sum).toString()
        val grouped = StringBuilder()
        digits.forEachIndexed { i, ch ->
            if (i > 0 && (digits.length - i) % 3 == 0) grouped.append(NBSP)
            grouped.append(ch)
        }
        val sign = if (sum < 0) "-" else ""
        return "$sign$grouped$NBSP${if (lang == "uz") "so‘m" else "сум"}"
    }

    /** «1 500 000» → 150000000 тийин; пусто, ноль или мусор → null. */
    fun parseSumToTiyin(input: String): Long? {
        val digits = input.filter { !it.isWhitespace() && it != NBSP }
        if (digits.isEmpty() || digits.length > 12 || !digits.all { it.isDigit() }) return null
        val sum = digits.toLong()
        return if (sum <= 0) null else sum * 100
    }
}

object Phone {
    /**
     * Узбекский номер в формате E.164. Принимает «+998 90 123-45-67», «998901234567», «901234567».
     * Возвращает null, если это не номер Узбекистана.
     */
    fun normalizeUz(input: String): String? {
        val d = input.filter { it.isDigit() }
        val national = when {
            d.length == 12 && d.startsWith("998") -> d.substring(3)
            d.length == 9 -> d
            else -> return null
        }
        return "+998$national"
    }

    /** +998901234567 → «+998 90 123 45 67». Чужой формат возвращается как есть. */
    fun display(e164: String?): String {
        if (e164 == null) return ""
        if (!Regex("^\\+998\\d{9}$").matches(e164)) return e164
        val n = e164.substring(4)
        return "+998 ${n.substring(0, 2)} ${n.substring(2, 5)} ${n.substring(5, 7)} ${n.substring(7)}"
    }
}

object Dates {
    private val TASHKENT: ZoneId = ZoneId.of("Asia/Tashkent")
    private val DATE_TIME = DateTimeFormatter.ofPattern("dd.MM.yyyy, HH:mm")
    private val DATE = DateTimeFormatter.ofPattern("dd.MM.yyyy")

    /** RFC 3339 → «05.11.2026, 14:30» по Ташкенту (как на сайте). Неразборчивая строка — как есть. */
    fun dateTime(iso: String): String = runCatching {
        OffsetDateTime.parse(iso).atZoneSameInstant(TASHKENT).format(DATE_TIME)
    }.getOrDefault(iso)

    fun date(iso: String): String = runCatching {
        OffsetDateTime.parse(iso).atZoneSameInstant(TASHKENT).format(DATE)
    }.getOrDefault(iso)
}

object Rating {
    /** 4.33 → «4,3» (как на сайте, одинаково для ru и uz). */
    fun format(avg: Double): String = String.format(java.util.Locale.ROOT, "%.1f", avg).replace('.', ',')
}
