import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:taxi_provider/features/home/data/provider_api.dart';
import 'package:taxi_provider/features/home/models/provider_booking.dart';

void main() {
  ProviderApi buildApi(Future<http.Response> Function(http.Request) handler) {
    return ProviderApi(
      config: ApiCoreConfig(baseUrl: 'http://localhost:8102/api/v1/taxi'),
      client: MockClient(handler),
    );
  }

  http.Response ok(Map<String, dynamic> data) => http.Response(
    jsonEncode({'success': true, 'data': data}),
    200,
    headers: {'content-type': 'application/json'},
  );

  Map<String, dynamic> bookingJson({
    String status = 'awaiting_acceptance',
    String pickupVerification = 'not_required',
    String deliveryVerification = 'not_required',
    String ridePurpose = 'passenger',
  }) => {
    'id': 'b-1234567890',
    'status': status,
    'pickup_verification_status': pickupVerification,
    'delivery_verification_status': deliveryVerification,
    'ride_purpose': ridePurpose,
    'vehicle_class': 'comfort',
    'fare_estimate_kobo': 271450,
    'distance_km': 10.7,
    'duration_minutes': 25,
    'stops': [
      {'sequence': 1, 'type': 'PICKUP', 'address': 'Yaba', 'lat': 6.5, 'lng': 3.4},
      {'sequence': 2, 'type': 'DROPOFF', 'address': 'Ikeja', 'lat': 6.6, 'lng': 3.3},
    ],
  };

  group('booking status', () {
    test('offers, active rides and terminals are grouped correctly', () {
      expect(BookingStatus.fromString('awaiting_acceptance').isOffer, isTrue);
      expect(BookingStatus.fromString('accepted').isActive, isTrue);
      expect(BookingStatus.fromString('in_progress').isActive, isTrue);
      expect(BookingStatus.fromString('completed').isTerminal, isTrue);
      expect(BookingStatus.fromString('unmatched').isTerminal, isTrue);
      // Declared by the service but never assigned; mapping it explicitly
      // keeps it from degrading to `unknown` if that ever changes.
      expect(
        BookingStatus.fromString('pickup_verification_pending'),
        BookingStatus.arrivedAtPickup,
      );
    });
  });

  test('an offer parses its route, fare and job type', () async {
    final api = buildApi((_) async => ok({'bookings': [bookingJson()]}));
    final bookings = await api.listActiveBookings(accessToken: 't');

    expect(bookings, hasLength(1));
    final b = bookings.first;
    expect(b.status.isOffer, isTrue);
    expect(b.isParcel, isFalse);
    expect(b.pickup?.address, 'Yaba');
    expect(b.dropoff?.address, 'Ikeja');
    expect(b.displayFareKobo, 271450);
    expect(b.shortId, 'B-123456');
  });

  test('a parcel job exposes its package and receiver detail', () async {
    final api = buildApi((_) async {
      final json = bookingJson(ridePurpose: 'parcel', status: 'accepted');
      json['receiver_name'] = 'Ada Receiver';
      json['receiver_phone'] = '+2348055512345';
      json['package_desc'] = 'A4 documents';
      json['is_fragile'] = true;
      return ok({'bookings': [json]});
    });

    final b = (await api.listActiveBookings(accessToken: 't')).first;
    expect(b.isParcel, isTrue);
    expect(b.receiverName, 'Ada Receiver');
    expect(b.receiverPhone, '+2348055512345');
    expect(b.isFragile, isTrue);
  });

  test('a pending pickup blocks the trip until verified', () {
    final waiting = ProviderBooking.fromJson(
      bookingJson(status: 'arrived_at_pickup', pickupVerification: 'pending'),
    );
    expect(waiting.awaitingPickupVerification, isTrue);

    final cleared = ProviderBooking.fromJson(
      bookingJson(status: 'arrived_at_pickup', pickupVerification: 'verified'),
    );
    expect(cleared.awaitingPickupVerification, isFalse);
  });

  test('a parcel in transit awaits its delivery code', () {
    final b = ProviderBooking.fromJson(
      bookingJson(
        status: 'in_progress',
        ridePurpose: 'parcel',
        deliveryVerification: 'pending',
      ),
    );
    expect(b.awaitingDeliveryVerification, isTrue);
  });

  test('verify-pickup and verify-delivery hit their own routes', () async {
    final paths = <String>[];
    final api = buildApi((request) async {
      paths.add(request.url.path);
      return ok(bookingJson());
    });

    await api.verifyPickup(accessToken: 't', bookingId: 'b-1', code: '111111');
    await api.verifyDelivery(accessToken: 't', bookingId: 'b-1', code: '222222');

    expect(paths, [
      '/api/v1/taxi/provider/bookings/b-1/verify-pickup',
      '/api/v1/taxi/provider/bookings/b-1/verify-delivery',
    ]);
  });

  test('going online sends the status the service expects', () async {
    late http.Request captured;
    final api = buildApi((request) async {
      captured = request;
      return ok({'online': true, 'heartbeat_seconds': 30});
    });

    final state = await api.setAvailability(
      accessToken: 't',
      online: true,
      lat: 6.5,
      lng: 3.4,
    );

    expect(captured.method, 'PUT');
    final body = jsonDecode(captured.body) as Map<String, dynamic>;
    expect(body['status'], 'online');
    expect(body['lat'], 6.5);
    expect(state.online, isTrue);
  });

  test('a refused go-online surfaces the service message verbatim', () async {
    // The gate names the specific compliance dimension that failed; a generic
    // "could not go online" would strand the driver with no idea what to fix.
    final api = buildApi((_) async {
      return http.Response(
        jsonEncode({
          'success': false,
          'error': {
            'code': 'conflict',
            'message': 'Your vehicle is not verified yet.',
          },
        }),
        409,
        headers: {'content-type': 'application/json'},
      );
    });

    expect(
      () => api.setAvailability(
        accessToken: 't',
        online: true,
        lat: 6.5,
        lng: 3.4,
      ),
      throwsA(
        isA<ApiException>().having(
          (e) => e.message,
          'message',
          'Your vehicle is not verified yet.',
        ),
      ),
    );
  });
}
