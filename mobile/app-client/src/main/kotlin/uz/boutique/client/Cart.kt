package uz.boutique.client

import android.net.Uri
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Remove
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
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
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import uz.boutique.core.api.ApiClient
import uz.boutique.core.api.ApiException
import uz.boutique.core.model.Cart
import uz.boutique.core.model.CartItem
import uz.boutique.core.model.CheckoutInput
import uz.boutique.core.model.CheckoutItem
import uz.boutique.core.model.CheckoutResult
import uz.boutique.core.model.DeliveryInput
import uz.boutique.core.util.CheckoutRules
import uz.boutique.core.util.Phone
import uz.boutique.ui.AppScaffold
import uz.boutique.ui.Brand
import uz.boutique.ui.ErrorBanner
import uz.boutique.ui.FormField
import uz.boutique.ui.InfoBanner
import uz.boutique.ui.LoadContent
import uz.boutique.ui.LocalGraph
import uz.boutique.ui.PrimaryButton
import uz.boutique.ui.ProductImage
import uz.boutique.ui.SecondaryButton
import uz.boutique.ui.EmptyState
import uz.boutique.ui.SectionCard
import uz.boutique.ui.SectionTitle
import uz.boutique.ui.fieldErrorText
import uz.boutique.ui.fields
import uz.boutique.ui.money
import uz.boutique.ui.rememberAction
import uz.boutique.ui.rememberLoader
import uz.boutique.ui.R as UiR

@Composable
fun CartScreen(onProduct: (String) -> Unit, onCheckout: () -> Unit, onCatalog: () -> Unit) {
    val api = LocalGraph.current.api
    val shop = LocalShop.current
    val loader = rememberLoader("cart") { api.shop.cart().also { shop.cartCount = it.totalQty } }
    val action = rememberAction()
    fun apply(c: Cart) {
        loader.set(c)
        shop.cartCount = c.totalQty
    }
    AppScaffold(title = stringResource(UiR.string.nav_cart)) { pad ->
        LoadContent(loader, Modifier.padding(pad)) { cart ->
            if (cart.items.isEmpty()) {
                EmptyState(stringResource(UiR.string.cart_empty), Modifier.padding(pad)) {
                    PrimaryButton(stringResource(UiR.string.cart_to_catalog), onClick = onCatalog)
                }
                return@LoadContent
            }
            Column(Modifier.padding(pad).fillMaxSize()) {
                LazyColumn(
                    Modifier.weight(1f),
                    contentPadding = androidx.compose.foundation.layout.PaddingValues(16.dp),
                    verticalArrangement = Arrangement.spacedBy(10.dp),
                ) {
                    items(cart.items, key = { it.key }) { ci ->
                        CartRow(
                            ci,
                            busy = action.busy,
                            onOpen = { onProduct(ci.productId) },
                            onQty = { q -> action.run { apply(api.shop.setQty(ci.productId, q, ci.size)) } },
                            onRemove = { action.run { apply(api.shop.remove(ci.productId, ci.size)) } },
                        )
                    }
                    item { ErrorBanner(action.error) }
                }
                Surface(color = MaterialTheme.colorScheme.surface, shadowElevation = 8.dp) {
                    Column(Modifier.fillMaxWidth().padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Text(stringResource(UiR.string.order_total), modifier = Modifier.weight(1f), style = MaterialTheme.typography.titleMedium)
                            Text(money(cart.total), style = MaterialTheme.typography.titleLarge, modifier = Modifier.testTag("cart_total"))
                        }
                        if (cart.hasUnavailable) InfoBanner(stringResource(UiR.string.cart_fix_problems), color = Brand.colors.warn)
                        Text(stringResource(UiR.string.cart_split_hint), style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted)
                        PrimaryButton(stringResource(UiR.string.cart_checkout), enabled = cart.canCheckout, modifier = Modifier.testTag("checkout"), onClick = {
                            shop.checkoutItems = cart.items.filter { it.available }
                            onCheckout()
                        })
                    }
                }
            }
        }
    }
}

