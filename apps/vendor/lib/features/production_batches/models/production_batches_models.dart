// GENERATED from actual request schemas/Go JSON fields.
import '../../../core/contracts/vendor_contract.dart';

class PostProductionBatchesRequest extends VendorPayload {
  PostProductionBatchesRequest({
    String? branchId,
    String? recipeId,
    String? recipeVersionId,
    int? plannedOutputNumerator,
    int? plannedOutputDenominator,
    String? productionJobId,
  }) : super({
         'branch_id': ?branchId,
         'recipe_id': ?recipeId,
         'recipe_version_id': ?recipeVersionId,
         'planned_output_numerator': ?plannedOutputNumerator,
         'planned_output_denominator': ?plannedOutputDenominator,
         'production_job_id': ?productionJobId,
       });
  PostProductionBatchesRequest.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  String? get recipeId => value('recipe_id') as String?;
  String? get recipeVersionId => value('recipe_version_id') as String?;
  int? get plannedOutputNumerator => value('planned_output_numerator') as int?;
  int? get plannedOutputDenominator =>
      value('planned_output_denominator') as int?;
  String? get productionJobId => value('production_job_id') as String?;
}

class PostProductionBatchesBatchIdActualConsumptionRequest
    extends VendorPayload {
  PostProductionBatchesBatchIdActualConsumptionRequest({
    String? kind,
    String? referenceId,
    int? quantityNumerator,
    int? quantityDenominator,
  }) : super({
         'kind': ?kind,
         'reference_id': ?referenceId,
         'quantity_numerator': ?quantityNumerator,
         'quantity_denominator': ?quantityDenominator,
       });
  PostProductionBatchesBatchIdActualConsumptionRequest.fromJson(super.json);
  String? get kind => value('kind') as String?;
  String? get referenceId => value('reference_id') as String?;
  int? get quantityNumerator => value('quantity_numerator') as int?;
  int? get quantityDenominator => value('quantity_denominator') as int?;
}

class PostProductionBatchesBatchIdCancelRequest extends VendorPayload {
  PostProductionBatchesBatchIdCancelRequest({String? reason})
    : super({'reason': ?reason});
  PostProductionBatchesBatchIdCancelRequest.fromJson(super.json);
  String? get reason => value('reason') as String?;
}

class PostProductionBatchesBatchIdCompleteRequest extends VendorPayload {
  PostProductionBatchesBatchIdCompleteRequest({
    int? actualOutputNumerator,
    int? actualOutputDenominator,
  }) : super({
         'actual_output_numerator': ?actualOutputNumerator,
         'actual_output_denominator': ?actualOutputDenominator,
       });
  PostProductionBatchesBatchIdCompleteRequest.fromJson(super.json);
  int? get actualOutputNumerator => value('actual_output_numerator') as int?;
  int? get actualOutputDenominator =>
      value('actual_output_denominator') as int?;
}

class PostProductionBatchesBatchIdInventoryAdjustmentsRequest
    extends VendorPayload {
  PostProductionBatchesBatchIdInventoryAdjustmentsRequest({
    List<dynamic>? lines,
  }) : super({'lines': ?lines});
  PostProductionBatchesBatchIdInventoryAdjustmentsRequest.fromJson(super.json);
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
}

class PostProductionBatchesBatchIdWasteRequest extends VendorPayload {
  PostProductionBatchesBatchIdWasteRequest({
    String? kind,
    String? referenceId,
    int? quantityNumerator,
    int? quantityDenominator,
    String? reasonCode,
  }) : super({
         'kind': ?kind,
         'reference_id': ?referenceId,
         'quantity_numerator': ?quantityNumerator,
         'quantity_denominator': ?quantityDenominator,
         'reason_code': ?reasonCode,
       });
  PostProductionBatchesBatchIdWasteRequest.fromJson(super.json);
  String? get kind => value('kind') as String?;
  String? get referenceId => value('reference_id') as String?;
  int? get quantityNumerator => value('quantity_numerator') as int?;
  int? get quantityDenominator => value('quantity_denominator') as int?;
  String? get reasonCode => value('reason_code') as String?;
}
