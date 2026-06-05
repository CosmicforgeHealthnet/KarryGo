import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:cosmicforge_logistics_ui_kit/cosmicforge_logistics_ui_kit.dart';

void main() {
  test('exposes Cosmicforge Logistics primary color', () {
    expect(CosmicforgeLogisticsColors.primary, const Color(0xFF20AD4E));
  });

  testWidgets('renders error banner message', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: CosmicforgeLogisticsTheme.light(),
        home: const Scaffold(
          body: CosmicforgeLogisticsErrorBanner(message: 'Invalid code. Please try again.'),
        ),
      ),
    );

    expect(find.text('Invalid code. Please try again.'), findsOneWidget);
  });
}