@Composable
fun CartRow(it: CartItem, busy: Boolean, onOpen: () -> Unit, onQty: (Int) -> Unit, onRemove: () -> Unit) {
    SectionCard(padding = androidx.compose.foundation.layout.PaddingValues(12.dp)) {
        Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            ProductImage(it.imageUrl, it.title, Modifier.size(width = 84.dp, height = 112.dp).clickable(onClick = onOpen))
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(it.title, style = MaterialTheme.typography.titleSmall, maxLines = 2)
                Text(it.storeName, style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted)
                if (!it.size.isNullOrBlank()) Text(stringResource(UiR.string.cart_size, it.size!!), style = MaterialTheme.typography.bodySmall)
                Text(money(it.price * it.qty), fontWeight = FontWeight.ExtraBold)
                val problem = when (it.unavailableReason) {
                    "out_of_stock" -> stringResource(UiR.string.product_out_of_stock)
                    "not_enough_stock" -> stringResource(UiR.string.cart_not_enough, it.sizeStock ?: 0)
                    "size_required" -> stringResource(UiR.string.cart_size_required)
                    "unavailable" -> stringResource(UiR.string.cart_unavailable)
                    else -> null
                }
                if (problem != null) Text(problem, color = Brand.colors.bad, style = MaterialTheme.typography.bodySmall)
                Row(verticalAlignment = Alignment.CenterVertically) {
                    IconButton(onClick = { onQty(it.qty - 1) }, enabled = !busy && it.qty > 1) {
                        Icon(Icons.Filled.Remove, contentDescription = stringResource(UiR.string.cart_decrease))
                    }
                    Text(it.qty.toString(), style = MaterialTheme.typography.titleMedium, modifier = Modifier.testTag("qty_${it.key}"))
                    IconButton(onClick = { onQty(it.qty + 1) }, enabled = !busy && it.available && it.qty < 99) {
                        Icon(Icons.Filled.Add, contentDescription = stringResource(UiR.string.cart_increase))
                    }
                    Spacer(Modifier.weight(1f))
                    IconButton(onClick = onRemove, enabled = !busy) {
                        Icon(Icons.Filled.Delete, contentDescription = stringResource(UiR.string.cart_remove))
                    }
                }
            }
        }
    }
}

