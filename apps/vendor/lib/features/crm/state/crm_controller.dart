import '../../../core/state/vendor_feature_controller.dart';
import '../data/crm_api.dart';

class VendorCrmFeatureController extends VendorFeatureController {
  VendorCrmFeatureController(super.scope, this.api);
  final VendorCrmApi api;
}
