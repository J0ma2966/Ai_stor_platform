package uz.boutique.client

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import uz.boutique.core.model.Store
import uz.boutique.ui.AppScaffold
import uz.boutique.ui.Brand
import uz.boutique.ui.EmptyState
import uz.boutique.ui.LoadContent
import uz.boutique.ui.LocalGraph
import uz.boutique.ui.R as UiR
import uz.boutique.ui.rememberLoader
import uz.boutique.ui.surfaceDepth

/** «Все бутики» (как /stores на сайте): список магазинов, нажатие открывает товары бутика. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun StoresScreen(onBack: () -> Unit, onStore: (Store) -> Unit) {
    val api = LocalGraph.current.api
    val stores = rememberLoader("stores") { api.catalog.stores() }
    AppScaffold(title = stringResource(UiR.string.stores_title), onBack = onBack) { pad ->
        LoadContent(stores, Modifier.padding(pad)) { list ->
            PullToRefreshBox(isRefreshing = stores.refreshing, onRefresh = { stores.reload() }, modifier = Modifier.padding(pad).fillMaxSize()) {
                if (list.isEmpty()) {
                    EmptyState(stringResource(UiR.string.stores_empty))
                } else {
                    LazyColumn(
                        contentPadding = PaddingValues(16.dp),
                        verticalArrangement = Arrangement.spacedBy(12.dp),
                        modifier = Modifier.fillMaxSize().testTag("stores_list"),
                    ) {
                        items(list, key = { it.id }) { s ->
                            Column(
                                Modifier
                                    .fillMaxWidth()
                                    .surfaceDepth(Brand.colors, MaterialTheme.shapes.large, elevation = 4.dp)
                                    .clip(MaterialTheme.shapes.large)
                                    .clickable { onStore(s) }
                                    .padding(14.dp)
                                    .testTag("stores_${s.slug}"),
                                verticalArrangement = Arrangement.spacedBy(8.dp),
                            ) {
                                StoreHeader(s)
                                if (s.description.isNotBlank()) {
                                    Text(s.description, style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted, maxLines = 3, overflow = TextOverflow.Ellipsis)
                                }
                                val place = s.address
                                if (!place.isNullOrBlank()) {
                                    Text(place, style = MaterialTheme.typography.labelSmall, color = Brand.colors.muted, maxLines = 1, overflow = TextOverflow.Ellipsis)
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}
