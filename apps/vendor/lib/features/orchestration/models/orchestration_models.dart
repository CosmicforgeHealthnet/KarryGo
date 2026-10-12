// GENERATED from actual request schemas/Go JSON fields.
import '../../../core/contracts/vendor_contract.dart';

class PostFulfilmentGroupsGroupIdOrchestrationAuthorityOverrideRequest
    extends VendorPayload {
  PostFulfilmentGroupsGroupIdOrchestrationAuthorityOverrideRequest({
    String? authority,
    String? productCode,
    String? reason,
  }) : super({
         'authority': ?authority,
         'product_code': ?productCode,
         'reason': ?reason,
       });
  PostFulfilmentGroupsGroupIdOrchestrationAuthorityOverrideRequest.fromJson(
    super.json,
  );
  String? get authority => value('authority') as String?;
  String? get productCode => value('product_code') as String?;
  String? get reason => value('reason') as String?;
}
