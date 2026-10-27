import 'dart:convert';
import 'dart:math';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:http/http.dart' as http;

import '../models/taxi_models.dart';

/// HTTP client for the driver-taxi-service passenger API
/// (base: /api/v1/taxi).
///
/// Route shapes differ from the dispatch/hauling clients in two ways worth
/// noting: cancel is a PUT (not POST), and booking creation REQUIRES an
/// `Idempotency-Key` header — the service rejects the request without one, and
/// replaying the same key with a different body is a 409 rather than a second
/// ride.
class TaxiApi {
  TaxiApi({
    required ApiCoreConfig config,
    http.Client? client,
    this.onAuthFailure,
  }) : _config = config,
       _client = client ?? http.Client();

  final ApiCoreConfig _config;
  final http.Client _client;

  /// Called whenever a request fails with an authentication error, so the app
  /// can drive a global logout when a session expires.
  final void Function()? onAuthFailure;

  static final Random _random = Random();

  /// A per-request idempotency key. The app has no uuid dependency and this
  /// value only needs to be unique per customer per attempt (the service
  /// scopes the key to the authenticated customer), so time plus randomness
  /// is sufficient without pulling in a package.
  static String newIdempotencyKey() {
    final now = DateTime.now().microsecondsSinceEpoch.toRadixString(36);
    final salt = _random.nextInt(1 << 32).toRadixString(36);
    return 'cust-$now-$salt';
  }

  // ─── Public (no auth) ──────────────────────────────────────────────────────

  /// Bookable classes. Public so the fare screen can render before login.
  Future<List<TaxiVehicleClass>> listVehicleClasses() async {
    final data = await _sendJson('GET', '/vehicle-classes');
    final raw = data['classes'];
    if (raw is! List) return const [];
    return raw
        .map((e) => TaxiVehicleClass.fromJson(Map<String, dynamic>.from(e as Map)))
        .toList();
  }

  /// One quote per class the pickup's zone supports, cheapest first.
  Future<List<TaxiFareTile>> estimateFare({
    required double pickupLat,
    required double pickupLng,
    required double dropoffLat,
    required double dropoffLng,
  }) async {
    final data = await _sendJson(
      'POST',
      '/customer/bookings/estimate',
      body: {
        'pickup_lat': pickupLat,
        'pickup_lng': pickupLng,
        'dropoff_lat': dropoffLat,
        'dropoff_lng': dropoffLng,
      },
    );
    final raw = data['fares'];
    if (raw is! List) return const [];
    return raw
        .map((e) => TaxiFareTile.fromJson(Map<String, dynamic>.from(e as Map)))
        .toList();
  }

  // ─── Availability ──────────────────────────────────────────────────────────

  Future<TaxiAvailability> checkAvailability({
    required String accessToken,
    required double lat,
    required double lng,
  }) async {
    final data = await _sendJson(
      'GET',
      '/customer/availability?lat=$lat&lng=$lng',
      accessToken: accessToken,
    );
    return TaxiAvailability.fromJson(data);
  }

  // ─── Bookings ──────────────────────────────────────────────────────────────

