package uz.boutique.client

import android.Manifest
import android.content.ActivityNotFoundException
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.PrimaryTabRow
import androidx.compose.material3.Tab
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.core.content.ContextCompat
import androidx.core.content.FileProvider
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import coil.compose.AsyncImage
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import uz.boutique.core.api.ApiClient
import uz.boutique.core.api.ApiException
import uz.boutique.core.api.AuthState
import uz.boutique.core.model.Me
import uz.boutique.core.tryon.Angles
import uz.boutique.core.tryon.CreateGeneration
import uz.boutique.core.tryon.FittingItem
import uz.boutique.core.tryon.FittingSelection
import uz.boutique.core.tryon.GenStatus
import uz.boutique.core.tryon.Generation
import uz.boutique.core.tryon.Photos
import uz.boutique.core.tryon.Quote
import uz.boutique.core.tryon.SavedPhoto
import uz.boutique.core.tryon.TryonRules
import uz.boutique.core.tryon.Zones
import uz.boutique.core.util.Dates
import uz.boutique.ui.AppScaffold
import uz.boutique.ui.Brand
import uz.boutique.ui.CenterLoading
import uz.boutique.ui.ConfirmDialog
import uz.boutique.ui.EmptyState
import uz.boutique.ui.ErrorBanner
import uz.boutique.ui.ErrorState
import uz.boutique.ui.InfoBanner
import uz.boutique.ui.Load
import uz.boutique.ui.LocalGraph
import uz.boutique.ui.PagedFooter
import uz.boutique.ui.PagedState
import uz.boutique.ui.Pill
import uz.boutique.ui.PrimaryButton
import uz.boutique.ui.ProductImage
import uz.boutique.ui.SecondaryButton
import uz.boutique.ui.SectionCard
import uz.boutique.ui.SectionTitle
import uz.boutique.ui.money
import uz.boutique.ui.readImageForUpload
import uz.boutique.ui.rememberAction
import uz.boutique.ui.rememberLoader
import uz.boutique.ui.rememberPaged
import java.io.File
import uz.boutique.ui.R as UiR

// AI-примерка (как на сайте: /fitting, /fitting/new, /fitting/{id}, /account/tryons).
// Фото тела и результаты не сохраняются на телефоне: только в памяти экрана и по
// подписанным ссылкам сервера (хранилище биометрии в Узбекистане). Снимок камеры
// удаляется из кэша сразу после загрузки.

/** Примерка выключена в сборке (release по умолчанию) — кнопки и экраны скрыты. */
val tryonEnabled: Boolean get() = BuildConfig.TRYON_ENABLED

@Composable
fun zoneLabel(zone: String): String = stringResource(
    when (zone) {
        Zones.HEADWEAR -> UiR.string.tryon_zone_headwear
        Zones.TOP -> UiR.string.tryon_zone_top
        Zones.BOTTOM -> UiR.string.tryon_zone_bottom
        else -> UiR.string.tryon_zone_shoes
    },
)

@Composable
private fun angleLabel(a: String): String = stringResource(
    when (a) {
        "front" -> UiR.string.tryon_angle_front
        "side" -> UiR.string.tryon_angle_side
        else -> UiR.string.tryon_angle_back
    },
)

@Composable
private fun angleHint(a: String): String = stringResource(
    when (a) {
        "front" -> UiR.string.tryon_hint_front
        "side" -> UiR.string.tryon_hint_side
        else -> UiR.string.tryon_hint_back
    },
)

@Composable
private fun genStatusLabel(s: GenStatus): String = stringResource(
    when (s) {
        GenStatus.QUEUED -> UiR.string.tryon_status_queued
        GenStatus.RUNNING -> UiR.string.tryon_status_running
        GenStatus.SUCCEEDED -> UiR.string.tryon_status_succeeded
        GenStatus.FAILED -> UiR.string.tryon_status_failed
    },
)

@Composable
private fun genErrorText(code: String?): String = stringResource(
    when (code) {
        "photo_rejected" -> UiR.string.tryon_err_photo_rejected
        "content_blocked" -> UiR.string.tryon_err_content_blocked
        "unsupported_items" -> UiR.string.tryon_err_unsupported_items
        "photo_missing" -> UiR.string.tryon_err_photo_missing
        "product_unavailable" -> UiR.string.tryon_err_product_unavailable
        "transfer_consent_required" -> UiR.string.tryon_err_transfer
        else -> UiR.string.tryon_err_provider
    },
)

