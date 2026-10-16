import 'dart:async';
import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:dispatch_provider/features/availability/data/availability_api.dart';
import 'package:dispatch_provider/features/availability/state/availability_controller.dart';
import 'package:dispatch_provider/features/trips/data/trips_api.dart';
import 'package:dispatch_provider/features/trips/state/trips_controller.dart';
import 'package:dispatch_provider/features/wallet/data/wallet_api.dart';
import 'package:dispatch_provider/features/wallet/state/wallet_controller.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

const config = ApiCoreConfig(baseUrl: 'https://dispatch.example.test');

void main() {
  test('logout discards late wallet response and clears earnings', () async {
    final reply = Completer<http.Response>();
    var token = 'provider-one';
    final controller = WalletController(
      api: WalletApi(config, client: MockClient((_) => reply.future)),
      getAccessToken: () => token,
    );
    addTearDown(controller.dispose);

    final reading = controller.loadEarnings();
    controller.resetSession();
    token = 'provider-two';
    reply.complete(http.Response(jsonEncode({
      'success': true,
      'data': {'available_kobo': 99900, 'pending_kobo': 0, 'currency': 'NGN'},
    }), 200));

    expect((await reading).isFailure, true);
    expect(controller.earnings, isNull);
    expect(controller.bankAccount, isNull);
    expect(controller.pinSet, isNull);
    expect(controller.isLoading, false);
  });

  test('logout discards late active trip response and pickup state', () async {
    final reply = Completer<http.Response>();
    var token = 'provider-one';
    final controller = TripsController(
      api: TripsApi(config, client: MockClient((_) => reply.future)),
      getAccessToken: () => token,
    );
    addTearDown(controller.dispose);

    final reading = controller.loadActiveTrip();
    controller.resetSession();
    token = 'provider-two';
    reply.complete(http.Response(jsonEncode({
      'success': true,
      'data': {
        'id': 'former-rider-trip',
        'booking_id': 'former-booking',
        'status': 'arrived_pickup',
        'pickup_address': 'Former address',
      },
    }), 200));
    expect((await reading).isFailure, true);
    expect(controller.activeTrip, isNull);
    expect(controller.pickupCredential, isNull);
    expect(controller.trips, isEmpty);
  });

  test('logout discards late availability response without starting GPS',
      () async {
    final reply = Completer<http.Response>();
    var token = 'provider-one';
    final controller = AvailabilityController(
      api: AvailabilityApi(config, client: MockClient((_) => reply.future)),
      getAccessToken: () => token,
    );
    addTearDown(controller.dispose);

    final reading = controller.loadAvailability();
    controller.resetSession();
    token = 'provider-two';
    reply.complete(http.Response(jsonEncode({
      'success': true,
      'data': {'status': 'online'},
    }), 200));
    expect((await reading).isFailure, true);
    expect(controller.availability, isNull);
    expect(controller.isOnline, false);
    expect(controller.lastLocation, isNull);
  });
}
