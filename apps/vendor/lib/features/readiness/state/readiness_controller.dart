import '../../../core/state/vendor_feature_controller.dart';
import '../data/readiness_api.dart';

class VendorReadinessFeatureController extends VendorFeatureController {
  VendorReadinessFeatureController(super.scope, this.api);
  final VendorReadinessApi api;
}
