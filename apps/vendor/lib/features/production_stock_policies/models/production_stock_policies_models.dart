// GENERATED from actual request schemas/Go JSON fields.
import '../../../core/contracts/vendor_contract.dart';

class PutProductionStockPoliciesBranchesBranchIdRequest extends VendorPayload {
  PutProductionStockPoliciesBranchesBranchIdRequest({
    String? targetType,
    String? targetRef,
    String? targetVariantRef,
    String? operatingModel,
    String? preparedInventoryItemId,
    int? outputInventoryNumerator,
    int? outputInventoryDenominator,
  }) : super({
         'target_type': ?targetType,
         'target_ref': ?targetRef,
         'target_variant_ref': ?targetVariantRef,
         'operating_model': ?operatingModel,
         'prepared_inventory_item_id': ?preparedInventoryItemId,
         'output_inventory_numerator': ?outputInventoryNumerator,
         'output_inventory_denominator': ?outputInventoryDenominator,
       });
  PutProductionStockPoliciesBranchesBranchIdRequest.fromJson(super.json);
  String? get targetType => value('target_type') as String?;
  String? get targetRef => value('target_ref') as String?;
  String? get targetVariantRef => value('target_variant_ref') as String?;
  String? get operatingModel => value('operating_model') as String?;
  String? get preparedInventoryItemId =>
      value('prepared_inventory_item_id') as String?;
  int? get outputInventoryNumerator =>
      value('output_inventory_numerator') as int?;
  int? get outputInventoryDenominator =>
      value('output_inventory_denominator') as int?;
}
