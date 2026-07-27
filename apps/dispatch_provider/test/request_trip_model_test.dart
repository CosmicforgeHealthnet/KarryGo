import 'package:flutter_test/flutter_test.dart';

import 'package:dispatch_provider/features/requests/data/request_model.dart';
import 'package:dispatch_provider/features/trips/data/trip_model.dart';

void main() {
  group('RequestModel.fromJson (real backend inbox shape)', () {
    // Mirrors ProviderRequestInboxItem from the dispatch backend:
    // GET /api/v1/provider/requests -> items keyed by `inbox_id` / `fare_amount`
    // / `received_at` (NOT `id` / `estimated_fare_kobo` / `created_at`).
    final backendItem = <String, dynamic>{
      'inbox_id': '11111111-1111-1111-1111-111111111111',
      'broadcast_id': '22222222-2222-2222-2222-222222222222',
      'booking_id': '33333333-3333-3333-3333-333333333333',
      'fare_amount': 150000,
      'currency': 'NGN',
      'pickup_address': 'Surulere',
      'dropoff_address': 'Victoria Island',
      'distance_km': 8.4,
      'receiver_name': 'Ada',
      'service_tier': 'standard',
      'remaining_seconds': 27,
      'received_at': '2026-07-21T10:00:00Z',
    };

    test('id is populated from inbox_id (not empty)', () {
      final model = RequestModel.fromJson(backendItem);
      expect(model.id, '11111111-1111-1111-1111-111111111111');
      expect(model.id, isNotEmpty);
    });

    test('fare is read from fare_amount', () {
      final model = RequestModel.fromJson(backendItem);
      expect(model.estimatedFareKobo, 150000);
    });

    test('created_at is read from received_at', () {
      final model = RequestModel.fromJson(backendItem);
      expect(model.createdAt, DateTime.parse('2026-07-21T10:00:00Z'));
    });

    test('legacy id/estimated_fare_kobo keys still work as a fallback', () {
      final model = RequestModel.fromJson({
        'id': '44444444-4444-4444-4444-444444444444',
        'booking_id': '33333333-3333-3333-3333-333333333333',
        'estimated_fare_kobo': 99000,
      });
      expect(model.id, '44444444-4444-4444-4444-444444444444');
      expect(model.estimatedFareKobo, 99000);
    });
  });

  group('TripModel.fromJson (real backend trip shape)', () {
    test('fare is read from fare_amount', () {
      final model = TripModel.fromJson(<String, dynamic>{
        'id': '55555555-5555-5555-5555-555555555555',
        'booking_id': '33333333-3333-3333-3333-333333333333',
        'status': 'assigned',
        'pickup_address': 'Surulere',
        'dropoff_address': 'Victoria Island',
        'distance_km': 8.4,
        'fare_amount': 150000,
        'receiver_name': 'Ada',
        'receiver_phone': '+2348000000000',
        'created_at': '2026-07-21T10:00:00Z',
      });
      expect(model.id, isNotEmpty);
      expect(model.estimatedFareKobo, 150000);
      expect(model.statusCode, TripStatusCode.assigned);
    });
  });
}
