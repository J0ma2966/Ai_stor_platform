package uz.boutique.ui

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Star
import androidx.compose.material.icons.filled.Visibility
import androidx.compose.material.icons.filled.VisibilityOff
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CenterAlignedTopAppBar
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import coil.compose.AsyncImage
import uz.boutique.core.model.OrderStatus
import uz.boutique.core.util.Money
import uz.boutique.core.util.Rating

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AppScaffold(
    title: String,
    onBack: (() -> Unit)? = null,
    actions: @Composable () -> Unit = {},
    snackbar: SnackbarHostState? = null,
    bottomBar: @Composable () -> Unit = {},
    floatingActionButton: @Composable () -> Unit = {},
    content: @Composable (PaddingValues) -> Unit,
) {
    Scaffold(
        floatingActionButton = floatingActionButton,
        topBar = {
            CenterAlignedTopAppBar(
                title = { Text(title, style = MaterialTheme.typography.titleLarge, maxLines = 1, overflow = TextOverflow.Ellipsis) },
                navigationIcon = {
                    if (onBack != null) {
                        IconButton(onClick = onBack) {
                            Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = stringResource(R.string.app_back))
                        }
                    }
                },
                actions = { actions() },
                colors = TopAppBarDefaults.centerAlignedTopAppBarColors(containerColor = MaterialTheme.colorScheme.background),
            )
        },
        bottomBar = bottomBar,
        snackbarHost = { if (snackbar != null) SnackbarHost(snackbar) },
        containerColor = MaterialTheme.colorScheme.background,
        content = content,
    )
}

