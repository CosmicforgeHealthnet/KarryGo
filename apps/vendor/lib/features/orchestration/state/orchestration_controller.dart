import '../../../core/state/vendor_feature_controller.dart';
import '../data/orchestration_api.dart';

class VendorOrchestrationFeatureController extends VendorFeatureController {
  VendorOrchestrationFeatureController(super.scope, this.api);
  final VendorOrchestrationApi api;
}
