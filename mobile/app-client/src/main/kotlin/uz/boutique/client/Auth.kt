package uz.boutique.client

import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Checkbox
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import uz.boutique.core.util.Passwords
import uz.boutique.core.util.Phone
import uz.boutique.ui.AppLang
import uz.boutique.ui.AppScaffold
import uz.boutique.ui.Brand
import uz.boutique.ui.ErrorBanner
import uz.boutique.ui.FormField
import uz.boutique.ui.InfoBanner
import uz.boutique.ui.LocalGraph
import uz.boutique.ui.PasswordField
import uz.boutique.ui.PrimaryButton
import uz.boutique.ui.ServerDialog
import uz.boutique.ui.currentLang
import uz.boutique.ui.fieldErrorText
import uz.boutique.ui.fields
import uz.boutique.ui.rememberAction
import uz.boutique.ui.R as UiR

@Composable
private fun FormColumn(pad: androidx.compose.foundation.layout.PaddingValues, content: @Composable () -> Unit) {
    Column(
        Modifier.padding(pad).fillMaxSize().imePadding().verticalScroll(rememberScrollState()).padding(horizontal = 20.dp, vertical = 12.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) { content() }
}

/** Вход покупателя по номеру и паролю (SMS — только для регистрации и сброса пароля). */
@Composable
fun LoginScreen(
    onBack: () -> Unit,
    onSms: () -> Unit,
    onForgot: () -> Unit,
    onDone: (next: String?) -> Unit,
    onRestart: () -> Unit,
) {
    val graph = LocalGraph.current
    val api = graph.api
    var phone by rememberSaveable { mutableStateOf("+998 ") }
    var password by rememberSaveable { mutableStateOf("") }
    var phoneError by rememberSaveable { mutableStateOf(false) }
    var showServer by rememberSaveable { mutableStateOf(false) }
    val action = rememberAction()

    AppScaffold(title = stringResource(UiR.string.login_title), onBack = onBack) { pad ->
        FormColumn(pad) {
            FormField(
                phone, { phone = it; phoneError = false }, stringResource(UiR.string.login_phone),
                keyboardType = KeyboardType.Phone,
                error = if (phoneError) stringResource(UiR.string.login_phone_invalid) else null,
                modifier = Modifier.testTag("phone"),
            )
            PasswordField(password, { password = it }, stringResource(UiR.string.login_password), modifier = Modifier.testTag("password"))
            ErrorBanner(action.error)
            PrimaryButton(
                stringResource(UiR.string.login_sign_in),
                busy = action.busy,
                enabled = password.isNotEmpty(),
                modifier = Modifier.testTag("sign_in"),
                onClick = {
                    val p = Phone.normalizeUz(phone)
                    if (p == null) {
                        phoneError = true
                    } else {
                        action.run {
                            api.auth.passwordLogin(p, password)
                            onDone(nextAfterLogin(api))
                        }
                    }
                },
            )
            TextButton(onClick = onForgot, modifier = Modifier.fillMaxWidth()) { Text(stringResource(UiR.string.login_forgot)) }
            TextButton(onClick = onSms, modifier = Modifier.fillMaxWidth().testTag("to_sms")) { Text(stringResource(UiR.string.login_to_sms)) }
            Text(stringResource(UiR.string.login_no_password_hint), style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted)
            if (graph.allowServerOverride) {
                TextButton(onClick = { showServer = true }) { Text(stringResource(UiR.string.app_server) + ": " + graph.baseUrl) }
            }
        }
    }
    if (showServer) ServerDialog(onDismiss = { showServer = false }, onSaved = { showServer = false; onRestart() })
}

/** Регистрация/вход по SMS-коду или восстановление пароля (код + новый пароль). */
@Composable
fun SmsScreen(reset: Boolean, onBack: () -> Unit, onDone: (next: String?) -> Unit) {
    val api = LocalGraph.current.api
    var phone by rememberSaveable { mutableStateOf("+998 ") }
    var sentTo by rememberSaveable { mutableStateOf<String?>(null) }
    var code by rememberSaveable { mutableStateOf("") }
    var password by rememberSaveable { mutableStateOf("") }
    var repeat by rememberSaveable { mutableStateOf("") }
    var phoneError by rememberSaveable { mutableStateOf(false) }
    var pwError by rememberSaveable { mutableStateOf<String?>(null) }
    val send = rememberAction()
    val verify = rememberAction()
    val tooShort = stringResource(UiR.string.password_hint)
    val mismatch = stringResource(UiR.string.password_mismatch)

    val title = stringResource(if (reset) UiR.string.login_reset_title else UiR.string.login_sms_title)
    AppScaffold(title = title, onBack = onBack) { pad ->
        FormColumn(pad) {
            InfoBanner(stringResource(if (reset) UiR.string.login_reset_hint else UiR.string.login_sms_hint))
            FormField(
                phone, { phone = it; phoneError = false }, stringResource(UiR.string.login_phone),
                keyboardType = KeyboardType.Phone,
                enabled = sentTo == null,
                error = if (phoneError) stringResource(UiR.string.login_phone_invalid) else fieldErrorText(send.error.fields()["phone"]),
                modifier = Modifier.testTag("phone"),
            )
            if (sentTo == null) {
                ErrorBanner(send.error)
                PrimaryButton(stringResource(UiR.string.login_send_code), busy = send.busy, modifier = Modifier.testTag("send_code"), onClick = {
                    val p = Phone.normalizeUz(phone)
                    if (p == null) phoneError = true else send.run {
                        api.auth.requestOtp(p)
                        sentTo = p
                    }
                })
            } else {
                Text(stringResource(UiR.string.login_code_sent, Phone.display(sentTo)), color = Brand.colors.muted)
                FormField(
                    code, { v -> code = v.filter { it.isDigit() }.take(6) }, stringResource(UiR.string.login_code),
                    keyboardType = KeyboardType.NumberPassword,
                    imeAction = if (reset) ImeAction.Next else ImeAction.Done,
                    modifier = Modifier.testTag("code"),
                )
                if (reset) {
                    PasswordField(password, { password = it; pwError = null }, stringResource(UiR.string.password_new), hint = tooShort, error = pwError, imeAction = ImeAction.Next)
                    PasswordField(repeat, { repeat = it; pwError = null }, stringResource(UiR.string.password_repeat))
                }
                ErrorBanner(verify.error ?: send.error)
                PrimaryButton(
                    stringResource(if (reset) UiR.string.login_reset_submit else UiR.string.login_sign_in),
                    busy = verify.busy,
                    enabled = code.length == 6,
                    modifier = Modifier.testTag("verify"),
                    onClick = {
                        val p = sentTo!!
                        if (reset && Passwords.check(password) != null) {
                            pwError = tooShort
                        } else if (reset && password != repeat) {
                            pwError = mismatch
                        } else {
                            verify.run {
                                if (reset) api.auth.resetPassword(p, code, password) else api.auth.verifyOtp(p, code)
                                onDone(nextAfterLogin(api))
                            }
                        }
                    },
                )
                TextButton(onClick = { send.run { api.auth.requestOtp(sentTo!!) } }, enabled = !send.busy, modifier = Modifier.fillMaxWidth()) {
                    Text(stringResource(UiR.string.login_resend))
                }
            }
        }
    }
}

/** Завершение регистрации: пол и согласие на обработку ПД (обязательно по закону). */
@Composable
fun WelcomeScreen(onDone: (next: String?) -> Unit) {
    val graph = LocalGraph.current
    val api = graph.api
    val context = LocalContext.current
    val lang = currentLang()
    var gender by rememberSaveable { mutableStateOf<String?>(null) }
    var consent by rememberSaveable { mutableStateOf(false) }
    val action = rememberAction()
    AppScaffold(title = stringResource(UiR.string.welcome_title)) { pad ->
        FormColumn(pad) {
            Text(stringResource(UiR.string.welcome_text))
            Text(stringResource(UiR.string.welcome_gender), style = MaterialTheme.typography.titleSmall)
            for ((value, label) in listOf("female" to UiR.string.gender_female, "male" to UiR.string.gender_male)) {
                Row(
                    Modifier.fillMaxWidth().selectable(selected = gender == value, onClick = { gender = value }).testTag("gender_$value"),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    RadioButton(selected = gender == value, onClick = { gender = value })
                    Text(stringResource(label))
                }
            }
            Row(verticalAlignment = Alignment.Top) {
                Checkbox(checked = consent, onCheckedChange = { consent = it }, modifier = Modifier.testTag("consent"))
                Text(stringResource(UiR.string.welcome_consent), modifier = Modifier.padding(top = 12.dp))
            }
            TextButton(onClick = {
                val prefix = if (lang == "uz") "/uz" else ""
                context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(graph.siteUrl + prefix + "/legal/privacy")))
            }) { Text(stringResource(UiR.string.welcome_documents)) }
            ErrorBanner(action.error)
            PrimaryButton(stringResource(UiR.string.welcome_submit), busy = action.busy, enabled = gender != null && consent, onClick = {
                action.run {
                    val me = api.auth.completeRegistration(gender!!, AppLang.current())
                    onDone(if (!me.hasPassword) Routes.password("set") else null)
                }
            })
        }
    }
}

