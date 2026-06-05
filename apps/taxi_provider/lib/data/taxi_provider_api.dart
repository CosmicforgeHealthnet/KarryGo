import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';

class TaxiProviderApi {
  const TaxiProviderApi(this.config);

  final ApiCoreConfig config;

  Uri endpoint(String path) => config.uri('/taxi/jobs$path');
}