private fun Throwable?.isTryonDisabled() = (this as? ApiException)?.code == "tryon_disabled"

/** Сервер запущен с TRYON_PROVIDER=off или сборка без примерки. */
@Composable
fun TryonDisabled(modifier: Modifier = Modifier, onCatalog: () -> Unit) {
    EmptyState(stringResource(UiR.string.tryon_disabled_title) + "\n\n" + stringResource(UiR.string.tryon_disabled_text), modifier.testTag("tryon_disabled")) {
        PrimaryButton(stringResource(UiR.string.tryon_to_catalog), onClick = onCatalog)
    }
}

private fun openUrl(context: android.content.Context, url: String) {
    try {
        context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)))
    } catch (_: ActivityNotFoundException) {
    }
}

// ---------------- примерочная ----------------

@Composable
fun FittingRoomScreen(
    onBack: () -> Unit,
    onProduct: (String) -> Unit,
    onStart: () -> Unit,
    onHistory: () -> Unit,
    onCatalog: () -> Unit,
) {
    val api = LocalGraph.current.api
    val shop = LocalShop.current
    val auth by api.session.state.collectAsStateWithLifecycle()
    val loggedIn = auth is AuthState.LoggedIn
    // Квота нужна только чтобы предупредить о зонах, которые модель не переодевает.
    val quote = rememberLoader(auth) { if (loggedIn && tryonEnabled) runCatching { api.tryon.quote() } else null }
    val items = shop.fittingItems

    AppScaffold(
        title = stringResource(UiR.string.tryon_room),
        onBack = onBack,
        actions = { if (loggedIn) TextButton(onClick = onHistory) { Text(stringResource(UiR.string.tryon_history)) } },
    ) { pad ->
        if (!tryonEnabled || quote.value?.exceptionOrNull().isTryonDisabled()) {
            TryonDisabled(Modifier.padding(pad), onCatalog)
            return@AppScaffold
        }
        Column(
            Modifier.padding(pad).fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            // Примерочная подстраивается сама: если выбранный верх — комплект (верх и низ
            // на одном фото), карточка «низа» скрыта.
            FittingSelection.visibleZones(items).forEach { zone ->
                val item = items.firstOrNull { it.zone == zone }
                SectionCard(Modifier.testTag("zone_$zone")) {
                    Text(zoneLabel(zone), style = MaterialTheme.typography.labelLarge, color = Brand.colors.muted)
                    if (item == null) {
                        Text(stringResource(UiR.string.tryon_empty_zone), color = Brand.colors.muted)
                    } else {
                        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                            ProductImage(item.imageUrl, null, Modifier.size(64.dp).clickable { onProduct(item.id) })
                            Column(Modifier.weight(1f).clickable { onProduct(item.id) }) {
                                Text(item.title, fontWeight = FontWeight.Bold, maxLines = 2)
                                Text(money(item.price), color = Brand.colors.muted)
                            }
                            IconButton(onClick = { shop.removeFromFitting(zone) }, modifier = Modifier.testTag("remove_$zone")) {
                                Icon(Icons.Filled.Close, contentDescription = stringResource(UiR.string.tryon_remove_item))
                            }
                        }
                    }
                }
            }
            val q = quote.value?.getOrNull()
            val unsupported = TryonRules.unsupported(items, q)
            if (unsupported.isNotEmpty()) {
                InfoBanner(stringResource(UiR.string.tryon_unsupported, unsupported.map { zoneLabel(it.zone) }.joinToString(", ")), color = Brand.colors.warn)
            }
            Text(stringResource(UiR.string.tryon_disclaimer), style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted)
            if (items.isEmpty()) {
                InfoBanner(stringResource(UiR.string.tryon_pick_first))
                SecondaryButton(stringResource(UiR.string.tryon_to_catalog), onClick = onCatalog)
            } else {
                if (!loggedIn) Text(stringResource(UiR.string.tryon_login_needed), color = Brand.colors.muted)
                PrimaryButton(
                    stringResource(UiR.string.tryon_generate) + (q?.let { " · " + stringResource(UiR.string.tryon_cost_short, it.cost) } ?: ""),
                    onClick = onStart,
                    modifier = Modifier.testTag("tryon_generate"),
                )
            }
        }
    }
}

