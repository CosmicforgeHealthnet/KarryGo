import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/app/data/mock_vendor_client.dart';
import 'package:vendor/app/data/vendor_preferences.dart';
import 'package:vendor/app/vendor_runtime.dart';

void main() {
  test(
    'new owner verifies before entering draft business onboarding',
    () async {
      final runtime = VendorRuntime(
        preferences: VendorPreferences(inMemory: true),
        mockLatency: Duration.zero,
      );
      addTearDown(runtime.dispose);
      await runtime.initialize();
      await runtime.start(
        'new-owner@example.com',
        email: true,
        registration: {
          'legal_name': 'New vendor kitchen',
          'currency': 'NGN',
          'timezone': 'Africa/Lagos',
        },
      );
      await runtime.verify('000000');
      expect(runtime.auth.session, isNull);
      expect(runtime.page, VendorPage.code);
      await runtime.resend();
      await runtime.verify(MockVendorClient.code);
      expect(runtime.page, VendorPage.signedIn);
      final app = runtime.backend;
      final business = await app.businessApi.getBusinessesBusinessId(
        businessId: app.scope.vendorId!,
      );
      expect(business.object['status'], 'DRAFT');
      expect(business.object['legal_name'], 'New vendor kitchen');
      expect(business.object['currency'], 'NGN');
    },
  );
}
