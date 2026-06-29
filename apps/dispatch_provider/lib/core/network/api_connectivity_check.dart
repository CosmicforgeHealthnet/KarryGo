import 'dart:async';

import 'package:http/http.dart' as http;

import '../config.dart';

class ApiConnectivityResult {
  const ApiConnectivityResult({
    required this.reachable,
    required this.baseUrl,
    this.statusCode,
    this.errorMessage,
  });

  final bool reachable;
  final int? statusCode;
  final String? errorMessage;
  final String baseUrl;
}

class ApiConnectivityCheck {
  static const timeout = Duration(seconds: 5);

  static Future<ApiConnectivityResult> check({
    String? baseUrl,
    http.Client? client,
  }) async {
    final selectedBaseUrl = _normalizeBaseUrl(baseUrl ?? AppConfig.apiBaseUrl);
    final Uri uri;

    try {
      uri = _healthUri(selectedBaseUrl);
    } on FormatException catch (error) {
      return ApiConnectivityResult(
        reachable: false,
        baseUrl: selectedBaseUrl,
        errorMessage: _shortError(error),
      );
    }

    final ownsClient = client == null;
    final httpClient = client ?? http.Client();

    try {
      final response = await httpClient
          .get(uri, headers: {'Accept': 'application/json'})
          .timeout(timeout);
      final reachable = response.statusCode >= 200 && response.statusCode < 300;
      return ApiConnectivityResult(
        reachable: reachable,
        baseUrl: selectedBaseUrl,
        statusCode: response.statusCode,
        errorMessage: reachable
            ? null
            : 'Health check failed (HTTP ${response.statusCode}).',
      );
    } on TimeoutException catch (error) {
      return ApiConnectivityResult(
        reachable: false,
        baseUrl: selectedBaseUrl,
        errorMessage: 'Health check timed out: ${_shortError(error)}',
      );
    } on FormatException catch (error) {
      return ApiConnectivityResult(
        reachable: false,
        baseUrl: selectedBaseUrl,
        errorMessage: _shortError(error),
      );
    } catch (error) {
      return ApiConnectivityResult(
        reachable: false,
        baseUrl: selectedBaseUrl,
        errorMessage: _shortError(error),
      );
    } finally {
      if (ownsClient) {
        httpClient.close();
      }
    }
  }

  static Uri _healthUri(String baseUrl) {
    final uri = Uri.parse('$baseUrl/health');
    if (!uri.hasScheme || uri.host.isEmpty) {
      throw FormatException('Invalid backend base URL: $baseUrl');
    }
    return uri;
  }

  static String _normalizeBaseUrl(String value) {
    var normalized = value.trim();
    while (normalized.endsWith('/') && normalized.length > 1) {
      normalized = normalized.substring(0, normalized.length - 1);
    }
    return normalized;
  }

  static String _shortError(Object error) {
    final message = error.toString();
    return message.length > 160 ? '${message.substring(0, 160)}...' : message;
  }
}
