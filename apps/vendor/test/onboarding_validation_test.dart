import 'package:flutter_test/flutter_test.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:vendor/features/onboarding/state/business_onboarding_controller.dart';
import 'dashboard_test.dart' show dashboardRuntime;

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  test(
    'Onboarding preserves server field errors through its save controller',
    () async {
      final runtime = await dashboardRuntime();
      addTearDown(runtime.dispose);
      final controller = BusinessOnboardingController(runtime.backend);
      addTearDown(controller.dispose);
      const failure = ApiException(
        code: 'validation_failed',
        message: 'Check your details.',
        statusCode: 422,
        fields: [
          ApiFieldError(field: 'address.city', message: 'City is required.'),
          ApiFieldError(
            field: 'timezone',
            message: 'Use a valid IANA timezone.',
          ),
        ],
      );
      expect(await controller.run(() async => throw failure), isFalse);
      expect(controller.saveError, same(failure));
      expect(controller.error, 'City is required.\nUse a valid IANA timezone.');
      expect(await controller.run(() async {}), isTrue);
      expect(controller.saveError, isNull);
      expect(controller.error, isNull);
    },
  );
}
