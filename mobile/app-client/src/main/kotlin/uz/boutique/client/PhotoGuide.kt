package uz.boutique.client

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.background
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.rotate
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.RoundRect
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.drawscope.DrawScope
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.drawscope.clipPath
import androidx.compose.ui.graphics.drawscope.withTransform
import androidx.compose.ui.graphics.luminance
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import uz.boutique.ui.Brand
import uz.boutique.ui.SectionCard
import kotlin.math.min
import uz.boutique.ui.R as UiR

// Примеры фото для AI-примерки: как надо (3 ракурса) и типичные ошибки.
// Схематичные рисунки (не фото людей). Геометрия в координатах 100×150 та же,
// что в web/src/components/PhotoGuide.tsx — меняйте вместе.

enum class PhotoFlaw { CROPPED, DARK, BAGGY, CROWD, SELFIE, COVERED }

private class PgColors(
    val skin: Color, val hair: Color, val top: Color, val bottom: Color, val shoe: Color,
    val wall: Color, val floor: Color, val ok: Color, val bad: Color,
)

@Composable
private fun pgColors(): PgColors {
    val dark = MaterialTheme.colorScheme.surface.luminance() < 0.5f
    return if (dark) {
        PgColors(Color(0xFFC9A48B), Color(0xFF17131A), Color(0xFF8F6FD0), Color(0xFF4A4456), Color(0xFF0C0B0F), Color(0xFF34303D), Color(0xFF2A2632), Brand.colors.ok, Brand.colors.bad)
    } else {
        PgColors(Color(0xFFD9B49B), Color(0xFF3B2F3A), Color(0xFF5B3D8F), Color(0xFF2F2A38), Color(0xFF17151C), Color(0xFFECE9F2), Color(0xFFDDD8E6), Brand.colors.ok, Brand.colors.bad)
    }
}

private val Silhouette = Color(0xFF1A1820)
private val Clutter = Color(0xFFB9B0C9)

private fun DrawScope.poly(c: Color, vararg p: Float) {
    val path = Path()
    path.moveTo(p[0], p[1])
    var i = 2
    while (i < p.size) { path.lineTo(p[i], p[i + 1]); i += 2 }
    path.close()
    drawPath(path, c)
}

private fun DrawScope.oval(c: Color, cx: Float, cy: Float, rx: Float, ry: Float) =
    drawOval(c, topLeft = Offset(cx - rx, cy - ry), size = Size(rx * 2, ry * 2))

private fun DrawScope.limb(c: Color, vararg p: Float) {
    val path = Path()
    path.moveTo(p[0], p[1])
    var i = 2
    while (i < p.size) { path.lineTo(p[i], p[i + 1]); i += 2 }
    drawPath(path, c, style = Stroke(width = 5f, cap = StrokeCap.Round, join = StrokeJoin.Round))
}

/** Фигура в полный рост; cx — центр по горизонтали (как Person в PhotoGuide.tsx). */
private fun DrawScope.person(k: PgColors, pose: String, cx: Float = 50f, silhouette: Boolean = false, arms: Boolean = true) {
    fun c(x: Color) = if (silhouette) Silhouette else x
    withTransform({ translate(cx - 50f, 0f) }) {
        if (pose == "side") {
            poly(c(k.bottom), 44f, 76f, 56f, 76f, 55f, 133f, 46f, 133f)
            oval(c(k.shoe), 52f, 134f, 7f, 2.6f)
            poly(c(k.top), 44f, 40f, 51f, 37.5f, 57f, 41f, 56f, 77f, 44f, 77f)
            drawRoundRect(c(k.skin), Offset(47.5f, 32f), Size(5f, 7f), CornerRadius(2f))
            oval(c(k.skin), 50f, 24f, 8.5f, 10.5f)
            drawCircle(c(k.skin), 1.8f, Offset(58f, 25f))
            poly(c(k.hair), 41.5f, 25f, 42.5f, 16f, 50f, 13f, 57f, 15f, 58.5f, 20f, 50f, 19f, 46f, 22f, 46f, 29f)
            if (arms) limb(c(k.skin), 50f, 43f, 51f, 80f)
            return@withTransform
        }
        val back = pose == "back"
        poly(c(k.bottom), 39f, 76f, 61f, 76f, 60f, 133f, 52f, 133f, 50f, 90f, 48f, 133f, 40f, 133f)
        oval(c(k.shoe), 44f, 134f, 5.5f, 2.4f)
        oval(c(k.shoe), 56f, 134f, 5.5f, 2.4f)
        if (arms) {
            limb(c(k.skin), 38f, 43f, 29f, 80f)
            limb(c(k.skin), 62f, 43f, 71f, 80f)
        }
        poly(c(k.top), 36f, 41f, 50f, 37.5f, 64f, 41f, 61f, 77f, 39f, 77f)
        drawRoundRect(c(k.skin), Offset(47f, 32f), Size(6f, 7f), CornerRadius(2f))
        oval(c(if (back) k.hair else k.skin), 50f, 24f, 9f, 10.5f)
        if (!back) poly(c(k.hair), 41f, 23f, 43f, 15.5f, 50f, 13f, 57f, 15.5f, 59f, 23f, 55f, 18f, 50f, 17f, 45f, 18f)
    }
}

