package uz.boutique.client

import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.Send
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.ui.graphics.Color
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.delay
import uz.boutique.core.api.ApiException
import uz.boutique.core.model.AuthMethods
import uz.boutique.core.model.TelegramStart
import uz.boutique.ui.SecondaryButton
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

/**
 * Вход и регистрация покупателя: Telegram-бот, Google, почта (docs/AUTH.md).
 * Способы берутся с сервера (/v1/auth/methods). Вход по номеру и паролю
 * показывается, только если на сервере включён SMS-вход.
 */
@Composable
fun LoginScreen(
    onBack: () -> Unit,
    onSms: () -> Unit,
    onForgot: () -> Unit,
    onEmail: (mode: String) -> Unit,
    onDone: (next: String?) -> Unit,
    onRestart: () -> Unit,
) {
    val graph = LocalGraph.current
    val api = graph.api
    val context = LocalContext.current
    var methods by remember { mutableStateOf<AuthMethods?>(null) }
    var methodsError by remember { mutableStateOf<Throwable?>(null) }
    var reload by remember { mutableIntStateOf(0) }
    var showServer by rememberSaveable { mutableStateOf(false) }
    var tg by remember { mutableStateOf<TelegramStart?>(null) }
    val tgAction = rememberAction()
    val google = rememberAction()
    var googleUnavailable by rememberSaveable { mutableStateOf(false) }

    LaunchedEffect(reload) {
        methodsError = null
        try {
            methods = api.auth.methods()
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            methodsError = e
        }
    }
    // Ждём подтверждения в боте: опрос раз в 2 секунды, пока экран открыт.
    LaunchedEffect(tg) {
        val attempt = tg ?: return@LaunchedEffect
        while (true) {
            delay(2_000)
            try {
                if (api.auth.telegramPoll(attempt) != null) {
                    tg = null
                    onDone(nextAfterLogin(api))
                    return@LaunchedEffect
                }
            } catch (e: CancellationException) {
                throw e
            } catch (e: ApiException) {
                if (e.code == "telegram_login_expired" || e.code == "account_blocked" || e.code == ApiException.WRONG_APP) {
                    tg = null
                    tgAction.error = e
                    return@LaunchedEffect
                }
            } catch (_: Exception) {
                // нет сети — пробуем ещё раз, ссылка живёт 5 минут
            }
        }
    }

    AppScaffold(title = stringResource(UiR.string.login_title), onBack = onBack) { pad ->
        FormColumn(pad) {
            Text(stringResource(UiR.string.login_heading), style = MaterialTheme.typography.headlineSmall)
            Text(stringResource(UiR.string.login_subtitle), color = Brand.colors.muted)
            val m = methods
            if (m == null) {
                if (methodsError != null) {
                    ErrorBanner(methodsError)
                    SecondaryButton(stringResource(UiR.string.app_retry), onClick = { reload++ })
                } else {
                    Box(Modifier.fillMaxWidth().padding(24.dp), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
                }
            } else {
                if (m.telegram != null) {
                    val attempt = tg
                    if (attempt == null) {
                        Button(
                            onClick = {
                                tgAction.run {
                                    val a = api.auth.telegramStart()
                                    openUrl(context, a.url)
                                    tg = a
                                }
                            },
                            enabled = !tgAction.busy,
                            colors = ButtonDefaults.buttonColors(containerColor = TelegramBlue, contentColor = Color.White),
                            shape = RoundedCornerShape(50),
                            modifier = Modifier.fillMaxWidth().heightIn(min = 52.dp).testTag("telegram"),
                        ) {
                            Icon(Icons.AutoMirrored.Filled.Send, contentDescription = null, modifier = Modifier.size(20.dp))
                            Spacer(Modifier.width(10.dp))
                            Text(stringResource(UiR.string.login_telegram), style = MaterialTheme.typography.titleSmall)
                        }
                    } else {
                        TelegramWaitCard(onOpenAgain = { openUrl(context, attempt.url) }, onCancel = { tg = null })
                    }
                    ErrorBanner(tgAction.error)
                }
                if (m.google != null) {
                    OutlinedButton(
                        onClick = {
                            google.run {
                                val token = googleIdToken(context, m.google!!.clientId)
                                if (token == null) {
                                    googleUnavailable = true
                                } else if (token.isNotEmpty()) {
                                    api.auth.google(token)
                                    onDone(nextAfterLogin(api))
                                }
                            }
                        },
                        enabled = !google.busy,
                        shape = RoundedCornerShape(50),
                        modifier = Modifier.fillMaxWidth().heightIn(min = 52.dp).testTag("google"),
                    ) {
                        Text("G", style = MaterialTheme.typography.titleMedium, color = GoogleBlue)
                        Spacer(Modifier.width(10.dp))
                        Text(stringResource(UiR.string.login_google), style = MaterialTheme.typography.titleSmall)
                    }
                    if (googleUnavailable) Text(stringResource(UiR.string.login_google_unavailable), color = Brand.colors.muted, style = MaterialTheme.typography.bodySmall)
                    ErrorBanner(google.error)
                }
                if (m.email) {
                    if (m.telegram != null || m.google != null) OrDivider()
                    PrimaryButton(stringResource(UiR.string.login_email), onClick = { onEmail("login") }, modifier = Modifier.testTag("email_login"))
                    TextButton(onClick = { onEmail("register") }, modifier = Modifier.fillMaxWidth().testTag("email_register")) {
                        Text(stringResource(UiR.string.login_email_register))
                    }
                }
                if (m.sms) PhonePasswordSection(onSms = onSms, onForgot = onForgot, onDone = onDone)
                Text(stringResource(UiR.string.login_legal), style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted)
            }
            if (graph.allowServerOverride) {
                TextButton(onClick = { showServer = true }) { Text(stringResource(UiR.string.app_server) + ": " + graph.baseUrl) }
            }
        }
    }
    if (showServer) ServerDialog(onDismiss = { showServer = false }, onSaved = { showServer = false; onRestart() })
}

private val TelegramBlue = Color(0xFF229ED9)
private val GoogleBlue = Color(0xFF4285F4)

private fun openUrl(context: Context, url: String) {
    try {
        context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
    } catch (_: ActivityNotFoundException) {
        // нет ни Telegram, ни браузера — ссылку можно открыть кнопкой «ещё раз» после установки
    }
}

@Composable
private fun OrDivider() {
    Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.fillMaxWidth().padding(vertical = 4.dp)) {
        HorizontalDivider(Modifier.weight(1f), color = Brand.colors.line)
        Text(stringResource(UiR.string.login_or), color = Brand.colors.muted, modifier = Modifier.padding(horizontal = 12.dp), style = MaterialTheme.typography.bodySmall)
        HorizontalDivider(Modifier.weight(1f), color = Brand.colors.line)
    }
}

