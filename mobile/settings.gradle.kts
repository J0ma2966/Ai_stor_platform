// Мобильные приложения маркетплейса (Android).
// core — чистый Kotlin/JVM (API-клиент, модели, логика); собирается и тестируется без Android SDK.
// ui — общие экраны, тема и компоненты; app-client и app-store — два отдельных приложения.
pluginManagement {
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}
dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        google()
        mavenCentral()
    }
}

rootProject.name = "boutique-mobile"
includeBuild("core")
include(":ui", ":app-client", ":app-store")
