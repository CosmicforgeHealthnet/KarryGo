// GENERATED from actual request schemas/Go JSON fields.
import '../../../core/contracts/vendor_contract.dart';

class PostIngredientsRequest extends VendorPayload {
  PostIngredientsRequest({
    String? code,
    String? name,
    String? description,
    String? measurementFamily,
    String? defaultUnitCode,
    String? inventoryControlMode,
    bool? perishable,
    bool? requiresExpiry,
    String? status,
  }) : super({
         'code': ?code,
         'name': ?name,
         'description': ?description,
         'measurement_family': ?measurementFamily,
         'default_unit_code': ?defaultUnitCode,
         'inventory_control_mode': ?inventoryControlMode,
         'perishable': ?perishable,
         'requires_expiry': ?requiresExpiry,
         'status': ?status,
       });
  PostIngredientsRequest.fromJson(super.json);
  String? get code => value('code') as String?;
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  String? get measurementFamily => value('measurement_family') as String?;
  String? get defaultUnitCode => value('default_unit_code') as String?;
  String? get inventoryControlMode =>
      value('inventory_control_mode') as String?;
  bool? get perishable => value('perishable') as bool?;
  bool? get requiresExpiry => value('requires_expiry') as bool?;
  String? get status => value('status') as String?;
}

class PutIngredientsIngredientIdRequest extends VendorPayload {
  PutIngredientsIngredientIdRequest({
    String? code,
    String? name,
    String? description,
    String? measurementFamily,
    String? defaultUnitCode,
    String? inventoryControlMode,
    bool? perishable,
    bool? requiresExpiry,
    String? status,
  }) : super({
         'code': ?code,
         'name': ?name,
         'description': ?description,
         'measurement_family': ?measurementFamily,
         'default_unit_code': ?defaultUnitCode,
         'inventory_control_mode': ?inventoryControlMode,
         'perishable': ?perishable,
         'requires_expiry': ?requiresExpiry,
         'status': ?status,
       });
  PutIngredientsIngredientIdRequest.fromJson(super.json);
  String? get code => value('code') as String?;
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  String? get measurementFamily => value('measurement_family') as String?;
  String? get defaultUnitCode => value('default_unit_code') as String?;
  String? get inventoryControlMode =>
      value('inventory_control_mode') as String?;
  bool? get perishable => value('perishable') as bool?;
  bool? get requiresExpiry => value('requires_expiry') as bool?;
  String? get status => value('status') as String?;
}

class PutIngredientsIngredientIdBranchesBranchIdRequest extends VendorPayload {
  PutIngredientsIngredientIdBranchesBranchIdRequest({
    bool? enabled,
    bool? productionEligible,
    String? inventoryControlModeOverride,
    String? inventoryItemId,
  }) : super({
         'enabled': ?enabled,
         'production_eligible': ?productionEligible,
         'inventory_control_mode_override': ?inventoryControlModeOverride,
         'inventory_item_id': ?inventoryItemId,
       });
  PutIngredientsIngredientIdBranchesBranchIdRequest.fromJson(super.json);
  bool? get enabled => value('enabled') as bool?;
  bool? get productionEligible => value('production_eligible') as bool?;
  String? get inventoryControlModeOverride =>
      value('inventory_control_mode_override') as String?;
  String? get inventoryItemId => value('inventory_item_id') as String?;
}

class PostIngredientsIngredientIdConversionsRequest extends VendorPayload {
  PostIngredientsIngredientIdConversionsRequest({
    String? measurementFamily,
    String? fromUnitCode,
    String? toUnitCode,
    String? conversionMode,
    int? numerator,
    int? denominator,
    String? sourceContext,
    String? status,
  }) : super({
         'measurement_family': ?measurementFamily,
         'from_unit_code': ?fromUnitCode,
         'to_unit_code': ?toUnitCode,
         'conversion_mode': ?conversionMode,
         'numerator': ?numerator,
         'denominator': ?denominator,
         'source_context': ?sourceContext,
         'status': ?status,
       });
  PostIngredientsIngredientIdConversionsRequest.fromJson(super.json);
  String? get measurementFamily => value('measurement_family') as String?;
  String? get fromUnitCode => value('from_unit_code') as String?;
  String? get toUnitCode => value('to_unit_code') as String?;
  String? get conversionMode => value('conversion_mode') as String?;
  int? get numerator => value('numerator') as int?;
  int? get denominator => value('denominator') as int?;
  String? get sourceContext => value('source_context') as String?;
  String? get status => value('status') as String?;
}

class PutIngredientsIngredientIdConversionsConversionIdRequest
    extends VendorPayload {
  PutIngredientsIngredientIdConversionsConversionIdRequest({
    String? measurementFamily,
    String? fromUnitCode,
    String? toUnitCode,
    String? conversionMode,
    int? numerator,
    int? denominator,
    String? sourceContext,
    String? status,
  }) : super({
         'measurement_family': ?measurementFamily,
         'from_unit_code': ?fromUnitCode,
         'to_unit_code': ?toUnitCode,
         'conversion_mode': ?conversionMode,
         'numerator': ?numerator,
         'denominator': ?denominator,
         'source_context': ?sourceContext,
         'status': ?status,
       });
  PutIngredientsIngredientIdConversionsConversionIdRequest.fromJson(super.json);
  String? get measurementFamily => value('measurement_family') as String?;
  String? get fromUnitCode => value('from_unit_code') as String?;
  String? get toUnitCode => value('to_unit_code') as String?;
  String? get conversionMode => value('conversion_mode') as String?;
  int? get numerator => value('numerator') as int?;
  int? get denominator => value('denominator') as int?;
  String? get sourceContext => value('source_context') as String?;
  String? get status => value('status') as String?;
}

