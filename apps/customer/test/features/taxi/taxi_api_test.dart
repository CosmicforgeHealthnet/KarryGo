import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:customer/features/taxi/data/taxi_api.dart';
import 'package:customer/features/taxi/models/taxi_models.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

void main() {
  TaxiApi buildApi(Future<http.Response> Function(http.Request) handler) {
    return TaxiApi(
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
    String status = 'pending_match',
    String pickupVerification = 'not_required',
  }) => {
    'id': 'b-1',
    'customer_id': 'c-1',
    'market_id': 'm-1',
    'operating_zone_id': 'z-1',
    'booking_mode': 'standard',
    'vehicle_class': 'comfort',
    'passenger_count': 2,
    'ride_notes': 'By the blue gate',
    'fare_estimate_kobo': 222150,
    'distance_km': 8.05,
    'duration_minutes': 19,
    'payment_method': 'wallet',
    'payment_status': 'unpaid',
    'status': status,
    'pickup_verification_status': pickupVerification,
    'created_at': '2026-08-31T18:00:00Z',
    'stops': [
      {
        'id': 's-1',
        'sequence': 1,
        'type': 'PICKUP',
        'address': 'Yaba',
        'lat': 6.5244,
        'lng': 3.3792,
        'status': 'pending',
      },
      {
        'id': 's-2',
        'sequence': 2,
        'type': 'DROPOFF',
        'address': 'Lekki Phase 1',
        'lat': 6.455,
        'lng': 3.4,
        'status': 'pending',
      },
    ],
  };

  group('status mapping', () {
    test('maps every status the service can emit', () {
      final cases = {
        'scheduled': TaxiBookingStatus.scheduled,
        'pending_match': TaxiBookingStatus.pendingMatch,
        'awaiting_acceptance': TaxiBookingStatus.awaitingAcceptance,
        'accepted': TaxiBookingStatus.accepted,
        'en_route_pickup': TaxiBookingStatus.enRoutePickup,
        'arrived_at_pickup': TaxiBookingStatus.arrivedAtPickup,
        'in_progress': TaxiBookingStatus.inProgress,
        'completed': TaxiBookingStatus.completed,
        'cancelled': TaxiBookingStatus.cancelled,
        'unmatched': TaxiBookingStatus.unmatched,
      };
      cases.forEach((wire, expected) {
        expect(TaxiBookingStatus.fromString(wire), expected, reason: wire);
      });
    });

    test('pickup_verification_pending collapses onto arrivedAtPickup', () {
      // The service declares this status but never assigns it to a booking;
      // mapping it explicitly stops it degrading to `unknown` if that changes.
      expect(
        TaxiBookingStatus.fromString('pickup_verification_pending'),
        TaxiBookingStatus.arrivedAtPickup,
      );
    });

    test('searching and active groupings match the service ladder', () {
      expect(TaxiBookingStatus.pendingMatch.isSearching, isTrue);
      expect(TaxiBookingStatus.awaitingAcceptance.isSearching, isTrue);
      expect(TaxiBookingStatus.accepted.isActive, isTrue);
      expect(TaxiBookingStatus.inProgress.isActive, isTrue);
      expect(TaxiBookingStatus.inProgress.isBeforePickup, isFalse);
      expect(TaxiBookingStatus.arrivedAtPickup.isBeforePickup, isTrue);
      expect(TaxiBookingStatus.completed.isTerminal, isTrue);
      expect(TaxiBookingStatus.unmatched.isTerminal, isTrue);
    });
  });

  group('createBooking', () {
    test('sends an Idempotency-Key and the stop-pair contract', () async {
      late http.Request captured;
      final api = buildApi((request) async {
        captured = request;
        return ok(bookingJson());
      });

      await api.createBooking(
        accessToken: 't',
        vehicleClass: 'comfort',
        pickupAddress: 'Yaba',
        pickupLat: 6.5244,
        pickupLng: 3.3792,
        dropoffAddress: 'Lekki Phase 1',
        dropoffLat: 6.455,
        dropoffLng: 3.4,
        rideNotes: 'By the blue gate',
      );

      // The service rejects a create without this header outright.
      expect(captured.headers['Idempotency-Key'], isNotEmpty);
      expect(captured.url.path, '/api/v1/taxi/customer/bookings');

      final body = jsonDecode(captured.body) as Map<String, dynamic>;
      expect(body['vehicle_class'], 'comfort');
      expect(body['booking_mode'], 'standard');
      // Wallet is the only method the service accepts.
      expect(body['payment_method'], 'wallet');
      // The app never asks for a passenger count, so it sends none; the
      // service defaults an absent value to 1.
      expect(body.containsKey('passenger_count'), isFalse);

      final stops = body['stops'] as List;
      expect(stops, hasLength(2));
      expect(stops.first, {
        'address': 'Yaba',
        'lat': 6.5244,
        'lng': 3.3792,
      });
      expect((stops.last as Map)['address'], 'Lekki Phase 1');
    });

    test('generates a distinct idempotency key per call', () {
      final a = TaxiApi.newIdempotencyKey();
      final b = TaxiApi.newIdempotencyKey();
      expect(a, isNot(equals(b)));
    });
  });

  test('cancelBooking uses PUT, not POST', () async {
    late http.Request captured;
    final api = buildApi((request) async {
      captured = request;
      return ok(bookingJson(status: 'cancelled'));
    });

    final booking = await api.cancelBooking(
      accessToken: 't',
      bookingId: 'b-1',
      reason: 'changed_my_mind',
    );

    expect(captured.method, 'PUT');
    expect(captured.url.path, '/api/v1/taxi/customer/bookings/b-1/cancel');
    expect(booking.status, TaxiBookingStatus.cancelled);
  });

  test('estimateFare unwraps the fares list, cheapest first', () async {
    final api = buildApi((request) async {
      return ok({
        'fares': [
          {
            'vehicle_class': 'economy',
            'label': 'Economy',
            'fare_kobo': 168500,
            'distance_km': 8.051,
            'duration_min': 19,
            'quote_reference': 'q-1',
          },
          {
            'vehicle_class': 'comfort',
            'label': 'Comfort',
            'fare_kobo': 222150,
            'distance_km': 8.051,
            'duration_min': 19,
            'quote_reference': 'q-2',
          },
        ],
      });
    });

    final tiles = await api.estimateFare(
      pickupLat: 6.5244,
      pickupLng: 3.3792,
      dropoffLat: 6.455,
      dropoffLng: 3.4,
    );

    expect(tiles, hasLength(2));
    expect(tiles.first.vehicleClass, 'economy');
    expect(tiles.first.fareKobo, 168500);
    expect(tiles.last.label, 'Comfort');
  });

  test('listVehicleClasses unwraps the classes list', () async {
    final api = buildApi((request) async {
      return ok({
        'classes': [
          {'vehicle_class': 'economy', 'label': 'Economy', 'max_passengers': 3},
          {'vehicle_class': 'xl', 'label': 'XL', 'max_passengers': 5},
        ],
      });
    });

    final classes = await api.listVehicleClasses();
    expect(classes.map((c) => c.vehicleClass), ['economy', 'xl']);
    expect(classes.last.maxPassengers, 5);
  });

  test('getPickupCode returns null before the driver arrives (404)', () async {
    final api = buildApi((request) async {
      return http.Response(
        jsonEncode({
          'success': false,
          'error': {'code': 'not_found', 'message': 'No code yet.'},
        }),
        404,
        headers: {'content-type': 'application/json'},
      );
    });

    final credential = await api.getPickupCode(
      accessToken: 't',
      bookingId: 'b-1',
    );
    expect(credential, isNull);
  });

  test('booking parses stops into pickup and dropoff', () async {
    final api = buildApi((request) async => ok(bookingJson()));
    final booking = await api.getBooking(accessToken: 't', bookingId: 'b-1');

    expect(booking.pickup?.address, 'Yaba');
    expect(booking.dropoff?.address, 'Lekki Phase 1');
    expect(booking.passengerCount, 2);
    expect(booking.displayFareKobo, 222150);
  });

  test('final fare wins over the estimate once settled', () async {
    final api = buildApi((request) async {
      final json = bookingJson(status: 'completed');
      json['fare_final_kobo'] = 229100;
      return ok(json);
    });

    final booking = await api.getBooking(accessToken: 't', bookingId: 'b-1');
    expect(booking.displayFareKobo, 229100);
  });

  group('parcel mode', () {
    test('a parcel create carries ride_purpose and the package keys', () async {
      late http.Request captured;
      final api = buildApi((request) async {
        captured = request;
        return ok(bookingJson());
      });

      await api.createBooking(
        accessToken: 't',
        vehicleClass: 'comfort',
        pickupAddress: 'Yaba',
        pickupLat: 6.5244,
        pickupLng: 3.3792,
        dropoffAddress: 'Lekki Phase 1',
        dropoffLat: 6.455,
        dropoffLng: 3.4,
        mode: TaxiRideMode.parcel,
        receiverName: 'Ada Receiver',
        receiverPhone: '08055512345',
        packageDesc: 'A4 documents',
        packageType: 'documents',
        packageSize: 'small',
        isFragile: true,
      );

      final body = jsonDecode(captured.body) as Map<String, dynamic>;
      expect(body['ride_purpose'], 'parcel');
      expect(body['receiver_name'], 'Ada Receiver');
      expect(body['receiver_phone'], '08055512345');
      expect(body['package_desc'], 'A4 documents');
      expect(body['package_size'], 'small');
      expect(body['is_fragile'], isTrue);
    });

    test('a passenger ride sends NO package keys', () async {
      // The service rejects stray package fields on a passenger ride outright,
      // so sending empty strings would turn every ordinary ride into a 422.
      late http.Request captured;
      final api = buildApi((request) async {
        captured = request;
        return ok(bookingJson());
      });

      await api.createBooking(
        accessToken: 't',
        vehicleClass: 'comfort',
        pickupAddress: 'Yaba',
        pickupLat: 6.5244,
        pickupLng: 3.3792,
        dropoffAddress: 'Lekki Phase 1',
        dropoffLat: 6.455,
        dropoffLng: 3.4,
      );

      final body = jsonDecode(captured.body) as Map<String, dynamic>;
      expect(body['ride_purpose'], 'passenger');
      for (final key in [
        'receiver_name',
        'receiver_phone',
        'package_desc',
        'package_type',
        'package_size',
        'is_fragile',
      ]) {
        expect(body.containsKey(key), isFalse, reason: key);
      }
    });

    test('booking parses ride_purpose and delivery verification', () async {
      final api = buildApi((request) async {
        final json = bookingJson(status: 'in_progress');
        json['ride_purpose'] = 'parcel';
        json['delivery_verification_status'] = 'pending';
        json['receiver_name'] = 'Ada Receiver';
        return ok(json);
      });

      final booking = await api.getBooking(accessToken: 't', bookingId: 'b-1');
      expect(booking.isParcel, isTrue);
      expect(booking.deliveryVerification, DeliveryVerification.pending);
      expect(booking.receiverName, 'Ada Receiver');
    });

    test('an absent ride_purpose reads as a passenger ride', () async {
      // Rows written before parcel mode existed carry no ride_purpose.
      final api = buildApi((request) async => ok(bookingJson()));
      final booking = await api.getBooking(accessToken: 't', bookingId: 'b-1');
      expect(booking.isParcel, isFalse);
      expect(booking.rideMode, TaxiRideMode.ride);
    });

    test('getDeliveryCode returns null on a passenger ride (404)', () async {
      final api = buildApi((request) async {
        return http.Response(
          jsonEncode({
            'success': false,
            'error': {'code': 'not_found', 'message': 'No delivery code.'},
          }),
          404,
          headers: {'content-type': 'application/json'},
        );
      });

      final credential = await api.getDeliveryCode(
        accessToken: 't',
        bookingId: 'b-1',
      );
      expect(credential, isNull);
    });

    test('resendDeliveryCode hits the delivery route, not the pickup one', () async {
      late http.Request captured;
      final api = buildApi((request) async {
        captured = request;
        return ok({'code': '123456', 'expires_at': '2026-09-01T12:00:00Z'});
      });

      final credential = await api.resendDeliveryCode(
        accessToken: 't',
        bookingId: 'b-1',
      );
      expect(captured.method, 'POST');
      expect(
        captured.url.path,
        '/api/v1/taxi/customer/bookings/b-1/delivery-code/resend',
      );
      expect(credential.code, '123456');
    });
  });

  test('reviews parse both sides with withholding honoured', () async {
    final api = buildApi((request) async {
      return ok({
        'mine': {
          'submitted': true,
          'visible': true,
          'rating': 5,
          'review_text': 'Great driver',
          'recommends_driver': true,
        },
        // The service withholds the driver's text until this passenger has
        // written theirs, and signals that with visible=false.
        'theirs': {'submitted': true, 'visible': false},
      });
    });

    final reviews = await api.getReviews(accessToken: 't', bookingId: 'b-1');
    expect(reviews, isNotNull);
    expect(reviews!.iHaveReviewed, isTrue);
    expect(reviews.mine.rating, 5);
    expect(reviews.theirs.submitted, isTrue);
    expect(reviews.theirs.visible, isFalse);
    expect(reviews.theirs.reviewText, isEmpty);
  });
}
