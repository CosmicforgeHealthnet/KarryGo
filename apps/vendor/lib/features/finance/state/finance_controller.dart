import '../../../core/state/vendor_feature_controller.dart';
import '../data/finance_api.dart';

class VendorFinanceFeatureController extends VendorFeatureController {
  VendorFinanceFeatureController(super.scope, this.api);
  final VendorFinanceApi api;
}