// ---------------- мастер: фото → параметры → запуск ----------------

private data class Slot(val key: String, val url: String, val saved: Boolean)

private class WizardData(val quote: Quote, val saved: List<SavedPhoto>, val me: Me)

@Composable
fun TryonWizardScreen(onBack: () -> Unit, onStarted: (String) -> Unit, onCatalog: () -> Unit) {
    val graph = LocalGraph.current
    val api = graph.api
    val shop = LocalShop.current
    val items = shop.fittingItems
    val loader = rememberLoader("wizard") { WizardData(api.tryon.quote(), api.tryon.photos(), api.auth.me()) }

    AppScaffold(title = stringResource(UiR.string.tryon_new), onBack = onBack) { pad ->
        when (val s = loader.state) {
            is Load.Loading -> CenterLoading(Modifier.padding(pad))
            is Load.Failed -> if (s.error.isTryonDisabled()) {
                TryonDisabled(Modifier.padding(pad), onCatalog)
            } else {
                ErrorState(s.error, onRetry = { loader.reload() }, modifier = Modifier.padding(pad))
            }
            is Load.Ok -> when {
                items.isEmpty() -> EmptyState(stringResource(UiR.string.tryon_pick_first), Modifier.padding(pad)) {
                    PrimaryButton(stringResource(UiR.string.tryon_to_catalog), onClick = onCatalog)
                }
                !s.value.quote.registered -> EmptyState(stringResource(UiR.string.tryon_registration_required), Modifier.padding(pad))
                else -> TryonWizard(s.value, items, Modifier.padding(pad), onStarted)
            }
        }
    }
}

