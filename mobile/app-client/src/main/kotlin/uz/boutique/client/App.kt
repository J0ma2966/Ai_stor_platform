package uz.boutique.client

import android.app.Application
import android.os.Bundle
import android.widget.Toast
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.appcompat.app.AppCompatActivity
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Favorite
import androidx.compose.material.icons.filled.Home
import androidx.compose.material.icons.filled.LocalShipping
import androidx.compose.material.icons.filled.Person
import androidx.compose.material.icons.filled.ShoppingCart
import androidx.compose.material3.Badge
import androidx.compose.material3.BadgedBox
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.Stable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.NavHostController
import androidx.navigation.NavType
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import androidx.navigation.navArgument
import uz.boutique.core.api.Api
import uz.boutique.core.api.KeyValueStore
import uz.boutique.core.api.MemoryStore
import uz.boutique.core.tryon.FittingItem
import uz.boutique.core.tryon.FittingSelection
import uz.boutique.core.api.AuthState
import uz.boutique.core.api.RefreshOutcome
import uz.boutique.core.model.CartItem
import uz.boutique.core.model.Role
import uz.boutique.core.util.NotificationTarget
import uz.boutique.ui.AppGraph
import uz.boutique.ui.AppTheme
import uz.boutique.ui.BoutiqueTheme
import uz.boutique.ui.CenterLoading
import uz.boutique.ui.ErrorState
import uz.boutique.ui.LocalGraph
import uz.boutique.ui.NotificationsScreen
import uz.boutique.ui.PrefsStore
import uz.boutique.ui.R as UiR

class ClientApp : Application() {
    lateinit var graph: AppGraph
        private set

    override fun onCreate() {
        super.onCreate()
        AppTheme.apply(this)
        graph = AppGraph(this, Role.CLIENT, BuildConfig.API_URL, BuildConfig.SERVER_OVERRIDE, BuildConfig.SITE_URL, BuildConfig.VERSION_NAME)
    }
}

class MainActivity : AppCompatActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        val graph = (application as ClientApp).graph
        setContent {
            BoutiqueTheme {
                CompositionLocalProvider(LocalGraph provides graph) {
                    ClientRoot(onRestart = { recreate() })
                }
            }
        }
    }
}

/** Общее состояние покупателя: избранное (сердечки) и число товаров в корзине (значок). */
@Stable
class ShopState(private val api: Api, fittingStore: KeyValueStore = MemoryStore()) {
    var favIds by mutableStateOf(setOf<String>())
        private set

    /** Примерочная: по одной вещи на зону, хранится на телефоне и работает без входа. */
    private val fitting = FittingSelection(fittingStore)
    var fittingItems by mutableStateOf(fitting.all)
        private set

    suspend fun addToFitting(item: FittingItem) {
        val isNew = fitting.add(item)
        fittingItems = fitting.all
        if (isNew) api.tryon.countFittingAdd(item.id)
    }

    fun removeFromFitting(zone: String) {
        fitting.remove(zone)
        fittingItems = fitting.all
    }

    /** «Изменить образ» / «Повторить»: вещи прошлой примерки снова в примерочной. */
    fun setFitting(items: List<FittingItem>) {
        fitting.clear()
        items.forEach { fitting.add(it) }
        fittingItems = fitting.all
    }
    var cartCount by mutableIntStateOf(0)
    /** Позиции, выбранные для оформления (корзина → оформление). */
    var checkoutItems: List<CartItem> = emptyList()

    suspend fun sync() {
        runCatching { favIds = api.shop.favoriteIds() }
        runCatching { cartCount = api.shop.cart().totalQty }
    }

    fun clear() {
        favIds = emptySet()
        cartCount = 0
        checkoutItems = emptyList()
    }

    suspend fun toggleFavorite(id: String) {
        val had = id in favIds
        favIds = if (had) favIds - id else favIds + id
        try {
            if (had) api.shop.removeFavorite(id) else api.shop.addFavorite(id)
        } catch (e: Exception) {
            favIds = if (had) favIds + id else favIds - id
            throw e
        }
    }
}

val LocalShop = staticCompositionLocalOf<ShopState> { error("ShopState is not provided") }

object Routes {
    const val CATALOG = "catalog"
    const val FAVORITES = "favorites"
    const val CART = "cart"
    const val ORDERS = "orders"
    const val PROFILE = "profile"
    const val LOGIN = "login"
    const val SMS = "sms/{mode}"
    const val EMAIL = "email/{mode}"
    const val WELCOME = "welcome"
    const val PASSWORD = "password/{mode}"
    const val PRODUCT = "product/{id}"
    const val STORE = "store/{slug}?name={name}"
    const val ORDER = "order/{id}"
    const val CHECKOUT = "checkout"
    const val NOTIFICATIONS = "notifications"
    const val FITTING = "fitting"
    const val TRYON_NEW = "tryon/new"
    const val GENERATION = "tryon/gen/{id}"
    const val TRYON_HISTORY = "tryon/history"
    const val TRYON_PHOTOS = "tryon/photos"

