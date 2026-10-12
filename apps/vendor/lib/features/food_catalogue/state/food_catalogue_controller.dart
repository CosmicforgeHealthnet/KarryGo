import '../../../core/state/vendor_feature_controller.dart';
import '../data/food_catalogue_api.dart';

class VendorFoodCatalogueFeatureController extends VendorFeatureController {
  VendorFoodCatalogueFeatureController(super.scope, this.api);
  final VendorFoodCatalogueApi api;
}