@Composable
private fun TryonWizard(data: WizardData, items: List<FittingItem>, modifier: Modifier, onStarted: (String) -> Unit) {
    val graph = LocalGraph.current
    val api = graph.api
    val context = LocalContext.current
    var quote by remember { mutableStateOf(data.quote) }
    var step by rememberSaveable { mutableStateOf(0) }
    val slots = remember {
        mutableStateMapOf<String, Slot>().apply { data.saved.forEach { put(it.angle, Slot(it.key, it.url, saved = true)) } }
    }
    var savePhotos by rememberSaveable { mutableStateOf(true) }
    var consent by rememberSaveable { mutableStateOf(data.quote.consent) }
    var transfer by rememberSaveable { mutableStateOf(!data.quote.transfer.required || data.quote.transfer.consent) }
    val body = remember {
        mutableStateMapOf<String, String>().apply {
            data.me.heightCm?.let { put("height", it.toString()) }
            data.me.weightKg?.let { put("weight", it.toString().removeSuffix(".0")) }
            TryonRules.BODY_PARAMS.forEach { k -> data.me.bodyParams[k]?.let { put(k, it.toString().removeSuffix(".0")) } }
        }
    }
    // Один ключ на попытку запуска: повтор после сетевой ошибки не спишет токены дважды.
    var idemKey by rememberSaveable { mutableStateOf(ApiClient.newIdempotencyKey()) }
    var uploadingAngle by remember { mutableStateOf<String?>(null) }
    val upload = rememberAction()
    val next = rememberAction()
    val submit = rememberAction()
    var cameraFile by remember { mutableStateOf<File?>(null) }

    fun uploadUri(angle: String, uri: Uri, temp: File? = null) {
        uploadingAngle = angle
        upload.run {
            try {
                val bytes = readImageForUpload(context, uri)
                val r = api.uploads.upload("body", bytes, "image/jpeg")
                slots[angle] = Slot(r.key, r.url, saved = false)
                idemKey = ApiClient.newIdempotencyKey()
            } finally {
                temp?.delete()
                uploadingAngle = null
            }
        }
    }

    val gallery = rememberLauncherForActivityResult(ActivityResultContracts.PickVisualMedia()) { uri: Uri? ->
        val a = pendingAngle.value
        if (uri != null && a != null) uploadUri(a, uri)
    }
    val camera = rememberLauncherForActivityResult(ActivityResultContracts.TakePicture()) { ok ->
        val a = pendingAngle.value
        val f = cameraFile
        if (ok && a != null && f != null) uploadUri(a, Uri.fromFile(f), temp = f) else f?.delete()
    }

    // Системная камера — запасной путь: без разрешения CAMERA или если своя камера не открылась.
    fun systemCamera(angle: String) {
        pendingAngle.value = angle
        val dir = File(context.cacheDir, "camera").apply { mkdirs() }
        val f = File(dir, "tryon_$angle.jpg")
        cameraFile = f
        val uri = FileProvider.getUriForFile(context, context.packageName + ".files", f)
        try {
            camera.launch(uri)
        } catch (_: ActivityNotFoundException) {
            f.delete()
        }
    }

    // Своя камера с таймером (TimerCamera.kt), чтобы снять себя в полный рост без помощника.
    var timerAngle by rememberSaveable { mutableStateOf<String?>(null) }
    val cameraPermission = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
        val a = pendingAngle.value
        if (a != null) { if (granted) timerAngle = a else systemCamera(a) }
    }

    fun takePhoto(angle: String) {
        pendingAngle.value = angle
        if (ContextCompat.checkSelfPermission(context, Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED) {
            timerAngle = angle
        } else {
            cameraPermission.launch(Manifest.permission.CAMERA)
        }
    }

    timerAngle?.let { a ->
        val shot = remember(a) { File(File(context.cacheDir, "camera").apply { mkdirs() }, "timer_$a.jpg") }
        TimerCameraDialog(
            angle = a,
            file = shot,
            onResult = { f ->
                timerAngle = null
                if (f != null) uploadUri(a, Uri.fromFile(f), temp = f)
            },
            onSystemCamera = {
                timerAngle = null
                systemCamera(a)
            },
        )
    }

    val allPhotos = Angles.ALL.all { slots[it] != null }

    Column(
        modifier.fillMaxSize().imePadding().verticalScroll(rememberScrollState()).padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        Text(
            listOf(UiR.string.tryon_step_photos, UiR.string.tryon_step_body, UiR.string.tryon_step_confirm)
                .mapIndexed { i, r -> "${i + 1}. " + stringResource(r) }.joinToString("   "),
            style = MaterialTheme.typography.labelLarge,
            color = MaterialTheme.colorScheme.primary,
        )
        when (step) {
            0 -> {
                SectionCard {
                    SectionTitle(stringResource(UiR.string.tryon_rules_title))
                    Text(stringResource(UiR.string.tryon_rules), style = MaterialTheme.typography.bodyMedium)
                    Text(
                        stringResource(UiR.string.tryon_privacy, quote.retention.photoHours, quote.retention.savedPhotoDays, quote.retention.resultDays),
                        style = MaterialTheme.typography.bodySmall,
                        color = Brand.colors.muted,
                    )
                }
                PhotoGuideCard(defaultOpen = !Angles.ALL.all { slots[it]?.saved == true })
                Angles.ALL.forEach { a ->
                    val s = slots[a]
                    SectionCard(Modifier.testTag("slot_$a")) {
                        Text(angleLabel(a), style = MaterialTheme.typography.titleSmall)
                        Text(angleHint(a), style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted)
                        Row(horizontalArrangement = Arrangement.spacedBy(12.dp), verticalAlignment = Alignment.CenterVertically) {
                            if (s == null) {
                                PoseFigure(Modifier.size(width = 72.dp, height = 96.dp).testTag("pose_$a"), pose = a, ghost = true)
                            } else {
                                ProductImage(s.url, angleLabel(a), Modifier.size(width = 72.dp, height = 96.dp))
                            }
                            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                                if (s?.saved == true) Pill(stringResource(UiR.string.tryon_saved_photo), Brand.colors.info)
                                if (uploadingAngle == a) {
                                    Row(verticalAlignment = Alignment.CenterVertically) {
                                        CircularProgressIndicator(Modifier.size(18.dp))
                                        Text("  " + stringResource(UiR.string.tryon_uploading))
                                    }
                                } else {
                                    Row {
                                        TextButton(onClick = { takePhoto(a) }, enabled = !upload.busy, modifier = Modifier.testTag("camera_$a")) {
                                            Text(stringResource(UiR.string.tryon_take_photo))
                                        }
                                        TextButton(
                                            onClick = {
                                                pendingAngle.value = a
                                                gallery.launch(PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageOnly))
                                            },
                                            enabled = !upload.busy,
                                            modifier = Modifier.testTag("gallery_$a"),
                                        ) { Text(stringResource(UiR.string.tryon_from_gallery)) }
                                    }
                                    if (s != null) {
                                        TextButton(onClick = { slots.remove(a) }) { Text(stringResource(UiR.string.tryon_remove_photo), color = Brand.colors.bad) }
                                    }
                                }
                            }
                        }
                    }
                }
                ErrorBanner(upload.error)
                CheckRow(savePhotos, { savePhotos = it }, stringResource(UiR.string.tryon_save_photos), "save_photos")
                if (!quote.consent) CheckRow(consent, { consent = it }, stringResource(UiR.string.tryon_consent), "consent")
                if (quote.transfer.required && !quote.transfer.consent) {
                    CheckRow(transfer, { transfer = it }, stringResource(UiR.string.tryon_transfer_consent, quote.transfer.recipient), "transfer_consent")
                }
                if (!allPhotos) Text(stringResource(UiR.string.tryon_need_all_photos), color = Brand.colors.warn, style = MaterialTheme.typography.bodySmall)
                ErrorBanner(next.error)
                PrimaryButton(
                    stringResource(UiR.string.tryon_next),
                    busy = next.busy,
                    enabled = allPhotos && consent && transfer && !upload.busy,
                    modifier = Modifier.testTag("next_photos"),
                    onClick = {
                        next.run {
                            if (!quote.consent) {
                                api.tryon.giveConsent()
                                quote = quote.copy(consent = true)
                            }
                            if (quote.transfer.required && !quote.transfer.consent) {
                                api.tryon.giveTransferConsent()
                                quote = quote.copy(transfer = quote.transfer.copy(consent = true))
                            }
                            step = 1
                        }
                    },
                )
            }
            1 -> {
                SectionCard {
                    Text(stringResource(UiR.string.tryon_body_reminder), style = MaterialTheme.typography.bodyMedium)
                    listOf(
                        "height" to UiR.string.tryon_height,
                        "weight" to UiR.string.tryon_weight,
                        "chest_cm" to UiR.string.tryon_chest,
                        "waist_cm" to UiR.string.tryon_waist,
                        "hips_cm" to UiR.string.tryon_hips,
                    ).forEach { (k, label) ->
                        OutlinedTextField(
                            value = body[k].orEmpty(),
                            onValueChange = { v -> body[k] = v.filter { it.isDigit() || it == '.' || it == ',' }.take(6) },
                            label = { Text(stringResource(label)) },
                            singleLine = true,
                            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal),
                            modifier = Modifier.fillMaxWidth().testTag("body_$k"),
                        )
                    }
                }
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    SecondaryButton(stringResource(UiR.string.app_back), onClick = { step = 0 }, modifier = Modifier.weight(1f))
                    PrimaryButton(stringResource(UiR.string.tryon_next), onClick = { step = 2 }, modifier = Modifier.weight(1f).testTag("next_body"))
                }
            }
            else -> {
                SectionCard {
                    SectionTitle(stringResource(UiR.string.tryon_confirm_title))
                    items.forEach { i ->
                        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                            ProductImage(i.imageUrl, null, Modifier.size(44.dp))
                            Text(zoneLabel(i.zone) + ": " + i.title, modifier = Modifier.weight(1f), maxLines = 2)
                            Text(money(i.price), color = Brand.colors.muted, style = MaterialTheme.typography.bodySmall)
                        }
                    }
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        Angles.ALL.forEach { a -> ProductImage(slots[a]?.url, angleLabel(a), Modifier.size(width = 54.dp, height = 72.dp)) }
                    }
                    Text(stringResource(UiR.string.tryon_cost_line, quote.cost, quote.balance), modifier = Modifier.testTag("cost_line"))
                    Text(stringResource(UiR.string.tryon_disclaimer), style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted)
                }
                if (!quote.enough) {
                    InfoBanner(stringResource(UiR.string.tryon_not_enough), color = Brand.colors.bad)
                    TextButton(onClick = { openUrl(context, graph.siteUrl + "/account/balance") }) { Text(stringResource(UiR.string.tryon_buy_tokens)) }
                }
                ErrorBanner(submit.error)
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    SecondaryButton(stringResource(UiR.string.app_back), onClick = { step = 1 }, enabled = !submit.busy, modifier = Modifier.weight(1f))
                    PrimaryButton(
                        stringResource(UiR.string.tryon_confirm_run, quote.cost),
                        busy = submit.busy,
                        enabled = quote.enough,
                        modifier = Modifier.weight(1f).testTag("run"),
                        onClick = {
                            submit.run {
                                val g = api.tryon.create(
                                    CreateGeneration(
                                        productIds = items.map { it.id },
                                        photos = Photos(slots["front"]!!.key, slots["side"]!!.key, slots["back"]!!.key),
                                        savePhotos = savePhotos && Angles.ALL.any { slots[it]?.saved != true },
                                        body = TryonRules.body(body["height"].orEmpty(), body["weight"].orEmpty(), body),
                                    ),
                                    idemKey,
                                )
                                onStarted(g.id)
                            }
                        },
                    )
                }
            }
        }
    }
}

