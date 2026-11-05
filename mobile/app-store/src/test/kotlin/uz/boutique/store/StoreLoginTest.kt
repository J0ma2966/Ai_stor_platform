package uz.boutique.store

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import uz.boutique.core.api.AuthState
import uz.boutique.core.api.MemoryStore
import uz.boutique.core.model.Role
import uz.boutique.ui.AppGraph
import uz.boutique.ui.BoutiqueTheme
import uz.boutique.ui.LocalGraph

@RunWith(AndroidJUnit4::class)
class StoreLoginTest {
    @get:Rule val rule = createComposeRule()
    private val server = MockWebServer()

    @After fun stop() = server.shutdown()

    private fun graph() = AppGraph(
        ApplicationProvider.getApplicationContext(), Role.STORE, server.url("/").toString(), false, "https://example.uz", "test", MemoryStore(),
    )

    private fun session(role: String) = MockResponse().setHeader("Content-Type", "application/json")
        .addHeader("Set-Cookie", "rt=r; Path=/v1/auth; Max-Age=600; HttpOnly")
        .setBody("""{"data":{"access_token":"t","expires_at":"2026-01-01T00:00:00Z","user":{"id":"u","role":"$role"}}}""")

    private fun login(g: AppGraph) {
        rule.setContent {
            CompositionLocalProvider(LocalGraph provides g) { BoutiqueTheme(dark = false) { StoreLoginScreen(onRestart = {}) } }
        }
        rule.onNodeWithTag("login").performTextInput("butik1")
        rule.onNodeWithTag("password").performTextInput("secret-password")
        rule.onNodeWithTag("sign_in").performClick()
    }

    @Test fun storeLogsIn() {
        server.enqueue(session("store"))
        val g = graph()
        login(g)
        rule.waitUntil(5_000) { g.api.session.state.value is AuthState.LoggedIn }
        val req = server.takeRequest()
        assertEquals("/v1/auth/login", req.path)
        assertEquals("""{"login":"butik1","password":"secret-password"}""", req.body.readUtf8())
    }

    @Test fun clientAccountIsRejected() {
        server.enqueue(session("client"))
        server.enqueue(MockResponse().setResponseCode(204)) // logout
        val g = graph()
        login(g)
        rule.waitUntil(5_000) { rule.onAllNodes(androidx.compose.ui.test.hasTestTag("error_banner")).fetchSemanticsNodes().isNotEmpty() }
        rule.onNodeWithTag("error_banner").assertExists()
        assertEquals(AuthState.LoggedOut, g.api.session.state.value)
    }
}
