import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:http/http.dart' as http;

import '../../auth/models/provider_auth_models.dart';
import '../../earnings/models/earnings_models.dart';
import '../../trips/models/customer_review.dart';

class ProviderApi {
  ProviderApi({required ApiCoreConfig config, http.Client? client})
    : _config = config,
      _client = client ?? http.Client();

  final ApiCoreConfig _config;
  final http.Client _client;

  // ─── Availability ─────────────────────────────────────────────────────────

  Future<void> setOnline({
    required String accessToken,
    required double lat,
    required double lng,
  }) async {
    await _put('/provider/availability', {'status': 'online', 'lat': lat, 'lng': lng}, accessToken: accessToken);
  }

  Future<void> setOffline({required String accessToken}) async {
    await _put('/provider/availability', {'status': 'offline', 'lat': 0.0, 'lng': 0.0}, accessToken: accessToken);
  }

  Future<void> heartbeat({
    required String accessToken,
    required double lat,
    required double lng,
  }) async {
    await _post('/provider/availability/heartbeat', {'lat': lat, 'lng': lng}, accessToken: accessToken);
  }

  Future<bool> getAvailability({required String accessToken}) async {
    final data = await _get('/provider/availability', accessToken: accessToken);
    return data['status'] == 'online';
  }

  // ─── Trucks ───────────────────────────────────────────────────────────────

  Future<List<ProviderTruck>> listTrucks({required String accessToken}) async {
    final list = await _getList('/provider/trucks', accessToken: accessToken);
    return list.map((e) => ProviderTruck.fromJson(Map<String, dynamic>.from(e as Map))).toList();
  }

  // ─── Earnings ─────────────────────────────────────────────────────────────

  Future<ProviderEarnings> getEarnings({required String accessToken}) async {
    final data = await _get('/provider/earnings', accessToken: accessToken);
    return ProviderEarnings.fromJson(data);
  }

  // ─── Bookings ─────────────────────────────────────────────────────────────

  Future<List<ProviderBooking>> listBookings({required String accessToken, String? status}) async {
    final q = status != null ? '?status=$status&limit=20' : '?limit=20';
    final list = await _getList('/provider/bookings$q', accessToken: accessToken);
    return list.map((e) => ProviderBooking.fromJson(Map<String, dynamic>.from(e as Map))).toList();
  }

  Future<ProviderBooking> getBooking({required String accessToken, required String bookingId}) async {
    final data = await _get('/provider/bookings/$bookingId', accessToken: accessToken);
    return ProviderBooking.fromJson(data);
  }

  /// Fetches the customer's review for a completed booking. Returns null when no
  /// review exists yet (the endpoint responds 404), which the trip detail screen
  /// treats as "no review submitted".
  Future<CustomerReview?> getBookingReview({required String accessToken, required String bookingId}) async {
    try {
      final data = await _get('/provider/bookings/$bookingId/review', accessToken: accessToken);
      return CustomerReview.fromJson(data);
    } on ApiException catch (e) {
      if (e.statusCode == 404) return null;
      rethrow;
    }
  }

  Future<ProviderBooking> acceptBooking({required String accessToken, required String bookingId}) async {
    final data = await _put('/provider/bookings/$bookingId/accept', {}, accessToken: accessToken);
    return ProviderBooking.fromJson(data);
  }

  Future<ProviderBooking> rejectBooking({required String accessToken, required String bookingId}) async {
    final data = await _put('/provider/bookings/$bookingId/reject', {}, accessToken: accessToken);
    return ProviderBooking.fromJson(data);
  }

  Future<ProviderBooking> markEnRoutePickup({required String accessToken, required String bookingId}) async {
    final data = await _put('/provider/bookings/$bookingId/en-route-pickup', {}, accessToken: accessToken);
    return ProviderBooking.fromJson(data);
  }

  Future<ProviderBooking> markArrived({required String accessToken, required String bookingId}) async {
    final data = await _put('/provider/bookings/$bookingId/arrived', {}, accessToken: accessToken);
    return ProviderBooking.fromJson(data);
  }

  Future<ProviderBooking> confirmPickup({required String accessToken, required String bookingId}) async {
    final data = await _put('/provider/bookings/$bookingId/pickup-confirmed', {}, accessToken: accessToken);
    return ProviderBooking.fromJson(data);
  }

  Future<ProviderBooking> markEnRouteDelivery({required String accessToken, required String bookingId}) async {
    final data = await _put('/provider/bookings/$bookingId/en-route-delivery', {}, accessToken: accessToken);
    return ProviderBooking.fromJson(data);
  }

  Future<ProviderBooking> confirmDelivery({required String accessToken, required String bookingId}) async {
    final data = await _put('/provider/bookings/$bookingId/delivered', {}, accessToken: accessToken);
    return ProviderBooking.fromJson(data);
  }

  Future<ProviderBooking> cancelActiveTrip({required String accessToken, required String bookingId}) async {
    final data = await _put('/provider/bookings/$bookingId/cancel', {}, accessToken: accessToken);
    return ProviderBooking.fromJson(data);
  }

  void close() => _client.close();

  // ─── HTTP helpers ─────────────────────────────────────────────────────────

  Future<Map<String, dynamic>> _put(String path, Map<String, dynamic> body, {required String accessToken}) async {
    try {
      final response = await _client.put(
        _config.uri(path),
        headers: _headers(accessToken),
        body: jsonEncode(body),
      );
      return _unwrap(response);
    } on ApiException {
      rethrow;
    } catch (e) {
      throw ApiException.network(e);
    }
  }

  Future<Map<String, dynamic>> _post(String path, Map<String, dynamic> body, {required String accessToken}) async {
    try {
      final response = await _client.post(
        _config.uri(path),
        headers: _headers(accessToken),
        body: jsonEncode(body),
      );
      return _unwrap(response);
    } on ApiException {
      rethrow;
    } catch (e) {
      throw ApiException.network(e);
    }
  }

  Future<Map<String, dynamic>> _get(String path, {required String accessToken}) async {
    try {
      final response = await _client.get(_config.uri(path), headers: _headers(accessToken));
      return _unwrap(response);
    } on ApiException {
      rethrow;
    } catch (e) {
      throw ApiException.network(e);
    }
  }

  Future<List<dynamic>> _getList(String path, {required String accessToken}) async {
    try {
      final response = await _client.get(_config.uri(path), headers: _headers(accessToken));
      final decoded = Map<String, dynamic>.from(jsonDecode(response.body) as Map);
      if (response.statusCode < 200 || response.statusCode >= 300) {
        throw ApiException.fromErrorEnvelope(decoded, statusCode: response.statusCode);
      }
      final raw = decoded['data'];
      if (raw is List) return raw;
      return const [];
    } on ApiException {
      rethrow;
    } catch (e) {
      throw ApiException.network(e);
    }
  }

  Map<String, String> _headers(String accessToken) => {
    'Content-Type': 'application/json',
    'Accept': 'application/json',
    'Authorization': 'Bearer $accessToken',
  };

  Map<String, dynamic> _unwrap(http.Response response) {
    final decoded = response.body.isEmpty
        ? <String, dynamic>{'success': true, 'data': <String, dynamic>{}}
        : Map<String, dynamic>.from(jsonDecode(response.body) as Map);
    if (response.statusCode < 200 || response.statusCode >= 300 || decoded['success'] != true) {
      throw ApiException.fromErrorEnvelope(decoded, statusCode: response.statusCode);
    }
    final raw = decoded['data'];
    if (raw is Map) return Map<String, dynamic>.from(raw);
    return const {};
  }
}
