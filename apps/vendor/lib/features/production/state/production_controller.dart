import '../../../core/state/vendor_feature_controller.dart';
import '../data/production_api.dart';

class VendorProductionFeatureController extends VendorFeatureController {
  VendorProductionFeatureController(super.scope, this.api);
  final VendorProductionApi api;
}
