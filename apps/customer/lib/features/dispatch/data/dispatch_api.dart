import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:http/http.dart' as http;

import '../models/dispatch_models.dart';

/// HTTP client for the dispatch-delivery-service customer API
/// (base: /api/v1/customer/dispatch).
class DispatchApi {
  DispatchApi({
    required ApiCoreConfig config,
    http.Client? client,
    this.onAuthFailure,
  }) : _config = config,
       _client = client ?? http.Client();

  final ApiCoreConfig _config;
  final http.Client _client;

  /// Called whenever a request fails with an authentication error (401 /
  /// unauthorized). Used to drive a global logout when a session expires.
  final void Function()? onAuthFailure;

  // ─── Availability ──────────────────────────────────────────────────────────

  Future<DispatchAvailabilityResult> checkAvailability({
    required String accessToken,
    required double lat,
    required double lng,
  }) async {
    final data = await _sendJson(
      'GET',
      '/availability?lat=$lat&lng=$lng',
      accessToken: accessToken,
    );
    return DispatchAvailabilityResult.fromJson(data);
  }

  // ─── Fare estimate ─────────────────────────────────────────────────────────

  Future<DispatchFareEstimate> estimateFare({
    required double pickupLat,
    required double pickupLng,
    required double dropoffLat,
    required double dropoffLng,
    String packageSize = 'small',
    String serviceTier = 'standard',
  }) async {
    final data = await _sendJson(
      'POST',
      '/bookings/estimate',
      body: {
        'pickup_lat': pickupLat,
        'pickup_lng': pickupLng,
        'dropoff_lat': dropoffLat,
        'dropoff_lng': dropoffLng,
        'package_size': packageSize,
        'service_tier': serviceTier,
      },
    );
    return DispatchFareEstimate.fromJson(data);
  }

  // ─── Rider info (for customer display) ─────────────────────────────────────

  Future<ProviderSnapshot> getProvider({
    required String accessToken,
    required String providerId,
  }) async {
    final data = await _sendJson(
      'GET',
      '/providers/$providerId',
      accessToken: accessToken,
    );
    return riderSnapshotFromPublicProfile(data);
  }

  // ─── Bookings ──────────────────────────────────────────────────────────────

  Future<DispatchBooking> createBooking({
    required String accessToken,
    required String pickupAddress,
    required double pickupLat,
    required double pickupLng,
    required String dropoffAddress,
    required double dropoffLat,
    required double dropoffLng,
    required String receiverName,
    required String receiverPhone,
    required String packageDesc,
    double packageWeight = 0,
    String packageType = '',
    String packageSize = 'small',
    bool isFragile = false,
    String serviceTier = 'standard',
    String paymentMethod = 'wallet',
  }) async {
    final data = await _sendJson(
      'POST',
      '/bookings',
      accessToken: accessToken,
      body: {
        'pickup': {
          'lat': pickupLat,
          'lng': pickupLng,
          'address': pickupAddress,
        },
        'dropoff': {
          'lat': dropoffLat,
          'lng': dropoffLng,
          'address': dropoffAddress,
        },
        'receiver_name': receiverName,
        'receiver_phone': receiverPhone,
        'package_desc': packageDesc,
        'package_weight': packageWeight,
        'package_type': packageType,
        'package_size': packageSize,
        'is_fragile': isFragile,
        'service_tier': serviceTier,
        'payment_method': paymentMethod,
      },
    );
    return DispatchBooking.fromJson(data);
  }

  Future<DispatchBookingReview> submitReview({
    required String accessToken,
    required String bookingId,
    required int rating,
    String reviewText = '',
    bool? recommendsRider,
  }) async {
    final body = <String, dynamic>{'rating': rating, 'review_text': reviewText};
    if (recommendsRider != null) {
      body['recommends_provider'] = recommendsRider;
    }
    final data = await _sendJson(
      'POST',
      '/bookings/$bookingId/review',
      accessToken: accessToken,
      body: body,
    );
    return DispatchBookingReview.fromJson(data);
  }

  /// The customer's own review for a booking, or null when they haven't
  /// reviewed it yet (backend returns 404 in that case).
  Future<DispatchBookingReview?> getReviewForBooking({
    required String accessToken,
    required String bookingId,
  }) async {
    try {
      final data = await _sendJson(
        'GET',
        '/bookings/$bookingId/review',
        accessToken: accessToken,
      );
      return DispatchBookingReview.fromJson(data);
    } on ApiException catch (e) {
      if (e.statusCode == 404) return null;
      rethrow;
    }
  }

  Future<DispatchBooking> getBooking({
    required String accessToken,
    required String bookingId,
  }) async {
    final data = await _sendJson(
      'GET',
      '/bookings/$bookingId',
      accessToken: accessToken,
    );
    return DispatchBooking.fromJson(data);
  }

  Future<List<DispatchBooking>> listBookings({
    required String accessToken,
    int limit = 20,
    int offset = 0,
  }) async {
    final data = await _sendJson(
      'GET',
      '/bookings?limit=$limit&offset=$offset',
      accessToken: accessToken,
    );
    final raw = data['bookings'];
    if (raw is! List) return const [];
    return raw
        .map(
          (e) => DispatchBooking.fromJson(Map<String, dynamic>.from(e as Map)),
        )
        .toList();
  }

