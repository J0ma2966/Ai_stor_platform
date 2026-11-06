package uz.boutique.client

import android.content.Context
import android.media.AudioManager
import android.media.ToneGenerator
import androidx.camera.core.CameraSelector
import androidx.camera.core.ImageCapture
import androidx.camera.core.ImageCaptureException
import androidx.camera.core.Preview
import androidx.camera.lifecycle.ProcessCameraProvider
import androidx.camera.view.PreviewView
import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.scaleIn
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Cameraswitch
import androidx.compose.material.icons.filled.Close
import androidx.compose.material3.FilterChip
import androidx.compose.material3.FilterChipDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.core.content.ContextCompat
import androidx.lifecycle.compose.LocalLifecycleOwner
import coil.compose.AsyncImage
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.job
import kotlinx.coroutines.launch
import kotlinx.coroutines.suspendCancellableCoroutine
import uz.boutique.ui.PrimaryButton
import uz.boutique.ui.SecondaryButton
import java.io.File
import kotlin.coroutines.resume
import uz.boutique.ui.R as UiR

// Камера с таймером для фото в полный рост: телефон ставят вертикально на уровне
// пояса, нажимают «Запустить», отходят на 2–3 м и встают в контур позы. Каждую
// секунду — короткий сигнал, последние три — двойной, затем снимок.
// Кадр пишется в кэш приложения и удаляется сразу после загрузки (как и снимок
// системной камеры), в галерею телефона он не попадает.

/** Варианты таймера в секундах; 0 — снять сразу (если снимает другой человек). */
val TimerChoices = listOf(0, 3, 5, 10)
private const val PREFS = "tryon_camera"
private const val PREF_TIMER = "timer_s"

private fun savedTimer(context: Context): Int =
    context.getSharedPreferences(PREFS, Context.MODE_PRIVATE).getInt(PREF_TIMER, 10).takeIf { it in TimerChoices } ?: 10

private fun saveTimer(context: Context, s: Int) =
    context.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit().putInt(PREF_TIMER, s).apply()

private suspend fun cameraProvider(context: Context): ProcessCameraProvider = suspendCancellableCoroutine { cont ->
    val f = ProcessCameraProvider.getInstance(context)
    f.addListener({ runCatching { f.get() }.onSuccess { if (cont.isActive) cont.resume(it) }.onFailure { cont.cancel(it) } }, ContextCompat.getMainExecutor(context))
}

private suspend fun takePicture(context: Context, ic: ImageCapture, file: File): Boolean = suspendCancellableCoroutine { cont ->
    ic.takePicture(
        ImageCapture.OutputFileOptions.Builder(file).build(),
        ContextCompat.getMainExecutor(context),
        object : ImageCapture.OnImageSavedCallback {
            override fun onImageSaved(output: ImageCapture.OutputFileResults) { if (cont.isActive) cont.resume(true) }
            override fun onError(exception: ImageCaptureException) { if (cont.isActive) cont.resume(false) }
        },
    )
}

/**
 * Полноэкранная камера для ракурса [angle]. [onResult] получает файл снимка
 * (вызывающий удаляет его после загрузки) или null, если пользователь закрыл экран.
 * [onSystemCamera] — запасной путь, если камера не открылась.
 */
@Composable
fun TimerCameraDialog(angle: String, file: File, onResult: (File?) -> Unit, onSystemCamera: () -> Unit) {
    Dialog(
        onDismissRequest = { file.delete(); onResult(null) },
        properties = DialogProperties(usePlatformDefaultWidth = false, decorFitsSystemWindows = false),
    ) {
        TimerCamera(angle, file, onResult, onSystemCamera)
    }
}

