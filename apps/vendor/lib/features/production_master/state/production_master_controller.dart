import '../../../core/state/vendor_feature_controller.dart';
import '../data/production_master_api.dart';

class VendorProductionMasterFeatureController extends VendorFeatureController {
  VendorProductionMasterFeatureController(super.scope, this.api);
  final VendorProductionMasterApi api;
}
