import '../../../core/state/vendor_feature_controller.dart';
import '../data/production_cost_api.dart';

class VendorProductionCostFeatureController extends VendorFeatureController {
  VendorProductionCostFeatureController(super.scope, this.api);
  final VendorProductionCostApi api;
}
