import '../../../core/state/vendor_feature_controller.dart';
import '../data/recipes_api.dart';

class VendorRecipesFeatureController extends VendorFeatureController {
  VendorRecipesFeatureController(super.scope, this.api);
  final VendorRecipesApi api;
}
