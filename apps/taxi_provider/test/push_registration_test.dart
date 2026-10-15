import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:taxi_provider/features/home/data/provider_api.dart';

void main() {
  test('registerDevice posts the FCM token to the taxi notification proxy',
      () async {
    late http.Request sent;
    final api = ProviderApi(
      config: const ApiCoreConfig(baseUrl: 'http://taxi.test/api/v1/taxi'),
      client: MockClient((request) async {
        sent = request;
        return http.Response(jsonEncode({'success': true, 'data': {}}), 200);
      }),
    );

    await api.registerDevice(
      accessToken: 'driver-token',
      token: 'fcm-token-1',
      platform: 'android',
    );

    expect(sent.method, 'POST');
    expect(sent.url.toString(),
        'http://taxi.test/api/v1/taxi/provider/notifications/devices');
    expect(sent.headers['Authorization'], 'Bearer driver-token');
    expect(jsonDecode(sent.body), {
      'token': 'fcm-token-1',
      'platform': 'android',
      'app': 'taxi_provider',
    });
  });
}
