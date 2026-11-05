import org.jetbrains.kotlin.gradle.dsl.JvmTarget

plugins {
    alias(libs.plugins.kotlin.jvm)
    alias(libs.plugins.kotlin.serialization)
}

group = "uz.boutique"
version = "1.0"

java {
    sourceCompatibility = JavaVersion.VERSION_17
    targetCompatibility = JavaVersion.VERSION_17
}
kotlin {
    compilerOptions { jvmTarget.set(JvmTarget.JVM_17) }
}

dependencies {
    api(libs.kotlinx.coroutines.core)
    api(libs.kotlinx.serialization.json)
    api(libs.okhttp)

    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
    testImplementation(libs.okhttp.mockwebserver)
    testImplementation(kotlin("test"))
}

tasks.test {
    // Интеграционный тест против запущенного бэкенда: BOUTIQUE_IT_API=http://localhost:8080
    for (k in listOf("BOUTIQUE_IT_API", "BOUTIQUE_IT_OTP_LOG", "BOUTIQUE_IT_STORE_LOGIN", "BOUTIQUE_IT_STORE_PASSWORD")) {
        environment(k, System.getenv(k) ?: "")
    }
    testLogging { events("failed"); exceptionFormat = org.gradle.api.tasks.testing.logging.TestExceptionFormat.FULL }
}
