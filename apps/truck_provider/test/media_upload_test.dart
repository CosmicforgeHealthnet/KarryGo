import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:truck_provider/features/media/data/media_file_api.dart';

void main() {
  const config = ApiCoreConfig(baseUrl: 'http://localhost/api/v1/hauling');

  test('uploads through hauling as the signed-in driver', () async {
    late http.Request seen;
    final api = MediaFileApi(
      config: config,
      accessToken: () => 'driver-token',
      client: MockClient((request) async {
        seen = request;
        return http.Response(
          jsonEncode({
            'success': true,
            'data': {'id': 'a1', 'url': 'https://files/a1'},
          }),
          201,
        );
      }),
    );

    final result = await api.uploadFile(
      ownerId: 'ignored',
      purpose: 'document_file',
      filename: 'licence.jpg',
      bytes: [0xFF, 0xD8, 0xFF],
      contentType: 'image/jpeg',
    );

    expect(result.url, 'https://files/a1');
    expect(seen.url.path, '/api/v1/hauling/provider/uploads');
    expect(seen.headers['Authorization'], 'Bearer driver-token');
    final body = utf8.decode(seen.bodyBytes, allowMalformed: true);
    expect(body, contains('name="purpose"'));
    expect(body, isNot(contains('owner_service')));
  });

  test('refreshes an expired session once and retries', () async {
    var token = 'old';
    var calls = 0;
    final api = MediaFileApi(
      config: config,
      accessToken: () => token,
      refreshSession: () async {
        token = 'new';
        return true;
      },
      client: MockClient((request) async {
        calls++;
        if (request.headers['Authorization'] == 'Bearer old') {
          return http.Response(
            jsonEncode({
              'success': false,
              'error': {'code': 'unauthorized', 'message': 'expired'},
            }),
            401,
          );
        }
        return http.Response(
          jsonEncode({
            'success': true,
            'data': {'id': 'a1', 'url': 'https://files/a1'},
          }),
          201,
        );
      }),
    );

    final result = await api.uploadFile(
      ownerId: 'x',
      purpose: 'profile_photo',
      filename: 'me.jpg',
      bytes: [1, 2, 3],
      contentType: 'image/jpeg',
    );
    expect(result.id, 'a1');
    expect(calls, 2);
  });
}
