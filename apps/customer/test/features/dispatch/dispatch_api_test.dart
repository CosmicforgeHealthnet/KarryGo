import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:customer/features/dispatch/data/dispatch_api.dart';
import 'package:customer/features/dispatch/models/dispatch_models.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

void main() {
  DispatchApi buildApi(Future<http.Response> Function(http.Request) handler) {
    return DispatchApi(
      config: ApiCoreConfig(
        baseUrl: 'http://localhost:8103/api/v1/customer/dispatch',
      ),
      client: MockClient(handler),
    );
  }

  Map<String, dynamic> bookingJson({String status = 'searching'}) => {
        'id': 'b-1',
        'customer_id': 'c-1',
        'provider_id': '',
        'pickup_address': 'Surulere',
        'pickup_lat': 6.5,
        'pickup_lng': 3.35,
        'dropoff_address': 'Ikeja',
        'dropoff_lat': 6.6,
        'dropoff_lng': 3.35,
        'distance_km': 11.2,
        'fare_amount_kobo': 281600,
        'currency': 'NGN',
        'payment_method': 'wallet',
        'payment_status': 'held',
        'receiver_name': 'Ada',
        'receiver_phone': '+2348011111111',
        'package_desc': 'Documents',
        'package_size': 'small',
        'is_fragile': false,
        'service_tier': 'standard',
        'status': status,
        'created_at': '2026-07-09T10:00:00Z',
      };

  test('status mapping covers every backend wire status', () {
    expect(DispatchBookingStatus.fromString('searching'),
        DispatchBookingStatus.searching);
    expect(DispatchBookingStatus.fromString('accepted'),
        DispatchBookingStatus.accepted);
    expect(DispatchBookingStatus.fromString('en_route_pickup'),
        DispatchBookingStatus.enRoutePickup);
    expect(DispatchBookingStatus.fromString('arrived_at_pickup'),
        DispatchBookingStatus.arrivedAtPickup);
    expect(DispatchBookingStatus.fromString('in_transit'),
        DispatchBookingStatus.inTransit);
    expect(DispatchBookingStatus.fromString('delivered'),
        DispatchBookingStatus.delivered);
    expect(DispatchBookingStatus.fromString('completed'),
        DispatchBookingStatus.completed);
    expect(DispatchBookingStatus.fromString('cancelled'),
        DispatchBookingStatus.cancelled);
    expect(DispatchBookingStatus.fromString('unmatched'),
        DispatchBookingStatus.unmatched);

    expect(DispatchBookingStatus.searching.isSearching, isTrue);
    expect(DispatchBookingStatus.inTransit.isActive, isTrue);
    expect(DispatchBookingStatus.delivered.isActive, isFalse);
    expect(DispatchBookingStatus.unmatched.isTerminal, isTrue);
  });

  test('createBooking sends the documented contract body', () async {
    late Map<String, dynamic> sentBody;
    final api = buildApi((request) async {
      expect(request.url.path, '/api/v1/customer/dispatch/bookings');
      expect(request.method, 'POST');
      sentBody = jsonDecode(request.body) as Map<String, dynamic>;
      return http.Response(
        jsonEncode({'success': true, 'data': bookingJson()}),
        201,
        headers: {'content-type': 'application/json'},
      );
    });

    final booking = await api.createBooking(
      accessToken: 'token',
      pickupAddress: 'Surulere',
      pickupLat: 6.5,
      pickupLng: 3.35,
      dropoffAddress: 'Ikeja',
      dropoffLat: 6.6,
      dropoffLng: 3.35,
      receiverName: 'Ada',
      receiverPhone: '+2348011111111',
      packageDesc: 'Documents',
      packageSize: 'small',
      serviceTier: 'express',
      isFragile: true,
    );

    expect(sentBody['pickup'], {
      'lat': 6.5,
      'lng': 3.35,
      'address': 'Surulere',
    });
    expect(sentBody['dropoff']['address'], 'Ikeja');
    expect(sentBody['receiver_name'], 'Ada');
    expect(sentBody['package_size'], 'small');
    expect(sentBody['service_tier'], 'express');
    expect(sentBody['is_fragile'], true);
    expect(sentBody['payment_method'], 'wallet');
    expect(booking.id, 'b-1');
    expect(booking.status, DispatchBookingStatus.searching);
    expect(booking.providerId, isNull); // empty string normalises to null
    expect(booking.fareAmountKobo, 281600);
  });

  test('listBookings unwraps the {bookings: []} envelope', () async {
    final api = buildApi((request) async {
      expect(request.url.path, '/api/v1/customer/dispatch/bookings');
      return http.Response(
        jsonEncode({
          'success': true,
          'data': {
            'bookings': [
              bookingJson(status: 'completed'),
              bookingJson(status: 'cancelled'),
            ],
          },
        }),
        200,
        headers: {'content-type': 'application/json'},
      );
    });

    final bookings = await api.listBookings(accessToken: 'token');
    expect(bookings, hasLength(2));
    expect(bookings.first.status, DispatchBookingStatus.completed);
    expect(bookings.last.status, DispatchBookingStatus.cancelled);
  });

  test('getBookingLocation maps 404 to an unavailable location', () async {
    final api = buildApi((request) async {
      return http.Response(
        jsonEncode({
          'success': false,
          'error': {
            'code': 'not_found',
            'message': 'No rider has been assigned yet.',
          },
        }),
        404,
        headers: {'content-type': 'application/json'},
      );
    });

    final location =
        await api.getBookingLocation(accessToken: 'token', bookingId: 'b-1');
    expect(location.available, isFalse);
    expect(location.hasFix, isFalse);
  });

  test('getReviewForBooking returns null on 404', () async {
    final api = buildApi((request) async {
      return http.Response(
        jsonEncode({
          'success': false,
          'error': {'code': 'not_found', 'message': 'No review found.'},
        }),
        404,
        headers: {'content-type': 'application/json'},
      );
    });

    final review =
        await api.getReviewForBooking(accessToken: 'token', bookingId: 'b-1');
    expect(review, isNull);
  });

  test('rider public profile maps onto ProviderSnapshot', () {
    final snapshot = riderSnapshotFromPublicProfile({
      'provider_id': 'p-1',
      'full_name': 'Chidi Eze Okafor',
      'profile_photo_url': 'https://example.test/rider.jpg',
      'avg_rating': 4.7,
      'total_trips': 210,
    });
    expect(snapshot.id, 'p-1');
    expect(snapshot.firstName, 'Chidi');
    expect(snapshot.lastName, 'Eze Okafor');
    expect(snapshot.displayName, 'Chidi Eze Okafor');
    expect(snapshot.rating, 4.7);
    expect(snapshot.totalTrips, 210);
  });
}
