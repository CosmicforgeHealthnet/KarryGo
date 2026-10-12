import '../../../core/state/vendor_feature_controller.dart';
import '../data/shop_catalogue_api.dart';

class VendorShopCatalogueFeatureController extends VendorFeatureController {
  VendorShopCatalogueFeatureController(super.scope, this.api);
  final VendorShopCatalogueApi api;
}