@Composable
fun CenterLoading(modifier: Modifier = Modifier) {
    Box(modifier.fillMaxSize(), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
}

@Composable
fun ErrorState(error: Throwable, onRetry: () -> Unit, modifier: Modifier = Modifier) {
    Column(
        modifier.fillMaxSize().padding(24.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text(errorText(error), textAlign = TextAlign.Center, style = MaterialTheme.typography.bodyLarge)
        Spacer(Modifier.height(16.dp))
        Button(onClick = onRetry) { Text(stringResource(R.string.app_retry)) }
    }
}

@Composable
fun EmptyState(text: String, modifier: Modifier = Modifier, action: (@Composable () -> Unit)? = null) {
    Column(
        modifier.fillMaxSize().padding(24.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text(text, textAlign = TextAlign.Center, color = Brand.colors.muted, style = MaterialTheme.typography.bodyLarge)
        if (action != null) {
            Spacer(Modifier.height(16.dp))
            action()
        }
    }
}

/** Содержимое по состоянию загрузки: индикатор, ошибка с повтором или данные. */
@Composable
fun <T> LoadContent(loader: Loader<T>, modifier: Modifier = Modifier, content: @Composable (T) -> Unit) {
    when (val s = loader.state) {
        is Load.Loading -> CenterLoading(modifier)
        is Load.Failed -> ErrorState(s.error, onRetry = { loader.reload() }, modifier = modifier)
        is Load.Ok -> content(s.value)
    }
}

@Composable
fun SectionCard(modifier: Modifier = Modifier, padding: PaddingValues = PaddingValues(16.dp), content: @Composable ColumnScope.() -> Unit) {
    val shape = MaterialTheme.shapes.large
    Surface(
        // объём как на сайте: мягкая тень, светлый верх панели, блик по верхнему краю
        modifier = modifier.fillMaxWidth().surfaceDepth(Brand.colors, shape),
        shape = shape,
        color = Color.Transparent,
        contentColor = MaterialTheme.colorScheme.onSurface,
        tonalElevation = 0.dp,
        shadowElevation = 0.dp,
    ) {
        Column(Modifier.padding(padding), verticalArrangement = Arrangement.spacedBy(8.dp), content = content)
    }
}

@Composable
fun ProductImage(url: String?, contentDescription: String?, modifier: Modifier = Modifier) {
    Box(modifier.clip(MaterialTheme.shapes.medium).background(Brand.colors.surface2)) {
        if (!url.isNullOrEmpty()) {
            AsyncImage(
                model = url,
                contentDescription = contentDescription,
                contentScale = ContentScale.Crop,
                modifier = Modifier.fillMaxSize(),
            )
        }
    }
}

@Composable
fun money(tiyin: Long): String = Money.format(tiyin, currentLang())

@Composable
fun statusLabel(s: OrderStatus): String = stringResource(
    when (s) {
        OrderStatus.PENDING -> R.string.status_pending
        OrderStatus.CONFIRMED -> R.string.status_confirmed
        OrderStatus.REJECTED -> R.string.status_rejected
        OrderStatus.ASSEMBLING -> R.string.status_assembling
        OrderStatus.IN_DELIVERY -> R.string.status_in_delivery
        OrderStatus.DELIVERED -> R.string.status_delivered
        OrderStatus.CANCELLED -> R.string.status_cancelled
    },
)

/** Надпись на кнопке магазина «перевести заказ в статус X». */
@Composable
fun actionLabel(s: OrderStatus): String = stringResource(
    when (s) {
        OrderStatus.CONFIRMED -> R.string.act_confirmed
        OrderStatus.REJECTED -> R.string.act_rejected
        OrderStatus.ASSEMBLING -> R.string.act_assembling
        OrderStatus.IN_DELIVERY -> R.string.act_in_delivery
        OrderStatus.DELIVERED -> R.string.act_delivered
        OrderStatus.CANCELLED -> R.string.act_cancelled
        OrderStatus.PENDING -> R.string.status_pending
    },
)

@Composable
fun StatusChip(status: OrderStatus, modifier: Modifier = Modifier) {
    val b = Brand.colors
    val color = when (status) {
        OrderStatus.PENDING -> b.warn
        OrderStatus.CONFIRMED, OrderStatus.ASSEMBLING, OrderStatus.IN_DELIVERY -> b.info
        OrderStatus.DELIVERED -> b.ok
        OrderStatus.REJECTED, OrderStatus.CANCELLED -> b.bad
    }
    Pill(statusLabel(status), color, modifier.testTag("status_${status.wire}"))
}

@Composable
fun Pill(text: String, color: Color, modifier: Modifier = Modifier) {
    Text(
        text,
        modifier = modifier
            .clip(RoundedCornerShape(50))
            .background(color.copy(alpha = 0.12f))
            .padding(horizontal = 10.dp, vertical = 4.dp),
        color = color,
        style = MaterialTheme.typography.labelMedium,
    )
}

@Composable
fun RatingRow(avg: Double, count: Int, modifier: Modifier = Modifier) {
    if (count <= 0) return
    Row(modifier, verticalAlignment = Alignment.CenterVertically) {
        Icon(Icons.Filled.Star, contentDescription = null, tint = Brand.colors.star, modifier = Modifier.size(16.dp))
        Spacer(Modifier.width(4.dp))
        Text("${Rating.format(avg)} ($count)", style = MaterialTheme.typography.labelMedium, color = Brand.colors.muted)
    }
}

@Composable
fun StarsInput(value: Int, onChange: (Int) -> Unit, modifier: Modifier = Modifier) {
    Row(modifier) {
        for (i in 1..5) {
            val label = stringResource(R.string.reviews_star, i)
            Icon(
                Icons.Filled.Star,
                contentDescription = label,
                tint = if (i <= value) Brand.colors.star else MaterialTheme.colorScheme.outline,
                modifier = Modifier.size(40.dp).clickable { onChange(i) }.padding(4.dp).testTag("star_$i"),
            )
        }
    }
}

@Composable
fun FormField(
    value: String,
    onChange: (String) -> Unit,
    label: String,
    modifier: Modifier = Modifier,
    error: String? = null,
    hint: String? = null,
    keyboardType: KeyboardType = KeyboardType.Text,
    imeAction: ImeAction = ImeAction.Next,
    singleLine: Boolean = true,
    minLines: Int = 1,
    maxLength: Int? = null,
    enabled: Boolean = true,
) {
    OutlinedTextField(
        value = value,
        onValueChange = { v -> onChange(if (maxLength != null) v.take(maxLength) else v) },
        label = { Text(label) },
        modifier = modifier.fillMaxWidth().insetDepth(Brand.colors),
        colors = fieldColors(),
        isError = error != null,
        supportingText = supporting(error ?: hint),
        keyboardOptions = KeyboardOptions(keyboardType = keyboardType, imeAction = imeAction),
        singleLine = singleLine,
        minLines = minLines,
        enabled = enabled,
        shape = MaterialTheme.shapes.medium,
    )
}

/** Поле «утоплено»: свой фон чуть светлее (темнее в тёмной теме) панели, рамка заметнее. */
@Composable
private fun fieldColors() = OutlinedTextFieldDefaults.colors(
    focusedContainerColor = Brand.colors.field,
    unfocusedContainerColor = Brand.colors.field,
    disabledContainerColor = Brand.colors.field.copy(alpha = 0.6f),
    errorContainerColor = Brand.colors.field,
    unfocusedBorderColor = MaterialTheme.colorScheme.outline,
)

private fun supporting(text: String?): (@Composable () -> Unit)? =
    if (text == null) null else { { Text(text) } }

@Composable
fun PasswordField(
    value: String,
    onChange: (String) -> Unit,
    label: String,
    modifier: Modifier = Modifier,
    error: String? = null,
    hint: String? = null,
    imeAction: ImeAction = ImeAction.Done,
) {
    var visible by rememberSaveable { mutableStateOf(false) }
    OutlinedTextField(
        value = value,
        onValueChange = { onChange(it.take(128)) },
        label = { Text(label) },
        modifier = modifier.fillMaxWidth().insetDepth(Brand.colors),
        colors = fieldColors(),
        isError = error != null,
        supportingText = supporting(error ?: hint),
        singleLine = true,
        visualTransformation = if (visible) VisualTransformation.None else PasswordVisualTransformation(),
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password, imeAction = imeAction),
        trailingIcon = {
            IconButton(onClick = { visible = !visible }) {
                Icon(
                    if (visible) Icons.Filled.VisibilityOff else Icons.Filled.Visibility,
                    contentDescription = stringResource(R.string.login_show_password),
                )
            }
        },
        shape = MaterialTheme.shapes.medium,
    )
}

@Composable
fun PrimaryButton(text: String, onClick: () -> Unit, modifier: Modifier = Modifier, busy: Boolean = false, enabled: Boolean = true) {
    val on = enabled && !busy
    Button(
        onClick = onClick,
        enabled = on,
        // VESTA + объём: «таблетка» с градиентом, бликом и цветным свечением
        modifier = modifier.fillMaxWidth().height(52.dp).alpha(if (on || busy) 1f else 0.55f).accentDepth(Brand.colors, CircleShape, enabled = on),
        shape = CircleShape,
        colors = ButtonDefaults.buttonColors(
            containerColor = Color.Transparent,
            contentColor = MaterialTheme.colorScheme.onPrimaryContainer,
            disabledContainerColor = Color.Transparent,
            disabledContentColor = MaterialTheme.colorScheme.onPrimaryContainer,
        ),
    ) {
        if (busy) {
            CircularProgressIndicator(Modifier.size(20.dp), strokeWidth = 2.dp, color = MaterialTheme.colorScheme.onPrimaryContainer)
        } else {
            Text(text, style = MaterialTheme.typography.labelLarge)
        }
    }
}

@Composable
fun SecondaryButton(text: String, onClick: () -> Unit, modifier: Modifier = Modifier, enabled: Boolean = true, color: Color? = null) {
    OutlinedButton(
        onClick = onClick,
        enabled = enabled,
        modifier = modifier.fillMaxWidth().height(48.dp).alpha(if (enabled) 1f else 0.55f).surfaceDepth(Brand.colors, CircleShape, elevation = 4.dp),
        shape = CircleShape,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outline),
    ) {
        Text(text, style = MaterialTheme.typography.labelLarge, color = color ?: MaterialTheme.colorScheme.primary)
    }
}

@Composable
fun ErrorBanner(error: Throwable?, modifier: Modifier = Modifier) {
    if (error == null) return
    Text(
        errorText(error),
        modifier = modifier
            .fillMaxWidth()
            .clip(MaterialTheme.shapes.medium)
            .background(Brand.colors.bad.copy(alpha = 0.1f))
            .padding(12.dp)
            .testTag("error_banner"),
        color = Brand.colors.bad,
        style = MaterialTheme.typography.bodyMedium,
    )
}

@Composable
fun InfoBanner(text: String, modifier: Modifier = Modifier, color: Color = Brand.colors.info) {
    Text(
        text,
        modifier = modifier
            .fillMaxWidth()
            .clip(MaterialTheme.shapes.medium)
            .background(color.copy(alpha = 0.1f))
            .padding(12.dp),
        color = color,
        style = MaterialTheme.typography.bodyMedium,
    )
}

@Composable
fun ConfirmDialog(
    text: String,
    confirmLabel: String,
    onConfirm: () -> Unit,
    onDismiss: () -> Unit,
    dismissLabel: String = stringResource(R.string.app_cancel),
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        text = { Text(text) },
        confirmButton = { TextButton(onClick = onConfirm, modifier = Modifier.testTag("confirm")) { Text(confirmLabel) } },
        dismissButton = { TextButton(onClick = onDismiss) { Text(dismissLabel) } },
    )
}

