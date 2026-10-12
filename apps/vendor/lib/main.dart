import 'core/theme/vendor_appearance.dart';
import 'package:flutter/material.dart';
import 'app/vendor_mobile_app.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  final appearance = VendorAppearance();
  await appearance.load();
  runApp(VendorMobileApp(appearance: appearance));
}
