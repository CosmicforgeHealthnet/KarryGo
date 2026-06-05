import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';

class TruckProviderApi {
  const TruckProviderApi(this.config);

  final ApiCoreConfig config;

  Uri endpoint(String path) => config.uri('/truck/jobs$path');
}
