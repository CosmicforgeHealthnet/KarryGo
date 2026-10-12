import '../../../core/state/vendor_feature_controller.dart';
import '../data/delivery_api.dart';

class VendorDeliveryFeatureController extends VendorFeatureController {
  VendorDeliveryFeatureController(super.scope, this.api);
  final VendorDeliveryApi api;
}
