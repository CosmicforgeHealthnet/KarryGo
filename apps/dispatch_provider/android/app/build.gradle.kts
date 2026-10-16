plugins {
    id("com.android.application")
    id("kotlin-android")
    // The Flutter Gradle Plugin must be applied after the Android and Kotlin Gradle plugins.
    id("dev.flutter.flutter-gradle-plugin")
    id("com.google.gms.google-services")
}

android {
    namespace = "com.cosmicforge.logistics.dispatch_provider"
    compileSdk = flutter.compileSdkVersion
    ndkVersion = flutter.ndkVersion

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = JavaVersion.VERSION_17.toString()
    }

    defaultConfig {
        // TODO: Specify your own unique Application ID (https://developer.android.com/studio/build/application-id.html).
        applicationId = "com.cosmicforge.logistics.dispatch_provider"
        // You can update the following values to match your application needs.
        // For more information, see: https://flutter.dev/to/review-gradle-config.
        minSdk = flutter.minSdkVersion
        targetSdk = flutter.targetSdkVersion
        versionCode = flutter.versionCode
        versionName = flutter.versionName
    }

    // Release signing material is injected through environment variables,
    // never stored in this repository or substituted with debug keys.
    signingConfigs {
        create("release") {
            val keystorePath = System.getenv("KARRYAM_ANDROID_KEYSTORE_PATH")
            val keystorePassword = System.getenv("KARRYAM_ANDROID_STORE_PASSWORD")
            val releaseAlias = System.getenv("KARRYAM_ANDROID_KEY_ALIAS")
            val releaseKeyPassword = System.getenv("KARRYAM_ANDROID_KEY_PASSWORD")
            if (!keystorePath.isNullOrBlank() &&
                !keystorePassword.isNullOrBlank() &&
                !releaseAlias.isNullOrBlank() &&
                !releaseKeyPassword.isNullOrBlank()) {
                storeFile = file(keystorePath)
                storePassword = keystorePassword
                keyAlias = releaseAlias
                keyPassword = releaseKeyPassword
            }
        }
    }

    val requestsRelease = gradle.startParameter.taskNames.any {
        it.contains("Release", ignoreCase = true) && !it.contains("UnitTest", ignoreCase = true)
    }
    if (requestsRelease) {
        val requiredKeys = listOf(
            "KARRYAM_ANDROID_KEYSTORE_PATH",
            "KARRYAM_ANDROID_STORE_PASSWORD",
            "KARRYAM_ANDROID_KEY_ALIAS",
            "KARRYAM_ANDROID_KEY_PASSWORD",
        )
        val missing = requiredKeys.filter { System.getenv(it).isNullOrBlank() }
        check(missing.isEmpty()) {
            "Release signing is not configured. Missing: " + missing.joinToString(", ")
        }
    }

    buildTypes {
        release {
            signingConfig = signingConfigs.getByName("release")
        }
    }
}

flutter {
    source = "../.."
}
