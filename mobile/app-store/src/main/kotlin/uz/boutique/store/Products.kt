package uz.boutique.store

import android.net.Uri
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Close
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonArray
import kotlinx.serialization.json.putJsonObject
import uz.boutique.core.api.ApiException
import uz.boutique.core.model.Category
import uz.boutique.core.model.Product
import uz.boutique.core.util.Money
import uz.boutique.ui.AppScaffold
import uz.boutique.ui.Brand
import uz.boutique.ui.ConfirmDialog
import uz.boutique.ui.EmptyState
import uz.boutique.ui.ErrorBanner
import uz.boutique.ui.ErrorState
import uz.boutique.ui.CenterLoading
import uz.boutique.ui.FormField
import uz.boutique.ui.InfoBanner
import uz.boutique.ui.Load
import uz.boutique.ui.LocalGraph
import uz.boutique.ui.Pill
import uz.boutique.ui.PrimaryButton
import uz.boutique.ui.ProductImage
import uz.boutique.ui.ScreenPadding
import uz.boutique.ui.SecondaryButton
import uz.boutique.ui.SectionCard
import uz.boutique.ui.SectionTitle
import uz.boutique.ui.currentLang
import uz.boutique.ui.fieldErrorText
import uz.boutique.ui.fields
import uz.boutique.ui.money
import uz.boutique.ui.rememberAction
import uz.boutique.ui.rememberLoader
import uz.boutique.ui.R as UiR