/** Оформление: адрес, получатель, фото места доставки. Оплата при получении. */
@Composable
fun CheckoutScreen(onBack: () -> Unit, onOrders: () -> Unit) {
    val api = LocalGraph.current.api
    val shop = LocalShop.current
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val items = remember { shop.checkoutItems }
    // Ключ живёт, пока открыт экран: повторная отправка не создаст второй заказ.
    val idemKey = rememberSaveable { ApiClient.newIdempotencyKey() }
    var name by rememberSaveable { mutableStateOf("") }
    var phone by rememberSaveable { mutableStateOf("") }
    var street by rememberSaveable { mutableStateOf("") }
    var house by rememberSaveable { mutableStateOf("") }
    var entrance by rememberSaveable { mutableStateOf("") }
    var floor by rememberSaveable { mutableStateOf("") }
    var apartment by rememberSaveable { mutableStateOf("") }
    var intercom by rememberSaveable { mutableStateOf("") }
    var addressComment by rememberSaveable { mutableStateOf("") }
    var orderComment by rememberSaveable { mutableStateOf("") }
    var timeComment by rememberSaveable { mutableStateOf("") }
    var photoUri by rememberSaveable { mutableStateOf<String?>(null) }
    var photoKey by rememberSaveable { mutableStateOf<String?>(null) }
    var localErrors by remember { mutableStateOf(mapOf<String, String>()) }
    var result by remember { mutableStateOf<CheckoutResult?>(null) }
    val upload = rememberAction()
    val submit = rememberAction()

    val picker = rememberLauncherForActivityResult(ActivityResultContracts.PickVisualMedia()) { uri: Uri? ->
        if (uri != null) upload.run {
            val (bytes, type) = withContext(Dispatchers.IO) {
                val cr = context.contentResolver
                val b = cr.openInputStream(uri)?.use { s -> s.readBytes() } ?: throw ApiException(0, "read_file", "cannot read")
                b to (cr.getType(uri) ?: "image/jpeg")
            }
            photoKey = api.uploads.upload("delivery", bytes, type).key
            photoUri = uri.toString()
        }
    }

    val done = result
    if (done != null) {
        CheckoutSuccess(done, onOrders)
        return
    }

    AppScaffold(title = stringResource(UiR.string.checkout_title), onBack = onBack) { pad ->
        if (items.isEmpty()) {
            EmptyState(stringResource(UiR.string.checkout_empty), Modifier.padding(pad))
            return@AppScaffold
        }
        val serverFields = submit.error.fields()
        @Composable
        fun err(key: String) = fieldErrorText(localErrors[key] ?: serverFields[key])

        Column(
            Modifier.padding(pad).fillMaxSize().imePadding().verticalScroll(rememberScrollState()).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            SectionCard {
                items.forEach {
                    Row {
                        Text("${it.title}${if (!it.size.isNullOrBlank()) " · " + stringResource(UiR.string.order_size, it.size!!) else ""} × ${it.qty}", modifier = Modifier.weight(1f))
                        Text(money(it.price * it.qty), fontWeight = FontWeight.Bold)
                    }
                }
                Row {
                    Text(stringResource(UiR.string.order_total), modifier = Modifier.weight(1f), fontWeight = FontWeight.ExtraBold)
                    Text(money(items.sumOf { it.price * it.qty }), fontWeight = FontWeight.ExtraBold)
                }
                Text(stringResource(UiR.string.order_pay_on_delivery), style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted)
            }
            SectionTitle(stringResource(UiR.string.checkout_contact))
            FormField(name, { name = it }, stringResource(UiR.string.checkout_name), error = err("customer_name"), maxLength = CheckoutRules.MAX_NAME, modifier = Modifier.testTag("name"))
            FormField(phone, { phone = it }, stringResource(UiR.string.checkout_phone), keyboardType = KeyboardType.Phone, hint = stringResource(UiR.string.checkout_phone_hint), error = err("customer_phone"))
            SectionTitle(stringResource(UiR.string.order_address))
            FormField(street, { street = it }, stringResource(UiR.string.checkout_street), error = err("delivery.street"), maxLength = CheckoutRules.MAX_STREET, modifier = Modifier.testTag("street"))
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                FormField(house, { house = it }, stringResource(UiR.string.checkout_house), error = err("delivery.house"), maxLength = CheckoutRules.MAX_HOUSE, modifier = Modifier.weight(1f).testTag("house"))
                FormField(apartment, { apartment = it }, stringResource(UiR.string.checkout_apartment), maxLength = CheckoutRules.MAX_APARTMENT, modifier = Modifier.weight(1f))
            }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                FormField(entrance, { entrance = it }, stringResource(UiR.string.checkout_entrance), maxLength = CheckoutRules.MAX_ENTRANCE, modifier = Modifier.weight(1f))
                FormField(floor, { floor = it }, stringResource(UiR.string.checkout_floor), maxLength = CheckoutRules.MAX_FLOOR, modifier = Modifier.weight(1f))
                FormField(intercom, { intercom = it }, stringResource(UiR.string.checkout_intercom), maxLength = CheckoutRules.MAX_INTERCOM, modifier = Modifier.weight(1f))
            }
            FormField(addressComment, { addressComment = it }, stringResource(UiR.string.checkout_address_comment), singleLine = false, minLines = 2, maxLength = CheckoutRules.MAX_COMMENT)

            SectionTitle(stringResource(UiR.string.checkout_photo))
            Text(stringResource(UiR.string.checkout_photo_hint), style = MaterialTheme.typography.bodySmall, color = Brand.colors.muted)
            if (photoUri != null && photoKey != null) {
                ProductImage(photoUri, null, Modifier.size(140.dp))
                TextButton(onClick = { photoUri = null; photoKey = null }) { Text(stringResource(UiR.string.checkout_photo_remove)) }
            } else {
                SecondaryButton(
                    stringResource(if (upload.busy) UiR.string.checkout_photo_uploading else UiR.string.checkout_photo_add),
                    enabled = !upload.busy,
                    onClick = { picker.launch(PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageOnly)) },
                )
            }
            ErrorBanner(upload.error)

            SectionTitle(stringResource(UiR.string.checkout_comments))
            FormField(orderComment, { orderComment = it }, stringResource(UiR.string.order_comment), singleLine = false, minLines = 2, maxLength = CheckoutRules.MAX_COMMENT)
            FormField(timeComment, { timeComment = it }, stringResource(UiR.string.checkout_time_comment), hint = stringResource(UiR.string.checkout_time_comment_ph), singleLine = false, maxLength = CheckoutRules.MAX_TIME_COMMENT)

            ErrorBanner(submit.error)
            Spacer(Modifier.height(4.dp))
            PrimaryButton(stringResource(UiR.string.checkout_submit), busy = submit.busy, enabled = !upload.busy, modifier = Modifier.testTag("submit_order"), onClick = {
                fun opt(s: String) = s.trim().ifEmpty { null }
                val input = CheckoutInput(
                    items = items.map { CheckoutItem(it.productId, it.qty, it.size?.ifEmpty { null }) },
                    customerName = name.trim(),
                    customerPhone = opt(phone)?.let { Phone.normalizeUz(it) ?: it },
                    customerComment = opt(orderComment),
                    deliveryTimeComment = opt(timeComment),
                    delivery = DeliveryInput(
                        street = street.trim(), house = house.trim(), entrance = opt(entrance), floor = opt(floor),
                        apartment = opt(apartment), intercom = opt(intercom), addressComment = opt(addressComment), photoKey = photoKey,
                    ),
                )
                localErrors = CheckoutRules.validate(input)
                if (localErrors.isEmpty()) submit.run {
                    val r = api.orders.checkout(input, idemKey)
                    shop.checkoutItems = emptyList()
                    scope.launch { shop.sync() }
                    result = r
                }
            })
            Spacer(Modifier.width(1.dp))
        }
    }
}

@Composable
private fun CheckoutSuccess(r: CheckoutResult, onOrders: () -> Unit) {
    AppScaffold(title = stringResource(UiR.string.checkout_success_title)) { pad ->
        Column(Modifier.padding(pad).padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text(stringResource(UiR.string.checkout_success_title), style = MaterialTheme.typography.headlineSmall, modifier = Modifier.testTag("checkout_done"))
            if (r.orders.size > 1) Text(stringResource(UiR.string.checkout_success_split))
            r.orders.forEach { o ->
                SectionCard { Text(stringResource(UiR.string.checkout_order_line, o.number.toString(), o.storeName, money(o.itemsTotal))) }
            }
            Text(stringResource(UiR.string.order_pending_hint), color = Brand.colors.muted)
            PrimaryButton(stringResource(UiR.string.checkout_to_orders), onClick = onOrders)
        }
    }
}