@Composable
private fun TelegramWaitCard(onOpenAgain: () -> Unit, onCancel: () -> Unit) {
    Surface(color = TelegramBlue.copy(alpha = 0.10f), shape = RoundedCornerShape(16.dp), modifier = Modifier.fillMaxWidth().testTag("telegram_wait")) {
        Row(Modifier.padding(16.dp), horizontalArrangement = Arrangement.spacedBy(14.dp)) {
            CircularProgressIndicator(color = TelegramBlue, strokeWidth = 3.dp, modifier = Modifier.size(22.dp))
            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Text(stringResource(UiR.string.login_tg_wait_title), style = MaterialTheme.typography.titleSmall)
                Text(stringResource(UiR.string.login_tg_wait_text), style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted)
                Row {
                    TextButton(onClick = onOpenAgain) { Text(stringResource(UiR.string.login_tg_open_again)) }
                    TextButton(onClick = onCancel) { Text(stringResource(UiR.string.app_cancel)) }
                }
            }
        }
    }
}

/** Вход по номеру и паролю — только при включённом на сервере SMS-входе. */
@Composable
private fun PhonePasswordSection(onSms: () -> Unit, onForgot: () -> Unit, onDone: (next: String?) -> Unit) {
    val api = LocalGraph.current.api
    var phone by rememberSaveable { mutableStateOf("+998 ") }
    var password by rememberSaveable { mutableStateOf("") }
    var phoneError by rememberSaveable { mutableStateOf(false) }
    val action = rememberAction()
    OrDivider()
    Text(stringResource(UiR.string.login_phone_section), style = MaterialTheme.typography.titleSmall)
    FormField(
        phone, { phone = it; phoneError = false }, stringResource(UiR.string.login_phone),
        keyboardType = KeyboardType.Phone,
        error = if (phoneError) stringResource(UiR.string.login_phone_invalid) else null,
        modifier = Modifier.testTag("phone"),
    )
    PasswordField(password, { password = it }, stringResource(UiR.string.login_password), modifier = Modifier.testTag("password"))
    ErrorBanner(action.error)
    SecondaryButton(
        stringResource(UiR.string.login_sign_in),
        enabled = password.isNotEmpty() && !action.busy,
        modifier = Modifier.fillMaxWidth().testTag("sign_in"),
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
}

/** Почта: вход по паролю, регистрация и восстановление пароля по коду из письма. */
@Composable
fun EmailScreen(initialMode: String, onBack: () -> Unit, onDone: (next: String?) -> Unit) {
    val api = LocalGraph.current.api
    val lang = currentLang()
    var mode by rememberSaveable { mutableStateOf(initialMode) }
    var email by rememberSaveable { mutableStateOf("") }
    var sentTo by rememberSaveable { mutableStateOf<String?>(null) }
    var code by rememberSaveable { mutableStateOf("") }
    var password by rememberSaveable { mutableStateOf("") }
    var repeat by rememberSaveable { mutableStateOf("") }
    var emailError by rememberSaveable { mutableStateOf(false) }
    var pwError by rememberSaveable { mutableStateOf<String?>(null) }
    val action = rememberAction()
    val tooShort = stringResource(UiR.string.password_hint)
    val mismatch = stringResource(UiR.string.password_mismatch)

    fun switch(m: String) {
        mode = m; sentTo = null; code = ""; password = ""; repeat = ""; pwError = null; action.error = null
    }

    val title = when (mode) {
        "register" -> UiR.string.email_register_title
        "reset" -> UiR.string.email_reset_title
        else -> UiR.string.email_login_title
    }
    AppScaffold(title = stringResource(title), onBack = onBack) { pad ->
        FormColumn(pad) {
            if (mode != "login" && sentTo == null) {
                InfoBanner(stringResource(if (mode == "register") UiR.string.email_register_hint else UiR.string.email_reset_hint))
            }
            FormField(
                email, { email = it.trim(); emailError = false }, stringResource(UiR.string.email_address),
                keyboardType = KeyboardType.Email,
                enabled = sentTo == null,
                error = if (emailError) stringResource(UiR.string.email_invalid) else fieldErrorText(action.error.fields()["email"]),
                modifier = Modifier.testTag("email"),
            )
            val addr = email.trim().lowercase()
            val valid = EMAIL_RE.matches(addr)
            when {
                mode == "login" -> {
                    PasswordField(password, { password = it }, stringResource(UiR.string.login_password), modifier = Modifier.testTag("password"))
                    ErrorBanner(action.error)
                    PrimaryButton(stringResource(UiR.string.login_sign_in), busy = action.busy, enabled = password.isNotEmpty(), modifier = Modifier.testTag("sign_in"), onClick = {
                        if (!valid) emailError = true else action.run {
                            api.auth.emailLogin(addr, password)
                            onDone(nextAfterLogin(api))
                        }
                    })
                    TextButton(onClick = { switch("register") }, modifier = Modifier.fillMaxWidth().testTag("to_register")) { Text(stringResource(UiR.string.email_to_register)) }
                    TextButton(onClick = { switch("reset") }, modifier = Modifier.fillMaxWidth()) { Text(stringResource(UiR.string.login_forgot)) }
                }
                sentTo == null -> {
                    ErrorBanner(action.error)
                    PrimaryButton(stringResource(UiR.string.login_send_code), busy = action.busy, modifier = Modifier.testTag("send_code"), onClick = {
                        if (!valid) emailError = true else action.run {
                            api.auth.sendEmailCode(addr, mode, lang)
                            sentTo = addr
                        }
                    })
                    TextButton(onClick = { switch("login") }, modifier = Modifier.fillMaxWidth()) { Text(stringResource(UiR.string.email_to_login)) }
                }
                else -> {
                    Text(stringResource(UiR.string.email_code_sent, sentTo!!), color = Brand.colors.muted)
                    FormField(
                        code, { v -> code = v.filter { it.isDigit() }.take(6) }, stringResource(UiR.string.email_code),
                        keyboardType = KeyboardType.NumberPassword, imeAction = ImeAction.Next, modifier = Modifier.testTag("code"),
                    )
                    PasswordField(password, { password = it; pwError = null }, stringResource(UiR.string.password_new), hint = tooShort, error = pwError, imeAction = ImeAction.Next, modifier = Modifier.testTag("new_password"))
                    PasswordField(repeat, { repeat = it; pwError = null }, stringResource(UiR.string.password_repeat), modifier = Modifier.testTag("repeat_password"))
                    ErrorBanner(action.error)
                    PrimaryButton(
                        stringResource(if (mode == "register") UiR.string.email_register_submit else UiR.string.login_reset_submit),
                        busy = action.busy, enabled = code.length == 6, modifier = Modifier.testTag("submit"),
                        onClick = {
                            when {
                                Passwords.check(password) != null -> pwError = tooShort
                                password != repeat -> pwError = mismatch
                                else -> action.run {
                                    if (mode == "register") api.auth.emailRegister(sentTo!!, code, password) else api.auth.emailReset(sentTo!!, code, password)
                                    onDone(nextAfterLogin(api))
                                }
                            }
                        },
                    )
                    TextButton(onClick = { action.run { api.auth.sendEmailCode(sentTo!!, mode, lang) } }, enabled = !action.busy, modifier = Modifier.fillMaxWidth()) {
                        Text(stringResource(UiR.string.login_resend))
                    }
                }
            }
        }
    }
}

private val EMAIL_RE = Regex("^[^@\\s]+@[^@\\s]+\\.[^@\\s]+$")

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
                    onDone(if (me.needsPassword) Routes.password("set") else null)
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