/** Диалог с причиной. [required] — без причины кнопка неактивна (отклонение и отмена магазином). */
@Composable
fun ReasonDialog(
    title: String,
    label: String,
    confirmLabel: String,
    required: Boolean,
    onConfirm: (String) -> Unit,
    onDismiss: () -> Unit,
    placeholder: String? = null,
    dismissLabel: String = stringResource(R.string.app_cancel),
) {
    var reason by rememberSaveable { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(title, style = MaterialTheme.typography.titleLarge, maxLines = 1, overflow = TextOverflow.Ellipsis) },
        text = {
            OutlinedTextField(
                value = reason,
                onValueChange = { reason = it.take(500) },
                label = { Text(label) },
                placeholder = supporting(placeholder),
                minLines = 2,
                modifier = Modifier.fillMaxWidth().testTag("reason"),
            )
        },
        confirmButton = {
            TextButton(
                onClick = { onConfirm(reason.trim()) },
                enabled = !required || reason.isNotBlank(),
                modifier = Modifier.testTag("confirm"),
            ) { Text(confirmLabel) }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text(dismissLabel) } },
    )
}

@Composable
fun KeyValue(label: String, value: String?, modifier: Modifier = Modifier) {
    if (value.isNullOrBlank()) return
    Column(modifier) {
        Text(label, style = MaterialTheme.typography.labelMedium, color = Brand.colors.muted)
        Text(value, style = MaterialTheme.typography.bodyLarge)
    }
}

