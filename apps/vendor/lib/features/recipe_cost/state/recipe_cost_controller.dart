import '../../../core/state/vendor_feature_controller.dart';
import '../data/recipe_cost_api.dart';

class VendorRecipeCostFeatureController extends VendorFeatureController {
  VendorRecipeCostFeatureController(super.scope, this.api);
  final VendorRecipeCostApi api;
}