/** Ракурс, для которого открыта камера или галерея (переживает возврат из другого приложения). */
private val pendingAngle = mutableStateOf<String?>(null)

@Composable
private fun CheckRow(checked: Boolean, onChange: (Boolean) -> Unit, text: String, tag: String) {
    Row(verticalAlignment = Alignment.Top, modifier = Modifier.fillMaxWidth().clickable { onChange(!checked) }) {
        Checkbox(checked = checked, onCheckedChange = onChange, modifier = Modifier.testTag(tag))
        Text(text, style = MaterialTheme.typography.bodySmall, modifier = Modifier.padding(top = 12.dp))
    }
}

// ---------------- результат ----------------

@Composable
fun GenerationScreen(
    id: String,
    onBack: () -> Unit,
    onProduct: (String) -> Unit,
    onFitting: () -> Unit,
    onRepeat: () -> Unit,
) {
    val api = LocalGraph.current.api
    val loader = rememberLoader(id) { api.tryon.get(id) }
    // Пока генерация в очереди или выполняется — опрашиваем раз в 3 секунды (как сайт по Retry-After).
    val active = loader.value?.status?.active == true
    LaunchedEffect(active) {
        while (active) {
            delay(3_000)
            runCatching { api.tryon.get(id) }.onSuccess { loader.set(it) }
        }
    }
    AppScaffold(title = stringResource(UiR.string.tryon_result), onBack = onBack) { pad ->
        when (val s = loader.state) {
            is Load.Loading -> CenterLoading(Modifier.padding(pad))
            is Load.Failed -> ErrorState(s.error, onRetry = { loader.reload() }, modifier = Modifier.padding(pad))
            is Load.Ok -> GenerationContent(
                s.value, Modifier.padding(pad),
                onChanged = { loader.set(it) },
                onDeleted = onBack,
                onProduct = onProduct,
                onFitting = onFitting,
                onRepeat = onRepeat,
            )
        }
    }
}

