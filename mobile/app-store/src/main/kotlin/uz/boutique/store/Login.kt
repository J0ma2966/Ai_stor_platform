package uz.boutique.store

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import uz.boutique.ui.AppScaffold
import uz.boutique.ui.Brand
import uz.boutique.ui.ErrorBanner
import uz.boutique.ui.FormField
import uz.boutique.ui.LanguagePicker
import uz.boutique.ui.LocalGraph
import uz.boutique.ui.PasswordField
import uz.boutique.ui.PrimaryButton
import uz.boutique.ui.ServerDialog
import uz.boutique.ui.rememberAction
import uz.boutique.ui.R as UiR

/** Вход сотрудника магазина по логину и паролю, которые выдал администратор платформы. */
@Composable
fun StoreLoginScreen(onRestart: () -> Unit) {
    val graph = LocalGraph.current
    val api = graph.api
    var login by rememberSaveable { mutableStateOf("") }
    var password by rememberSaveable { mutableStateOf("") }
    var showServer by rememberSaveable { mutableStateOf(false) }
    val action = rememberAction()
    AppScaffold(title = stringResource(UiR.string.store_login_title)) { pad ->
        Column(
            Modifier.padding(pad).fillMaxSize().imePadding().verticalScroll(rememberScrollState()).padding(20.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Text(stringResource(UiR.string.store_login_hint), color = Brand.colors.muted)
            FormField(login, { login = it.trim().take(64) }, stringResource(UiR.string.store_login), keyboardType = KeyboardType.Ascii, modifier = Modifier.testTag("login"))
            PasswordField(password, { password = it }, stringResource(UiR.string.login_password), modifier = Modifier.testTag("password"))
            ErrorBanner(action.error)
            PrimaryButton(
                stringResource(UiR.string.login_sign_in),
                busy = action.busy,
                enabled = login.length >= 3 && password.isNotEmpty(),
                modifier = Modifier.testTag("sign_in"),
                onClick = { action.run { api.auth.staffLogin(login, password) } },
            )
            Spacer(Modifier.height(8.dp))
            LanguagePicker()
            if (graph.allowServerOverride) {
                TextButton(onClick = { showServer = true }) {
                    Text(stringResource(UiR.string.app_server) + ": " + graph.baseUrl, style = MaterialTheme.typography.bodySmall)
                }
            }
        }
    }
    if (showServer) ServerDialog(onDismiss = { showServer = false }, onSaved = { showServer = false; onRestart() })
}
