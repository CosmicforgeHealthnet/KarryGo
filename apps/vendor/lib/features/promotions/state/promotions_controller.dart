import '../../../core/state/vendor_feature_controller.dart';
import '../data/promotions_api.dart';

class VendorPromotionsFeatureController extends VendorFeatureController {
  VendorPromotionsFeatureController(super.scope, this.api);
  final VendorPromotionsApi api;
}
