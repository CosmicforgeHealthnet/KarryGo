import 'package:karrygo_api_core/karrygo_api_core.dart';

class TruckProviderApi {
  const TruckProviderApi(this.config);

  final ApiCoreConfig config;

  Uri endpoint(String path) => config.uri('/truck/jobs$path');
}
