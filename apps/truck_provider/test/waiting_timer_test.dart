import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:truck_provider/features/home/models/booking_waiting.dart';
import 'package:truck_provider/features/home/ui/widgets/waiting_timer_card.dart';

BookingWaiting waitingJson({DateTime? pickupArrived, DateTime? pickupLeft, int pickupFee = 0}) {
  return BookingWaiting.fromJson({
    'free_minutes': 60,
    'block_minutes': 30,
    'block_fee_kobo': 200000,
    'total_fee_kobo': pickupFee,
    'fee_status': 'PENDING',
    'stops': [
      {
        'stop': 'PICKUP',
        if (pickupArrived != null) 'arrived_at': pickupArrived.toUtc().toIso8601String(),
        if (pickupLeft != null) 'left_at': pickupLeft.toUtc().toIso8601String(),
        if (pickupArrived != null) 'free_until': pickupArrived.add(const Duration(hours: 1)).toUtc().toIso8601String(),
        'waited_seconds': pickupLeft != null && pickupArrived != null ? pickupLeft.difference(pickupArrived).inSeconds : 0,
        'fee_kobo': pickupFee,
        'running': pickupArrived != null && pickupLeft == null,
      },
      {'stop': 'DROPOFF', 'waited_seconds': 0, 'fee_kobo': 0, 'running': false},
    ],
  });
}

Widget host(Widget child) => MaterialApp(home: Scaffold(body: ListView(children: [child])));

void main() {
  test('fee rule matches the server: free time, then every started block', () {
    final w = waitingJson();
    expect(w.feeFor(const Duration(minutes: 60)), 0);
    expect(w.feeFor(const Duration(minutes: 60, seconds: 1)), 200000);
    expect(w.feeFor(const Duration(minutes: 95)), 400000);
  });

  testWidgets('shows a running pickup clock still in free time', (tester) async {
    final arrived = DateTime.now().subtract(const Duration(minutes: 5));
    await tester.pumpWidget(host(WaitingTimerCard(
      status: 'arrived_at_pickup',
      load: () async => waitingJson(pickupArrived: arrived),
    )));
    await tester.pump();
    expect(find.text('Waiting at pickup'), findsOneWidget);
    expect(find.textContaining('Free for'), findsOneWidget);
  });

  testWidgets('cash completion tells the driver to collect the fee', (tester) async {
    final arrived = DateTime.now().subtract(const Duration(minutes: 95));
    await tester.pumpWidget(host(WaitingTimerCard(
      status: 'en_route_delivery',
      load: () async => waitingJson(pickupArrived: arrived, pickupLeft: arrived.add(const Duration(minutes: 95)), pickupFee: 400000),
      feeNote: true,
      isCash: true,
    )));
    await tester.pump();
    expect(find.textContaining('Collect it in cash'), findsOneWidget);
    expect(find.text("I've arrived at drop-off"), findsNothing);
  });

  testWidgets('offers the drop-off arrival and starts that clock', (tester) async {
    var arrivedCalls = 0;
    await tester.pumpWidget(host(WaitingTimerCard(
      status: 'en_route_delivery',
      load: () async => waitingJson(),
      onArrivedAtDropoff: () async {
        arrivedCalls++;
        return BookingWaiting.fromJson({
          'free_minutes': 60, 'block_minutes': 30, 'block_fee_kobo': 200000, 'fee_status': 'PENDING',
          'stops': [
            {'stop': 'PICKUP', 'waited_seconds': 0, 'fee_kobo': 0, 'running': false},
            {'stop': 'DROPOFF', 'arrived_at': DateTime.now().toUtc().toIso8601String(), 'waited_seconds': 0, 'fee_kobo': 0, 'running': true},
          ],
        });
      },
    )));
    await tester.pump();
    await tester.tap(find.text("I've arrived at drop-off"));
    await tester.pump();
    expect(arrivedCalls, 1);
    expect(find.text('Waiting at drop-off'), findsOneWidget);
  });
}