@Composable
fun GenerationContent(
    g: Generation,
    modifier: Modifier,
    onChanged: (Generation) -> Unit,
    onDeleted: () -> Unit,
    onProduct: (String) -> Unit,
    onFitting: () -> Unit,
    onRepeat: () -> Unit,
) {
    val api = LocalGraph.current.api
    val shop = LocalShop.current
    val context = LocalContext.current
    val action = rememberAction()
    var confirmDelete by rememberSaveable { mutableStateOf(false) }

    fun toFitting() = shop.setFitting(g.products.filter { it.zone in Zones.ALL }.map {
        FittingItem(it.id, it.title, it.brand, it.price, it.imageUrl, it.zone, it.storeName, it.isSet && it.zone == Zones.TOP)
    })

    Column(
        modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Pill(
                genStatusLabel(g.status),
                when (g.status) {
                    GenStatus.SUCCEEDED -> Brand.colors.ok
                    GenStatus.FAILED -> Brand.colors.bad
                    else -> Brand.colors.info
                },
                Modifier.testTag("gen_status"),
            )
            if (g.saved) Pill(stringResource(UiR.string.tryon_saved), Brand.colors.info)
        }
        when (g.status) {
            GenStatus.QUEUED, GenStatus.RUNNING -> SectionCard {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    CircularProgressIndicator(Modifier.size(28.dp))
                    Text(stringResource(if (g.status == GenStatus.QUEUED) UiR.string.tryon_queued_text else UiR.string.tryon_running_text))
                }
                Text(stringResource(UiR.string.tryon_can_leave), style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted)
            }
            GenStatus.FAILED -> {
                InfoBanner(genErrorText(g.errorCode), color = Brand.colors.bad, modifier = Modifier.testTag("gen_error"))
                if (g.refunded) InfoBanner(stringResource(UiR.string.tryon_refunded, g.cost), color = Brand.colors.ok)
            }
            GenStatus.SUCCEEDED -> {
                val url = g.resultUrl
                if (url == null) {
                    InfoBanner(stringResource(UiR.string.tryon_result_deleted))
                } else {
                    AsyncImage(
                        model = url,
                        contentDescription = stringResource(UiR.string.tryon_result_alt),
                        contentScale = ContentScale.Fit,
                        modifier = Modifier.fillMaxWidth().aspectRatio(1.5f).clickable { openUrl(context, url) }.testTag("result_image"),
                    )
                    g.resultExpiresAt?.let {
                        Text(stringResource(UiR.string.tryon_expires, Dates.date(it)), style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted)
                    }
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        SecondaryButton(stringResource(UiR.string.tryon_open_full), onClick = { openUrl(context, url) }, modifier = Modifier.weight(1f))
                        g.downloadUrl?.let { d ->
                            SecondaryButton(stringResource(UiR.string.tryon_download), onClick = { openUrl(context, d) }, modifier = Modifier.weight(1f))
                        }
                    }
                    PrimaryButton(
                        stringResource(if (g.saved) UiR.string.tryon_unsave_look else UiR.string.tryon_save_look),
                        busy = action.busy,
                        modifier = Modifier.testTag("save_look"),
                        onClick = { action.run { onChanged(api.tryon.setSaved(g.id, !g.saved)) } },
                    )
                }
            }
        }
        ErrorBanner(action.error)
        if (g.products.isNotEmpty()) {
            SectionTitle(stringResource(UiR.string.tryon_used_items))
            g.products.forEach { p ->
                SectionCard(Modifier.clickable { onProduct(p.id) }) {
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                        ProductImage(p.imageUrl, null, Modifier.size(56.dp))
                        Column(Modifier.weight(1f)) {
                            Text(p.title, fontWeight = FontWeight.Bold, maxLines = 2)
                            Text(zoneLabel(p.zone) + " · " + money(p.price), color = Brand.colors.muted, style = MaterialTheme.typography.bodySmall)
                        }
                        if (!p.available) Pill(stringResource(UiR.string.tryon_unavailable), Brand.colors.bad)
                    }
                }
            }
        }
        if (!g.status.active) {
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                SecondaryButton(stringResource(UiR.string.tryon_edit_look), onClick = { toFitting(); onFitting() }, modifier = Modifier.weight(1f))
                SecondaryButton(stringResource(UiR.string.tryon_repeat), onClick = { toFitting(); onRepeat() }, modifier = Modifier.weight(1f).testTag("repeat"))
            }
            TextButton(onClick = { confirmDelete = true }, modifier = Modifier.testTag("delete_gen")) {
                Text(stringResource(UiR.string.app_delete), color = Brand.colors.bad)
            }
        }
    }
    if (confirmDelete) {
        ConfirmDialog(
            text = stringResource(UiR.string.tryon_delete_confirm),
            confirmLabel = stringResource(UiR.string.app_delete),
            onConfirm = {
                confirmDelete = false
                action.run { api.tryon.delete(g.id); onDeleted() }
            },
            onDismiss = { confirmDelete = false },
        )
    }
}