  Future<TaxiBooking> createBooking({
    required String accessToken,
    required String vehicleClass,
    required String pickupAddress,
    required double pickupLat,
    required double pickupLng,
    required String dropoffAddress,
    required double dropoffLat,
    required double dropoffLng,
    String rideNotes = '',
    TaxiRideMode mode = TaxiRideMode.ride,
    String receiverName = '',
    String receiverPhone = '',
    String packageDesc = '',
    String packageType = '',
    String packageSize = '',
    bool isFragile = false,
    String? idempotencyKey,
  }) async {
    final data = await _sendJson(
      'POST',
      '/customer/bookings',
      accessToken: accessToken,
      idempotencyKey: idempotencyKey ?? newIdempotencyKey(),
      body: {
        'vehicle_class': vehicleClass,
        'booking_mode': 'standard',
        'ride_notes': rideNotes,
        // Wallet is the only method the service accepts. passenger_count is
        // deliberately not sent: the app never asks, and CreateBooking
        // defaults an absent/zero value to 1.
        'payment_method': 'wallet',
        'ride_purpose': mode.wireValue,
        // Package keys are sent ONLY for a parcel: the service rejects any of
        // them on a passenger ride rather than quietly ignoring them.
        if (mode.isParcel) ...{
          'receiver_name': receiverName,
          'receiver_phone': receiverPhone,
          'package_desc': packageDesc,
          'package_type': packageType,
          'package_size': packageSize,
          'is_fragile': isFragile,
        },
        'stops': [
          {'address': pickupAddress, 'lat': pickupLat, 'lng': pickupLng},
          {'address': dropoffAddress, 'lat': dropoffLat, 'lng': dropoffLng},
        ],
      },
    );
    return TaxiBooking.fromJson(data);
  }

  Future<TaxiBooking> getBooking({
    required String accessToken,
    required String bookingId,
  }) async {
    final data = await _sendJson(
      'GET',
      '/customer/bookings/$bookingId',
      accessToken: accessToken,
    );
    return TaxiBooking.fromJson(data);
  }

  /// When the driver should reach the pickup and the drop-off. Both are null
  /// when tracking is off or no estimate is possible right now.
  Future<(DateTime?, DateTime?)> getEtas({
    required String accessToken,
    required String bookingId,
  }) async {
    final data = await _sendJson(
      'GET',
      '/customer/bookings/$bookingId/location',
      accessToken: accessToken,
    );
    final seconds = data['eta_seconds'];
    DateTime? at(String key) {
      final secs = seconds is Map ? (seconds[key] as num?)?.toInt() : null;
      return secs == null ? null : DateTime.now().add(Duration(seconds: secs));
    }

    return (at('pickup'), at('dropoff'));
  }

  Future<List<TaxiBooking>> listBookings({required String accessToken}) async {
    final data = await _sendJson(
      'GET',
      '/customer/bookings',
      accessToken: accessToken,
    );
    final raw = data['bookings'];
    if (raw is! List) return const [];
    return raw
        .map((e) => TaxiBooking.fromJson(Map<String, dynamic>.from(e as Map)))
        .toList();
  }

  /// Cancels a ride. Note this is a PUT, unlike the dispatch equivalent.
  Future<TaxiBooking> cancelBooking({
    required String accessToken,
    required String bookingId,
    String reason = '',
  }) async {
    final data = await _sendJson(
      'PUT',
      '/customer/bookings/$bookingId/cancel',
      accessToken: accessToken,
      body: {'reason': reason},
    );
    return TaxiBooking.fromJson(data);
  }

  // ─── Pickup verification (passenger displays, driver verifies) ─────────────

  /// The passenger's current pickup code. Returns null before the driver has
  /// arrived, since no code exists yet and the service answers 404.
  Future<TaxiPickupCredential?> getPickupCode({
    required String accessToken,
    required String bookingId,
  }) async {
    try {
      final data = await _sendJson(
        'GET',
        '/customer/bookings/$bookingId/pickup-code',
        accessToken: accessToken,
      );
      return TaxiPickupCredential.fromJson(data);
    } on ApiException catch (e) {
      if (e.statusCode == 404) return null;
      rethrow;
    }
  }

  /// Regenerates the pickup code. The previous value stops verifying.
  Future<TaxiPickupCredential> resendPickupCode({
    required String accessToken,
    required String bookingId,
  }) async {
    final data = await _sendJson(
      'POST',
      '/customer/bookings/$bookingId/pickup-code/resend',
      accessToken: accessToken,
    );
    return TaxiPickupCredential.fromJson(data);
  }

  // ─── Delivery verification (parcel only: receiver shows, driver verifies) ──

