import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:customer/features/auth/data/customer_auth_api.dart';
import 'package:customer/features/auth/data/customer_session_store.dart';
import 'package:customer/features/media/data/media_file_api.dart';
import 'package:customer/features/media/data/media_upload_service.dart';
import 'package:customer/features/auth/state/customer_auth_controller.dart';
import 'package:customer/features/taxi/data/taxi_api.dart';
import 'package:customer/features/taxi/models/taxi_models.dart';
import 'package:customer/features/taxi/state/taxi_booking_controller.dart';
import 'package:customer/features/wallet/data/wallet_api.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

/// The auth controller's media dependency is irrelevant here but required by
/// its constructor; this satisfies it without touching the network.
class _StubMediaUploadService extends MediaUploadService {
  _StubMediaUploadService()
    : super(
        api: MediaFileApi(
          config: const ApiCoreConfig(
            baseUrl: 'http://localhost:8109/api/v1/media-files',
          ),
          serviceToken: 'test-token',
          client: MockClient((_) async => http.Response('{}', 500)),
        ),
      );
}

/// Drives the flow-state machine only: no network is exercised beyond the
/// stubs below, because what is under test is which screen comes next.
void main() {
  TaxiBookingController buildController() {
    final client = MockClient((request) async {
      return http.Response(
        jsonEncode({'success': true, 'data': {}}),
        200,
        headers: {'content-type': 'application/json'},
      );
    });
    return TaxiBookingController(
      api: TaxiApi(
        config: ApiCoreConfig(baseUrl: 'http://localhost:8102/api/v1/taxi'),
        client: client,
      ),
      authController: CustomerAuthController(
        api: CustomerAuthApi(
          config: ApiCoreConfig(baseUrl: 'http://localhost:8101/api/v1/customer'),
          client: client,
        ),
        sessionStore: MemoryCustomerSessionStore(),
        mediaUploadService: _StubMediaUploadService(),
      ),
      walletApi: WalletApi(
        config: ApiCoreConfig(
          baseUrl: 'http://localhost:8105/api/v1/payment-wallet',
        ),
        client: client,
      ),
    );
  }

  test('the flow opens on the ride-versus-package choice', () {
    final c = buildController();
    expect(c.state.status, TaxiFlowStatus.idle);
    c.startTaxiFlow();
    expect(c.state.status, TaxiFlowStatus.modeSelection);
  });

  test('choosing a mode advances to location entry and records the mode', () {
    final c = buildController();
    c.startTaxiFlow();
    c.selectMode(TaxiRideMode.parcel);
    expect(c.state.status, TaxiFlowStatus.locationEntry);
    expect(c.state.rideMode, TaxiRideMode.parcel);
    expect(c.state.rideMode.wireValue, 'parcel');
  });

  test('a ride skips package details; a parcel does not', () {
    final tile = const TaxiFareTile(
      vehicleClass: 'comfort',
      label: 'Comfort',
      fareKobo: 222150,
      distanceKm: 8.05,
      durationMin: 19,
      quoteReference: 'q-1',
    );

    final ride = buildController()
      ..startTaxiFlow()
      ..selectMode(TaxiRideMode.ride)
      ..selectClass(tile)
      ..proceedToRideOptions();
    expect(ride.state.status, TaxiFlowStatus.rideOptions);

    final parcel = buildController()
      ..startTaxiFlow()
      ..selectMode(TaxiRideMode.parcel)
      ..selectClass(tile)
      ..proceedToRideOptions();
    expect(parcel.state.status, TaxiFlowStatus.parcelDetails);
  });

  test('package details gate on description, receiver name and phone', () {
    final c = buildController()
      ..startTaxiFlow()
      ..selectMode(TaxiRideMode.parcel);

    expect(c.state.canSubmitParcelDetails, isFalse);
    c.setPackageDesc('A4 documents');
    expect(c.state.canSubmitParcelDetails, isFalse);
    c.setReceiverName('Ada Receiver');
    expect(c.state.canSubmitParcelDetails, isFalse);
    c.setReceiverPhone('08055512345');
    expect(c.state.canSubmitParcelDetails, isTrue);

    c.proceedFromParcelDetails();
    expect(c.state.status, TaxiFlowStatus.rideOptions);
  });

  test('an incomplete parcel cannot advance past package details', () {
    final c = buildController()
      ..startTaxiFlow()
      ..selectMode(TaxiRideMode.parcel)
      ..setPackageDesc('A4 documents');

    // Missing receiver name and phone: the advance must be refused rather
    // than deferred to a server-side field error.
    c.proceedFromParcelDetails();
    expect(c.state.status, isNot(TaxiFlowStatus.rideOptions));
  });

  test('back from confirm returns to package details for a parcel only', () {
    final parcel = buildController()
      ..startTaxiFlow()
      ..selectMode(TaxiRideMode.parcel)
      ..backToParcelDetails();
    expect(parcel.state.status, TaxiFlowStatus.parcelDetails);

    final ride = buildController()
      ..startTaxiFlow()
      ..selectMode(TaxiRideMode.ride)
      ..backToFareSelection();
    expect(ride.state.status, TaxiFlowStatus.fareSelection);
  });
}
