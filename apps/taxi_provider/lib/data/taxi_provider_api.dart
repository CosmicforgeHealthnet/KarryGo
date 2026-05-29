import 'package:karrygo_api_core/karrygo_api_core.dart';

class TaxiProviderApi {
  const TaxiProviderApi(this.config);

  final ApiCoreConfig config;

  Uri endpoint(String path) => config.uri('/taxi/jobs$path');
}
