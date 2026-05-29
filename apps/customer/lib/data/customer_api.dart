import 'package:karrygo_api_core/karrygo_api_core.dart';

class CustomerApi {
  const CustomerApi(this.config);

  final ApiCoreConfig config;

  Uri endpoint(String path) => config.uri('/customer/bookings$path');
}
