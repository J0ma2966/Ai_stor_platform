package uz.boutique.client

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import uz.boutique.ui.BoutiqueTheme

@RunWith(AndroidJUnit4::class)
class HomeHeroTest {
    @get:Rule val rule = createComposeRule()

    private fun show(dark: Boolean = false, showFitting: Boolean = true, fitting: Int = 0, fav: Int = 0, clicks: MutableList<String> = mutableListOf()) {
        rule.setContent {
            BoutiqueTheme(dark = dark) {
                HomeHero(
                    showFitting = showFitting, fittingCount = fitting, favCount = fav, heroImage = null,
                    onFitting = { clicks += "fitting" }, onNew = { clicks += "new" }, onFavorites = { clicks += "fav" },
                )
            }
        }
    }

    @Test fun heroLeadsToFittingRoom() {
        val clicks = mutableListOf<String>()
        show(clicks = clicks)
        rule.onNodeWithText("Найдите свой стиль.").assertIsDisplayed()
        rule.onNodeWithText("ВИРТУАЛЬНАЯ ПРИМЕРОЧНАЯ").assertIsDisplayed()
        rule.onNodeWithText("ПРИМЕРИТЬ С AI").assertIsDisplayed()
        rule.onNodeWithTag("home_fitting").performClick()
        assertEquals(listOf("fitting"), clicks)
    }

    @Test fun panelsWork() {
        val clicks = mutableListOf<String>()
        show(fav = 3, clicks = clicks)
        rule.onNodeWithText("Сохранено: 3").assertIsDisplayed()
        rule.onNodeWithTag("home_new").performClick()
        rule.onNodeWithTag("home_favorites").performClick()
        assertEquals(listOf("new", "fav"), clicks)
    }

    @Test fun fittingCountIsShown() {
        show(fitting = 2)
        rule.onNodeWithText("В примерочной: 2").assertIsDisplayed()
    }

    @Test fun heroShowsNewArrivalsWhenTryonIsOff() {
        val clicks = mutableListOf<String>()
        show(showFitting = false, clicks = clicks)
        rule.onNodeWithTag("home_fitting").assertDoesNotExist()
        rule.onNodeWithText("СМОТРЕТЬ НОВИНКИ").assertIsDisplayed()
        rule.onNodeWithTag("home_hero_new").performClick()
        assertEquals(listOf("new"), clicks)
    }

    @Test fun rendersInDarkTheme() {
        show(dark = true)
        rule.onNodeWithTag("home_fitting").assertIsDisplayed()
        rule.onNodeWithTag("home_favorites").assertIsDisplayed()
    }
}
