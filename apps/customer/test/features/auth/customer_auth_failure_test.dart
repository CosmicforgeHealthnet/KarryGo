import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:customer/features/auth/data/customer_auth_api.dart';
import 'package:customer/features/auth/data/customer_session_store.dart';
import 'package:customer/features/auth/models/customer_auth_models.dart';
import 'package:customer/features/auth/state/customer_auth_controller.dart';
import 'package:customer/features/media/data/media_file_api.dart';
import 'package:customer/features/media/data/media_upload_service.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

MediaUploadService _stubMediaService() => MediaUploadService(
      api: MediaFileApi(
        config: const ApiCoreConfig(
          baseUrl: 'http://localhost:8109/api/v1/media-files',
        ),
        serviceToken: 'test-token',
        client: MockClient((_) async => http.Response('{}', 500)),
      ),
    );

http.Response _unauthorized() => http.Response(
      jsonEncode({
        'success': false,
        'error': {'code': 'unauthorized', 'message': 'Token expired.'},
      }),
      401,
    );

void main() {
  group('CustomerAuthApi onAuthFailure', () {
    test('fires on a 401 from a bearer-authenticated request', () async {
      var called = 0;
      final api = CustomerAuthApi(
        config: const ApiCoreConfig(
          baseUrl: 'http://localhost:8101/api/v1/customer',
        ),
        client: MockClient((_) async => _unauthorized()),
        onAuthFailure: () => called++,
      );

      await expectLater(
        api.me(accessToken: 'expired-token'),
        throwsA(isA<ApiException>()),
      );
      expect(called, 1);
    });

    test('does NOT fire on a 401 from the unauthenticated auth flow', () async {
      var called = 0;
      final api = CustomerAuthApi(
        config: const ApiCoreConfig(
          baseUrl: 'http://localhost:8101/api/v1/customer',
        ),
        client: MockClient((_) async => _unauthorized()),
        onAuthFailure: () => called++,
      );

      // /auth/start carries no access token — a 401 here must not log out.
      await expectLater(
        api.startAuth(phone: '+2348012345678'),
        throwsA(isA<ApiException>()),
      );
      expect(called, 0);
    });
  });

  group('CustomerAuthController.handleAuthFailure', () {
    CustomerAuthController build(CustomerSessionStore store) {
      return CustomerAuthController(
        api: CustomerAuthApi(
          config: const ApiCoreConfig(
            baseUrl: 'http://localhost:8101/api/v1/customer',
          ),
          client: MockClient((_) async => http.Response('{}', 500)),
        ),
        sessionStore: store,
        mediaUploadService: _stubMediaService(),
      );
    }

    test('clears session and routes to phone entry from authenticated state',
        () async {
      final store = MemoryCustomerSessionStore();
      await store.saveSession(
        CustomerSession(
          accessToken: 'a',
          refreshToken: 'r',
          expiresAt: DateTime.now().add(const Duration(hours: 1)),
          customer: const CustomerProfile(
            id: 'cust-1',
            phone: '+2348012345678',
            firstName: 'Ada',
            lastName: 'Okafor',
            onboardingStatus: 'complete',
          ),
        ),
      );
      final controller = build(store);
      // Simulate being signed in.
      controller.debugSetStatus(CustomerAuthStatus.authenticated);

      await controller.handleAuthFailure();

      expect(controller.state.status, CustomerAuthStatus.phoneEntry);
      expect(controller.state.error?.code, ApiErrorCode.unauthorized);
      expect(await store.readSession(), isNull);
    });

    test('is a no-op during the pre-authentication flow', () async {
      final controller = build(MemoryCustomerSessionStore());
      controller.debugSetStatus(CustomerAuthStatus.otpVerification);

      await controller.handleAuthFailure();

      expect(controller.state.status, CustomerAuthStatus.otpVerification);
      expect(controller.state.error, isNull);
    });
  });
}
