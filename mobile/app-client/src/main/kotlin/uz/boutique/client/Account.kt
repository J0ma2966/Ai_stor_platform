package uz.boutique.client

import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.GridItemSpan
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.material3.Badge
import androidx.compose.material3.Checkbox
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.launch
import uz.boutique.core.api.AuthState
import uz.boutique.core.model.CartItem
import uz.boutique.core.util.Phone
import uz.boutique.ui.AppScaffold
import uz.boutique.ui.Brand
import uz.boutique.ui.ConfirmDialog
import uz.boutique.ui.EmptyState
import uz.boutique.ui.ErrorBanner
import uz.boutique.ui.LanguagePicker
import uz.boutique.ui.LocalGraph
import uz.boutique.ui.PagedFooter
import uz.boutique.ui.PagedState
import uz.boutique.ui.PrimaryButton
import uz.boutique.ui.ProductImage
import uz.boutique.ui.SecondaryButton
import uz.boutique.ui.SectionCard
import uz.boutique.ui.SectionTitle
import uz.boutique.ui.ServerDialog
import uz.boutique.ui.currentLang
import uz.boutique.ui.money
import uz.boutique.ui.rememberAction
import uz.boutique.ui.rememberLoader
import uz.boutique.ui.rememberPaged
import uz.boutique.ui.R as UiR

