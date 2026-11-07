package uz.boutique.store

import android.app.Application
import android.os.Bundle
import android.widget.Toast
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.appcompat.app.AppCompatActivity
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Checkroom
import androidx.compose.material.icons.filled.LocalShipping
import androidx.compose.material.icons.filled.Menu
import androidx.compose.material.icons.filled.RateReview
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.NavType
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import androidx.navigation.navArgument
import uz.boutique.core.api.ApiException
import uz.boutique.core.api.AuthState
import uz.boutique.core.api.RefreshOutcome
import uz.boutique.core.model.Role
import uz.boutique.core.util.NotificationTarget
import uz.boutique.ui.AppGraph
import uz.boutique.ui.AppTheme
import uz.boutique.ui.BoutiqueTheme
import uz.boutique.ui.CenterLoading
import uz.boutique.ui.ErrorState
import uz.boutique.ui.LocalGraph
import uz.boutique.ui.NotificationsScreen
import uz.boutique.ui.R as UiR

class StoreApp : Application() {
    lateinit var graph: AppGraph
        private set

    override fun onCreate() {
        super.onCreate()
        AppTheme.apply(this)
        graph = AppGraph(this, Role.STORE, BuildConfig.API_URL, BuildConfig.SERVER_OVERRIDE, BuildConfig.SITE_URL, BuildConfig.VERSION_NAME)
    }
}

class MainActivity : AppCompatActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        val graph = (application as StoreApp).graph
        setContent {
            BoutiqueTheme {
                CompositionLocalProvider(LocalGraph provides graph) {
                    StoreRoot(onRestart = { recreate() })
                }
            }
        }
    }
}

object Routes {
    const val ORDERS = "orders"
    const val PRODUCTS = "products"
    const val REVIEWS = "reviews"
    const val MORE = "more"
    const val ORDER = "order/{id}"
    const val PRODUCT = "product/{id}"
    const val NEW_PRODUCT = "product-new"
    const val NOTIFICATIONS = "notifications"
    const val STATS = "stats"

    fun order(id: String) = "order/$id"
    fun product(id: String) = "product/$id"
    val TABS = listOf(ORDERS, PRODUCTS, REVIEWS, MORE)
}

private data class Tab(val route: String, val label: Int, val icon: ImageVector)

private val tabs = listOf(
    Tab(Routes.ORDERS, UiR.string.nav_store_orders, Icons.Filled.LocalShipping),
    Tab(Routes.PRODUCTS, UiR.string.nav_store_products, Icons.Filled.Checkroom),
    Tab(Routes.REVIEWS, UiR.string.nav_store_reviews, Icons.Filled.RateReview),
    Tab(Routes.MORE, UiR.string.nav_store_more, Icons.Filled.Menu),
)

@Composable
fun StoreRoot(onRestart: () -> Unit) {
    val api = LocalGraph.current.api
    val auth by api.session.state.collectAsStateWithLifecycle()
    val lost by api.session.lostEvents.collectAsStateWithLifecycle()
    val context = LocalContext.current
    val lostText = stringResource(UiR.string.app_session_lost)
    var boot by remember(api) { mutableStateOf<RefreshOutcome?>(null) }
    var bootTry by remember(api) { mutableIntStateOf(0) }
    LaunchedEffect(api, bootTry) {
        boot = if (api.session.state.value is AuthState.LoggedIn) RefreshOutcome.OK else api.client.refresh()
    }
    LaunchedEffect(lost) { if (lost > 0) Toast.makeText(context, lostText, Toast.LENGTH_LONG).show() }
    when {
        boot == null -> CenterLoading()
        boot == RefreshOutcome.FAILED && auth == AuthState.Unknown -> ErrorState(ApiException.network(), onRetry = { boot = null; bootTry++ })
        auth is AuthState.LoggedIn -> StoreNav(onRestart)
        else -> StoreLoginScreen(onRestart)
    }
}

@Composable
fun StoreNav(onRestart: () -> Unit) {
    val nav = rememberNavController()
    val entry by nav.currentBackStackEntryAsState()
    val route = entry?.destination?.route
    Scaffold(
        bottomBar = {
            if (route in Routes.TABS) {
                NavigationBar(containerColor = MaterialTheme.colorScheme.surface) {
                    tabs.forEach { t ->
                        NavigationBarItem(
                            selected = route == t.route,
                            onClick = {
                                nav.navigate(t.route) {
                                    popUpTo(Routes.ORDERS) { saveState = true }
                                    launchSingleTop = true
                                    restoreState = true
                                }
                            },
                            icon = { Icon(t.icon, contentDescription = null) },
                            label = { Text(stringResource(t.label), maxLines = 1) },
                            modifier = Modifier.testTag("tab_${t.route}"),
                        )
                    }
                }
            }
        },
    ) { pad ->
        NavHost(nav, startDestination = Routes.ORDERS, modifier = Modifier.padding(bottom = pad.calculateBottomPadding())) {
            composable(Routes.ORDERS) { StoreOrdersScreen(onOrder = { nav.navigate(Routes.order(it)) }) }
            composable(Routes.ORDER, arguments = listOf(navArgument("id") { type = NavType.StringType })) { e ->
                StoreOrderScreen(id = e.arguments?.getString("id").orEmpty(), onBack = { nav.popBackStack() })
            }
            composable(Routes.PRODUCTS) {
                StoreProductsScreen(onProduct = { nav.navigate(Routes.product(it)) }, onNew = { nav.navigate(Routes.NEW_PRODUCT) })
            }
            composable(Routes.NEW_PRODUCT) {
                ProductEditScreen(id = null, onBack = { nav.popBackStack() }, onCreated = { id ->
                    nav.navigate(Routes.product(id)) { popUpTo(Routes.PRODUCTS) }
                })
            }
            composable(Routes.PRODUCT, arguments = listOf(navArgument("id") { type = NavType.StringType })) { e ->
                ProductEditScreen(id = e.arguments?.getString("id"), onBack = { nav.popBackStack() }, onCreated = {})
            }
            composable(Routes.REVIEWS) { StoreReviewsScreen() }
            composable(Routes.MORE) {
                MoreScreen(onStats = { nav.navigate(Routes.STATS) }, onNotifications = { nav.navigate(Routes.NOTIFICATIONS) }, onRestart = onRestart)
            }
            composable(Routes.STATS) { StatsScreen(onBack = { nav.popBackStack() }) }
            composable(Routes.NOTIFICATIONS) {
                NotificationsScreen(onBack = { nav.popBackStack() }) { t ->
                    when (t) {
                        is NotificationTarget.StoreOrder -> if (t.id != null) nav.navigate(Routes.order(t.id!!)) else nav.navigate(Routes.ORDERS)
                        is NotificationTarget.StoreProduct -> if (t.id != null) nav.navigate(Routes.product(t.id!!)) else nav.navigate(Routes.PRODUCTS)
                        is NotificationTarget.StoreReviews -> nav.navigate(Routes.REVIEWS)
                        else -> Unit
                    }
                }
            }
        }
    }
}
