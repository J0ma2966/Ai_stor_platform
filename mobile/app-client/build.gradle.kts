plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.compose)
}

val apiUrl = providers.gradleProperty("boutique.apiUrl").get()
val debugApiUrl = providers.gradleProperty("boutique.debugApiUrl").get()
val siteUrl = providers.gradleProperty("boutique.siteUrl").get()

android {
    namespace = "uz.boutique.client"
    compileSdk = 36

    defaultConfig {
        applicationId = "uz.boutique.client"
        minSdk = 26
        targetSdk = 36
        versionCode = 1
        versionName = "1.0.0"
        buildConfigField("String", "SITE_URL", "\"$siteUrl\"")
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    signingConfigs {
        // Подпись release: переменные окружения задаются в CI (секреты) или локально.
        val ks = System.getenv("BOUTIQUE_KEYSTORE")
        if (ks != null) {
            create("release") {
                storeFile = file(ks)
                storePassword = System.getenv("BOUTIQUE_KEYSTORE_PASSWORD")
                keyAlias = System.getenv("BOUTIQUE_KEY_ALIAS")
                keyPassword = System.getenv("BOUTIQUE_KEY_PASSWORD")
            }
        }
    }

    buildTypes {
        debug {
            applicationIdSuffix = ".debug"
            buildConfigField("String", "API_URL", "\"$debugApiUrl\"")
            buildConfigField("boolean", "SERVER_OVERRIDE", "true")
            buildConfigField("boolean", "TRYON_ENABLED", "true")
        }
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
            buildConfigField("String", "API_URL", "\"$apiUrl\"")
            buildConfigField("boolean", "SERVER_OVERRIDE", "false")
            // В проде примерка выключена (как NEXT_PUBLIC_TRYON_ENABLED на сайте); включить: -Pboutique.tryonRelease=true
            buildConfigField("boolean", "TRYON_ENABLED", providers.gradleProperty("boutique.tryonRelease").getOrElse("false"))
            signingConfigs.findByName("release")?.let { signingConfig = it }
        }
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions {
        jvmTarget = "17"
        freeCompilerArgs += listOf(
            "-opt-in=androidx.compose.material3.ExperimentalMaterial3Api",
            "-opt-in=androidx.compose.foundation.ExperimentalFoundationApi",
            "-opt-in=androidx.compose.foundation.layout.ExperimentalLayoutApi",
        )
    }
    buildFeatures {
        compose = true
        buildConfig = true
    }
    testOptions { unitTests.isIncludeAndroidResources = true }
    lint {
        abortOnError = true
        warningsAsErrors = false
    }
}

dependencies {
    implementation(project(":ui"))
    testImplementation(libs.junit)
    testImplementation(libs.robolectric)
    testImplementation(libs.androidx.test.core)
    testImplementation(libs.androidx.test.junit)
    testImplementation(libs.androidx.compose.ui.test.junit4)
    testImplementation(libs.okhttp.mockwebserver)
    debugImplementation(libs.androidx.compose.ui.test.manifest)
}
