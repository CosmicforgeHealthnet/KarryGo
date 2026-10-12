import '../../../core/state/vendor_feature_controller.dart';
import '../data/business_api.dart';

class VendorBusinessFeatureController extends VendorFeatureController {
  VendorBusinessFeatureController(super.scope, this.api);
  final VendorBusinessApi api;
}
