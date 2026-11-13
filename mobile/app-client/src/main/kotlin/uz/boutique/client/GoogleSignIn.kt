package uz.boutique.client

import android.content.Context
import androidx.credentials.CredentialManager
import androidx.credentials.CustomCredential
import androidx.credentials.GetCredentialRequest
import androidx.credentials.exceptions.GetCredentialCancellationException
import androidx.credentials.exceptions.GetCredentialException
import com.google.android.libraries.identity.googleid.GetSignInWithGoogleOption
import com.google.android.libraries.identity.googleid.GoogleIdTokenCredential

/**
 * ID-токен Google через Credential Manager. serverClientId — веб-клиент OAuth из
 * /v1/auth/methods: сервер проверяет, что токен выписан для него (docs/AUTH.md).
 * null — Google на этом телефоне недоступен (нет сервисов Google или аккаунта);
 * пустая строка — человек закрыл окно выбора аккаунта.
 */
suspend fun googleIdToken(context: Context, serverClientId: String): String? {
    val request = GetCredentialRequest.Builder()
        .addCredentialOption(GetSignInWithGoogleOption.Builder(serverClientId).build())
        .build()
    return try {
        val cred = CredentialManager.create(context).getCredential(context, request).credential
        if (cred is CustomCredential && cred.type == GoogleIdTokenCredential.TYPE_GOOGLE_ID_TOKEN_CREDENTIAL) {
            GoogleIdTokenCredential.createFrom(cred.data).idToken
        } else {
            null
        }
    } catch (_: GetCredentialCancellationException) {
        ""
    } catch (_: GetCredentialException) {
        null
    }
}