// ---------------- история и сохранённые образы ----------------

@Composable
fun TryonHistoryScreen(onBack: () -> Unit, onGeneration: (String) -> Unit, onPhotos: () -> Unit) {
    val api = LocalGraph.current.api
    var saved by rememberSaveable { mutableStateOf(false) }
    val paged = rememberPaged(saved) { c -> api.tryon.list(saved, c) }
    AppScaffold(
        title = stringResource(UiR.string.tryon_history),
        onBack = onBack,
        actions = { TextButton(onClick = onPhotos) { Text(stringResource(UiR.string.tryon_my_photos_short)) } },
    ) { pad ->
        Column(Modifier.padding(pad).fillMaxSize()) {
            PrimaryTabRow(selectedTabIndex = if (saved) 1 else 0) {
                Tab(selected = !saved, onClick = { saved = false }, text = { Text(stringResource(UiR.string.tryon_all)) })
                Tab(selected = saved, onClick = { saved = true }, text = { Text(stringResource(UiR.string.tryon_saved_looks)) }, modifier = Modifier.testTag("tab_saved"))
            }
            PagedState(paged, stringResource(if (saved) UiR.string.tryon_no_looks else UiR.string.tryon_no_tryons)) {
                LazyColumn(contentPadding = PaddingValues(16.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
                    items(paged.items, key = { it.id }) { g ->
                        SectionCard(Modifier.clickable { onGeneration(g.id) }.testTag("gen_${g.id}")) {
                            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                                ProductImage(g.resultUrl ?: g.products.firstOrNull()?.imageUrl, null, Modifier.size(width = 72.dp, height = 56.dp))
                                Column(Modifier.weight(1f)) {
                                    Text(g.products.joinToString(", ") { it.title }.ifEmpty { "—" }, maxLines = 2, fontWeight = FontWeight.Bold)
                                    Text(Dates.dateTime(g.createdAt), style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted)
                                }
                                Pill(genStatusLabel(g.status), if (g.status == GenStatus.FAILED) Brand.colors.bad else Brand.colors.info)
                            }
                        }
                    }
                    item { PagedFooter(paged) }
                }
            }
        }
    }
}

