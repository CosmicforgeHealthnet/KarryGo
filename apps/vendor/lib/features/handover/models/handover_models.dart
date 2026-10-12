// GENERATED from actual request schemas/Go JSON fields.
import '../../../core/contracts/vendor_contract.dart';

class PostCommerceOrdersVendorOrderIdMissionsMissionIdHandoverPickupConfirmRequest
    extends VendorPayload {
  PostCommerceOrdersVendorOrderIdMissionsMissionIdHandoverPickupConfirmRequest({
    required String handoverRef,
    required String method,
    required String credential,
  }) : super({
         'handover_ref': handoverRef,
         'method': method,
         'credential': credential,
       });
  PostCommerceOrdersVendorOrderIdMissionsMissionIdHandoverPickupConfirmRequest.fromJson(
    super.json,
  );
  String? get handoverRef => value('handover_ref') as String?;
  String? get method => value('method') as String?;
  String? get credential => value('credential') as String?;
}
