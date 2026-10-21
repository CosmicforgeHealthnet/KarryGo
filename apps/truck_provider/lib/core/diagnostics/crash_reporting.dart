import 'package:firebase_crashlytics/firebase_crashlytics.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/widgets.dart';

import '../../features/notifications/data/firebase_push.dart';

/// Starts the app with crashes reported to Firebase Crashlytics, so failures
/// on drivers' phones are seen and fixed. Debug builds report nothing, and a
/// platform without Firebase config starts normally.
Future<void> runWithCrashReporting(Widget app) async {
  WidgetsFlutterBinding.ensureInitialized();
  if (await FirebasePush.ensureInitialized()) {
    try {
      final crashlytics = FirebaseCrashlytics.instance;
      await crashlytics.setCrashlyticsCollectionEnabled(!kDebugMode);
      FlutterError.onError = crashlytics.recordFlutterFatalError;
      PlatformDispatcher.instance.onError = (error, stack) {
        crashlytics.recordError(error, stack, fatal: true);
        return true;
      };
    } catch (error) {
      if (kDebugMode) debugPrint('crash reporting unavailable: $error');
    }
  }
  runApp(app);
}
