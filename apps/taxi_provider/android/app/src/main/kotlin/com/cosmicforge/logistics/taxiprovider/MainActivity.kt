package com.cosmicforge.logistics.taxiprovider

import android.content.pm.PackageManager
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel

class MainActivity : FlutterActivity() {
    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)
        // Tells the map widget whether this build carries a Maps SDK key, so
        // the map shows whenever one is configured and a placeholder when not.
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, "karryam/maps")
            .setMethodCallHandler { call, result ->
                if (call.method == "isConfigured") {
                    result.success(mapsKeyConfigured())
                } else {
                    result.notImplemented()
                }
            }
    }

    private fun mapsKeyConfigured(): Boolean = try {
        val info = packageManager.getApplicationInfo(packageName, PackageManager.GET_META_DATA)
        !info.metaData?.getString("com.google.android.geo.API_KEY").isNullOrBlank()
    } catch (e: PackageManager.NameNotFoundException) {
        false
    }
}
