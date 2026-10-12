import '../../../core/state/vendor_feature_controller.dart';
import '../data/inventory_api.dart';

class VendorInventoryFeatureController extends VendorFeatureController {
  VendorInventoryFeatureController(super.scope, this.api);
  final VendorInventoryApi api;
}