@Composable
fun SectionTitle(text: String, modifier: Modifier = Modifier) {
    Text(text, modifier = modifier, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.ExtraBold)
}

/** Нижняя кнопка «Показать ещё» / индикатор подгрузки в списке. */
@Composable
fun <T> PagedFooter(paged: Paged<T>) {
    Box(Modifier.fillMaxWidth().padding(16.dp), contentAlignment = Alignment.Center) {
        when {
            paged.loading -> CircularProgressIndicator(Modifier.size(28.dp))
            paged.error != null -> TextButton(onClick = { if (paged.loadedOnce) paged.loadMore() else paged.refresh() }) {
                Text(errorText(paged.error) + " · " + stringResource(R.string.app_retry), textAlign = TextAlign.Center)
            }
            !paged.end && paged.loadedOnce -> TextButton(onClick = { paged.loadMore() }) { Text(stringResource(R.string.app_more)) }
        }
    }
}

/** Экран списка с пагинацией: первая загрузка, ошибка, пусто. */
@Composable
fun <T> PagedState(paged: Paged<T>, emptyText: String, modifier: Modifier = Modifier, content: @Composable () -> Unit) {
    when {
        !paged.loadedOnce && paged.error != null -> ErrorState(paged.error!!, onRetry = { paged.refresh() }, modifier = modifier)
        !paged.loadedOnce -> CenterLoading(modifier)
        paged.items.isEmpty() -> EmptyState(emptyText, modifier)
        else -> content()
    }
}

val ScreenPadding = PaddingValues(horizontal = 16.dp, vertical = 12.dp)
