import '../../../core/state/vendor_feature_controller.dart';
import '../data/production_batches_api.dart';

class VendorProductionBatchesFeatureController extends VendorFeatureController {
  VendorProductionBatchesFeatureController(super.scope, this.api);
  final VendorProductionBatchesApi api;
}