    fun sms(mode: String) = "sms/$mode"
    fun email(mode: String) = "email/$mode"
    fun password(mode: String) = "password/$mode"
    fun product(id: String) = "product/$id"
    fun order(id: String) = "order/$id"
    fun generation(id: String) = "tryon/gen/$id"
    fun store(slug: String, name: String) = "store/$slug?name=${android.net.Uri.encode(name)}"

    val TABS = listOf(CATALOG, FAVORITES, CART, ORDERS, PROFILE)
}

private data class Tab(val route: String, val label: Int, val icon: ImageVector)

private val tabs = listOf(
    Tab(Routes.CATALOG, UiR.string.nav_catalog, Icons.Filled.Home),
    Tab(Routes.FAVORITES, UiR.string.nav_favorites, Icons.Filled.Favorite),
    Tab(Routes.CART, UiR.string.nav_cart, Icons.Filled.ShoppingCart),
    Tab(Routes.ORDERS, UiR.string.nav_orders, Icons.Filled.LocalShipping),
    Tab(Routes.PROFILE, UiR.string.nav_profile, Icons.Filled.Person),
)

/** Куда вести после входа: незавершённая регистрация обязательна; пароль — только после SMS (как на сайте). */
suspend fun nextAfterLogin(api: Api): String? {
    val me = api.auth.me()
    return when {
        !me.registrationComplete -> Routes.WELCOME
        me.needsPassword -> Routes.password("set")
        else -> null
    }
}

fun NavHostController.finishLogin(next: String?) {
    if (next == null) {
        if (!popBackStack(Routes.LOGIN, inclusive = true)) {
            // вход открыт не с экрана входа (например, после SMS-регистрации из профиля)
            navigate(Routes.CATALOG) { popUpTo(0) }
        }
    } else {
        navigate(next) { popUpTo(Routes.LOGIN) { inclusive = true } }
    }
}

/** Переход на вкладку нижнего меню с сохранением состояния вкладок. */
fun NavHostController.switchTab(route: String) {
    navigate(route) {
        popUpTo(Routes.CATALOG) { saveState = true }
        launchSingleTop = true
        restoreState = true
    }
}

@Composable
fun ClientRoot(onRestart: () -> Unit) {
    val graph = LocalGraph.current
    val api = graph.api
    val auth by api.session.state.collectAsStateWithLifecycle()
    var boot by remember(api) { mutableStateOf<RefreshOutcome?>(null) }
    var bootTry by remember(api) { mutableIntStateOf(0) }
    LaunchedEffect(api, bootTry) {
        boot = if (api.session.state.value is AuthState.LoggedIn) RefreshOutcome.OK else api.client.refresh()
    }
    when {
        boot == null -> CenterLoading()
        boot == RefreshOutcome.FAILED && auth == AuthState.Unknown ->
            ErrorState(uz.boutique.core.api.ApiException.network(), onRetry = { boot = null; bootTry++ })
        else -> {
            val context = LocalContext.current
            val shop = remember(api) { ShopState(api, PrefsStore(context, "fitting")) }
            CompositionLocalProvider(LocalShop provides shop) {
                ClientNav(onRestart)
            }
        }
    }
}