@Composable
private fun TimerCamera(angle: String, file: File, onResult: (File?) -> Unit, onSystemCamera: () -> Unit) {
    val context = LocalContext.current
    val owner = LocalLifecycleOwner.current
    val scope = rememberCoroutineScope()
    var front by rememberSaveable { mutableStateOf(false) }
    var timer by rememberSaveable { mutableIntStateOf(savedTimer(context)) }
    var countdown by remember { mutableStateOf<Int?>(null) }
    var job by remember { mutableStateOf<Job?>(null) }
    var taken by remember { mutableStateOf(false) }
    var failed by remember { mutableStateOf(false) }
    var provider by remember { mutableStateOf<ProcessCameraProvider?>(null) }
    val previewView = remember { PreviewView(context).apply { scaleType = PreviewView.ScaleType.FIT_CENTER } }
    val capture = remember { ImageCapture.Builder().setCaptureMode(ImageCapture.CAPTURE_MODE_MAXIMIZE_QUALITY).build() }

    LaunchedEffect(front, taken) {
        if (taken) return@LaunchedEffect
        try {
            val p = cameraProvider(context)
            provider = p
            val preview = Preview.Builder().build()
            preview.setSurfaceProvider(previewView.surfaceProvider)
            p.unbindAll()
            p.bindToLifecycle(owner, if (front) CameraSelector.DEFAULT_FRONT_CAMERA else CameraSelector.DEFAULT_BACK_CAMERA, preview, capture)
            failed = false
        } catch (e: kotlinx.coroutines.CancellationException) {
            throw e
        } catch (_: Exception) {
            failed = true
        }
    }
    DisposableEffect(Unit) {
        onDispose { provider?.unbindAll() }
    }

    fun stop() {
        job?.cancel()
        job = null
        countdown = null
        previewView.keepScreenOn = false
    }

    fun start() {
        stop()
        job = scope.launch {
            // ToneGenerator может не создаться на части устройств — тогда отсчёт без звука.
            val tone = runCatching { ToneGenerator(AudioManager.STREAM_MUSIC, 90) }.getOrNull()
            previewView.keepScreenOn = true
            try {
                for (s in timer downTo 1) {
                    countdown = s
                    tone?.startTone(if (s <= 3) ToneGenerator.TONE_PROP_BEEP2 else ToneGenerator.TONE_PROP_BEEP, 150)
                    delay(1000)
                }
                countdown = null
                tone?.startTone(ToneGenerator.TONE_PROP_ACK, 250)
                if (takePicture(context, capture, file)) {
                    taken = true
                    provider?.unbindAll()
                } else {
                    failed = true
                }
            } finally {
                tone?.release()
                // Отменённый отсчёт не должен сбросить состояние уже запущенного нового.
                if (job === coroutineContext.job) {
                    countdown = null
                    previewView.keepScreenOn = false
                    job = null
                }
            }
        }
    }

    Box(Modifier.fillMaxSize().background(Color.Black).testTag("timer_camera")) {
        if (taken) {
            AsyncImage(
                model = file,
                contentDescription = angleTitle(angle),
                contentScale = ContentScale.Fit,
                modifier = Modifier.fillMaxSize(),
            )
        } else {
            AndroidView(factory = { previewView }, modifier = Modifier.fillMaxSize())
            // Контур позы: голова и ступни должны поместиться внутрь.
            PoseFigure(
                Modifier.align(Alignment.Center).fillMaxHeight(0.78f).aspectRatio(100f / 150f),
                pose = angle,
                overlay = true,
                ghost = true,
            )
            val c = countdown
            if (c != null) {
                AnimatedContent(
                    targetState = c,
                    transitionSpec = { (scaleIn(tween(250), initialScale = 1.6f) + fadeIn(tween(250))) togetherWith fadeOut(tween(150)) },
                    label = "countdown",
                    modifier = Modifier.align(Alignment.Center),
                ) { n ->
                    Text(
                        n.toString(),
                        color = Color.White,
                        fontSize = 140.sp,
                        fontWeight = FontWeight.ExtraBold,
                        modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite }.testTag("countdown"),
                    )
                }
            }
        }

        // Верхняя панель: закрыть, ракурс, смена камеры.
        Row(
            Modifier.fillMaxWidth().statusBarsPadding().padding(8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            IconButton(onClick = { stop(); file.delete(); onResult(null) }) {
                Icon(Icons.Filled.Close, contentDescription = stringResource(UiR.string.app_close), tint = Color.White)
            }
            Text(
                stringResource(UiR.string.cam_title, angleTitle(angle)),
                color = Color.White,
                style = MaterialTheme.typography.titleMedium,
                modifier = Modifier.weight(1f),
                textAlign = TextAlign.Center,
            )
            IconButton(onClick = { front = !front }, enabled = !taken && job == null, modifier = Modifier.testTag("cam_switch")) {
                Icon(Icons.Filled.Cameraswitch, contentDescription = stringResource(UiR.string.cam_switch), tint = Color.White)
            }
        }

        // Нижняя панель.
        Column(
            Modifier.align(Alignment.BottomCenter).fillMaxWidth()
                .background(Color.Black.copy(alpha = 0.55f))
                .navigationBarsPadding().padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            when {
                taken -> Row(horizontalArrangement = Arrangement.spacedBy(12.dp), modifier = Modifier.fillMaxWidth()) {
                    SecondaryButton(stringResource(UiR.string.cam_retake), onClick = { file.delete(); taken = false }, modifier = Modifier.weight(1f).testTag("cam_retake"))
                    PrimaryButton(stringResource(UiR.string.cam_use), onClick = { onResult(file) }, modifier = Modifier.weight(1f).testTag("cam_use"))
                }
                failed -> {
                    Text(stringResource(UiR.string.cam_error), color = Color.White, textAlign = TextAlign.Center)
                    PrimaryButton(stringResource(UiR.string.cam_system), onClick = { stop(); file.delete(); onSystemCamera() }, modifier = Modifier.fillMaxWidth())
                }
                else -> {
                    Text(
                        stringResource(UiR.string.cam_hint),
                        color = Color.White,
                        style = MaterialTheme.typography.bodySmall,
                        textAlign = TextAlign.Center,
                    )
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                        Text(stringResource(UiR.string.cam_timer), color = Color.White, style = MaterialTheme.typography.labelLarge)
                        TimerChoices.forEach { s ->
                            FilterChip(
                                selected = timer == s,
                                onClick = { timer = s; saveTimer(context, s) },
                                enabled = job == null,
                                label = { Text(if (s == 0) stringResource(UiR.string.cam_timer_off) else stringResource(UiR.string.cam_timer_s, s)) },
                                colors = FilterChipDefaults.filterChipColors(
                                    labelColor = Color.White,
                                    selectedContainerColor = Color.White,
                                    selectedLabelColor = Color.Black,
                                ),
                                modifier = Modifier.testTag("timer_$s"),
                            )
                        }
                    }
                    val running = job != null
                    Box(
                        Modifier.size(76.dp).background(if (running) Color(0xFFE8484E) else Color.White, CircleShape),
                        contentAlignment = Alignment.Center,
                    ) {
                        IconButton(
                            onClick = { if (running) stop() else start() },
                            modifier = Modifier.fillMaxSize().testTag("cam_shutter"),
                        ) {
                            Text(
                                stringResource(
                                    when {
                                        running -> UiR.string.cam_cancel
                                        timer == 0 -> UiR.string.cam_shoot
                                        else -> UiR.string.cam_start
                                    },
                                ),
                                color = if (running) Color.White else Color.Black,
                                style = MaterialTheme.typography.labelMedium,
                                textAlign = TextAlign.Center,
                                modifier = Modifier.background(Color.Transparent, RoundedCornerShape(8.dp)),
                            )
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun angleTitle(a: String): String = stringResource(
    when (a) {
        "front" -> UiR.string.tryon_angle_front
        "side" -> UiR.string.tryon_angle_side
        else -> UiR.string.tryon_angle_back
    },
)
