package uz.boutique.ui

import androidx.annotation.StringRes
import androidx.compose.runtime.Composable
import androidx.compose.ui.res.stringResource
import uz.boutique.core.api.ApiException
import uz.boutique.core.model.Role

/** Строка для кода ошибки API или null, если код неизвестен. */
@StringRes
fun errorRes(code: String, appRole: Role): Int? = when (code) {
    "network" -> R.string.err_network
    "unauthorized" -> R.string.err_unauthorized
    "token_invalid", "refresh_invalid" -> R.string.err_session
    "token_revoked" -> R.string.err_token_revoked
    "forbidden" -> R.string.err_forbidden
    "account_blocked" -> R.string.err_account_blocked
    "not_found" -> R.string.err_not_found
    "validation_failed" -> R.string.err_validation_failed
    "rate_limited" -> R.string.err_rate_limited
    "otp_cooldown" -> R.string.err_otp_cooldown
    "otp_invalid" -> R.string.err_otp_invalid
    "bad_credentials" -> R.string.err_bad_credentials
    "store_not_active" -> R.string.err_store_not_active
    "phone_not_allowed" -> R.string.err_phone_not_allowed
    "products_unavailable" -> R.string.err_products_unavailable
    "idempotency_key_reused" -> R.string.err_idempotency_key_reused
    "request_in_progress" -> R.string.err_request_in_progress
    "invalid_transition" -> R.string.err_invalid_transition
    "status_changed", "conflict" -> R.string.err_status_changed
    "file_too_large", "payload_too_large" -> R.string.err_file_too_large
    "unsupported_type" -> R.string.err_unsupported_type
    "upload_in_progress" -> R.string.err_upload_in_progress
    "upload_busy" -> R.string.err_upload_busy
    "internal", "bad_response" -> R.string.err_internal
    "unavailable" -> R.string.err_unavailable
    "out_of_stock" -> R.string.err_out_of_stock
    "not_enough_stock" -> R.string.err_not_enough_stock
    "size_required" -> R.string.err_size_required
    "size_unknown" -> R.string.err_size_unknown
    "cart_full" -> R.string.err_cart_full
    "favorites_full" -> R.string.err_favorites_full
    "images_required" -> R.string.err_images_required
    "product_blocked" -> R.string.err_product_blocked
    "sku_taken" -> R.string.err_sku_taken
    "cannot_cancel" -> R.string.err_cannot_cancel
    "current_password_required" -> R.string.err_current_password_required
    "current_password_wrong" -> R.string.err_current_password_wrong
    "review_not_allowed" -> R.string.err_review_not_allowed
    "review_hidden" -> R.string.err_review_hidden
    "registration_incomplete" -> R.string.err_registration_incomplete
    "corrupt_image" -> R.string.err_corrupt_image
    "read_file" -> R.string.err_read_file
    "tryon_consent_required" -> R.string.err_tryon_consent_required
    "tryon_transfer_consent_required" -> R.string.err_tryon_transfer_consent_required
    "insufficient_tokens" -> R.string.err_insufficient_tokens
    "too_many_active_generations" -> R.string.err_too_many_active_generations
    "generation_in_progress" -> R.string.err_generation_in_progress
    "tryon_disabled" -> R.string.err_tryon_disabled
    ApiException.WRONG_APP -> if (appRole == Role.STORE) R.string.err_wrong_app_store else R.string.err_wrong_app_client
    else -> null
}

/** Текст ошибки для пользователя + код для поддержки (request_id), как на сайте. */
@Composable
fun errorText(e: Throwable?): String {
    if (e == null) return ""
    val role = LocalGraph.current.role
    if (e !is ApiException) return stringResource(R.string.err_internal)
    val res = errorRes(e.code, role)
    val main = if (res != null) stringResource(res) else stringResource(R.string.err_unknown, e.code)
    return if (e.requestId.isNotEmpty() && (e.status >= 500 || res == null)) {
        main + "\n" + stringResource(R.string.err_request_id, e.requestId)
    } else {
        main
    }
}

/** Ошибка конкретного поля формы из ответа 422 (fields) или локальной проверки. */
@Composable
fun fieldErrorText(code: String?): String? = when (code) {
    null -> null
    "required" -> stringResource(R.string.field_required)
    "max", "length" -> stringResource(R.string.field_max)
    "e164" -> stringResource(R.string.field_e164)
    "gt" -> stringResource(R.string.field_gt)
    "range" -> stringResource(R.string.field_range)
    else -> stringResource(R.string.field_invalid)
}

fun Throwable?.fields(): Map<String, String> = (this as? ApiException)?.fields ?: emptyMap()
