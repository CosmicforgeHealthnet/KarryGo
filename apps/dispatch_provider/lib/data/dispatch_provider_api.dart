import 'package:karrygo_api_core/karrygo_api_core.dart';

class DispatchProviderApi {
  const DispatchProviderApi(this.config);

  final ApiCoreConfig config;

  Uri endpoint(String path) => config.uri('/dispatch/jobs$path');
}
