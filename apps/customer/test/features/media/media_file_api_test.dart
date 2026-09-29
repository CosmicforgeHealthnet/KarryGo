import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:customer/features/auth/data/customer_session_store.dart';
import 'package:customer/features/auth/models/customer_auth_models.dart';
import 'package:customer/features/media/data/media_file_api.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

void main() {
  const profile = CustomerProfile(
    id: 'customer-one',
    phone: '08030000000',
    onboardingStatus: 'complete',
  );

  test(
    'profile upload uses customer bearer and never a media service credential',
    () async {
      final store = MemoryCustomerSessionStore();
      await store.saveSession(
        CustomerSession(
          accessToken: 'customer-access',
          refreshToken: 'refresh',
          expiresAt: DateTime.now().add(const Duration(minutes: 10)),
          customer: profile,
        ),
      );
      var calls = 0;
      final api = MediaFileApi(
        config: const ApiCoreConfig(
          baseUrl: 'https://customer.example/api/v1/customer',
        ),
        sessionStore: store,
        client: MockClient((request) async {
          calls++;
          expect(request.url.path, '/api/v1/customer/profile/photo');
          expect(request.headers['authorization'], 'Bearer customer-access');
          expect(request.headers.containsKey('x-karrygo-service'), isFalse);
          expect(
            request.headers['content-type'],
            startsWith('multipart/form-data'),
          );
          return http.Response(
            jsonEncode({
              'success': true,
              'data': {
                'profile_photo_asset_id': 'media-one',
                'profile_photo_url': 'https://media.example/media-one/download',
              },
            }),
            200,
          );
        }),
      );
      final result = await api.uploadFile(
        ownerId: profile.id,
        purpose: 'profile_photo',
        filename: 'photo.jpg',
        bytes: [0xff, 0xd8, 0xff],
        contentType: 'image/jpeg',
      );
      expect(result.id, 'media-one');
      expect(calls, 1);
      api.close();
    },
  );

  test(
    'cross-customer owner and unsupported purpose never reach the network',
    () async {
      final store = MemoryCustomerSessionStore();
      await store.saveSession(
        CustomerSession(
          accessToken: 'customer-access',
          refreshToken: 'refresh',
          expiresAt: DateTime.now().add(const Duration(minutes: 10)),
          customer: profile,
        ),
      );
      var calls = 0;
      final api = MediaFileApi(
        config: const ApiCoreConfig(
          baseUrl: 'https://customer.example/api/v1/customer',
        ),
        sessionStore: store,
        client: MockClient((_) async {
          calls++;
          return http.Response('{}', 500);
        }),
      );
      for (final attempt in [
        ('someone-else', 'profile_photo'),
        (profile.id, 'document_file'),
      ]) {
        await expectLater(
          api.uploadFile(
            ownerId: attempt.$1,
            purpose: attempt.$2,
            filename: 'photo.jpg',
            bytes: [0xff, 0xd8],
            contentType: 'image/jpeg',
          ),
          throwsA(isA<ApiException>()),
        );
      }
      expect(calls, 0);
      api.close();
    },
  );
}
