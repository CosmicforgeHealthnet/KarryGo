import '../../../core/state/vendor_feature_controller.dart';
import '../data/auth_api.dart';

class VendorAuthFeatureController extends VendorFeatureController {
  VendorAuthFeatureController(super.scope, this.api);
  final VendorAuthApi api;
}