class PostPackagingMaterialsRequest extends VendorPayload {
  PostPackagingMaterialsRequest({
    String? code,
    String? name,
    String? description,
    String? measurementFamily,
    String? defaultUnitCode,
    String? inventoryControlMode,
    String? status,
  }) : super({
         'code': ?code,
         'name': ?name,
         'description': ?description,
         'measurement_family': ?measurementFamily,
         'default_unit_code': ?defaultUnitCode,
         'inventory_control_mode': ?inventoryControlMode,
         'status': ?status,
       });
  PostPackagingMaterialsRequest.fromJson(super.json);
  String? get code => value('code') as String?;
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  String? get measurementFamily => value('measurement_family') as String?;
  String? get defaultUnitCode => value('default_unit_code') as String?;
  String? get inventoryControlMode =>
      value('inventory_control_mode') as String?;
  String? get status => value('status') as String?;
}

class PutPackagingMaterialsPackagingIdRequest extends VendorPayload {
  PutPackagingMaterialsPackagingIdRequest({
    String? code,
    String? name,
    String? description,
    String? measurementFamily,
    String? defaultUnitCode,
    String? inventoryControlMode,
    String? status,
  }) : super({
         'code': ?code,
         'name': ?name,
         'description': ?description,
         'measurement_family': ?measurementFamily,
         'default_unit_code': ?defaultUnitCode,
         'inventory_control_mode': ?inventoryControlMode,
         'status': ?status,
       });
  PutPackagingMaterialsPackagingIdRequest.fromJson(super.json);
  String? get code => value('code') as String?;
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  String? get measurementFamily => value('measurement_family') as String?;
  String? get defaultUnitCode => value('default_unit_code') as String?;
  String? get inventoryControlMode =>
      value('inventory_control_mode') as String?;
  String? get status => value('status') as String?;
}

class PutPackagingMaterialsPackagingIdBranchesBranchIdRequest
    extends VendorPayload {
  PutPackagingMaterialsPackagingIdBranchesBranchIdRequest({
    bool? enabled,
    bool? productionEligible,
    String? inventoryControlModeOverride,
    String? inventoryItemId,
  }) : super({
         'enabled': ?enabled,
         'production_eligible': ?productionEligible,
         'inventory_control_mode_override': ?inventoryControlModeOverride,
         'inventory_item_id': ?inventoryItemId,
       });
  PutPackagingMaterialsPackagingIdBranchesBranchIdRequest.fromJson(super.json);
  bool? get enabled => value('enabled') as bool?;
  bool? get productionEligible => value('production_eligible') as bool?;
  String? get inventoryControlModeOverride =>
      value('inventory_control_mode_override') as String?;
  String? get inventoryItemId => value('inventory_item_id') as String?;
}

class PostPackagingMaterialsPackagingIdConversionsRequest
    extends VendorPayload {
  PostPackagingMaterialsPackagingIdConversionsRequest({
    String? measurementFamily,
    String? fromUnitCode,
    String? toUnitCode,
    String? conversionMode,
    int? numerator,
    int? denominator,
    String? sourceContext,
    String? status,
  }) : super({
         'measurement_family': ?measurementFamily,
         'from_unit_code': ?fromUnitCode,
         'to_unit_code': ?toUnitCode,
         'conversion_mode': ?conversionMode,
         'numerator': ?numerator,
         'denominator': ?denominator,
         'source_context': ?sourceContext,
         'status': ?status,
       });
  PostPackagingMaterialsPackagingIdConversionsRequest.fromJson(super.json);
  String? get measurementFamily => value('measurement_family') as String?;
  String? get fromUnitCode => value('from_unit_code') as String?;
  String? get toUnitCode => value('to_unit_code') as String?;
  String? get conversionMode => value('conversion_mode') as String?;
  int? get numerator => value('numerator') as int?;
  int? get denominator => value('denominator') as int?;
  String? get sourceContext => value('source_context') as String?;
  String? get status => value('status') as String?;
}

class PutPackagingMaterialsPackagingIdConversionsConversionIdRequest
    extends VendorPayload {
  PutPackagingMaterialsPackagingIdConversionsConversionIdRequest({
    String? measurementFamily,
    String? fromUnitCode,
    String? toUnitCode,
    String? conversionMode,
    int? numerator,
    int? denominator,
    String? sourceContext,
    String? status,
  }) : super({
         'measurement_family': ?measurementFamily,
         'from_unit_code': ?fromUnitCode,
         'to_unit_code': ?toUnitCode,
         'conversion_mode': ?conversionMode,
         'numerator': ?numerator,
         'denominator': ?denominator,
         'source_context': ?sourceContext,
         'status': ?status,
       });
  PutPackagingMaterialsPackagingIdConversionsConversionIdRequest.fromJson(
    super.json,
  );
  String? get measurementFamily => value('measurement_family') as String?;
  String? get fromUnitCode => value('from_unit_code') as String?;
  String? get toUnitCode => value('to_unit_code') as String?;
  String? get conversionMode => value('conversion_mode') as String?;
  int? get numerator => value('numerator') as int?;
  int? get denominator => value('denominator') as int?;
  String? get sourceContext => value('source_context') as String?;
  String? get status => value('status') as String?;
}
