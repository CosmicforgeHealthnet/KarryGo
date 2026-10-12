import '../../../core/state/vendor_feature_controller.dart';
import '../data/cloud_kitchen_api.dart';

class VendorCloudKitchenFeatureController extends VendorFeatureController {
  VendorCloudKitchenFeatureController(super.scope, this.api);
  final VendorCloudKitchenApi api;
}
