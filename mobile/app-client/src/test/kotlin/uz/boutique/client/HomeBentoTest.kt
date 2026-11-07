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
class HomeBentoTest {
    @get:Rule val rule = createComposeRule()

    private fun show(dark: Boolean = false, showFitting: Boolean = true, fitting: Int = 0, fav: Int = 0, clicks: MutableList<String> = mutableListOf()) {
        rule.setContent {
            BoutiqueTheme(dark = dark) {
                HomeBento(
                    showFitting = showFitting, fittingCount = fitting, favCount = fav,
                    onFitting = { clicks += "fitting" }, onNew = { clicks += "new" }, onFavorites = { clicks += "fav" },
                )
            }
        }
    }

    @Test fun bigTileLeadsToFittingRoom() {
        val clicks = mutableListOf<String>()
        show(clicks = clicks)
        rule.onNodeWithText("ПРИМЕРЬ НА СЕБЕ").assertIsDisplayed()
        rule.onNodeWithText("Открыть примерочную").assertIsDisplayed()
        rule.onNodeWithTag("home_fitting").performClick()
        assertEquals(listOf("fitting"), clicks)
    }

    @Test fun otherTilesWork() {
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

    @Test fun noFittingTileWhenTryonIsOff() {
        show(showFitting = false)
        rule.onNodeWithTag("home_fitting").assertDoesNotExist()
        rule.onNodeWithTag("home_new").assertIsDisplayed()
    }

    @Test fun rendersInDarkTheme() {
        show(dark = true)
        rule.onNodeWithTag("home_fitting").assertIsDisplayed()
        rule.onNodeWithTag("home_favorites").assertIsDisplayed()
    }
}