/**
 * Карточка-пример. [mark]: true — зелёная галочка, false — красный крестик, null — без значка.
 * [ghost] — бледный контур для пустого слота загрузки.
 */
@Composable
fun PoseFigure(modifier: Modifier = Modifier, pose: String = "front", flaw: PhotoFlaw? = null, mark: Boolean? = null, ghost: Boolean = false) {
    val k = pgColors()
    Canvas(modifier.alpha(if (ghost) 0.45f else 1f).clearAndSetSemantics {}) {
        val s = min(size.width / 100f, size.height / 150f)
        val dx = (size.width - 100f * s) / 2f
        val dy = (size.height - 150f * s) / 2f
        withTransform({ translate(dx, dy); scale(s, s, Offset.Zero) }) {
            val frame = Path().apply { addRoundRect(RoundRect(0f, 0f, 100f, 150f, CornerRadius(10f))) }
            clipPath(frame) {
                val dark = flaw == PhotoFlaw.DARK
                drawRect(if (dark) Color(0xFF24212B) else k.wall, size = Size(100f, 150f))
                drawRect(if (dark) Color(0xFF1B1920) else k.floor, Offset(0f, 128f), Size(100f, 22f))
                when (flaw) {
                    PhotoFlaw.CROPPED -> withTransform({ translate(50f, 52f); scale(1.55f, 1.55f, Offset.Zero); translate(-50f, -40f) }) { person(k, pose) }
                    PhotoFlaw.DARK -> {
                        drawRoundRect(Color(0xFFFFF3CF), Offset(26f, 16f), Size(48f, 96f), CornerRadius(3f))
                        person(k, pose, silhouette = true)
                    }
                    PhotoFlaw.BAGGY -> {
                        person(k, pose, arms = false)
                        poly(Color(0xFF8A7F70), 33f, 39f, 50f, 35f, 67f, 39f, 79f, 116f, 21f, 116f)
                        drawLine(Color(0xFF6F6558), Offset(50f, 38f), Offset(50f, 116f), strokeWidth = 1.2f)
                    }
                    PhotoFlaw.CROWD -> {
                        drawRoundRect(Clutter, Offset(4f, 20f), Size(26f, 4f), CornerRadius(1f))
                        drawRoundRect(Clutter, Offset(6f, 10f), Size(6f, 10f), CornerRadius(1f))
                        drawRoundRect(Clutter, Offset(14f, 13f), Size(5f, 7f), CornerRadius(1f))
                        drawRoundRect(Clutter, Offset(70f, 40f), Size(24f, 30f), CornerRadius(2f))
                        drawCircle(Clutter, 9f, Offset(12f, 104f))
                        drawRect(Clutter, Offset(10f, 108f), Size(4f, 20f))
                        withTransform({ translate(78f, 30f); scale(0.8f, 0.8f, Offset.Zero); translate(-50f, 0f) }) { person(k, "front") }
                        person(k, pose, cx = 40f)
                    }
                    PhotoFlaw.SELFIE -> {
                        person(k, pose, arms = false)
                        limb(k.skin, 38f, 43f, 29f, 80f)
                        limb(k.skin, 62f, 43f, 68f, 31f, 58f, 26f)
                        drawRoundRect(Color(0xFF17151C), Offset(47f, 14f), Size(13f, 19f), CornerRadius(2.5f))
                        drawRoundRect(Clutter, Offset(4f, 4f), Size(92f, 142f), CornerRadius(8f), style = Stroke(3f))
                    }
                    PhotoFlaw.COVERED -> {
                        person(k, pose, arms = false)
                        drawRoundRect(Color(0xFFA0522D), Offset(31f, 56f), Size(38f, 30f), CornerRadius(5f))
                        limb(k.skin, 38f, 43f, 33f, 62f, 60f, 64f)
                        limb(k.skin, 62f, 43f, 67f, 62f, 40f, 66f)
                    }
                    null -> person(k, pose)
                }
            }
            if (mark != null) {
                drawCircle(if (mark) k.ok else k.bad, 9f, Offset(86f, 14f))
                val ico = Path()
                if (mark) {
                    ico.moveTo(81.5f, 14.5f); ico.lineTo(85f, 18f); ico.lineTo(91f, 10.5f)
                } else {
                    ico.moveTo(82f, 10f); ico.lineTo(90f, 18f); ico.moveTo(90f, 10f); ico.lineTo(82f, 18f)
                }
                drawPath(ico, Color.White, style = Stroke(2.4f, cap = StrokeCap.Round, join = StrokeJoin.Round))
            }
        }
    }
}