/** Первый пароль после SMS-регистрации или смена пароля из профиля. */
@Composable
fun PasswordScreen(change: Boolean, onBack: (() -> Unit)?, onDone: () -> Unit) {
    val api = LocalGraph.current.api
    val context = LocalContext.current
    var current by rememberSaveable { mutableStateOf("") }
    var password by rememberSaveable { mutableStateOf("") }
    var repeat by rememberSaveable { mutableStateOf("") }
    var localError by rememberSaveable { mutableStateOf<String?>(null) }
    val action = rememberAction()
    val tooShort = stringResource(UiR.string.password_hint)
    val mismatch = stringResource(UiR.string.password_mismatch)
    val changed = stringResource(UiR.string.password_changed)
    AppScaffold(title = stringResource(if (change) UiR.string.password_change_title else UiR.string.password_set_title), onBack = onBack) { pad ->
        FormColumn(pad) {
            Text(stringResource(if (change) UiR.string.password_change_text else UiR.string.password_set_text))
            if (change) {
                PasswordField(current, { current = it }, stringResource(UiR.string.password_current), imeAction = ImeAction.Next)
            }
            PasswordField(password, { password = it; localError = null }, stringResource(UiR.string.password_new), hint = tooShort, error = localError, imeAction = ImeAction.Next, modifier = Modifier.testTag("new_password"))
            PasswordField(repeat, { repeat = it; localError = null }, stringResource(UiR.string.password_repeat), modifier = Modifier.testTag("repeat_password"))
            ErrorBanner(action.error)
            PrimaryButton(
                stringResource(if (change) UiR.string.password_change_submit else UiR.string.password_set_submit),
                busy = action.busy,
                modifier = Modifier.testTag("save_password"),
                onClick = {
                    when {
                        Passwords.check(password) != null -> localError = tooShort
                        password != repeat -> localError = mismatch
                        else -> action.run {
                            api.auth.setPassword(password, if (change) current else null)
                            android.widget.Toast.makeText(context, changed, android.widget.Toast.LENGTH_SHORT).show()
                            onDone()
                        }
                    }
                },
            )
        }
    }
}
