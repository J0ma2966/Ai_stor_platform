package uz.boutique.ui

import android.content.Context
import androidx.appcompat.app.AppCompatDelegate
import androidx.compose.ui.test.assertIsSelected
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class AppThemeTest {
    @get:Rule val rule = createComposeRule()
    private val context: Context get() = ApplicationProvider.getApplicationContext()

    @Before fun clean() {
        context.getSharedPreferences("ui", Context.MODE_PRIVATE).edit().clear().commit()
        AppCompatDelegate.setDefaultNightMode(AppCompatDelegate.MODE_NIGHT_FOLLOW_SYSTEM)
    }

    @After fun reset() = clean()

    @Test fun defaultFollowsSystem() {
        assertEquals(AppTheme.SYSTEM, AppTheme.current(context))
        AppTheme.apply(context)
        assertEquals(AppCompatDelegate.MODE_NIGHT_FOLLOW_SYSTEM, AppCompatDelegate.getDefaultNightMode())
    }

    @Test fun choiceIsSavedAndApplied() {
        AppTheme.set(context, AppTheme.DARK)
        assertEquals(AppTheme.DARK, AppTheme.current(context))
        assertEquals(AppCompatDelegate.MODE_NIGHT_YES, AppCompatDelegate.getDefaultNightMode())

        AppTheme.set(context, AppTheme.LIGHT)
        assertEquals(AppCompatDelegate.MODE_NIGHT_NO, AppCompatDelegate.getDefaultNightMode())

        // после перезапуска приложения выбор восстанавливается
        AppCompatDelegate.setDefaultNightMode(AppCompatDelegate.MODE_NIGHT_FOLLOW_SYSTEM)
        AppTheme.apply(context)
        assertEquals(AppCompatDelegate.MODE_NIGHT_NO, AppCompatDelegate.getDefaultNightMode())
    }

    @Test fun unknownStoredValueFallsBackToSystem() {
        context.getSharedPreferences("ui", Context.MODE_PRIVATE).edit().putString("theme", "neon").commit()
        assertEquals(AppTheme.SYSTEM, AppTheme.current(context))
    }

    @Test fun pickerSwitchesTheme() {
        rule.setContent { BoutiqueTheme(dark = false) { ThemePicker() } }
        rule.onNodeWithText("Тема").assertExists()
        rule.onNodeWithTag("theme_system").assertIsSelected()
        rule.onNodeWithTag("theme_dark").performClick()
        rule.onNodeWithTag("theme_dark").assertIsSelected()
        assertEquals(AppTheme.DARK, AppTheme.current(context))
        assertEquals(AppCompatDelegate.MODE_NIGHT_YES, AppCompatDelegate.getDefaultNightMode())
    }

    @Test fun toggleButtonGoesDarkFromLight() {
        rule.setContent { BoutiqueTheme(dark = false) { ThemeToggleButton() } }
        rule.onNodeWithTag("theme_toggle").performClick()
        assertEquals(AppTheme.DARK, AppTheme.current(context))
    }
}
