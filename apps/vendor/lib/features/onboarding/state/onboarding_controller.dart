import '../../../core/state/vendor_feature_controller.dart';
import '../data/onboarding_api.dart';

class VendorOnboardingFeatureController extends VendorFeatureController {
  VendorOnboardingFeatureController(super.scope, this.api);
  final VendorOnboardingApi api;
}
