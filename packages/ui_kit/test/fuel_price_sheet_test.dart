import 'package:cosmicforge_logistics_ui_kit/cosmicforge_logistics_ui_kit.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

Future<bool?> _open(WidgetTester tester, Future<String?> Function(FuelPriceEntry) onSubmit) async {
  bool? result;
  await tester.pumpWidget(MaterialApp(
    home: Builder(
      builder: (context) => TextButton(
        onPressed: () async => result = await FuelPriceSheet.show(context, onSubmit: onSubmit, prompted: true),
        child: const Text('open'),
      ),
    ),
  ));
  await tester.tap(find.text('open'));
  await tester.pumpAndSettle();
  return result;
}

void main() {
  testWidgets('sends the chosen fuel and the price in kobo', (tester) async {
    FuelPriceEntry? sent;
    await _open(tester, (e) async {
      sent = e;
      return null;
    });
    expect(find.text('What did you pay for fuel today?'), findsOneWidget);
    await tester.tap(find.text('Diesel'));
    await tester.enterText(find.byType(TextField), '1150');
    await tester.pump();
    await tester.tap(find.text('Send price'));
    await tester.pumpAndSettle();
    expect(sent?.fuelType, FuelType.diesel);
    expect(sent?.pricePerLitreKobo, 115000);
    expect(find.text('What did you pay for fuel today?'), findsNothing);
  });

  testWidgets('refuses an out-of-range price and shows a send error', (tester) async {
    await _open(tester, (e) async => 'No connection');
    await tester.enterText(find.byType(TextField), '50');
    await tester.pump();
    expect(find.text('Enter between ₦100 and ₦10,000'), findsOneWidget);
    expect(tester.widget<FilledButton>(find.byType(FilledButton)).onPressed, isNull);
    await tester.enterText(find.byType(TextField), '950');
    await tester.pump();
    await tester.tap(find.text('Send price'));
    await tester.pumpAndSettle();
    expect(find.text('No connection'), findsOneWidget);
    await tester.tap(find.text('Not today'));
    await tester.pumpAndSettle();
    expect(find.text('No connection'), findsNothing);
  });
}
