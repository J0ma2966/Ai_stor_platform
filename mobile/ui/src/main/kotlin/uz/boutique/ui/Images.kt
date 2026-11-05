package uz.boutique.ui

import android.content.Context
import android.content.SharedPreferences
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.graphics.ImageDecoder
import android.graphics.Matrix
import android.media.ExifInterface
import android.net.Uri
import android.os.Build
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import uz.boutique.core.api.ApiException
import uz.boutique.core.api.KeyValueStore
import java.io.ByteArrayOutputStream
import kotlin.math.max

/**
 * Фото для загрузки: уменьшаем до [maxSide] по длинной стороне и пережимаем в JPEG.
 * Поворот по EXIF применяется здесь, сами EXIF (в т.ч. геометка) в файл не попадают.
 * Сервер всё равно перекодирует картинку, но так телефон не отправляет 10+ МБ.
 */
suspend fun readImageForUpload(context: Context, uri: Uri, maxSide: Int = 2560): ByteArray = withContext(Dispatchers.IO) {
    val cr = context.contentResolver
    val bitmap: Bitmap = try {
        if (Build.VERSION.SDK_INT >= 28) {
            ImageDecoder.decodeBitmap(ImageDecoder.createSource(cr, uri)) { decoder, info, _ ->
                val s = max(info.size.width, info.size.height)
                if (s > maxSide) {
                    val k = maxSide.toDouble() / s
                    decoder.setTargetSize((info.size.width * k).toInt().coerceAtLeast(1), (info.size.height * k).toInt().coerceAtLeast(1))
                }
                decoder.allocator = ImageDecoder.ALLOCATOR_SOFTWARE
            }
        } else {
            val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
            cr.openInputStream(uri)?.use { BitmapFactory.decodeStream(it, null, bounds) }
            var sample = 1
            while (max(bounds.outWidth, bounds.outHeight) / (sample * 2) >= maxSide) sample *= 2
            val raw = cr.openInputStream(uri)?.use { BitmapFactory.decodeStream(it, null, BitmapFactory.Options().apply { inSampleSize = sample }) }
                ?: throw ApiException(0, "corrupt_image", "cannot decode")
            val rotation = cr.openInputStream(uri)?.use {
                when (ExifInterface(it).getAttributeInt(ExifInterface.TAG_ORIENTATION, ExifInterface.ORIENTATION_NORMAL)) {
                    ExifInterface.ORIENTATION_ROTATE_90 -> 90f
                    ExifInterface.ORIENTATION_ROTATE_180 -> 180f
                    ExifInterface.ORIENTATION_ROTATE_270 -> 270f
                    else -> 0f
                }
            } ?: 0f
            val s = max(raw.width, raw.height)
            val k = if (s > maxSide) maxSide.toFloat() / s else 1f
            if (rotation == 0f && k == 1f) raw else {
                val m = Matrix().apply { postScale(k, k); postRotate(rotation) }
                Bitmap.createBitmap(raw, 0, 0, raw.width, raw.height, m, true)
            }
        }
    } catch (e: ApiException) {
        throw e
    } catch (e: Exception) {
        throw ApiException(0, "corrupt_image", e.message ?: "cannot decode")
    }
    ByteArrayOutputStream().use { out ->
        bitmap.compress(Bitmap.CompressFormat.JPEG, 92, out)
        out.toByteArray()
    }
}

/** Несекретные настройки на телефоне (например, выбор в примерочной). */
class PrefsStore(context: Context, name: String) : KeyValueStore {
    private val prefs: SharedPreferences = context.applicationContext.getSharedPreferences(name, Context.MODE_PRIVATE)
    override fun get(key: String): String? = prefs.getString(key, null)
    override fun put(key: String, value: String?) {
        prefs.edit().apply { if (value == null) remove(key) else putString(key, value) }.apply()
    }
}
