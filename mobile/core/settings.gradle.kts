// Отдельная сборка, чтобы core собирался и тестировался без Android SDK:
//   cd mobile/core && ../gradlew test
pluginManagement {
    repositories {
        mavenCentral()
        gradlePluginPortal()
    }
}
dependencyResolutionManagement {
    repositories { mavenCentral() }
    versionCatalogs {
        create("libs") { from(files("../gradle/libs.versions.toml")) }
    }
}
rootProject.name = "core"
