import 'dart:convert';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:vendor/app/vendor_app.dart';
import 'package:vendor/core/config/vendor_app_config.dart';
import 'package:vendor/features/auth/data/vendor_session_store.dart';
import 'package:vendor/features/auth/models/vendor_session.dart';
import 'package:vendor/features/onboarding/state/business_onboarding_controller.dart';

void main() {
  test(
    'bank management loads independently of the onboarding checklist',
    () async {
      final paths = <String>[];
      final sessions = MemoryVendorSessionStore();
      await sessions.save(
        const VendorSession(
          accessToken: 'access',
          refreshToken: 'refresh',
          expiresIn: 900,
          staffId: 'staff',
          vendorId: 'business',
        ),
      );
      final transport = MockClient((request) async {
        paths.add(request.url.path);
        return http.Response(
          jsonEncode({
            'success': false,
            'error': {
              'code': 'not_found',
              'message': 'No bank profile has been added.',
            },
          }),
          404,
        );
      });
      final app = VendorApp(
        config: VendorAppConfig(origin: 'https://example.test'),
        sessions: sessions,
        httpClient: transport,
      );
      addTearDown(app.dispose);
      addTearDown(transport.close);
      await app.initialize();
      final controller = BusinessOnboardingController(app);
      addTearDown(controller.dispose);
      expect(await controller.loadBankProfile(), isTrue);
      expect(controller.bank, isEmpty);
      expect(controller.error, isNull);
      expect(paths, ['/api/v1/vendor/onboarding/bank-profile']);
    },
  );
}