@Composable
private fun flawLabel(f: PhotoFlaw): String = stringResource(
    when (f) {
        PhotoFlaw.CROPPED -> UiR.string.tryon_guide_cropped
        PhotoFlaw.DARK -> UiR.string.tryon_guide_dark
        PhotoFlaw.BAGGY -> UiR.string.tryon_guide_baggy
        PhotoFlaw.CROWD -> UiR.string.tryon_guide_crowd
        PhotoFlaw.SELFIE -> UiR.string.tryon_guide_selfie
        PhotoFlaw.COVERED -> UiR.string.tryon_guide_covered
    },
)

private class Example(val label: String, val pose: String = "front", val flaw: PhotoFlaw? = null, val ok: Boolean = false)

@Composable
private fun ExampleRow(cells: List<Example>) {
    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
        cells.forEach { e ->
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                PoseFigure(Modifier.fillMaxWidth().aspectRatio(100f / 150f), pose = e.pose, flaw = e.flaw, mark = e.ok)
                Text(e.label, style = MaterialTheme.typography.bodySmall)
            }
        }
        repeat(3 - cells.size) { Spacer(Modifier.weight(1f)) }
    }
}

/** Сворачиваемый блок «Примеры фото»: в мастере примерки и в «Моих фото». */
@Composable
fun PhotoGuideCard(defaultOpen: Boolean, modifier: Modifier = Modifier) {
    var open by rememberSaveable { mutableStateOf(defaultOpen) }
    val arrow by animateFloatAsState(if (open) 180f else 0f, label = "guide_arrow")
    SectionCard(modifier.testTag("photo_guide")) {
        Row(
            Modifier.fillMaxWidth().heightIn(min = 44.dp).clickable { open = !open }.testTag("photo_guide_toggle"),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(stringResource(UiR.string.tryon_guide_title), fontWeight = FontWeight.ExtraBold, style = MaterialTheme.typography.titleMedium, modifier = Modifier.weight(1f).semantics { heading() })
            Icon(Icons.Filled.KeyboardArrowDown, contentDescription = null, modifier = Modifier.rotate(arrow))
        }
        AnimatedVisibility(open) {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(stringResource(UiR.string.tryon_guide_good), color = Brand.colors.ok, fontWeight = FontWeight.Bold)
                Text(stringResource(UiR.string.tryon_guide_good_hint), style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted)
                ExampleRow(
                    listOf(
                        Example(stringResource(UiR.string.tryon_angle_front), pose = "front", ok = true),
                        Example(stringResource(UiR.string.tryon_angle_side), pose = "side", ok = true),
                        Example(stringResource(UiR.string.tryon_angle_back), pose = "back", ok = true),
                    ),
                )
                Text(stringResource(UiR.string.tryon_guide_bad), color = Brand.colors.bad, fontWeight = FontWeight.Bold, modifier = Modifier.padding(top = 4.dp))
                PhotoFlaw.entries.chunked(3).forEach { row ->
                    ExampleRow(row.map { f -> Example(flawLabel(f), flaw = f) })
                }
                Text(
                    stringResource(UiR.string.tryon_guide_tip),
                    style = MaterialTheme.typography.bodySmall,
                    modifier = Modifier.fillMaxWidth().background(Brand.colors.accentSoft, RoundedCornerShape(10.dp)).padding(10.dp),
                )
            }
        }
    }
}
