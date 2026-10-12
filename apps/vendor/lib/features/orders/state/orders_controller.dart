import '../../../core/state/vendor_feature_controller.dart';
import '../data/orders_api.dart';

class VendorOrdersFeatureController extends VendorFeatureController {
  VendorOrdersFeatureController(super.scope, this.api);
  final VendorOrdersApi api;
}
