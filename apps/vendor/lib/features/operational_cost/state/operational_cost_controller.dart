import '../../../core/state/vendor_feature_controller.dart';
import '../data/operational_cost_api.dart';

class VendorOperationalCostFeatureController extends VendorFeatureController {
  VendorOperationalCostFeatureController(super.scope, this.api);
  final VendorOperationalCostApi api;
}
