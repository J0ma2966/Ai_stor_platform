package uz.boutique.store

import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Badge
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
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
import kotlinx.coroutines.launch
import uz.boutique.core.model.StoreStats
import uz.boutique.ui.AppScaffold
import uz.boutique.ui.Brand
import uz.boutique.ui.ConfirmDialog
import uz.boutique.ui.LanguagePicker
import uz.boutique.ui.LoadContent
import uz.boutique.ui.LocalGraph
import uz.boutique.ui.ProductImage
import uz.boutique.ui.SecondaryButton
import uz.boutique.ui.SectionCard
import uz.boutique.ui.SectionTitle
import uz.boutique.ui.ServerDialog
import uz.boutique.ui.currentLang
import uz.boutique.ui.money
import uz.boutique.ui.rememberLoader
import uz.boutique.ui.R as UiR
import java.time.LocalDate
import java.time.ZoneId

@Composable
fun MoreScreen(onStats: () -> Unit, onNotifications: () -> Unit, onRestart: () -> Unit) {
    val graph = LocalGraph.current
    val api = graph.api
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val lang = currentLang()
    val profile = rememberLoader("store-profile") { api.store.profile() }
    val unread = rememberLoader("unread") { runCatching { api.notifications.unread() }.getOrDefault(0) }
    var confirmLogout by rememberSaveable { mutableStateOf(false) }
    var showServer by rememberSaveable { mutableStateOf(false) }
    AppScaffold(title = stringResource(UiR.string.nav_store_more)) { pad ->
        Column(
            Modifier.padding(pad).fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            profile.value?.let { s ->
                SectionCard {
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                        ProductImage(s.logoUrl, s.name, Modifier.size(56.dp))
                        Column {
                            Text(s.name, style = MaterialTheme.typography.titleMedium)
                            Text(stringResource(UiR.string.order_items_count, s.productCount), color = Brand.colors.muted)
                        }
                    }
                }
            }
            SectionCard(Modifier.clickable(onClick = onNotifications).testTag("notifications")) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(stringResource(UiR.string.app_notifications), style = MaterialTheme.typography.titleSmall, modifier = Modifier.weight(1f))
                    val n = unread.value ?: 0
                    if (n > 0) Badge { Text(n.toString()) }
                }
            }
            SectionCard(Modifier.clickable(onClick = onStats).testTag("stats")) {
                Text(stringResource(UiR.string.stats_title), style = MaterialTheme.typography.titleSmall)
            }
            SectionCard {
                Text(stringResource(UiR.string.store_profile_web_hint), color = Brand.colors.muted)
                SecondaryButton(stringResource(UiR.string.store_open_web), onClick = {
                    context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(graph.siteUrl + (if (lang == "uz") "/uz" else "") + "/store")))
                })
            }
            SectionCard { LanguagePicker() }
            SecondaryButton(stringResource(UiR.string.app_logout), onClick = { confirmLogout = true }, modifier = Modifier.testTag("logout"))
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

@Composable
fun StatsScreen(onBack: () -> Unit) {
    val api = LocalGraph.current.api
    var days by rememberSaveable { mutableStateOf(7) }
    val loader = rememberLoader(days) {
        val to = LocalDate.now(ZoneId.of("Asia/Tashkent"))
        api.store.stats(to.minusDays((days - 1).toLong()).toString(), to.toString())
    }
    AppScaffold(title = stringResource(UiR.string.stats_title), onBack = onBack) { pad ->
        Column(Modifier.padding(pad).fillMaxSize()) {
            Row(Modifier.padding(horizontal = 16.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                FilterChip(selected = days == 7, onClick = { days = 7 }, label = { Text(stringResource(UiR.string.stats_period_7)) })
                FilterChip(selected = days == 30, onClick = { days = 30 }, label = { Text(stringResource(UiR.string.stats_period_30)) })
            }
            LoadContent(loader) { s -> StatsContent(s) }
        }
    }
}

@Composable
fun StatsContent(s: StoreStats) {
    Column(
        Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            StatTile(stringResource(UiR.string.stats_views), s.views.toString(), Modifier.weight(1f))
            StatTile(stringResource(UiR.string.stats_orders), s.orders.toString(), Modifier.weight(1f))
        }
        Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            StatTile(stringResource(UiR.string.stats_delivered), s.ordersDelivered.toString(), Modifier.weight(1f))
            StatTile(stringResource(UiR.string.stats_revenue), money(s.revenue), Modifier.weight(1f))
        }
        Text(stringResource(UiR.string.stats_products, s.productsTotal, s.productsPublished), color = Brand.colors.muted)
        if (s.topProducts.isNotEmpty()) {
            SectionCard {
                SectionTitle(stringResource(UiR.string.stats_top))
                s.topProducts.forEach { t ->
                    Column {
                        Text(t.title, fontWeight = FontWeight.Bold)
                        Text(stringResource(UiR.string.stats_top_line, t.views.toInt(), t.ordered.toInt()), style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted)
                    }
                }
            }
        }
    }
}

@Composable
private fun StatTile(label: String, value: String, modifier: Modifier = Modifier) {
    SectionCard(modifier) {
        Text(label, style = MaterialTheme.typography.labelMedium, color = Brand.colors.muted)
        Text(value, style = MaterialTheme.typography.titleLarge, modifier = Modifier.testTag("stat_$label"))
    }
}
