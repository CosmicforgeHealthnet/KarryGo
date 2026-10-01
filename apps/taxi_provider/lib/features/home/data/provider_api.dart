import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:http/http.dart' as http;

import '../models/provider_booking.dart';

/// Availability, offers and the trip lifecycle — everything the driver does
/// once signed in.
class ProviderApi {
  ProviderApi({
    required ApiCoreConfig config,
    http.Client? client,
    this.onAuthFailure,
  }) : _config = config,
       _client = client ?? http.Client();

  final ApiCoreConfig _config;
  final http.Client _client;
  final void Function()? onAuthFailure;

  // ─── Availability ──────────────────────────────────────────────────────────

  Future<AvailabilityState> getAvailability({
    required String accessToken,
  }) async {
    final data = await _sendJson(
      'GET',
      '/provider/availability',
      accessToken: accessToken,
    );
    return AvailabilityState.fromJson(data);
  }

  /// Going online is gated server-side on compliance and on having an active
  /// vehicle; a refusal comes back as a plain message to show the driver.
  Future<AvailabilityState> setAvailability({
    required String accessToken,
    required bool online,
    required double lat,
    required double lng,
    String vehicleId = '',
    double heading = 0,
    double speed = 0,
    double accuracy = 0,
  }) async {
    final data = await _sendJson(
      'PUT',
      '/provider/availability',
      accessToken: accessToken,
      body: {
        'status': online ? 'online' : 'offline',
        if (vehicleId.isNotEmpty) 'vehicle_id': vehicleId,
        'lat': lat,
        'lng': lng,
        'heading': heading,
        'speed': speed,
        'accuracy': accuracy,
      },
    );
    return AvailabilityState.fromJson(data);
  }

  Future<AvailabilityState> heartbeat({
    required String accessToken,
    required double lat,
    required double lng,
    String vehicleId = '',
    double heading = 0,
    double speed = 0,
    double accuracy = 0,
    bool onTrip = false,
  }) async {
    final data = await _sendJson(
      'POST',
      '/provider/availability/heartbeat',
      accessToken: accessToken,
      body: {
        if (vehicleId.isNotEmpty) 'vehicle_id': vehicleId,
        'lat': lat,
        'lng': lng,
        'heading': heading,
        'speed': speed,
        'accuracy': accuracy,
        'on_trip': onTrip,
      },
    );
    return AvailabilityState.fromJson(data);
  }

  // ─── Bookings ──────────────────────────────────────────────────────────────

  /// The driver's current offer and/or committed ride. The service returns
  /// only non-terminal bookings assigned to them.
  Future<List<ProviderBooking>> listActiveBookings({
    required String accessToken,
  }) async {
    final data = await _sendJson(
      'GET',
      '/provider/bookings',
      accessToken: accessToken,
    );
    final raw = data['bookings'];
    if (raw is! List) return const [];
    return raw
        .map((e) => ProviderBooking.fromJson(Map<String, dynamic>.from(e as Map)))
        .toList();
  }

  Future<ProviderBooking> getBooking({
    required String accessToken,
    required String bookingId,
  }) async {
    final data = await _sendJson(
      'GET',
      '/provider/bookings/$bookingId',
      accessToken: accessToken,
    );
    return ProviderBooking.fromJson(data);
  }

  // ─── Lifecycle ─────────────────────────────────────────────────────────────

  Future<ProviderBooking> accept({
    required String accessToken,
    required String bookingId,
  }) => _transition(accessToken, bookingId, 'accept');

  Future<ProviderBooking> reject({
    required String accessToken,
    required String bookingId,
  }) => _transition(accessToken, bookingId, 'reject');

  Future<ProviderBooking> markEnRoutePickup({
    required String accessToken,
    required String bookingId,
  }) => _transition(accessToken, bookingId, 'en-route-pickup');

  Future<ProviderBooking> markArrived({
    required String accessToken,
    required String bookingId,
  }) => _transition(accessToken, bookingId, 'arrived');

  Future<ProviderBooking> startTrip({
    required String accessToken,
    required String bookingId,
  }) => _transition(accessToken, bookingId, 'start');