@Composable
fun ClientNav(onRestart: () -> Unit) {
    val api = LocalGraph.current.api
    val shop = LocalShop.current
    val nav = rememberNavController()
    val context = LocalContext.current
    val auth by api.session.state.collectAsStateWithLifecycle()
    val lost by api.session.lostEvents.collectAsStateWithLifecycle()
    val sessionLostText = stringResource(UiR.string.app_session_lost)

    LaunchedEffect(auth) {
        if (auth is AuthState.LoggedIn) {
            shop.sync()
            // старый аккаунт без пароля или незавершённая регистрация — довести до конца
            runCatching { nextAfterLogin(api) }.getOrNull()?.let { next ->
                if (nav.currentDestination?.route !in listOf(Routes.WELCOME, Routes.PASSWORD, Routes.LOGIN, Routes.SMS, Routes.EMAIL)) nav.navigate(next)
            }
        } else {
            shop.clear()
        }
    }
    LaunchedEffect(lost) {
        if (lost > 0) {
            Toast.makeText(context, sessionLostText, Toast.LENGTH_LONG).show()
            nav.navigate(Routes.LOGIN)
        }
    }

    val entry by nav.currentBackStackEntryAsState()
    val route = entry?.destination?.route
    val loggedIn = auth is AuthState.LoggedIn

    androidx.compose.material3.Scaffold(
        bottomBar = {
            if (route in Routes.TABS) {
                NavigationBar(containerColor = MaterialTheme.colorScheme.surface) {
                    tabs.forEach { t ->
                        NavigationBarItem(
                            selected = route == t.route,
                            onClick = { nav.switchTab(t.route) },
                            icon = {
                                if (t.route == Routes.CART && shop.cartCount > 0) {
                                    BadgedBox(badge = { Badge { Text(shop.cartCount.coerceAtMost(99).toString()) } }) {
                                        Icon(t.icon, contentDescription = null)
                                    }
                                } else {
                                    Icon(t.icon, contentDescription = null)
                                }
                            },
                            label = { Text(stringResource(t.label), maxLines = 1) },
                            modifier = Modifier.testTag("tab_${t.route}"),
                        )
                    }
                }
            }
        },
    ) { pad ->
        NavHost(nav, startDestination = Routes.CATALOG, modifier = Modifier.padding(bottom = pad.calculateBottomPadding())) {
            composable(Routes.CATALOG) {
                CatalogScreen(
                    storeSlug = null, storeName = null,
                    onProduct = { nav.navigate(Routes.product(it)) },
                    onBack = null,
                    onNeedLogin = { nav.navigate(Routes.LOGIN) },
                    onFitting = { nav.navigate(Routes.FITTING) },
                    onFavorites = { nav.switchTab(Routes.FAVORITES) },
                )
            }
            composable(
                Routes.STORE,
                arguments = listOf(navArgument("slug") { type = NavType.StringType }, navArgument("name") { type = NavType.StringType; defaultValue = "" }),
            ) { e ->
                CatalogScreen(
                    storeSlug = e.arguments?.getString("slug"), storeName = e.arguments?.getString("name"),
                    onProduct = { nav.navigate(Routes.product(it)) },
                    onBack = { nav.popBackStack() },
                    onNeedLogin = { nav.navigate(Routes.LOGIN) },
                    onFitting = { nav.navigate(Routes.FITTING) },
                )
            }
            composable(Routes.PRODUCT, arguments = listOf(navArgument("id") { type = NavType.StringType })) { e ->
                ProductScreen(
                    id = e.arguments?.getString("id").orEmpty(),
                    onBack = { nav.popBackStack() },
                    onNeedLogin = { nav.navigate(Routes.LOGIN) },
                    onCart = { nav.navigate(Routes.CART) { launchSingleTop = true } },
                    onStore = { slug, name -> nav.navigate(Routes.store(slug, name)) },
                    onFitting = { nav.navigate(Routes.FITTING) },
                )
            }
            composable(Routes.FAVORITES) {
                if (loggedIn) FavoritesScreen(onProduct = { nav.navigate(Routes.product(it)) }) else GuestScreen(UiR.string.nav_favorites) { nav.navigate(Routes.LOGIN) }
            }
            composable(Routes.CART) {
                if (loggedIn) {
                    CartScreen(
                        onProduct = { nav.navigate(Routes.product(it)) },
                        onCheckout = { nav.navigate(Routes.CHECKOUT) },
                        onCatalog = { nav.navigate(Routes.CATALOG) { popUpTo(Routes.CATALOG) { inclusive = true } } },
                    )
                } else {
                    GuestScreen(UiR.string.nav_cart) { nav.navigate(Routes.LOGIN) }
                }
            }
            composable(Routes.CHECKOUT) {
                CheckoutScreen(
                    onBack = { nav.popBackStack() },
                    onOrders = { nav.navigate(Routes.ORDERS) { popUpTo(Routes.CATALOG) } },
                )
            }
            composable(Routes.ORDERS) {
                if (loggedIn) OrdersScreen(onOrder = { nav.navigate(Routes.order(it)) }) else GuestScreen(UiR.string.orders_title) { nav.navigate(Routes.LOGIN) }
            }
            composable(Routes.ORDER, arguments = listOf(navArgument("id") { type = NavType.StringType })) { e ->
                OrderDetailScreen(id = e.arguments?.getString("id").orEmpty(), onBack = { nav.popBackStack() }, onProduct = { nav.navigate(Routes.product(it)) })
            }
            composable(Routes.PROFILE) {
                ProfileScreen(
                    onLogin = { nav.navigate(Routes.LOGIN) },
                    onChangePassword = { nav.navigate(Routes.password("change")) },
                    onNotifications = { nav.navigate(Routes.NOTIFICATIONS) },
                    onFitting = { nav.navigate(Routes.FITTING) },
                    onTryonHistory = { nav.navigate(Routes.TRYON_HISTORY) },
                    onTryonPhotos = { nav.navigate(Routes.TRYON_PHOTOS) },
                    onRestart = onRestart,
                )
            }
            composable(Routes.FITTING) {
                FittingRoomScreen(
                    onBack = { nav.popBackStack() },
                    onProduct = { nav.navigate(Routes.product(it)) },
                    onStart = { if (loggedIn) nav.navigate(Routes.TRYON_NEW) else nav.navigate(Routes.LOGIN) },
                    onHistory = { if (loggedIn) nav.navigate(Routes.TRYON_HISTORY) else nav.navigate(Routes.LOGIN) },
                    onCatalog = { nav.navigate(Routes.CATALOG) { popUpTo(Routes.CATALOG) { inclusive = true } } },
                )
            }
            composable(Routes.TRYON_NEW) {
                TryonWizardScreen(
                    onBack = { nav.popBackStack() },
                    onStarted = { id -> nav.navigate(Routes.generation(id)) { popUpTo(Routes.TRYON_NEW) { inclusive = true } } },
                    onCatalog = { nav.navigate(Routes.CATALOG) { popUpTo(Routes.CATALOG) { inclusive = true } } },
                )
            }
            composable(Routes.GENERATION, arguments = listOf(navArgument("id") { type = NavType.StringType })) { e ->
                GenerationScreen(
                    id = e.arguments?.getString("id").orEmpty(),
                    onBack = { nav.popBackStack() },
                    onProduct = { nav.navigate(Routes.product(it)) },
                    onFitting = { nav.navigate(Routes.FITTING) },
                    onRepeat = { nav.navigate(Routes.TRYON_NEW) },
                )
            }
            composable(Routes.TRYON_HISTORY) {
                TryonHistoryScreen(
                    onBack = { nav.popBackStack() },
                    onGeneration = { nav.navigate(Routes.generation(it)) },
                    onPhotos = { nav.navigate(Routes.TRYON_PHOTOS) },
                )
            }
            composable(Routes.TRYON_PHOTOS) {
                TryonPhotosScreen(onBack = { nav.popBackStack() })
            }
            composable(Routes.NOTIFICATIONS) {
                NotificationsScreen(onBack = { nav.popBackStack() }) { t ->
                    when (t) {
                        is NotificationTarget.ClientOrder -> if (t.id != null) nav.navigate(Routes.order(t.id!!)) else nav.navigate(Routes.ORDERS)
                        is NotificationTarget.ProductPage -> nav.navigate(Routes.product(t.id))
                        is NotificationTarget.TryonResult -> nav.navigate(Routes.generation(t.id))
                        else -> Unit
                    }
                }
            }
            composable(Routes.LOGIN) {
                LoginScreen(
                    onBack = { nav.popBackStack() },
                    onSms = { nav.navigate(Routes.sms("signup")) },
                    onForgot = { nav.navigate(Routes.sms("reset")) },
                    onEmail = { mode -> nav.navigate(Routes.email(mode)) },
                    onDone = { next -> nav.finishLogin(next) },
                    onRestart = onRestart,
                )
            }
            composable(Routes.EMAIL, arguments = listOf(navArgument("mode") { type = NavType.StringType })) { e ->
                EmailScreen(
                    initialMode = e.arguments?.getString("mode") ?: "login",
                    onBack = { nav.popBackStack() },
                    onDone = { next -> nav.finishLogin(next) },
                )
            }
            composable(Routes.SMS, arguments = listOf(navArgument("mode") { type = NavType.StringType })) { e ->
                SmsScreen(
                    reset = e.arguments?.getString("mode") == "reset",
                    onBack = { nav.popBackStack() },
                    onDone = { next -> nav.finishLogin(next) },
                )
            }
            composable(Routes.WELCOME) {
                WelcomeScreen(onDone = { next ->
                    if (next == null) nav.navigate(Routes.CATALOG) { popUpTo(0) } else nav.navigate(next) { popUpTo(Routes.WELCOME) { inclusive = true } }
                })
            }
            composable(Routes.PASSWORD, arguments = listOf(navArgument("mode") { type = NavType.StringType })) { e ->
                val change = e.arguments?.getString("mode") == "change"
                PasswordScreen(change = change, onBack = if (change) ({ nav.popBackStack() }) else null, onDone = {
                    if (change) nav.popBackStack() else nav.navigate(Routes.CATALOG) { popUpTo(0) }
                })
            }
        }
    }
}
