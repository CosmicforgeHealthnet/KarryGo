import '../../../core/state/vendor_feature_controller.dart';
import '../data/purchasing_api.dart';

class VendorPurchasingFeatureController extends VendorFeatureController {
  VendorPurchasingFeatureController(super.scope, this.api);
  final VendorPurchasingApi api;
}