  Future<ProviderBooking> cancel({
    required String accessToken,
    required String bookingId,
    String reason = '',
  }) => _transition(accessToken, bookingId, 'cancel', body: {'reason': reason});

  Future<ProviderBooking> completeTrip({
    required String accessToken,
    required String bookingId,
    required double actualDistanceKm,
    required int actualDurationMin,
  }) => _transition(accessToken, bookingId, 'complete', body: {
    'actual_distance_km': actualDistanceKm,
    'actual_duration_min': actualDurationMin,
  });

  Future<ProviderBooking> _transition(
    String accessToken,
    String bookingId,
    String action, {
    Map<String, dynamic>? body,
  }) async {
    final data = await _sendJson(
      'PUT',
      '/provider/bookings/$bookingId/$action',
      accessToken: accessToken,
      body: body,
    );
    return ProviderBooking.fromJson(data);
  }

  // ─── Handover verification ─────────────────────────────────────────────────

  /// Submits the code the passenger (or parcel sender) displayed. Success
  /// unlocks starting the trip.
  Future<ProviderBooking> verifyPickup({
    required String accessToken,
    required String bookingId,
    required String code,
  }) async {
    final data = await _sendJson(
      'POST',
      '/provider/bookings/$bookingId/verify-pickup',
      accessToken: accessToken,
      body: {'code': code},
    );
    return ProviderBooking.fromJson(data);
  }

  /// Parcel only: the receiver's code at drop-off. Until this succeeds the
  /// service refuses to complete the trip.
  Future<ProviderBooking> verifyDelivery({
    required String accessToken,
    required String bookingId,
    required String code,
  }) async {
    final data = await _sendJson(
      'POST',
      '/provider/bookings/$bookingId/verify-delivery',
      accessToken: accessToken,
      body: {'code': code},
    );
    return ProviderBooking.fromJson(data);
  }

  // ─── Telemetry ─────────────────────────────────────────────────────────────

  /// Batched GPS upload for an active trip. Best-effort: a rejected point
  /// never fails the batch, so this returns nothing to act on.
  Future<void> sendLocationPoints({
    required String accessToken,
    required List<Map<String, dynamic>> points,
  }) async {
    if (points.isEmpty) return;
    await _sendJson(
      'POST',
      '/provider/location/events',
      accessToken: accessToken,
      body: {'points': points},
    );
  }

  // ─── Earnings ──────────────────────────────────────────────────────────────

  Future<Map<String, dynamic>> getEarningsInsights({
    required String accessToken,
    DateTime? from,
    DateTime? until,
  }) async {
    final query = <String>[];
    if (from != null) query.add('from=${from.toUtc().toIso8601String()}');
    if (until != null) query.add('until=${until.toUtc().toIso8601String()}');
    final suffix = query.isEmpty ? '' : '?${query.join('&')}';
    return _sendJson(
      'GET',
      '/provider/earnings/insights$suffix',
      accessToken: accessToken,
    );
  }

  void close() => _client.close();

  Future<Map<String, dynamic>> _sendJson(
    String method,
    String path, {
    Map<String, dynamic>? body,
    String? accessToken,
  }) async {
    try {
      final uri = _config.uri(path);
      final headers = {
        'Content-Type': 'application/json',
        'Accept': 'application/json',
        if (accessToken != null) 'Authorization': 'Bearer $accessToken',
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
        _ => throw UnsupportedError('Unsupported HTTP method: $method'),
      };

      final decoded = _decode(response);
      if (response.statusCode < 200 ||
          response.statusCode >= 300 ||
          decoded['success'] != true) {
        throw ApiException.fromErrorEnvelope(
          decoded,
          statusCode: response.statusCode,
        );
      }
      final raw = decoded['data'];
      if (raw is Map) return Map<String, dynamic>.from(raw);
      return const {};
    } on ApiException catch (error) {
      if (error.isAuthFailure) onAuthFailure?.call();
      rethrow;
    } catch (error) {
      throw ApiException.network(error);
    }
  }

  Map<String, dynamic> _decode(http.Response response) {
    if (response.body.isEmpty) return const {'success': true, 'data': {}};
    final decoded = jsonDecode(response.body);
    if (decoded is Map) return Map<String, dynamic>.from(decoded);
    return const {'success': false};
  }
}
