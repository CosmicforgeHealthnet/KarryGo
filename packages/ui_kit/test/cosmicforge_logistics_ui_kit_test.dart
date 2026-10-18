import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:cosmicforge_logistics_ui_kit/cosmicforge_logistics_ui_kit.dart';

void main() {
  test('exposes Cosmicforge Logistics primary color', () {
    expect(CosmicforgeLogisticsColors.primary, const Color(0xFF05124D));
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

  testWidgets('OTP digits stay dark on their white circles in a dark theme',
      (tester) async {
    await tester.pumpWidget(MaterialApp(
      theme: ThemeData(brightness: Brightness.dark),
      home: const Scaffold(
        body: CosmicforgeLogisticsOtpInput(length: 6, value: '482913'),
      ),
    ));
    final digit = tester.widget<Text>(find.text('4'));
    expect(digit.style?.color, CosmicforgeLogisticsColors.text);
  });
}
