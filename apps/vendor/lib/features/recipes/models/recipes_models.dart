// GENERATED from actual request schemas/Go JSON fields.
import '../../../core/contracts/vendor_contract.dart';

class PostRecipesRequest extends VendorPayload {
  PostRecipesRequest({
    String? targetType,
    String? targetRef,
    String? targetVariantRef,
    String? name,
    String? internalDescription,
  }) : super({
         'target_type': ?targetType,
         'target_ref': ?targetRef,
         'target_variant_ref': ?targetVariantRef,
         'name': ?name,
         'internal_description': ?internalDescription,
       });
  PostRecipesRequest.fromJson(super.json);
  String? get targetType => value('target_type') as String?;
  String? get targetRef => value('target_ref') as String?;
  String? get targetVariantRef => value('target_variant_ref') as String?;
  String? get name => value('name') as String?;
  String? get internalDescription => value('internal_description') as String?;
}

class PostRecipesRecipeIdVersionsRequest extends VendorPayload {
  PostRecipesRecipeIdVersionsRequest({
    String? internalNotes,
    int? expectedOutputNumerator,
    int? expectedOutputDenominator,
    String? outputUnitCode,
    int? servingCount,
    int? expectedWasteBps,
  }) : super({
         'internal_notes': ?internalNotes,
         'expected_output_numerator': ?expectedOutputNumerator,
         'expected_output_denominator': ?expectedOutputDenominator,
         'output_unit_code': ?outputUnitCode,
         'serving_count': ?servingCount,
         'expected_waste_bps': ?expectedWasteBps,
       });
  PostRecipesRecipeIdVersionsRequest.fromJson(super.json);
  String? get internalNotes => value('internal_notes') as String?;
  int? get expectedOutputNumerator =>
      value('expected_output_numerator') as int?;
  int? get expectedOutputDenominator =>
      value('expected_output_denominator') as int?;
  String? get outputUnitCode => value('output_unit_code') as String?;
  int? get servingCount => value('serving_count') as int?;
  int? get expectedWasteBps => value('expected_waste_bps') as int?;
}

class PutRecipesRecipeIdVersionsVersionIdRequest extends VendorPayload {
  PutRecipesRecipeIdVersionsVersionIdRequest({
    String? internalNotes,
    int? expectedOutputNumerator,
    int? expectedOutputDenominator,
    String? outputUnitCode,
    int? servingCount,
    int? expectedWasteBps,
  }) : super({
         'internal_notes': ?internalNotes,
         'expected_output_numerator': ?expectedOutputNumerator,
         'expected_output_denominator': ?expectedOutputDenominator,
         'output_unit_code': ?outputUnitCode,
         'serving_count': ?servingCount,
         'expected_waste_bps': ?expectedWasteBps,
       });
  PutRecipesRecipeIdVersionsVersionIdRequest.fromJson(super.json);
  String? get internalNotes => value('internal_notes') as String?;
  int? get expectedOutputNumerator =>
      value('expected_output_numerator') as int?;
  int? get expectedOutputDenominator =>
      value('expected_output_denominator') as int?;
  String? get outputUnitCode => value('output_unit_code') as String?;
  int? get servingCount => value('serving_count') as int?;
  int? get expectedWasteBps => value('expected_waste_bps') as int?;
}

class PutRecipesRecipeIdVersionsVersionIdComponentsRequest
    extends VendorPayload {
  PutRecipesRecipeIdVersionsVersionIdComponentsRequest({
    List<dynamic>? components,
  }) : super({'components': ?components});
  PutRecipesRecipeIdVersionsVersionIdComponentsRequest.fromJson(super.json);
  List<dynamic>? get components => value('components') as List<dynamic>?;
}

class PutRecipesRecipeIdVersionsVersionIdPackagingRequest
    extends VendorPayload {
  PutRecipesRecipeIdVersionsVersionIdPackagingRequest({
    List<dynamic>? requirements,
  }) : super({'requirements': ?requirements});
  PutRecipesRecipeIdVersionsVersionIdPackagingRequest.fromJson(super.json);
  List<dynamic>? get requirements => value('requirements') as List<dynamic>?;
}

class PostRecipesRecipeIdVersionsVersionIdScaleRequest extends VendorPayload {
  PostRecipesRecipeIdVersionsVersionIdScaleRequest({
    int? outputNumerator,
    int? outputDenominator,
    String? outputUnitCode,
  }) : super({
         'output_numerator': ?outputNumerator,
         'output_denominator': ?outputDenominator,
         'output_unit_code': ?outputUnitCode,
       });
  PostRecipesRecipeIdVersionsVersionIdScaleRequest.fromJson(super.json);
  int? get outputNumerator => value('output_numerator') as int?;
  int? get outputDenominator => value('output_denominator') as int?;
  String? get outputUnitCode => value('output_unit_code') as String?;
}