@Composable
fun ProductStatusPill(status: String?) {
    val (label, color) = when (status) {
        "published" -> UiR.string.prod_status_published to Brand.colors.ok
        "unpublished" -> UiR.string.prod_status_unpublished to Brand.colors.warn
        "blocked" -> UiR.string.prod_status_blocked to Brand.colors.bad
        else -> UiR.string.prod_status_draft to Brand.colors.muted
    }
    Pill(stringResource(label), color)
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun StoreProductsScreen(onProduct: (String) -> Unit, onNew: () -> Unit) {
    val api = LocalGraph.current.api
    val loader = rememberLoader("store-products") { api.store.products() }
    AppScaffold(
        title = stringResource(UiR.string.nav_store_products),
        floatingActionButton = {
            FloatingActionButton(onClick = onNew, modifier = Modifier.testTag("new_product")) {
                Icon(Icons.Filled.Add, contentDescription = stringResource(UiR.string.store_product_new))
            }
        },
    ) { pad ->
        run {
            PullToRefreshBox(isRefreshing = loader.refreshing, onRefresh = { loader.reload() }, modifier = Modifier.padding(pad).fillMaxSize()) {
                when (val s = loader.state) {
                    is Load.Loading -> CenterLoading()
                    is Load.Failed -> ErrorState(s.error, onRetry = { loader.reload() })
                    is Load.Ok -> if (s.value.isEmpty()) {
                        EmptyState(stringResource(UiR.string.store_products_empty))
                    } else {
                        LazyColumn(contentPadding = ScreenPadding, verticalArrangement = Arrangement.spacedBy(10.dp)) {
                            items(s.value, key = { it.id }) { p -> StoreProductRow(p) { onProduct(p.id) } }
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun StoreProductRow(p: Product, onClick: () -> Unit) {
    SectionCard(Modifier.clickable(onClick = onClick).testTag("store_product_${p.id}"), padding = androidx.compose.foundation.layout.PaddingValues(12.dp)) {
        Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            ProductImage(p.cardImage, p.title, Modifier.size(width = 64.dp, height = 86.dp))
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(p.title, style = MaterialTheme.typography.titleSmall, maxLines = 2)
                if (!p.sku.isNullOrBlank()) Text(p.sku!!, style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted)
                Text(money(p.price), fontWeight = FontWeight.ExtraBold)
                Row(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalAlignment = Alignment.CenterVertically) {
                    ProductStatusPill(p.status)
                    if (!p.inStock) Pill(stringResource(UiR.string.product_out_of_stock), Brand.colors.bad)
                }
                p.views?.let { Text(stringResource(UiR.string.prod_views, it.toInt()), style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted) }
            }
        }
    }
}

/** Строка редактора размеров. */
private data class SizeRow(val size: String, val stock: String)

/**
 * Создание (id == null) и редактирование товара. Создание требует три фото (как на сайте):
 * товар создаётся черновиком, публикация — отдельной кнопкой.
 */
@Composable
fun ProductEditScreen(id: String?, onBack: () -> Unit, onCreated: (String) -> Unit) {
    val api = LocalGraph.current.api
    val categories = rememberLoader("categories") { api.catalog.categories() }
    if (id == null) {
        AppScaffold(title = stringResource(UiR.string.store_product_new), onBack = onBack) { pad ->
            when (val c = categories.state) {
                is Load.Loading -> CenterLoading(Modifier.padding(pad))
                is Load.Failed -> ErrorState(c.error, { categories.reload() }, Modifier.padding(pad))
                is Load.Ok -> ProductForm(null, c.value, Modifier.padding(pad), onCreated = onCreated, onDeleted = onBack)
            }
        }
    } else {
        val product = rememberLoader(id) { api.store.product(id) }
        AppScaffold(title = product.value?.title ?: stringResource(UiR.string.store_product_edit), onBack = onBack) { pad ->
            val p = product.state
            val c = categories.state
            when {
                p is Load.Failed -> ErrorState(p.error, { product.reload() }, Modifier.padding(pad))
                c is Load.Failed -> ErrorState(c.error, { categories.reload() }, Modifier.padding(pad))
                p is Load.Ok && c is Load.Ok -> ProductForm(p.value, c.value, Modifier.padding(pad), onSaved = { product.set(it) }, onCreated = {}, onDeleted = onBack)
                else -> CenterLoading(Modifier.padding(pad))
            }
        }
    }
}

@Composable
private fun ProductForm(
    p: Product?,
    categories: List<Category>,
    modifier: Modifier,
    onSaved: (Product) -> Unit = {},
    onCreated: (String) -> Unit,
    onDeleted: () -> Unit,
) {
    val api = LocalGraph.current.api
    val context = LocalContext.current
    val lang = currentLang()
    val key = p?.id ?: "new"
    var title by rememberSaveable(key) { mutableStateOf(p?.title.orEmpty()) }
    var description by rememberSaveable(key) { mutableStateOf(p?.description.orEmpty()) }
    var brand by rememberSaveable(key) { mutableStateOf(p?.brand.orEmpty()) }
    var price by rememberSaveable(key) { mutableStateOf(p?.let { (it.price / 100).toString() }.orEmpty()) }
    var sku by rememberSaveable(key) { mutableStateOf(p?.sku.orEmpty()) }
    var categoryId by rememberSaveable(key) { mutableStateOf(p?.categoryId ?: "") }
    var inStock by rememberSaveable(key) { mutableStateOf(p?.inStock ?: true) }
    val sizes = remember(key) { mutableStateListOf<SizeRow>().apply { p?.sizes?.forEach { add(SizeRow(it.size, (it.stock ?: 0).toString())) } } }
    // ракурс → (ключ загрузки, ссылка для предпросмотра); ключи только у новых фото
    val newImages = remember(key) { mutableStateOf(mapOf<String, Pair<String, String>>()) }
    var catMenu by remember { mutableStateOf(false) }
    var localErrors by remember { mutableStateOf(mapOf<String, String>()) }
    var confirmDelete by rememberSaveable { mutableStateOf(false) }
    var pickAngle by rememberSaveable { mutableStateOf<String?>(null) }
    val save = rememberAction()
    val upload = rememberAction()
    val status = rememberAction()
    val savedText = stringResource(UiR.string.prod_saved)
    val blocked = p?.status == "blocked"

    val picker = rememberLauncherForActivityResult(ActivityResultContracts.PickVisualMedia()) { uri: Uri? ->
        val angle = pickAngle
        if (uri != null && angle != null) upload.run {
            val (bytes, type) = withContext(Dispatchers.IO) {
                val cr = context.contentResolver
                val b = cr.openInputStream(uri)?.use { it.readBytes() } ?: throw ApiException(0, "read_file", "cannot read")
                b to (cr.getType(uri) ?: "image/jpeg")
            }
            val res = api.uploads.upload("product", bytes, type)
            newImages.value = newImages.value + (angle to (res.key to uri.toString()))
        }
    }

    fun body(forCreate: Boolean): JsonObject? {
        val errs = mutableMapOf<String, String>()
        val tiyin = Money.parseSumToTiyin(price)
        if (title.isBlank()) errs["title"] = "required"
        if (description.isBlank()) errs["description"] = "required"
        if (brand.isBlank()) errs["brand"] = "required"
        if (tiyin == null) errs["price"] = "gt"
        if (categoryId.isBlank()) errs["category_id"] = "required"
        if (forCreate && newImages.value.size < 3) errs["images"] = "required"
        val parsedSizes = sizes.filter { it.size.isNotBlank() }.map { it.size.trim() to (it.stock.toIntOrNull() ?: -1) }
        parsedSizes.forEachIndexed { i, (_, st) -> if (st < 0) errs["sizes[$i].stock"] = "invalid" }
        localErrors = errs
        if (errs.isNotEmpty()) return null
        return buildJsonObject {
            put("title", title.trim())
            put("description", description.trim())
            put("brand", brand.trim())
            put("price", tiyin!!)
            put("category_id", categoryId)
            put("in_stock", inStock)
            if (sku.isNotBlank()) put("sku", sku.trim())
            if (newImages.value.isNotEmpty()) putJsonObject("images") { newImages.value.forEach { (a, v) -> put(a, v.first) } }
            putJsonArray("sizes") {
                parsedSizes.forEach { (s, st) -> add(buildJsonObject { put("size", s); put("stock", st) }) }
            }
        }
    }

    val serverFields = save.error.fields()
    @Composable
    fun err(k: String) = fieldErrorText(localErrors[k] ?: serverFields[k])

    Column(
        modifier.fillMaxSize().imePadding().verticalScroll(rememberScrollState()).padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        if (p != null) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                ProductStatusPill(p.status)
                if (!p.sku.isNullOrBlank()) Text(p.sku!!, color = Brand.colors.muted)
            }
            if (!p.moderationReason.isNullOrBlank()) InfoBanner(stringResource(UiR.string.prod_moderation_reason, p.moderationReason!!), color = Brand.colors.warn)
            if (blocked) InfoBanner(stringResource(UiR.string.prod_blocked_hint), color = Brand.colors.bad)
        }

        SectionTitle(stringResource(UiR.string.prod_photos))
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            for ((angle, label) in listOf("front" to UiR.string.angle_front, "side" to UiR.string.angle_side, "back" to UiR.string.angle_back)) {
                Column(Modifier.weight(1f), horizontalAlignment = Alignment.CenterHorizontally) {
                    val url = newImages.value[angle]?.second ?: p?.images?.get(angle)
                    Box(
                        Modifier.aspectRatio(3f / 4f).clickable(enabled = !blocked && !upload.busy) {
                            pickAngle = angle
                            picker.launch(PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageOnly))
                        }.testTag("photo_$angle"),
                        contentAlignment = Alignment.Center,
                    ) {
                        ProductImage(url, stringResource(label), Modifier.fillMaxSize())
                        if (url == null) Icon(Icons.Filled.Add, contentDescription = stringResource(UiR.string.prod_photo_pick))
                    }
                    Text(stringResource(label), style = MaterialTheme.typography.labelSmall)
                }
            }
        }
        if (upload.busy) Text(stringResource(UiR.string.prod_uploading), color = Brand.colors.muted)
        ErrorBanner(upload.error)
        if (localErrors["images"] != null) Text(stringResource(UiR.string.prod_photos_required), color = Brand.colors.bad)

        FormField(title, { title = it }, stringResource(UiR.string.prod_title), error = err("title"), maxLength = 300, enabled = !blocked, modifier = Modifier.testTag("title"))
        FormField(brand, { brand = it }, stringResource(UiR.string.prod_brand), error = err("brand"), maxLength = 120, enabled = !blocked, modifier = Modifier.testTag("brand"))
        FormField(price, { v -> price = v.filter { it.isDigit() }.take(12) }, stringResource(UiR.string.prod_price), keyboardType = KeyboardType.Number, error = err("price"), enabled = !blocked, modifier = Modifier.testTag("price"))
        FormField(sku, { sku = it }, stringResource(UiR.string.prod_sku), hint = if (p == null) stringResource(UiR.string.prod_sku_hint) else null, error = err("sku"), maxLength = 64, enabled = !blocked)
        Box {
            val catName = categories.firstOrNull { it.id == categoryId }?.name(lang).orEmpty()
            FormField(catName, {}, stringResource(UiR.string.prod_category), error = err("category_id"), enabled = false)
            Box(Modifier.matchParentSize().clickable(enabled = !blocked) { catMenu = true }.testTag("category"))
            DropdownMenu(expanded = catMenu, onDismissRequest = { catMenu = false }) {
                categories.forEach { c ->
                    DropdownMenuItem(text = { Text(c.name(lang)) }, onClick = { categoryId = c.id; catMenu = false })
                }
            }
        }
        FormField(description, { description = it }, stringResource(UiR.string.prod_description), error = err("description"), singleLine = false, minLines = 3, maxLength = 10000, enabled = !blocked)

        SectionTitle(stringResource(UiR.string.prod_sizes))
        Text(stringResource(UiR.string.prod_sizes_hint), style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted)
        if (sizes.isEmpty()) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(stringResource(UiR.string.prod_in_stock), modifier = Modifier.weight(1f))
                Switch(checked = inStock, onCheckedChange = { inStock = it }, enabled = !blocked, modifier = Modifier.testTag("in_stock"))
            }
        }
        sizes.forEachIndexed { i, row ->
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                FormField(row.size, { sizes[i] = row.copy(size = it.take(20)) }, stringResource(UiR.string.prod_size_name), modifier = Modifier.weight(1f), enabled = !blocked)
                FormField(row.stock, { v -> sizes[i] = row.copy(stock = v.filter { it.isDigit() }.take(5)) }, stringResource(UiR.string.prod_size_stock), keyboardType = KeyboardType.Number, error = err("sizes[$i].stock"), modifier = Modifier.weight(1f), enabled = !blocked)
                IconButton(onClick = { sizes.removeAt(i) }, enabled = !blocked) {
                    Icon(Icons.Filled.Close, contentDescription = stringResource(UiR.string.prod_remove_size))
                }
            }
        }
        TextButton(onClick = { sizes.add(SizeRow("", "1")) }, enabled = !blocked) { Text(stringResource(UiR.string.prod_add_size)) }

        ErrorBanner(save.error ?: status.error)
        if (!blocked) {
            PrimaryButton(
                stringResource(if (p == null) UiR.string.prod_create else UiR.string.app_save),
                busy = save.busy,
                enabled = !upload.busy,
                modifier = Modifier.testTag("save_product"),
                onClick = {
                    val b = body(forCreate = p == null)
                    if (b != null) save.run {
                        if (p == null) {
                            val created = api.store.create(b)
                            android.widget.Toast.makeText(context, context.getString(UiR.string.prod_created), android.widget.Toast.LENGTH_LONG).show()
                            onCreated(created.id)
                        } else {
                            val updated = api.store.update(p.id, b)
                            newImages.value = emptyMap()
                            onSaved(updated)
                            android.widget.Toast.makeText(context, savedText, android.widget.Toast.LENGTH_SHORT).show()
                        }
                    }
                },
            )
        }
        if (p != null && !blocked) {
            if (p.status == "published") {
                SecondaryButton(stringResource(UiR.string.prod_unpublish), enabled = !status.busy, onClick = {
                    status.run { api.store.setStatus(p.id, "draft"); onSaved(api.store.product(p.id)) }
                })
            } else {
                SecondaryButton(stringResource(UiR.string.prod_publish), enabled = !status.busy, modifier = Modifier.testTag("publish"), onClick = {
                    status.run { api.store.setStatus(p.id, "published"); onSaved(api.store.product(p.id)) }
                })
            }
            TextButton(onClick = { confirmDelete = true }) { Text(stringResource(UiR.string.prod_delete), color = Brand.colors.bad) }
        }
    }
    if (confirmDelete && p != null) {
        ConfirmDialog(
            text = stringResource(UiR.string.prod_delete_confirm, p.title),
            confirmLabel = stringResource(UiR.string.app_delete),
            onConfirm = {
                confirmDelete = false
                status.run { api.store.delete(p.id); onDeleted() }
            },
            onDismiss = { confirmDelete = false },
        )
    }
}

