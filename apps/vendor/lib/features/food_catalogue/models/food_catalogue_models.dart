// GENERATED from actual request schemas/Go JSON fields.
import '../../../core/contracts/vendor_contract.dart';

class PatchFoodItemsItemIdRequest extends VendorPayload {
  PatchFoodItemsItemIdRequest({
    String? name,
    String? description,
    int? basePriceMinor,
    List<dynamic>? dietaryCodes,
    List<dynamic>? allergenCodes,
    String? status,
    int? expectedVersion,
  }) : super({
         'name': ?name,
         'description': ?description,
         'base_price_minor': ?basePriceMinor,
         'dietary_codes': ?dietaryCodes,
         'allergen_codes': ?allergenCodes,
         'status': ?status,
         'expected_version': ?expectedVersion,
       });
  PatchFoodItemsItemIdRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  int? get basePriceMinor => value('base_price_minor') as int?;
  List<dynamic>? get dietaryCodes => value('dietary_codes') as List<dynamic>?;
  List<dynamic>? get allergenCodes => value('allergen_codes') as List<dynamic>?;
  String? get status => value('status') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostFoodItemsItemIdArchiveRequest extends VendorPayload {
  PostFoodItemsItemIdArchiveRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostFoodItemsItemIdArchiveRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PutFoodItemsItemIdAvailabilityRequest extends VendorPayload {
  PutFoodItemsItemIdAvailabilityRequest({
    String? status,
    String? reason,
    String? effectiveUntil,
    int? expectedVersion,
  }) : super({
         'status': ?status,
         'reason': ?reason,
         'effective_until': ?effectiveUntil,
         'expected_version': ?expectedVersion,
       });
  PutFoodItemsItemIdAvailabilityRequest.fromJson(super.json);
  String? get status => value('status') as String?;
  String? get reason => value('reason') as String?;
  String? get effectiveUntil => value('effective_until') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class DeleteFoodItemsItemIdAvailabilitySourceRequest extends VendorPayload {
  DeleteFoodItemsItemIdAvailabilitySourceRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  DeleteFoodItemsItemIdAvailabilitySourceRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PutFoodItemsItemIdAvailabilitySourceRequest extends VendorPayload {
  PutFoodItemsItemIdAvailabilitySourceRequest({
    String? mode,
    String? inventoryRecipeRef,
    int? expectedVersion,
  }) : super({
         'mode': ?mode,
         'inventory_recipe_ref': ?inventoryRecipeRef,
         'expected_version': ?expectedVersion,
       });
  PutFoodItemsItemIdAvailabilitySourceRequest.fromJson(super.json);
  String? get mode => value('mode') as String?;
  String? get inventoryRecipeRef => value('inventory_recipe_ref') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class DeleteFoodItemsItemIdBranchOverrideRequest extends VendorPayload {
  DeleteFoodItemsItemIdBranchOverrideRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  DeleteFoodItemsItemIdBranchOverrideRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PutFoodItemsItemIdBranchOverrideRequest extends VendorPayload {
  PutFoodItemsItemIdBranchOverrideRequest({
    int? basePriceOverrideMinor,
    bool? clearBasePriceOverride,
    int? prepMinMinutes,
    int? prepMaxMinutes,
    bool? clearPrepOverride,
    int? expectedVersion,
  }) : super({
         'base_price_override_minor': ?basePriceOverrideMinor,
         'clear_base_price_override': ?clearBasePriceOverride,
         'prep_min_minutes': ?prepMinMinutes,
         'prep_max_minutes': ?prepMaxMinutes,
         'clear_prep_override': ?clearPrepOverride,
         'expected_version': ?expectedVersion,
       });
  PutFoodItemsItemIdBranchOverrideRequest.fromJson(super.json);
  int? get basePriceOverrideMinor => value('base_price_override_minor') as int?;
  bool? get clearBasePriceOverride =>
      value('clear_base_price_override') as bool?;
  int? get prepMinMinutes => value('prep_min_minutes') as int?;
  int? get prepMaxMinutes => value('prep_max_minutes') as int?;
  bool? get clearPrepOverride => value('clear_prep_override') as bool?;
  int? get expectedVersion => value('expected_version') as int?;
}

class DeleteFoodItemsItemIdLogisticsProfileRequest extends VendorPayload {
  DeleteFoodItemsItemIdLogisticsProfileRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  DeleteFoodItemsItemIdLogisticsProfileRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PutFoodItemsItemIdLogisticsProfileRequest extends VendorPayload {
  PutFoodItemsItemIdLogisticsProfileRequest({
    String? temperatureClass,
    String? packageSizeClass,
    bool? spillRisk,
    bool? fragile,
    int? approxWeightGrams,
    List<dynamic>? vehicleHints,
    int? expectedVersion,
  }) : super({
         'temperature_class': ?temperatureClass,
         'package_size_class': ?packageSizeClass,
         'spill_risk': ?spillRisk,
         'fragile': ?fragile,
         'approx_weight_grams': ?approxWeightGrams,
         'vehicle_hints': ?vehicleHints,
         'expected_version': ?expectedVersion,
       });
  PutFoodItemsItemIdLogisticsProfileRequest.fromJson(super.json);
  String? get temperatureClass => value('temperature_class') as String?;
  String? get packageSizeClass => value('package_size_class') as String?;
  bool? get spillRisk => value('spill_risk') as bool?;
  bool? get fragile => value('fragile') as bool?;
  int? get approxWeightGrams => value('approx_weight_grams') as int?;
  List<dynamic>? get vehicleHints => value('vehicle_hints') as List<dynamic>?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PutFoodItemsItemIdMediaRequest extends VendorPayload {
  PutFoodItemsItemIdMediaRequest({
    List<JsonObject>? media,
    int? expectedVersion,
  }) : super({'media': ?media, 'expected_version': ?expectedVersion});
  PutFoodItemsItemIdMediaRequest.fromJson(super.json);
  List<dynamic>? get media => value('media') as List<dynamic>?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostFoodItemsItemIdModifierGroupsReorderRequest extends VendorPayload {
  PostFoodItemsItemIdModifierGroupsReorderRequest({List<dynamic>? orderedIds})
    : super({'ordered_ids': ?orderedIds});
  PostFoodItemsItemIdModifierGroupsReorderRequest.fromJson(super.json);
  List<dynamic>? get orderedIds => value('ordered_ids') as List<dynamic>?;
}

class DeleteFoodItemsItemIdPrepProfileRequest extends VendorPayload {
  DeleteFoodItemsItemIdPrepProfileRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  DeleteFoodItemsItemIdPrepProfileRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PutFoodItemsItemIdPrepProfileRequest extends VendorPayload {
  PutFoodItemsItemIdPrepProfileRequest({
    int? prepMinMinutes,
    int? prepMaxMinutes,
    int? expectedVersion,
  }) : super({
         'prep_min_minutes': ?prepMinMinutes,
         'prep_max_minutes': ?prepMaxMinutes,
         'expected_version': ?expectedVersion,
       });
  PutFoodItemsItemIdPrepProfileRequest.fromJson(super.json);
  int? get prepMinMinutes => value('prep_min_minutes') as int?;
  int? get prepMaxMinutes => value('prep_max_minutes') as int?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostFoodItemsItemIdPricingResolveRequest extends VendorPayload {
  PostFoodItemsItemIdPricingResolveRequest({
    String? variantRef,
    List<dynamic>? modifierRefs,
  }) : super({'variant_ref': ?variantRef, 'modifier_refs': ?modifierRefs});
  PostFoodItemsItemIdPricingResolveRequest.fromJson(super.json);
  String? get variantRef => value('variant_ref') as String?;
  List<dynamic>? get modifierRefs => value('modifier_refs') as List<dynamic>?;
}

class PostFoodItemsItemIdVariantsRequest extends VendorPayload {
  PostFoodItemsItemIdVariantsRequest({String? name, int? priceOverrideMinor})
    : super({'name': ?name, 'price_override_minor': ?priceOverrideMinor});
  PostFoodItemsItemIdVariantsRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  int? get priceOverrideMinor => value('price_override_minor') as int?;
}

class PostFoodItemsItemIdVariantsReorderRequest extends VendorPayload {
  PostFoodItemsItemIdVariantsReorderRequest({List<dynamic>? orderedIds})
    : super({'ordered_ids': ?orderedIds});
  PostFoodItemsItemIdVariantsReorderRequest.fromJson(super.json);
  List<dynamic>? get orderedIds => value('ordered_ids') as List<dynamic>?;
}

class PatchFoodItemsItemIdVariantsVariantIdRequest extends VendorPayload {
  PatchFoodItemsItemIdVariantsVariantIdRequest({
    String? name,
    int? priceOverrideMinor,
    bool? clearPriceOverride,
    int? expectedVersion,
  }) : super({
         'name': ?name,
         'price_override_minor': ?priceOverrideMinor,
         'clear_price_override': ?clearPriceOverride,
         'expected_version': ?expectedVersion,
       });
  PatchFoodItemsItemIdVariantsVariantIdRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  int? get priceOverrideMinor => value('price_override_minor') as int?;
  bool? get clearPriceOverride => value('clear_price_override') as bool?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostFoodItemsItemIdVariantsVariantIdArchiveRequest extends VendorPayload {
  PostFoodItemsItemIdVariantsVariantIdArchiveRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostFoodItemsItemIdVariantsVariantIdArchiveRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostFoodMenusRequest extends VendorPayload {
  PostFoodMenusRequest({
    String? branchId,
    String? capabilityType,
    String? name,
    String? description,
  }) : super({
         'branch_id': ?branchId,
         'capability_type': ?capabilityType,
         'name': ?name,
         'description': ?description,
       });
  PostFoodMenusRequest.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  String? get capabilityType => value('capability_type') as String?;
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
}

class PatchFoodMenusMenuIdRequest extends VendorPayload {
  PatchFoodMenusMenuIdRequest({
    String? name,
    String? description,
    int? expectedVersion,
  }) : super({
         'name': ?name,
         'description': ?description,
         'expected_version': ?expectedVersion,
       });
  PatchFoodMenusMenuIdRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostFoodMenusMenuIdActivateRequest extends VendorPayload {
  PostFoodMenusMenuIdActivateRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostFoodMenusMenuIdActivateRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostFoodMenusMenuIdArchiveRequest extends VendorPayload {
  PostFoodMenusMenuIdArchiveRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostFoodMenusMenuIdArchiveRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostFoodMenusMenuIdPauseRequest extends VendorPayload {
  PostFoodMenusMenuIdPauseRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostFoodMenusMenuIdPauseRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostFoodMenusMenuIdSchedulesRequest extends VendorPayload {
  PostFoodMenusMenuIdSchedulesRequest({
    String? label,
    List<dynamic>? daysOfWeek,
    String? startLocalTime,
    String? endLocalTime,
    String? effectiveStartDate,
    String? effectiveEndDate,
  }) : super({
         'label': ?label,
         'days_of_week': ?daysOfWeek,
         'start_local_time': ?startLocalTime,
         'end_local_time': ?endLocalTime,
         'effective_start_date': ?effectiveStartDate,
         'effective_end_date': ?effectiveEndDate,
       });
  PostFoodMenusMenuIdSchedulesRequest.fromJson(super.json);
  String? get label => value('label') as String?;
  List<dynamic>? get daysOfWeek => value('days_of_week') as List<dynamic>?;
  String? get startLocalTime => value('start_local_time') as String?;
  String? get endLocalTime => value('end_local_time') as String?;
  String? get effectiveStartDate => value('effective_start_date') as String?;
  String? get effectiveEndDate => value('effective_end_date') as String?;
}

class PatchFoodMenusMenuIdSchedulesScheduleIdRequest extends VendorPayload {
  PatchFoodMenusMenuIdSchedulesScheduleIdRequest({
    String? label,
    List<dynamic>? daysOfWeek,
    String? startLocalTime,
    String? endLocalTime,
    String? effectiveStartDate,
    String? effectiveEndDate,
    bool? clearEffectiveStartDate,
    bool? clearEffectiveEndDate,
    int? expectedVersion,
  }) : super({
         'label': ?label,
         'days_of_week': ?daysOfWeek,
         'start_local_time': ?startLocalTime,
         'end_local_time': ?endLocalTime,
         'effective_start_date': ?effectiveStartDate,
         'effective_end_date': ?effectiveEndDate,
         'clear_effective_start_date': ?clearEffectiveStartDate,
         'clear_effective_end_date': ?clearEffectiveEndDate,
         'expected_version': ?expectedVersion,
       });
  PatchFoodMenusMenuIdSchedulesScheduleIdRequest.fromJson(super.json);
  String? get label => value('label') as String?;
  List<dynamic>? get daysOfWeek => value('days_of_week') as List<dynamic>?;
  String? get startLocalTime => value('start_local_time') as String?;
  String? get endLocalTime => value('end_local_time') as String?;
  String? get effectiveStartDate => value('effective_start_date') as String?;
  String? get effectiveEndDate => value('effective_end_date') as String?;
  bool? get clearEffectiveStartDate =>
      value('clear_effective_start_date') as bool?;
  bool? get clearEffectiveEndDate => value('clear_effective_end_date') as bool?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostFoodMenusMenuIdSchedulesScheduleIdArchiveRequest
    extends VendorPayload {
  PostFoodMenusMenuIdSchedulesScheduleIdArchiveRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostFoodMenusMenuIdSchedulesScheduleIdArchiveRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostFoodMenusMenuIdSectionsRequest extends VendorPayload {
  PostFoodMenusMenuIdSectionsRequest({String? name, String? description})
    : super({'name': ?name, 'description': ?description});
  PostFoodMenusMenuIdSectionsRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
}

class PostFoodMenusMenuIdSectionsReorderRequest extends VendorPayload {
  PostFoodMenusMenuIdSectionsReorderRequest({List<dynamic>? orderedIds})
    : super({'ordered_ids': ?orderedIds});
  PostFoodMenusMenuIdSectionsReorderRequest.fromJson(super.json);
  List<dynamic>? get orderedIds => value('ordered_ids') as List<dynamic>?;
}

class PatchFoodMenusMenuIdSectionsSectionIdRequest extends VendorPayload {
  PatchFoodMenusMenuIdSectionsSectionIdRequest({
    String? name,
    String? description,
    int? expectedVersion,
  }) : super({
         'name': ?name,
         'description': ?description,
         'expected_version': ?expectedVersion,
       });
  PatchFoodMenusMenuIdSectionsSectionIdRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostFoodMenusMenuIdSectionsSectionIdArchiveRequest extends VendorPayload {
  PostFoodMenusMenuIdSectionsSectionIdArchiveRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostFoodMenusMenuIdSectionsSectionIdArchiveRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostFoodMenusMenuIdSectionsSectionIdItemsRequest extends VendorPayload {
  PostFoodMenusMenuIdSectionsSectionIdItemsRequest({
    String? name,
    String? description,
    int? basePriceMinor,
    List<dynamic>? dietaryCodes,
    List<dynamic>? allergenCodes,
  }) : super({
         'name': ?name,
         'description': ?description,
         'base_price_minor': ?basePriceMinor,
         'dietary_codes': ?dietaryCodes,
         'allergen_codes': ?allergenCodes,
       });
  PostFoodMenusMenuIdSectionsSectionIdItemsRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  int? get basePriceMinor => value('base_price_minor') as int?;
  List<dynamic>? get dietaryCodes => value('dietary_codes') as List<dynamic>?;
  List<dynamic>? get allergenCodes => value('allergen_codes') as List<dynamic>?;
}

class PostFoodMenusMenuIdSectionsSectionIdItemsReorderRequest
    extends VendorPayload {
  PostFoodMenusMenuIdSectionsSectionIdItemsReorderRequest({
    List<dynamic>? orderedIds,
  }) : super({'ordered_ids': ?orderedIds});
  PostFoodMenusMenuIdSectionsSectionIdItemsReorderRequest.fromJson(super.json);
  List<dynamic>? get orderedIds => value('ordered_ids') as List<dynamic>?;
}

class PostFoodModifierGroupsRequest extends VendorPayload {
  PostFoodModifierGroupsRequest({
    String? name,
    String? description,
    int? minSelect,
    int? maxSelect,
  }) : super({
         'name': ?name,
         'description': ?description,
         'min_select': ?minSelect,
         'max_select': ?maxSelect,
       });
  PostFoodModifierGroupsRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  int? get minSelect => value('min_select') as int?;
  int? get maxSelect => value('max_select') as int?;
}

class PatchFoodModifierGroupsGroupIdRequest extends VendorPayload {
  PatchFoodModifierGroupsGroupIdRequest({
    String? name,
    String? description,
    int? minSelect,
    int? maxSelect,
    int? expectedVersion,
  }) : super({
         'name': ?name,
         'description': ?description,
         'min_select': ?minSelect,
         'max_select': ?maxSelect,
         'expected_version': ?expectedVersion,
       });
  PatchFoodModifierGroupsGroupIdRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  int? get minSelect => value('min_select') as int?;
  int? get maxSelect => value('max_select') as int?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostFoodModifierGroupsGroupIdArchiveRequest extends VendorPayload {
  PostFoodModifierGroupsGroupIdArchiveRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostFoodModifierGroupsGroupIdArchiveRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostFoodModifierGroupsGroupIdModifiersRequest extends VendorPayload {
  PostFoodModifierGroupsGroupIdModifiersRequest({
    String? name,
    int? priceDeltaMinor,
  }) : super({'name': ?name, 'price_delta_minor': ?priceDeltaMinor});
  PostFoodModifierGroupsGroupIdModifiersRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  int? get priceDeltaMinor => value('price_delta_minor') as int?;
}

class PostFoodModifierGroupsGroupIdModifiersReorderRequest
    extends VendorPayload {
  PostFoodModifierGroupsGroupIdModifiersReorderRequest({
    List<dynamic>? orderedIds,
  }) : super({'ordered_ids': ?orderedIds});
  PostFoodModifierGroupsGroupIdModifiersReorderRequest.fromJson(super.json);
  List<dynamic>? get orderedIds => value('ordered_ids') as List<dynamic>?;
}

class PatchFoodModifierGroupsGroupIdModifiersModifierIdRequest
    extends VendorPayload {
  PatchFoodModifierGroupsGroupIdModifiersModifierIdRequest({
    String? name,
    int? priceDeltaMinor,
    int? expectedVersion,
  }) : super({
         'name': ?name,
         'price_delta_minor': ?priceDeltaMinor,
         'expected_version': ?expectedVersion,
       });
  PatchFoodModifierGroupsGroupIdModifiersModifierIdRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  int? get priceDeltaMinor => value('price_delta_minor') as int?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostFoodModifierGroupsGroupIdModifiersModifierIdArchiveRequest
    extends VendorPayload {
  PostFoodModifierGroupsGroupIdModifiersModifierIdArchiveRequest({
    int? expectedVersion,
  }) : super({'expected_version': ?expectedVersion});
  PostFoodModifierGroupsGroupIdModifiersModifierIdArchiveRequest.fromJson(
    super.json,
  );
  int? get expectedVersion => value('expected_version') as int?;
}

class FoodCatalogueSection9aFoodCatalogueMenu extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueMenu.fromJson(super.json);
  String? get id => value('id') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get capabilityType => value('capability_type') as String?;
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  String? get status => value('status') as String?;
  int? get version => value('version') as int?;
  String? get createdAt => value('created_at') as String?;
  String? get updatedAt => value('updated_at') as String?;
  String? get archivedAt => value('archived_at') as String?;
}

class FoodCatalogueSection9aFoodCatalogueMenuPage extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueMenuPage.fromJson(super.json);
  List<dynamic>? get menus => value('menus') as List<dynamic>?;
  JsonObject? get pagination =>
      value('pagination') == null ? null : jsonObject(value('pagination'));
}

class FoodCatalogueSection9aFoodCataloguePagination extends VendorPayload {
  FoodCatalogueSection9aFoodCataloguePagination.fromJson(super.json);
  int? get limit => value('limit') as int?;
  int? get offset => value('offset') as int?;
  bool? get hasMore => value('has_more') as bool?;
}

class FoodCatalogueSection9aFoodCatalogueSection extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueSection.fromJson(super.json);
  String? get id => value('id') as String?;
  String? get menuId => value('menu_id') as String?;
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  int? get position => value('position') as int?;
  String? get status => value('status') as String?;
  int? get version => value('version') as int?;
  String? get createdAt => value('created_at') as String?;
  String? get updatedAt => value('updated_at') as String?;
  String? get archivedAt => value('archived_at') as String?;
}

class FoodCatalogueSection9aFoodCatalogueSectionList extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueSectionList.fromJson(super.json);
  List<dynamic>? get sections => value('sections') as List<dynamic>?;
}

class FoodCatalogueSection9aFoodCatalogueItemMedia extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueItemMedia.fromJson(super.json);
  String? get mediaRef => value('media_ref') as String?;
  int? get position => value('position') as int?;
  bool? get isPrimary => value('is_primary') as bool?;
}

class FoodCatalogueSection9aFoodCatalogueItem extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueItem.fromJson(super.json);
  String? get id => value('id') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get menuId => value('menu_id') as String?;
  String? get sectionId => value('section_id') as String?;
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  int? get basePriceMinor => value('base_price_minor') as int?;
  String? get currency => value('currency') as String?;
  List<dynamic>? get dietaryCodes => value('dietary_codes') as List<dynamic>?;
  List<dynamic>? get allergenCodes => value('allergen_codes') as List<dynamic>?;
  List<dynamic>? get media => value('media') as List<dynamic>?;
  String? get status => value('status') as String?;
  int? get position => value('position') as int?;
  int? get version => value('version') as int?;
  String? get createdAt => value('created_at') as String?;
  String? get updatedAt => value('updated_at') as String?;
  String? get archivedAt => value('archived_at') as String?;
}

class FoodCatalogueSection9aFoodCatalogueItemPage extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueItemPage.fromJson(super.json);
  List<dynamic>? get items => value('items') as List<dynamic>?;
  JsonObject? get pagination =>
      value('pagination') == null ? null : jsonObject(value('pagination'));
}

class FoodCatalogueSection9aFoodCatalogueItemOrder extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueItemOrder.fromJson(super.json);
  List<dynamic>? get orderedItemIds =>
      value('ordered_item_ids') as List<dynamic>?;
}

class FoodCatalogueSection9aFoodCatalogueMediaList extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueMediaList.fromJson(super.json);
  List<dynamic>? get media => value('media') as List<dynamic>?;
}

class FoodCatalogueSection9aFoodCatalogueMediaReplaceResult
    extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueMediaReplaceResult.fromJson(super.json);
  String? get itemId => value('item_id') as String?;
  int? get version => value('version') as int?;
  List<dynamic>? get media => value('media') as List<dynamic>?;
}

class FoodCatalogueSection9aFoodCatalogueVariant extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueVariant.fromJson(super.json);
  String? get id => value('id') as String?;
  String? get itemId => value('item_id') as String?;
  String? get name => value('name') as String?;
  int? get priceOverrideMinor => value('price_override_minor') as int?;
  bool? get inheritsBasePrice => value('inherits_base_price') as bool?;
  int? get position => value('position') as int?;
  String? get status => value('status') as String?;
  int? get version => value('version') as int?;
  String? get createdAt => value('created_at') as String?;
  String? get updatedAt => value('updated_at') as String?;
  String? get archivedAt => value('archived_at') as String?;
}

class FoodCatalogueSection9aFoodCatalogueVariantList extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueVariantList.fromJson(super.json);
  List<dynamic>? get variants => value('variants') as List<dynamic>?;
}

class FoodCatalogueSection9aFoodCatalogueVariantOrder extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueVariantOrder.fromJson(super.json);
  List<dynamic>? get orderedVariantIds =>
      value('ordered_variant_ids') as List<dynamic>?;
}

class FoodCatalogueSection9aFoodCatalogueModifierGroup extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueModifierGroup.fromJson(super.json);
  String? get id => value('id') as String?;
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  int? get minSelect => value('min_select') as int?;
  int? get maxSelect => value('max_select') as int?;
  bool? get requiredValue => value('required') as bool?;
  String? get status => value('status') as String?;
  int? get version => value('version') as int?;
  String? get createdAt => value('created_at') as String?;
  String? get updatedAt => value('updated_at') as String?;
  String? get archivedAt => value('archived_at') as String?;
}

class FoodCatalogueSection9aFoodCatalogueModifierGroupPage
    extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueModifierGroupPage.fromJson(super.json);
  List<dynamic>? get modifierGroups =>
      value('modifier_groups') as List<dynamic>?;
  JsonObject? get pagination =>
      value('pagination') == null ? null : jsonObject(value('pagination'));
}

class FoodCatalogueSection9aFoodCatalogueModifier extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueModifier.fromJson(super.json);
  String? get id => value('id') as String?;
  String? get modifierGroupId => value('modifier_group_id') as String?;
  String? get name => value('name') as String?;
  int? get priceDeltaMinor => value('price_delta_minor') as int?;
  int? get position => value('position') as int?;
  String? get status => value('status') as String?;
  int? get version => value('version') as int?;
  String? get createdAt => value('created_at') as String?;
  String? get updatedAt => value('updated_at') as String?;
  String? get archivedAt => value('archived_at') as String?;
}

class FoodCatalogueSection9aFoodCatalogueModifierList extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueModifierList.fromJson(super.json);
  List<dynamic>? get modifiers => value('modifiers') as List<dynamic>?;
}

class FoodCatalogueSection9aFoodCatalogueModifierOrder extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueModifierOrder.fromJson(super.json);
  List<dynamic>? get orderedModifierIds =>
      value('ordered_modifier_ids') as List<dynamic>?;
}

class FoodCatalogueSection9aFoodCatalogueAttachment extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueAttachment.fromJson(super.json);
  String? get itemId => value('item_id') as String?;
  String? get modifierGroupId => value('modifier_group_id') as String?;
  int? get position => value('position') as int?;
  int? get version => value('version') as int?;
}

class FoodCatalogueSection9aFoodCatalogueAttachedGroup extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueAttachedGroup.fromJson(super.json);
  String? get modifierGroupId => value('modifier_group_id') as String?;
  int? get position => value('position') as int?;
  int? get version => value('version') as int?;
  JsonObject? get group =>
      value('group') == null ? null : jsonObject(value('group'));
}

class FoodCatalogueSection9aFoodCatalogueAttachedGroupList
    extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueAttachedGroupList.fromJson(super.json);
  List<dynamic>? get modifierGroups =>
      value('modifier_groups') as List<dynamic>?;
}

class FoodCatalogueSection9aFoodCatalogueAttachedGroupOrder
    extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueAttachedGroupOrder.fromJson(super.json);
  List<dynamic>? get orderedModifierGroupIds =>
      value('ordered_modifier_group_ids') as List<dynamic>?;
}

class FoodCatalogueSection9aFoodCatalogueDetachResult extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueDetachResult.fromJson(super.json);
  String? get itemId => value('item_id') as String?;
  String? get modifierGroupId => value('modifier_group_id') as String?;
  bool? get detached => value('detached') as bool?;
}

class FoodCatalogueSection9aFoodCatalogueCreateMenuRequest
    extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueCreateMenuRequest.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  String? get capabilityType => value('capability_type') as String?;
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
}

