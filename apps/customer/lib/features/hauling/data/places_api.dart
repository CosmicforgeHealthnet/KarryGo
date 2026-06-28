import 'dart:convert';
import 'dart:developer' as developer;

import 'package:http/http.dart' as http;

class PlaceSuggestion {
  const PlaceSuggestion({required this.placeId, required this.description});

  final String placeId;
  final String description;
}

/// Thrown when the Places API responds but with a non-OK status
/// (e.g. REQUEST_DENIED, OVER_QUERY_LIMIT, billing disabled).
/// Surfaced so the UI can show a real reason instead of "no results".
class PlacesApiException implements Exception {
  const PlacesApiException(this.status, this.message);

  final String status;
  final String message;

  @override
  String toString() => 'PlacesApiException($status): $message';
}

class PlacesApi {
  const PlacesApi({required this.apiKey});

  final String apiKey;

  Future<List<PlaceSuggestion>> autocomplete(String query) async {
    if (query.trim().isEmpty) return const [];
    if (apiKey.isEmpty) {
      throw const PlacesApiException(
        'MISSING_KEY',
        'Google Maps API key is not configured.',
      );
    }
    final uri = Uri.https('maps.googleapis.com', '/maps/api/place/autocomplete/json', {
      'input': query,
      'components': 'country:ng',
      'key': apiKey,
    });

    late final http.Response response;
    try {
      response = await http.get(uri).timeout(const Duration(seconds: 8));
    } catch (e) {
      developer.log('Places autocomplete network error', name: 'PlacesApi', error: e);
      throw PlacesApiException('NETWORK_ERROR', e.toString());
    }

    if (response.statusCode != 200) {
      throw PlacesApiException(
        'HTTP_${response.statusCode}',
        'Places autocomplete returned HTTP ${response.statusCode}.',
      );
    }

    final body = jsonDecode(response.body) as Map<String, dynamic>;
    final status = body['status'] as String? ?? 'UNKNOWN';
    // ZERO_RESULTS is a valid empty response, not an error.
    if (status != 'OK' && status != 'ZERO_RESULTS') {
      final detail = body['error_message'] as String? ?? status;
      developer.log('Places autocomplete denied: $status — $detail', name: 'PlacesApi');
      throw PlacesApiException(status, detail);
    }

    final predictions = body['predictions'] as List<dynamic>? ?? [];
    return predictions.map((p) {
      return PlaceSuggestion(
        placeId: p['place_id'] as String,
        description: p['description'] as String,
      );
    }).toList();
  }

  Future<({double lat, double lng})?> getLatLng(String placeId) async {
    if (apiKey.isEmpty) return null;
    final uri = Uri.https('maps.googleapis.com', '/maps/api/place/details/json', {
      'place_id': placeId,
      'fields': 'geometry',
      'key': apiKey,
    });

    late final http.Response response;
    try {
      response = await http.get(uri).timeout(const Duration(seconds: 8));
    } catch (e) {
      developer.log('Places details network error', name: 'PlacesApi', error: e);
      return null;
    }
    if (response.statusCode != 200) return null;

    final body = jsonDecode(response.body) as Map<String, dynamic>;
    final status = body['status'] as String? ?? 'UNKNOWN';
    if (status != 'OK') {
      developer.log('Places details denied: $status', name: 'PlacesApi');
      return null;
    }
    final location = (body['result'] as Map<String, dynamic>?)?['geometry']?['location'];
    if (location == null) return null;
    return (lat: (location['lat'] as num).toDouble(), lng: (location['lng'] as num).toDouble());
  }

  /// Reverse-geocodes coordinates to a human-readable formatted address.
  /// Returns null on any failure (missing key, network, non-OK status) so the
  /// caller can fall back to a generic label.
  Future<String?> reverseGeocode(double lat, double lng) async {
    if (apiKey.isEmpty) return null;
    final uri = Uri.https('maps.googleapis.com', '/maps/api/geocode/json', {
      'latlng': '$lat,$lng',
      'key': apiKey,
    });

    late final http.Response response;
    try {
      response = await http.get(uri).timeout(const Duration(seconds: 8));
    } catch (e) {
      developer.log('Reverse geocode network error', name: 'PlacesApi', error: e);
      return null;
    }
    if (response.statusCode != 200) return null;

    final body = jsonDecode(response.body) as Map<String, dynamic>;
    final status = body['status'] as String? ?? 'UNKNOWN';
    if (status != 'OK') {
      developer.log('Reverse geocode denied: $status', name: 'PlacesApi');
      return null;
    }
    final results = body['results'] as List<dynamic>? ?? [];
    if (results.isEmpty) return null;
    return (results.first as Map<String, dynamic>)['formatted_address'] as String?;
  }
}
