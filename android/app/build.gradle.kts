import java.util.Properties
import org.jetbrains.kotlin.gradle.dsl.JvmTarget

plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.compose)
}

// URL del servidor radiofm fijada al compilar (gradle.properties o -Pclub25.serverUrl=...).
val serverUrlRelease = (findProperty("club25.serverUrl") as String).trimEnd('/')
val serverUrlDebug = (findProperty("club25.serverUrl.debug") as String).trimEnd('/')
require(serverUrlRelease.startsWith("https://")) { "club25.serverUrl debe empezar con https:// (Google Play y Android bloquean http)" }

// Firma de release: datos en android/keystore.properties (no se versiona; ver README.md).
val keystoreProps = Properties().apply {
    val f = rootProject.file("keystore.properties")
    if (f.exists()) f.inputStream().use { load(it) }
}

android {
    namespace = "pe.club25.radio"
    compileSdk = 36

    defaultConfig {
        applicationId = "pe.club25peru.radio"
        minSdk = 26
        targetSdk = 36
        // Suba versionCode en cada publicación en Play (o pase -Pclub25.versionCode=N).
        versionCode = (findProperty("club25.versionCode") as String?)?.toInt() ?: 1
        versionName = (findProperty("club25.versionName") as String?) ?: "1.0.0"
    }

    signingConfigs {
        if (keystoreProps.isNotEmpty()) {
            create("release") {
                storeFile = rootProject.file(keystoreProps.getProperty("storeFile"))
                storePassword = keystoreProps.getProperty("storePassword")
                keyAlias = keystoreProps.getProperty("keyAlias")
                keyPassword = keystoreProps.getProperty("keyPassword")
            }
        }
    }

    buildTypes {
        debug {
            applicationIdSuffix = ".debug"
            versionNameSuffix = "-dev"
            buildConfigField("String", "SERVER_URL", "\"$serverUrlDebug\"")
        }
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
            buildConfigField("String", "SERVER_URL", "\"$serverUrlRelease\"")
            signingConfig = signingConfigs.findByName("release")
        }
    }

    testOptions {
        // Los tests de JVM usan clases de Android solo de paso (SystemClock): devuelven 0.
        unitTests.isReturnDefaultValues = true
    }
    buildFeatures {
        compose = true
        buildConfig = true
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
}

kotlin {
    compilerOptions { jvmTarget.set(JvmTarget.JVM_17) }
}

dependencies {
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    implementation(platform(libs.compose.bom))
    implementation(libs.compose.ui)
    implementation(libs.compose.ui.tooling.preview)
    implementation(libs.compose.material3)
    implementation(libs.media3.exoplayer)
    implementation(libs.media3.exoplayer.hls)
    implementation(libs.media3.session)
    implementation(libs.media3.ui)
    implementation(libs.kotlinx.coroutines.android)
    implementation(libs.kotlinx.coroutines.guava)
    implementation(libs.coil.compose)
    implementation(libs.coil.svg)

    // Tests de JVM (./gradlew test): org.json real, porque el de Android es un stub.
    testImplementation(libs.junit)
    testImplementation(libs.org.json)
}
