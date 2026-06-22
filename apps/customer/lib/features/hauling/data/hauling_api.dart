import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:http/http.dart' as http;

import '../models/hauling_models.dart';

class HaulingApi {
  HaulingApi({
    required ApiCoreConfig config,
    http.Client? client,
    this.onAuthFailure,
  }) : _config = config,
       _client = client ?? http.Client();

  final ApiCoreConfig _config;
  final http.Client _client;

  /// Called whenever a request fails with an authentication error (401 /
  /// unauthorized). Used to drive a global logout when a session expires or is
  /// revoked while the user is active in the app.
  final void Function()? onAuthFailure;

  // ─── Availability ──────────────────────────────────────────────────────────

  Future<AvailabilityResult> checkAvailability({
    required String accessToken,
  }) async {
    final data = await _sendJson('GET', '/customer/availability', accessToken: accessToken);
    return AvailabilityResult.fromJson(data);
  }

  // ─── Fare estimate ─────────────────────────────────────────────────────────

  Future<FareEstimate> estimateFare({
    required double pickupLat,
    required double pickupLng,
    required double dropoffLat,
    required double dropoffLng,
    required int cargoWeightKg,
    int helperCount = 0,
  }) async {
    final data = await _sendJson('POST', '/customer/bookings/estimate', body: {
      'pickup_lat': pickupLat,
      'pickup_lng': pickupLng,
      'dropoff_lat': dropoffLat,
      'dropoff_lng': dropoffLng,
      'cargo_weight_kg': cargoWeightKg,
      'helper_count': helperCount,
    });
    return FareEstimate.fromJson(data);
  }

  // ─── Provider + truck info (for customer display) ──────────────────────────

  Future<ProviderSnapshot> getProvider({
    required String accessToken,
    required String providerId,
  }) async {
    final data = await _sendJson('GET', '/customer/providers/$providerId', accessToken: accessToken);
    return ProviderSnapshot.fromJson(data);
  }

  Future<TruckSnapshot> getTruck({
    required String accessToken,
    required String truckId,
  }) async {
    final data = await _sendJson('GET', '/customer/trucks/$truckId', accessToken: accessToken);
    return TruckSnapshot.fromJson(data);
  }

  // ─── Bookings ──────────────────────────────────────────────────────────────

  Future<HaulageBooking> createBooking({
    required String accessToken,
    required String pickupAddress,
    required double pickupLat,
    required double pickupLng,
    required String dropoffAddress,
    required double dropoffLat,
    required double dropoffLng,
    required String preferredTruckType,
    required int cargoWeightKg,
    String cargoDescription = '',
    bool requiresHelpers = false,
    int helperCount = 0,
    String weightCategory = '',
    String receiverName = '',
    String receiverPhone = '',
    String packageContent = '',
    String packageSize = '',
    bool isFragile = false,
    DateTime? scheduledAt,
  }) async {
    final body = <String, dynamic>{
      'pickup_address': pickupAddress,
      'pickup_lat': pickupLat,
      'pickup_lng': pickupLng,
      'dropoff_address': dropoffAddress,
      'dropoff_lat': dropoffLat,
      'dropoff_lng': dropoffLng,
      'preferred_truck_type': preferredTruckType,
      'cargo_weight_kg': cargoWeightKg,
      'cargo_description': cargoDescription,
      'requires_helpers': requiresHelpers,
      'helper_count': helperCount,
      'weight_category': weightCategory,
      'receiver_name': receiverName,
      'receiver_phone': receiverPhone,
      'package_content': packageContent,
      'package_size': packageSize,
      'is_fragile': isFragile,
    };
    if (scheduledAt != null) {
      body['scheduled_at'] = scheduledAt.toUtc().toIso8601String();
    }
    final data = await _sendJson('POST', '/customer/bookings', accessToken: accessToken, body: body);
    return HaulageBooking.fromJson(data);
  }

  Future<BookingReview> submitReview({
    required String accessToken,
    required String bookingId,
    required int rating,
    String reviewText = '',
    bool? recommendsDriver,
  }) async {
    final body = <String, dynamic>{
      'rating': rating,
      'review_text': reviewText,
    };
    if (recommendsDriver != null) {
      body['recommends_driver'] = recommendsDriver;
    }
    final data = await _sendJson(
      'POST',
      '/customer/bookings/$bookingId/review',
      accessToken: accessToken,
      body: body,
    );
    return BookingReview.fromJson(data);
  }

  Future<HaulageBooking> getBooking({
    required String accessToken,
    required String bookingId,
  }) async {
    final data = await _sendJson(
      'GET',
      '/customer/bookings/$bookingId',
      accessToken: accessToken,
    );
    return HaulageBooking.fromJson(data);
  }

  Future<List<HaulageBooking>> listBookings({
    required String accessToken,
    int limit = 20,
    int offset = 0,
  }) async {
    final raw = await _sendJsonList(
      'GET',
      '/customer/bookings?limit=$limit&offset=$offset',
      accessToken: accessToken,
    );
    return raw.map((e) => HaulageBooking.fromJson(Map<String, dynamic>.from(e as Map))).toList();
  }

  Future<HaulageBooking> cancelBooking({
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
    return HaulageBooking.fromJson(data);
  }

  void close() => _client.close();

  // ─── HTTP helpers ──────────────────────────────────────────────────────────

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
        'POST' => await _client.post(uri, headers: headers, body: jsonEncode(body ?? const {})),
        'PUT' => await _client.put(uri, headers: headers, body: jsonEncode(body ?? const {})),
        'DELETE' => await _client.delete(uri, headers: headers),
        _ => throw UnsupportedError('Unsupported HTTP method: $method'),
      };

      final decoded = _decodeResponse(response);
      if (response.statusCode < 200 || response.statusCode >= 300) {
        throw ApiException.fromErrorEnvelope(decoded, statusCode: response.statusCode);
      }
      if (decoded['success'] != true) {
        throw ApiException.fromErrorEnvelope(decoded, statusCode: response.statusCode);
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

  Future<List<dynamic>> _sendJsonList(
    String method,
    String path, {
    String? accessToken,
  }) async {
    try {
      final uri = _config.uri(path);
      final headers = {
        'Content-Type': 'application/json',
        'Accept': 'application/json',
        if (accessToken != null) 'Authorization': 'Bearer $accessToken',
      };

      final response = await _client.get(uri, headers: headers);
      final decoded = _decodeResponse(response);
      if (response.statusCode < 200 || response.statusCode >= 300) {
        throw ApiException.fromErrorEnvelope(decoded, statusCode: response.statusCode);
      }
      final rawData = decoded['data'];
      if (rawData is List) return rawData;
      return const [];
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
    return const {'success': false, 'error': {'code': 'unknown', 'message': 'Unexpected response.'}};
  }
}