// ---------------- мои фото ----------------

@Composable
fun TryonPhotosScreen(onBack: () -> Unit) {
    val api = LocalGraph.current.api
    val loader = rememberLoader("photos") { api.tryon.photos() }
    val action = rememberAction()
    var confirmRevoke by rememberSaveable { mutableStateOf(false) }
    AppScaffold(title = stringResource(UiR.string.tryon_my_photos), onBack = onBack) { pad ->
        Column(
            Modifier.padding(pad).fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            Text(stringResource(UiR.string.tryon_photos_intro), color = Brand.colors.muted)
            when (val s = loader.state) {
                is Load.Loading -> CircularProgressIndicator()
                is Load.Failed -> ErrorBanner(s.error)
                is Load.Ok -> if (s.value.isEmpty()) {
                    Text(stringResource(UiR.string.tryon_photos_none))
                } else {
                    s.value.forEach { p ->
                        SectionCard(Modifier.testTag("photo_${p.angle}")) {
                            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                                ProductImage(p.url, angleLabel(p.angle), Modifier.size(width = 72.dp, height = 96.dp))
                                Column(Modifier.weight(1f)) {
                                    Text(angleLabel(p.angle), fontWeight = FontWeight.Bold)
                                    if (p.expiresAt.isNotEmpty()) {
                                        Text(stringResource(UiR.string.tryon_photo_expires, Dates.date(p.expiresAt)), style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted)
                                    }
                                }
                                TextButton(onClick = { action.run { api.tryon.deletePhoto(p.angle); loader.set(s.value - p) } }) {
                                    Text(stringResource(UiR.string.app_delete), color = Brand.colors.bad)
                                }
                            }
                        }
                    }
                }
            }
            ErrorBanner(action.error)
            SectionCard {
                Text(stringResource(UiR.string.tryon_revoke_text), style = MaterialTheme.typography.bodySmall)
                SecondaryButton(stringResource(UiR.string.tryon_revoke), color = Brand.colors.bad, onClick = { confirmRevoke = true }, modifier = Modifier.testTag("revoke"))
            }
        }
    }
    if (confirmRevoke) {
        ConfirmDialog(
            text = stringResource(UiR.string.tryon_revoke_confirm),
            confirmLabel = stringResource(UiR.string.app_delete),
            onConfirm = {
                confirmRevoke = false
                action.run { api.tryon.revokeConsent(); loader.reload() }
            },
            onDismiss = { confirmRevoke = false },
        )
    }
}

/** Кнопка «В примерочную» на странице товара. */
@Composable
fun FittingButton(item: FittingItem, onFitting: () -> Unit) {
    val shop = LocalShop.current
    val scope = rememberCoroutineScope()
    val inRoom = shop.fittingItems.any { it.id == item.id }
    SecondaryButton(
        stringResource(if (inRoom) UiR.string.tryon_added else UiR.string.tryon_add),
        onClick = { if (inRoom) onFitting() else scope.launch { shop.addToFitting(item) } },
        modifier = Modifier.fillMaxWidth().testTag("to_fitting"),
    )
}
