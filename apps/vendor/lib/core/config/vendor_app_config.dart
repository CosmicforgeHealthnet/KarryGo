import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';

class VendorAppConfig {
  VendorAppConfig({
    required this.origin,
    this.timeout = const Duration(seconds: 30),
  }) {
    final uri = Uri.parse(origin);
    if (!uri.hasAuthority ||
        !['http', 'https'].contains(uri.scheme) ||
        (uri.path.isNotEmpty && uri.path != '/') ||
        uri.hasQuery ||
        uri.hasFragment ||
        uri.userInfo.isNotEmpty) {
      throw ArgumentError(
        'origin must be an HTTP(S) origin without an API path or credentials',
      );
    }
  }
  factory VendorAppConfig.fromEnvironment() => VendorAppConfig(
    origin: const String.fromEnvironment('VENDOR_API_ORIGIN'),
  );
  final String origin;
  final Duration timeout;
  ApiCoreConfig get api => ApiCoreConfig(
    baseUrl: origin.endsWith('/')
        ? origin.substring(0, origin.length - 1)
        : origin,
  );
}
