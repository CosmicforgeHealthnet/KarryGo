import 'package:customer/shared/widgets/trip_eta_row.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('clock times read like a person would say them', () {
    expect(formatClockTime(DateTime(2026, 10, 27, 0, 5)), '12:05 AM');
    expect(formatClockTime(DateTime(2026, 10, 27, 13, 42)), '1:42 PM');
  });

  testWidgets('shows pickup and drop-off times with countdowns', (tester) async {
    final pickup = DateTime.now().add(const Duration(minutes: 7, seconds: 10));
    final dropoff = DateTime.now().add(const Duration(minutes: 45));
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(body: TripEtaRow(pickupEta: pickup, dropoffEta: dropoff)),
    ));
    expect(find.text('Pickup'), findsOneWidget);
    expect(find.text('Drop-off'), findsOneWidget);
    expect(find.text(formatClockTime(pickup)), findsOneWidget);
    expect(find.text('in 8 min'), findsOneWidget);
  });

  testWidgets('shows nothing without an estimate', (tester) async {
    await tester.pumpWidget(const MaterialApp(home: Scaffold(body: TripEtaRow())));
    expect(find.text('Pickup'), findsNothing);
  });
}
