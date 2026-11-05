package uz.boutique.core

import uz.boutique.core.util.Dates
import uz.boutique.core.util.Money
import uz.boutique.core.util.Phone
import uz.boutique.core.util.Rating
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class FormatTest {
    @Test fun money() {
        assertEquals("1 500 000 сум", Money.format(150_000_000, "ru"))
        assertEquals("990 so‘m", Money.format(99_000, "uz"))
        assertEquals("0 сум", Money.format(0, "ru"))
        assertEquals("1 000 сум", Money.format(99_950, "ru")) // округление тийинов
    }

    @Test fun parseSum() {
        assertEquals(150_000_000L, Money.parseSumToTiyin("1 500 000"))
        assertEquals(100L, Money.parseSumToTiyin("1"))
        assertNull(Money.parseSumToTiyin(""))
        assertNull(Money.parseSumToTiyin("0"))
        assertNull(Money.parseSumToTiyin("12,5"))
        assertNull(Money.parseSumToTiyin("-5"))
    }

    @Test fun phone() {
        assertEquals("+998901234567", Phone.normalizeUz("+998 90 123-45-67"))
        assertEquals("+998901234567", Phone.normalizeUz("998901234567"))
        assertEquals("+998901234567", Phone.normalizeUz("90 123 45 67"))
        assertNull(Phone.normalizeUz("+7 900 123 45 67"))
        assertNull(Phone.normalizeUz("12345"))
        assertEquals("+998 90 123 45 67", Phone.display("+998901234567"))
        assertEquals("+12025550100", Phone.display("+12025550100"))
    }

    @Test fun dates() {
        assertEquals("05.11.2026, 23:30", Dates.dateTime("2026-11-05T18:30:00Z"))
        assertEquals("06.11.2026", Dates.date("2026-11-05T19:30:00.123456Z"))
        assertEquals("not a date", Dates.dateTime("not a date"))
    }

    @Test fun rating() {
        assertEquals("4,3", Rating.format(4.33))
        assertEquals("5,0", Rating.format(5.0))
    }
}
