import Flutter
import UIKit
import GoogleMaps

@main
@objc class AppDelegate: FlutterAppDelegate, FlutterImplicitEngineDelegate {
  private var mapsKeyConfigured = false

  override func application(
    _ application: UIApplication,
    didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]?
  ) -> Bool {
    if let key = Bundle.main.object(forInfoDictionaryKey: "GMSApiKey") as? String,
       !key.isEmpty, !key.hasPrefix("$(") {
      GMSServices.provideAPIKey(key)
      mapsKeyConfigured = true
    }
    return super.application(application, didFinishLaunchingWithOptions: launchOptions)
  }

  func didInitializeImplicitFlutterEngine(_ engineBridge: FlutterImplicitEngineBridge) {
    GeneratedPluginRegistrant.register(with: engineBridge.pluginRegistry)
    // Tells the map widget whether a Maps SDK key was provided above. A
    // GoogleMap created without one crashes on iOS, so the widget shows a
    // placeholder instead.
    if let registrar = engineBridge.pluginRegistry.registrar(forPlugin: "KarryamMaps") {
      let configured = mapsKeyConfigured
      FlutterMethodChannel(name: "karryam/maps", binaryMessenger: registrar.messenger())
        .setMethodCallHandler { call, result in
          if call.method == "isConfigured" {
            result(configured)
          } else {
            result(FlutterMethodNotImplemented)
          }
        }
    }
  }
}