class FoodCatalogueSection9aFoodCatalogueUpdateMenuRequest
    extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueUpdateMenuRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class FoodCatalogueSection9aFoodCatalogueVersionedRequest
    extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueVersionedRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class FoodCatalogueSection9aFoodCatalogueReorderRequest extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueReorderRequest.fromJson(super.json);
  List<dynamic>? get orderedIds => value('ordered_ids') as List<dynamic>?;
}

class FoodCatalogueSection9aFoodCatalogueCreateSectionRequest
    extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueCreateSectionRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
}

class FoodCatalogueSection9aFoodCatalogueUpdateSectionRequest
    extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueUpdateSectionRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class FoodCatalogueSection9aFoodCatalogueCreateItemRequest
    extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueCreateItemRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  int? get basePriceMinor => value('base_price_minor') as int?;
  List<dynamic>? get dietaryCodes => value('dietary_codes') as List<dynamic>?;
  List<dynamic>? get allergenCodes => value('allergen_codes') as List<dynamic>?;
}

class FoodCatalogueSection9aFoodCatalogueUpdateItemRequest
    extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueUpdateItemRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  int? get basePriceMinor => value('base_price_minor') as int?;
  List<dynamic>? get dietaryCodes => value('dietary_codes') as List<dynamic>?;
  List<dynamic>? get allergenCodes => value('allergen_codes') as List<dynamic>?;
  String? get status => value('status') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class FoodCatalogueSection9aFoodCatalogueReplaceMediaRequest
    extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueReplaceMediaRequest.fromJson(super.json);
  List<dynamic>? get media => value('media') as List<dynamic>?;
  int? get expectedVersion => value('expected_version') as int?;
}

