import '../../../core/state/vendor_feature_controller.dart';
import '../data/handover_api.dart';

class VendorHandoverFeatureController extends VendorFeatureController {
  VendorHandoverFeatureController(super.scope, this.api);
  final VendorHandoverApi api;
}
