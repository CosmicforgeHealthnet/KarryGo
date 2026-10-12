import '../../../core/state/vendor_feature_controller.dart';
import '../data/taxonomy_api.dart';

class VendorTaxonomyFeatureController extends VendorFeatureController {
  VendorTaxonomyFeatureController(super.scope, this.api);
  final VendorTaxonomyApi api;
}
