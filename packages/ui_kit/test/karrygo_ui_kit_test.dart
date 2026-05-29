import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:karrygo_ui_kit/karrygo_ui_kit.dart';

void main() {
  test('exposes KarryGo primary color', () {
    expect(KarryGoColors.primary, const Color(0xFF20AD4E));
  });

  testWidgets('renders error banner message', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: KarryGoTheme.light(),
        home: const Scaffold(
          body: KarryGoErrorBanner(message: 'Invalid code. Please try again.'),
        ),
      ),
    );

    expect(find.text('Invalid code. Please try again.'), findsOneWidget);
  });
}
