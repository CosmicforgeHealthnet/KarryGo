import '../../../core/state/vendor_feature_controller.dart';
import '../data/scheduled_commerce_api.dart';

class VendorScheduledCommerceFeatureController extends VendorFeatureController {
  VendorScheduledCommerceFeatureController(super.scope, this.api);
  final VendorScheduledCommerceApi api;
}
