import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:dispatch_provider/app/dispatch_provider_app.dart';

void main() {
  testWidgets('App splash screen smoke test', (WidgetTester tester) async {
    // Configure virtual screen size to avoid layout overflow issues in tests
    tester.view.physicalSize = const Size(1080, 2400);
    tester.view.devicePixelRatio = 1.0;

    // Build our app and trigger a frame.
    await tester.pumpWidget(const DispatchProviderApp());

    // Verify that splash screen is shown and shows "Cosmicforge Logistics".
    expect(find.text('Cosmicforge Logistics'), findsOneWidget);

    // Let the splash timer finish to transition to onboarding
    await tester.pump(const Duration(seconds: 3));
    await tester.pump();
  });
}