  /// Live location of the assigned rider for a booking (map marker). Returns
  /// an unavailable location instead of throwing when no rider is assigned or
  /// no recent GPS fix exists (backend 404).
  Future<ProviderLocation> getBookingLocation({
    required String accessToken,
    required String bookingId,
  }) async {
    try {
      final data = await _sendJson(
        'GET',
        '/bookings/$bookingId/location',
        accessToken: accessToken,
      );
      final (pickupEta, dropoffEta) = ProviderLocation.etasFromJson(data);
      return ProviderLocation(
        lat: (data['lat'] as num?)?.toDouble() ?? 0,
        lng: (data['lng'] as num?)?.toDouble() ?? 0,
        available: true,
        pickupEta: pickupEta,
        dropoffEta: dropoffEta,
      );
    } on ApiException catch (e) {
      if (e.statusCode == 404) {
        return const ProviderLocation(lat: 0, lng: 0, available: false);
      }
      rethrow;
    }
  }

  /// The websocket URL that streams the assigned rider's live position.
  ///
  /// The token travels in the query string because a websocket handshake
  /// cannot carry an Authorization header, which is why the service validates
  /// it there. Returns null when the base URL is not an http(s) origin.
  Uri? bookingTrackUri({required String accessToken, required String bookingId}) {
    final base = Uri.tryParse(_config.baseUrl);
    if (base == null || !base.hasScheme) return null;

    final scheme = base.scheme == 'https' ? 'wss' : 'ws';
    final path =
        '${base.path}/bookings/$bookingId/track'.replaceAll('//', '/');
    return base.replace(
      scheme: scheme,
      path: path,
      queryParameters: {'token': accessToken},
    );
  }

  Future<DispatchBooking> cancelBooking({
    required String accessToken,
    required String bookingId,
    String reason = '',
  }) async {
    final data = await _sendJson(
      'POST',
      '/bookings/$bookingId/cancel',
      accessToken: accessToken,
      body: {'reason': reason},
    );
    return DispatchBooking.fromJson(data);
  }

  /// Submits the rider's pickup credential (entered manually or scanned as a
  /// QR) for the customer to verify before the rider can proceed.
  Future<DispatchBooking> verifyDriver({
    required String accessToken,
    required String bookingId,
    required String code,
  }) async {
    final data = await _sendJson(
      'POST',
      '/bookings/$bookingId/verify-driver',
      accessToken: accessToken,
      body: {'code': code},
    );
    return DispatchBooking.fromJson(data);
  }

  /// Fetches the customer's own delivery verification code for redisplay.
  Future<VerificationCodeResult> getVerificationCode({
    required String accessToken,
    required String bookingId,
    String step = 'delivery',
  }) async {
    final data = await _sendJson(
      'GET',
      '/bookings/$bookingId/verification-code?step=$step',
      accessToken: accessToken,
    );
    return VerificationCodeResult.fromJson(data);
  }

  /// Regenerates the customer's own delivery verification code.
  Future<VerificationCodeResult> resendVerificationCode({
    required String accessToken,
    required String bookingId,
    String step = 'delivery',
  }) async {
    final data = await _sendJson(
      'POST',
      '/bookings/$bookingId/verification-code/resend?step=$step',
      accessToken: accessToken,
    );
    return VerificationCodeResult.fromJson(data);
  }

  // ─── Realtime ──────────────────────────────────────────────────────────────

  /// Mints a short-lived websocket token from the dispatch-service notification
  /// proxy. The proxy lives at /api/v1/customer/notifications on the service
  /// ORIGIN — the configured base URL includes the /api/v1/customer/dispatch
  /// feature prefix, so the path is built against the origin instead.
  ///
  /// Returns null when the proxy endpoint returns 404 (dispatch-service not
  /// configured with a notification-service URL). The caller should treat null
  /// as "realtime unavailable" and fall back to polling.
  Future<RealtimeToken?> fetchRealtimeToken({
    required String accessToken,
  }) async {
    final base = Uri.parse(_config.baseUrl);
    final uri = Uri(
      scheme: base.scheme,
      host: base.host,
      port: base.hasPort ? base.port : null,
      path: '/api/v1/customer/notifications/realtime-token',
    );
    try {
      final data = await _sendJson(
        'POST',
        uri.path,
        accessToken: accessToken,
        overrideUri: uri,
      );
      return RealtimeToken(
        token: (data['token'] as String?) ?? '',
        expiresAt: (data['expires_at'] as String?) ?? '',
      );
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
    Uri? overrideUri,
  }) async {
    try {
      final uri = overrideUri ?? _config.uri(path);
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
        'DELETE' => await _client.delete(uri, headers: headers),
        _ => throw UnsupportedError('Unsupported HTTP method: $method'),
      };

      final decoded = _decodeResponse(response);
      if (response.statusCode < 200 || response.statusCode >= 300) {
        throw ApiException.fromErrorEnvelope(
          decoded,
          statusCode: response.statusCode,
        );
      }
      if (decoded['success'] != true) {
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
    return const {
      'success': false,
      'error': {'code': 'unknown', 'message': 'Unexpected response.'},
    };
  }
}