class FoodCatalogueSection9aFoodCatalogueCreateVariantRequest
    extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueCreateVariantRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  int? get priceOverrideMinor => value('price_override_minor') as int?;
}

class FoodCatalogueSection9aFoodCatalogueUpdateVariantRequest
    extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueUpdateVariantRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  int? get priceOverrideMinor => value('price_override_minor') as int?;
  bool? get clearPriceOverride => value('clear_price_override') as bool?;
  int? get expectedVersion => value('expected_version') as int?;
}

class FoodCatalogueSection9aFoodCatalogueCreateModifierGroupRequest
    extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueCreateModifierGroupRequest.fromJson(
    super.json,
  );
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  int? get minSelect => value('min_select') as int?;
  int? get maxSelect => value('max_select') as int?;
}

class FoodCatalogueSection9aFoodCatalogueUpdateModifierGroupRequest
    extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueUpdateModifierGroupRequest.fromJson(
    super.json,
  );
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  int? get minSelect => value('min_select') as int?;
  int? get maxSelect => value('max_select') as int?;
  int? get expectedVersion => value('expected_version') as int?;
}

class FoodCatalogueSection9aFoodCatalogueCreateModifierRequest
    extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueCreateModifierRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  int? get priceDeltaMinor => value('price_delta_minor') as int?;
}

class FoodCatalogueSection9aFoodCatalogueUpdateModifierRequest
    extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueUpdateModifierRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  int? get priceDeltaMinor => value('price_delta_minor') as int?;
  int? get expectedVersion => value('expected_version') as int?;
}

class FoodCatalogueSection9aFoodCatalogueError extends VendorPayload {
  FoodCatalogueSection9aFoodCatalogueError.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get error =>
      value('error') == null ? null : jsonObject(value('error'));
}
