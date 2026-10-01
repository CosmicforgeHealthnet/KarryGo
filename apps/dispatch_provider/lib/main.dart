import 'package:flutter/material.dart';

import 'app/dispatch_provider_app.dart';
import 'core/app_mode.dart';
import 'core/theme/theme_controller.dart';

Future<void> main() async {
  // Required before any plugin (FlutterSecureStorage) is used during startup.
  WidgetsFlutterBinding.ensureInitialized();

  // Load the persisted theme choice before the first frame so the app
  // launches in the correct brightness (no light-to-dark flash).
  final themeController = ThemeController();
  await themeController.load();

  // Load the persisted MOCK/LIVE mode so hot restart doesn't reset it.
  await AppModeConfig.instance.load();

  runApp(DispatchProviderApp(themeController: themeController));
}