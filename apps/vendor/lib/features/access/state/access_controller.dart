import '../../../core/state/vendor_feature_controller.dart';
import '../data/access_api.dart';

class VendorAccessFeatureController extends VendorFeatureController {
  VendorAccessFeatureController(super.scope, this.api);
  final VendorAccessApi api;
}
