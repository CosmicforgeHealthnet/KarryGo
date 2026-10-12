import '../../../core/state/vendor_feature_controller.dart';
import '../data/support_api.dart';

class VendorSupportFeatureController extends VendorFeatureController {
  VendorSupportFeatureController(super.scope, this.api);
  final VendorSupportApi api;
}
