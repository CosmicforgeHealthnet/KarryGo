import 'dart:io' show Platform;
import 'package:flutter/foundation.dart' show kIsWeb;

class AppConfig {
  static String get apiBaseUrl {
    // Windows desktop testing: http://localhost:8103
    // Android real device testing with adb reverse: http://127.0.0.1:8103
    if (!kIsWeb && Platform.isAndroid) {
      return 'http://127.0.0.1:8103';
    }
    return 'http://localhost:8103';
  }
}