/** Раздел требует входа: короткое объяснение и кнопка «Войти». */
@Composable
fun GuestScreen(title: Int, onLogin: () -> Unit) {
    AppScaffold(title = stringResource(title)) { pad ->
        EmptyState(stringResource(UiR.string.login_guest_hint), Modifier.padding(pad)) {
            PrimaryButton(stringResource(UiR.string.login_sign_in), onClick = onLogin, modifier = Modifier.testTag("guest_login"))
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun FavoritesScreen(onProduct: (String) -> Unit) {
    val api = LocalGraph.current.api
    val shop = LocalShop.current
    val paged = rememberPaged<CartItem>("favorites", shop.favIds.size) { c -> api.shop.favorites(c) }
    AppScaffold(title = stringResource(UiR.string.nav_favorites)) { pad ->
        PullToRefreshBox(isRefreshing = paged.loading && paged.loadedOnce, onRefresh = { paged.refresh() }, modifier = Modifier.padding(pad).fillMaxSize()) {
            PagedState(paged, stringResource(UiR.string.fav_empty)) {
                LazyVerticalGrid(
                    columns = GridCells.Adaptive(160.dp),
                    contentPadding = PaddingValues(16.dp),
                    horizontalArrangement = Arrangement.spacedBy(12.dp),
                    verticalArrangement = Arrangement.spacedBy(16.dp),
                ) {
                    items(paged.items, key = { it.productId }) { f ->
                        Column(Modifier.clickable { onProduct(f.productId) }, verticalArrangement = Arrangement.spacedBy(4.dp)) {
                            Box {
                                ProductImage(f.imageUrl, f.title, Modifier.fillMaxWidth().aspectRatio(3f / 4f))
                                FavoriteButton(f.productId, onNeedLogin = {}, modifier = Modifier.align(Alignment.TopEnd).padding(6.dp))
                            }
                            Text(money(f.price), style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.ExtraBold)
                            Text(f.title, maxLines = 2)
                            if (!f.available) Text(stringResource(UiR.string.product_out_of_stock), color = Brand.colors.bad, style = MaterialTheme.typography.labelSmall)
                        }
                    }
                    item(span = { GridItemSpan(maxLineSpan) }) { PagedFooter(paged) }
                }
            }
        }
    }
}

@Composable
fun ProfileScreen(onLogin: () -> Unit, onChangePassword: () -> Unit, onNotifications: () -> Unit, onRestart: () -> Unit) {
    val graph = LocalGraph.current
    val api = graph.api
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val lang = currentLang()
    val auth by api.session.state.collectAsStateWithLifecycle()
    val loggedIn = auth is AuthState.LoggedIn
    val me = rememberLoader(auth) { if (loggedIn) api.auth.me() else null }
    val unread = rememberLoader(auth, "unread") { if (loggedIn) runCatching { api.notifications.unread() }.getOrDefault(0) else 0 }
    val platform = rememberLoader("platform") { runCatching { api.catalog.platform() }.getOrNull() }
    var confirmLogout by rememberSaveable { mutableStateOf(false) }
    var deleteOpen by rememberSaveable { mutableStateOf(false) }
    var deleteChecked by rememberSaveable { mutableStateOf(false) }
    var showServer by rememberSaveable { mutableStateOf(false) }
    val action = rememberAction()
    val deletedText = stringResource(UiR.string.profile_deleted)

    fun open(url: String) = context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)))

    AppScaffold(title = stringResource(UiR.string.nav_profile)) { pad ->
        Column(
            Modifier.padding(pad).fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            if (!loggedIn) {
                SectionCard {
                    Text(stringResource(UiR.string.profile_guest), style = MaterialTheme.typography.titleMedium)
                    Text(stringResource(UiR.string.login_guest_hint), color = Brand.colors.muted)
                    PrimaryButton(stringResource(UiR.string.login_sign_in), onClick = onLogin, modifier = Modifier.testTag("profile_login"))
                }
            } else {
                SectionCard {
                    Text(stringResource(UiR.string.profile_phone), style = MaterialTheme.typography.labelMedium, color = Brand.colors.muted)
                    Text(Phone.display(me.value?.phone), style = MaterialTheme.typography.titleMedium)
                }
                SectionCard(Modifier.clickable(onClick = onNotifications)) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text(stringResource(UiR.string.app_notifications), style = MaterialTheme.typography.titleSmall, modifier = Modifier.weight(1f))
                        val n = unread.value ?: 0
                        if (n > 0) Badge { Text(n.toString()) }
                    }
                }
                SectionCard {
                    SectionTitle(stringResource(UiR.string.profile_password))
                    Text(stringResource(UiR.string.profile_password_text), color = Brand.colors.muted)
                    SecondaryButton(stringResource(UiR.string.password_change_title), onClick = onChangePassword)
                }
            }
            SectionCard {
                LanguagePicker(onChanged = { code -> if (loggedIn) scope.launch { runCatching { api.auth.setLang(code) } } })
            }
            SectionCard {
                SectionTitle(stringResource(UiR.string.app_support))
                val p = platform.value
                p?.supportPhone?.takeIf { it.isNotBlank() }?.let { ph ->
                    TextButton(onClick = { context.startActivity(Intent(Intent.ACTION_DIAL, Uri.parse("tel:$ph"))) }) { Text(Phone.display(ph)) }
                }
                p?.supportTelegram?.takeIf { it.isNotBlank() }?.let { tg ->
                    TextButton(onClick = { open("https://t.me/" + tg.removePrefix("@")) }) { Text("Telegram: $tg") }
                }
                p?.supportEmail?.takeIf { it.isNotBlank() }?.let { em ->
                    TextButton(onClick = { context.startActivity(Intent(Intent.ACTION_SENDTO, Uri.parse("mailto:$em"))) }) { Text(em) }
                }
                TextButton(onClick = { open(graph.siteUrl + (if (lang == "uz") "/uz" else "") + "/legal/privacy") }) {
                    Text(stringResource(UiR.string.profile_documents))
                }
            }
            if (loggedIn) {
                SecondaryButton(stringResource(UiR.string.app_logout), onClick = { confirmLogout = true }, modifier = Modifier.testTag("logout"))
                SectionCard {
                    SectionTitle(stringResource(UiR.string.profile_delete_title))
                    Text(stringResource(UiR.string.profile_delete_text), color = Brand.colors.muted, style = MaterialTheme.typography.bodySmall)
                    if (deleteOpen) {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Checkbox(checked = deleteChecked, onCheckedChange = { deleteChecked = it })
                            Text(stringResource(UiR.string.profile_delete_confirm))
                        }
                        SecondaryButton(stringResource(UiR.string.profile_delete_btn), color = Brand.colors.bad, enabled = deleteChecked && !action.busy, onClick = {
                            action.run {
                                api.auth.deleteAccount()
                                android.widget.Toast.makeText(context, deletedText, android.widget.Toast.LENGTH_LONG).show()
                            }
                        })
                    } else {
                        TextButton(onClick = { deleteOpen = true }) { Text(stringResource(UiR.string.profile_delete_title), color = Brand.colors.bad) }
                    }
                    ErrorBanner(action.error)
                }
            }
            Text(
                stringResource(UiR.string.app_version, BuildConfig.VERSION_NAME) + if (graph.allowServerOverride) " · " + graph.baseUrl else "",
                style = MaterialTheme.typography.bodySmall,
                color = Brand.colors.muted,
                modifier = if (graph.allowServerOverride) Modifier.clickable { showServer = true } else Modifier,
            )
        }
    }
    if (confirmLogout) {
        ConfirmDialog(
            text = stringResource(UiR.string.app_logout_confirm),
            confirmLabel = stringResource(UiR.string.app_logout),
            onConfirm = { confirmLogout = false; scope.launch { api.auth.logout() } },
            onDismiss = { confirmLogout = false },
        )
    }
    if (showServer) ServerDialog(onDismiss = { showServer = false }, onSaved = { showServer = false; onRestart() })
}
