import '../../../core/state/vendor_feature_controller.dart';
import '../data/production_stock_policies_api.dart';

class VendorProductionStockPoliciesFeatureController
    extends VendorFeatureController {
  VendorProductionStockPoliciesFeatureController(super.scope, this.api);
  final VendorProductionStockPoliciesApi api;
}