  /// The receiver's drop-off code. Returns null when the service has none for
  /// this booking yet or the ride is not a parcel (404 either way).
  Future<TaxiHandoverCredential?> getDeliveryCode({
    required String accessToken,
    required String bookingId,
  }) async {
    try {
      final data = await _sendJson(
        'GET',
        '/customer/bookings/$bookingId/delivery-code',
        accessToken: accessToken,
      );
      return TaxiHandoverCredential.fromJson(data);
    } on ApiException catch (e) {
      if (e.statusCode == 404) return null;
      rethrow;
    }
  }

  Future<TaxiHandoverCredential> resendDeliveryCode({
    required String accessToken,
    required String bookingId,
  }) async {
    final data = await _sendJson(
      'POST',
      '/customer/bookings/$bookingId/delivery-code/resend',
      accessToken: accessToken,
    );
    return TaxiHandoverCredential.fromJson(data);
  }

  // ─── Reviews ───────────────────────────────────────────────────────────────

  Future<TaxiBookingReviews> submitReview({
    required String accessToken,
    required String bookingId,
    required int rating,
    String reviewText = '',
    List<String> tags = const [],
    bool? recommendsDriver,
  }) async {
    final body = <String, dynamic>{
      'rating': rating,
      'review_text': reviewText,
      if (tags.isNotEmpty) 'tags': tags,
      'recommends_driver': ?recommendsDriver,
    };
    final data = await _sendJson(
      'POST',
      '/customer/bookings/$bookingId/review',
      accessToken: accessToken,
      body: body,
    );
    return TaxiBookingReviews.fromJson(data);
  }

  /// Both sides of the two-way review. Returns null when the service has no
  /// review record for the booking at all.
  Future<TaxiBookingReviews?> getReviews({
    required String accessToken,
    required String bookingId,
  }) async {
    try {
      final data = await _sendJson(
        'GET',
        '/customer/bookings/$bookingId/review',
        accessToken: accessToken,
      );
      return TaxiBookingReviews.fromJson(data);
    } on ApiException catch (e) {
      if (e.statusCode == 404) return null;
      rethrow;
    }
  }

  void close() => _client.close();

  // ─── HTTP helpers ──────────────────────────────────────────────────────────

  Future<Map<String, dynamic>> _sendJson(
    String method,
    String path, {
    Map<String, dynamic>? body,
    String? accessToken,
    String? idempotencyKey,
  }) async {
    try {
      final uri = _config.uri(path);
      final headers = {
        'Content-Type': 'application/json',
        'Accept': 'application/json',
        if (accessToken != null) 'Authorization': 'Bearer $accessToken',
        'Idempotency-Key': ?idempotencyKey,
      };

      final response = switch (method) {
        'GET' => await _client.get(uri, headers: headers),
        'POST' => await _client.post(
          uri,
          headers: headers,
          body: jsonEncode(body ?? const {}),
        ),
        'PUT' => await _client.put(
          uri,
          headers: headers,
          body: jsonEncode(body ?? const {}),
        ),
        'DELETE' => await _client.delete(uri, headers: headers),
        _ => throw UnsupportedError('Unsupported HTTP method: $method'),
      };

      final decoded = _decodeResponse(response);
      if (response.statusCode < 200 ||
          response.statusCode >= 300 ||
          decoded['success'] != true) {
        throw ApiException.fromErrorEnvelope(
          decoded,
          statusCode: response.statusCode,
        );
      }

      final rawData = decoded['data'];
      if (rawData is Map) return Map<String, dynamic>.from(rawData);
      return const {};
    } on ApiException catch (error) {
      if (error.isAuthFailure) onAuthFailure?.call();
      rethrow;
    } catch (error) {
      throw ApiException.network(error);
    }
  }

  Map<String, dynamic> _decodeResponse(http.Response response) {
    if (response.body.isEmpty) return const {'success': true, 'data': {}};
    final decoded = jsonDecode(response.body);
    if (decoded is Map) return Map<String, dynamic>.from(decoded);
    return const {'success': false};
  }
}
