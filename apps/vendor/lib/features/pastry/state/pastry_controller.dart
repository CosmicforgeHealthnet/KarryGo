import '../../../core/state/vendor_feature_controller.dart';
import '../data/pastry_api.dart';

class VendorPastryFeatureController extends VendorFeatureController {
  VendorPastryFeatureController(super.scope, this.api);
  final VendorPastryApi api;
}
