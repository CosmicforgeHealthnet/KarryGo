// GENERATED from actual request schemas/Go JSON fields.
import '../../../core/contracts/vendor_contract.dart';

class PutShopBranchesBranchIdInventoryStockBasesRequest extends VendorPayload {
  PutShopBranchesBranchIdInventoryStockBasesRequest({
    String? listingId,
    String? measureId,
    String? consumptionMode,
    String? allocationPolicy,
    bool? lotTracking,
    int? expectedVersion,
  }) : super({
         'listing_id': ?listingId,
         'measure_id': ?measureId,
         'consumption_mode': ?consumptionMode,
         'allocation_policy': ?allocationPolicy,
         'lot_tracking': ?lotTracking,
         'expected_version': ?expectedVersion,
       });
  PutShopBranchesBranchIdInventoryStockBasesRequest.fromJson(super.json);
  String? get listingId => value('listing_id') as String?;
  String? get measureId => value('measure_id') as String?;
  String? get consumptionMode => value('consumption_mode') as String?;
  String? get allocationPolicy => value('allocation_policy') as String?;
  bool? get lotTracking => value('lot_tracking') as bool?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostShopBranchesBranchIdInventoryStocktakesRequest extends VendorPayload {
  PostShopBranchesBranchIdInventoryStocktakesRequest({
    String? stocktakeType,
    List<dynamic>? stockBasisIds,
  }) : super({
         'stocktake_type': ?stocktakeType,
         'stock_basis_ids': ?stockBasisIds,
       });
  PostShopBranchesBranchIdInventoryStocktakesRequest.fromJson(super.json);
  String? get stocktakeType => value('stocktake_type') as String?;
  List<dynamic>? get stockBasisIds =>
      value('stock_basis_ids') as List<dynamic>?;
}

class PostShopBranchesBranchIdListingsRequest extends VendorPayload {
  PostShopBranchesBranchIdListingsRequest({
    String? variantId,
    String? operatingModel,
    bool? retailEnabled,
    bool? wholesaleEnabled,
    int? retailOverride,
    int? wholesaleOverride,
    int? minimumOrderQty,
    int? orderMultiple,
    int? maximumOrderQty,
    String? preorderPolicy,
    String? backorderPolicy,
    bool? autoHideUnavailable,
    bool? logisticsAvailable,
    List<dynamic>? sellUnitIds,
  }) : super({
         'variant_id': ?variantId,
         'operating_model': ?operatingModel,
         'retail_enabled': ?retailEnabled,
         'wholesale_enabled': ?wholesaleEnabled,
         'retail_override': ?retailOverride,
         'wholesale_override': ?wholesaleOverride,
         'minimum_order_qty': ?minimumOrderQty,
         'order_multiple': ?orderMultiple,
         'maximum_order_qty': ?maximumOrderQty,
         'preorder_policy': ?preorderPolicy,
         'backorder_policy': ?backorderPolicy,
         'auto_hide_unavailable': ?autoHideUnavailable,
         'logistics_available': ?logisticsAvailable,
         'sell_unit_ids': ?sellUnitIds,
       });
  PostShopBranchesBranchIdListingsRequest.fromJson(super.json);
  String? get variantId => value('variant_id') as String?;
  String? get operatingModel => value('operating_model') as String?;
  bool? get retailEnabled => value('retail_enabled') as bool?;
  bool? get wholesaleEnabled => value('wholesale_enabled') as bool?;
  int? get retailOverride => value('retail_override') as int?;
  int? get wholesaleOverride => value('wholesale_override') as int?;
  int? get minimumOrderQty => value('minimum_order_qty') as int?;
  int? get orderMultiple => value('order_multiple') as int?;
  int? get maximumOrderQty => value('maximum_order_qty') as int?;
  String? get preorderPolicy => value('preorder_policy') as String?;
  String? get backorderPolicy => value('backorder_policy') as String?;
  bool? get autoHideUnavailable => value('auto_hide_unavailable') as bool?;
  bool? get logisticsAvailable => value('logistics_available') as bool?;
  List<dynamic>? get sellUnitIds => value('sell_unit_ids') as List<dynamic>?;
}

class PutShopBranchesBranchIdOperatingModelsRequest extends VendorPayload {
  PutShopBranchesBranchIdOperatingModelsRequest({
    List<dynamic>? models,
    String? primaryModel,
    int? expectedVersion,
  }) : super({
         'models': ?models,
         'primary_model': ?primaryModel,
         'expected_version': ?expectedVersion,
       });
  PutShopBranchesBranchIdOperatingModelsRequest.fromJson(super.json);
  List<dynamic>? get models => value('models') as List<dynamic>?;
  String? get primaryModel => value('primary_model') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostShopBranchesBranchIdOperatingModelsArchiveRequest
    extends VendorPayload {
  PostShopBranchesBranchIdOperatingModelsArchiveRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostShopBranchesBranchIdOperatingModelsArchiveRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostShopBulkOperationsRequest extends VendorPayload {
  PostShopBulkOperationsRequest({
    String? action,
    String? actualMeasureId,
    int? actualQuantity,
    String? allocationPolicy,
    String? availableFrom,
    String? batchNumber,
    String? bestBeforeAt,
    String? branchId,
    String? consumptionMode,
    int? countedQuantity,
    String? cutoffAt,
    int? damagedQuantity,
    String? destinationBranchId,
    String? destinationStockBasisId,
    bool? dryRun,
    int? expectedCatalogueRevision,
    int? expectedInventoryRevision,
    String? expectedRestockAt,
    int? expectedRevision,
    int? expectedVersion,
    String? expiresAt,
    int? expiringSoonHours,
    int? leadTimeDays,
    String? lineId,
    List<dynamic>? lines,
    String? listingId,
    String? listingRef,
    String? lotNumber,
    bool? lotTracking,
    String? manufacturedAt,
    int? maximumOutstandingQuantity,
    int? maximumPerRequest,
    String? measureId,
    String? measuredAt,
    String? measuredBy,
    int? minimumQuantity,
    int? missingQuantity,
    String? mode,
    String? note,
    String? notes,
    String? operationType,
    int? quantity,
    int? quantityDelta,
    String? reasonCode,
    String? receivedAt,
    int? receivedQuantity,
    String? reservationLineId,
    List<dynamic>? rows,
    String? salesMode,
    String? sellUnitRef,
    String? sourceBranchId,
    String? sourceRef,
    String? sourceStockBasisId,
    String? sourceType,
    String? stockBasisId,
    List<dynamic>? stockBasisIds,
    String? stocktakeType,
    String? supplierRef,
    int? targetQuantity,
    int? unitCostMinor,
    String? variantRef,
  }) : super({
         'action': ?action,
         'actual_measure_id': ?actualMeasureId,
         'actual_quantity': ?actualQuantity,
         'allocation_policy': ?allocationPolicy,
         'available_from': ?availableFrom,
         'batch_number': ?batchNumber,
         'best_before_at': ?bestBeforeAt,
         'branch_id': ?branchId,
         'consumption_mode': ?consumptionMode,
         'counted_quantity': ?countedQuantity,
         'cutoff_at': ?cutoffAt,
         'damaged_quantity': ?damagedQuantity,
         'destination_branch_id': ?destinationBranchId,
         'destination_stock_basis_id': ?destinationStockBasisId,
         'dry_run': ?dryRun,
         'expected_catalogue_revision': ?expectedCatalogueRevision,
         'expected_inventory_revision': ?expectedInventoryRevision,
         'expected_restock_at': ?expectedRestockAt,
         'expected_revision': ?expectedRevision,
         'expected_version': ?expectedVersion,
         'expires_at': ?expiresAt,
         'expiring_soon_hours': ?expiringSoonHours,
         'lead_time_days': ?leadTimeDays,
         'line_id': ?lineId,
         'lines': ?lines,
         'listing_id': ?listingId,
         'listing_ref': ?listingRef,
         'lot_number': ?lotNumber,
         'lot_tracking': ?lotTracking,
         'manufactured_at': ?manufacturedAt,
         'maximum_outstanding_quantity': ?maximumOutstandingQuantity,
         'maximum_per_request': ?maximumPerRequest,
         'measure_id': ?measureId,
         'measured_at': ?measuredAt,
         'measured_by': ?measuredBy,
         'minimum_quantity': ?minimumQuantity,
         'missing_quantity': ?missingQuantity,
         'mode': ?mode,
         'note': ?note,
         'notes': ?notes,
         'operation_type': ?operationType,
         'quantity': ?quantity,
         'quantity_delta': ?quantityDelta,
         'reason_code': ?reasonCode,
         'received_at': ?receivedAt,
         'received_quantity': ?receivedQuantity,
         'reservation_line_id': ?reservationLineId,
         'rows': ?rows,
         'sales_mode': ?salesMode,
         'sell_unit_ref': ?sellUnitRef,
         'source_branch_id': ?sourceBranchId,
         'source_ref': ?sourceRef,
         'source_stock_basis_id': ?sourceStockBasisId,
         'source_type': ?sourceType,
         'stock_basis_id': ?stockBasisId,
         'stock_basis_ids': ?stockBasisIds,
         'stocktake_type': ?stocktakeType,
         'supplier_ref': ?supplierRef,
         'target_quantity': ?targetQuantity,
         'unit_cost_minor': ?unitCostMinor,
         'variant_ref': ?variantRef,
       });
  PostShopBulkOperationsRequest.fromJson(super.json);
  String? get action => value('action') as String?;
  String? get actualMeasureId => value('actual_measure_id') as String?;
  int? get actualQuantity => value('actual_quantity') as int?;
  String? get allocationPolicy => value('allocation_policy') as String?;
  String? get availableFrom => value('available_from') as String?;
  String? get batchNumber => value('batch_number') as String?;
  String? get bestBeforeAt => value('best_before_at') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get consumptionMode => value('consumption_mode') as String?;
  int? get countedQuantity => value('counted_quantity') as int?;
  String? get cutoffAt => value('cutoff_at') as String?;
  int? get damagedQuantity => value('damaged_quantity') as int?;
  String? get destinationBranchId => value('destination_branch_id') as String?;
  String? get destinationStockBasisId =>
      value('destination_stock_basis_id') as String?;
  bool? get dryRun => value('dry_run') as bool?;
  int? get expectedCatalogueRevision =>
      value('expected_catalogue_revision') as int?;
  int? get expectedInventoryRevision =>
      value('expected_inventory_revision') as int?;
  String? get expectedRestockAt => value('expected_restock_at') as String?;
  int? get expectedRevision => value('expected_revision') as int?;
  int? get expectedVersion => value('expected_version') as int?;
  String? get expiresAt => value('expires_at') as String?;
  int? get expiringSoonHours => value('expiring_soon_hours') as int?;
  int? get leadTimeDays => value('lead_time_days') as int?;
  String? get lineId => value('line_id') as String?;
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
  String? get listingId => value('listing_id') as String?;
  String? get listingRef => value('listing_ref') as String?;
  String? get lotNumber => value('lot_number') as String?;
  bool? get lotTracking => value('lot_tracking') as bool?;
  String? get manufacturedAt => value('manufactured_at') as String?;
  int? get maximumOutstandingQuantity =>
      value('maximum_outstanding_quantity') as int?;
  int? get maximumPerRequest => value('maximum_per_request') as int?;
  String? get measureId => value('measure_id') as String?;
  String? get measuredAt => value('measured_at') as String?;
  String? get measuredBy => value('measured_by') as String?;
  int? get minimumQuantity => value('minimum_quantity') as int?;
  int? get missingQuantity => value('missing_quantity') as int?;
  String? get mode => value('mode') as String?;
  String? get note => value('note') as String?;
  String? get notes => value('notes') as String?;
  String? get operationType => value('operation_type') as String?;
  int? get quantity => value('quantity') as int?;
  int? get quantityDelta => value('quantity_delta') as int?;
  String? get reasonCode => value('reason_code') as String?;
  String? get receivedAt => value('received_at') as String?;
  int? get receivedQuantity => value('received_quantity') as int?;
  String? get reservationLineId => value('reservation_line_id') as String?;
  List<dynamic>? get rows => value('rows') as List<dynamic>?;
  String? get salesMode => value('sales_mode') as String?;
  String? get sellUnitRef => value('sell_unit_ref') as String?;
  String? get sourceBranchId => value('source_branch_id') as String?;
  String? get sourceRef => value('source_ref') as String?;
  String? get sourceStockBasisId => value('source_stock_basis_id') as String?;
  String? get sourceType => value('source_type') as String?;
  String? get stockBasisId => value('stock_basis_id') as String?;
  List<dynamic>? get stockBasisIds =>
      value('stock_basis_ids') as List<dynamic>?;
  String? get stocktakeType => value('stocktake_type') as String?;
  String? get supplierRef => value('supplier_ref') as String?;
  int? get targetQuantity => value('target_quantity') as int?;
  int? get unitCostMinor => value('unit_cost_minor') as int?;
  String? get variantRef => value('variant_ref') as String?;
}

class PostShopBulkOperationsJobIdApplyRequest extends VendorPayload {
  PostShopBulkOperationsJobIdApplyRequest({
    String? action,
    String? actualMeasureId,
    int? actualQuantity,
    String? allocationPolicy,
    String? availableFrom,
    String? batchNumber,
    String? bestBeforeAt,
    String? branchId,
    String? consumptionMode,
    int? countedQuantity,
    String? cutoffAt,
    int? damagedQuantity,
    String? destinationBranchId,
    String? destinationStockBasisId,
    bool? dryRun,
    int? expectedCatalogueRevision,
    int? expectedInventoryRevision,
    String? expectedRestockAt,
    int? expectedRevision,
    int? expectedVersion,
    String? expiresAt,
    int? expiringSoonHours,
    int? leadTimeDays,
    String? lineId,
    List<dynamic>? lines,
    String? listingId,
    String? listingRef,
    String? lotNumber,
    bool? lotTracking,
    String? manufacturedAt,
    int? maximumOutstandingQuantity,
    int? maximumPerRequest,
    String? measureId,
    String? measuredAt,
    String? measuredBy,
    int? minimumQuantity,
    int? missingQuantity,
    String? mode,
    String? note,
    String? notes,
    String? operationType,
    int? quantity,
    int? quantityDelta,
    String? reasonCode,
    String? receivedAt,
    int? receivedQuantity,
    String? reservationLineId,
    List<dynamic>? rows,
    String? salesMode,
    String? sellUnitRef,
    String? sourceBranchId,
    String? sourceRef,
    String? sourceStockBasisId,
    String? sourceType,
    String? stockBasisId,
    List<dynamic>? stockBasisIds,
    String? stocktakeType,
    String? supplierRef,
    int? targetQuantity,
    int? unitCostMinor,
    String? variantRef,
  }) : super({
         'action': ?action,
         'actual_measure_id': ?actualMeasureId,
         'actual_quantity': ?actualQuantity,
         'allocation_policy': ?allocationPolicy,
         'available_from': ?availableFrom,
         'batch_number': ?batchNumber,
         'best_before_at': ?bestBeforeAt,
         'branch_id': ?branchId,
         'consumption_mode': ?consumptionMode,
         'counted_quantity': ?countedQuantity,
         'cutoff_at': ?cutoffAt,
         'damaged_quantity': ?damagedQuantity,
         'destination_branch_id': ?destinationBranchId,
         'destination_stock_basis_id': ?destinationStockBasisId,
         'dry_run': ?dryRun,
         'expected_catalogue_revision': ?expectedCatalogueRevision,
         'expected_inventory_revision': ?expectedInventoryRevision,
         'expected_restock_at': ?expectedRestockAt,
         'expected_revision': ?expectedRevision,
         'expected_version': ?expectedVersion,
         'expires_at': ?expiresAt,
         'expiring_soon_hours': ?expiringSoonHours,
         'lead_time_days': ?leadTimeDays,
         'line_id': ?lineId,
         'lines': ?lines,
         'listing_id': ?listingId,
         'listing_ref': ?listingRef,
         'lot_number': ?lotNumber,
         'lot_tracking': ?lotTracking,
         'manufactured_at': ?manufacturedAt,
         'maximum_outstanding_quantity': ?maximumOutstandingQuantity,
         'maximum_per_request': ?maximumPerRequest,
         'measure_id': ?measureId,
         'measured_at': ?measuredAt,
         'measured_by': ?measuredBy,
         'minimum_quantity': ?minimumQuantity,
         'missing_quantity': ?missingQuantity,
         'mode': ?mode,
         'note': ?note,
         'notes': ?notes,
         'operation_type': ?operationType,
         'quantity': ?quantity,
         'quantity_delta': ?quantityDelta,
         'reason_code': ?reasonCode,
         'received_at': ?receivedAt,
         'received_quantity': ?receivedQuantity,
         'reservation_line_id': ?reservationLineId,
         'rows': ?rows,
         'sales_mode': ?salesMode,
         'sell_unit_ref': ?sellUnitRef,
         'source_branch_id': ?sourceBranchId,
         'source_ref': ?sourceRef,
         'source_stock_basis_id': ?sourceStockBasisId,
         'source_type': ?sourceType,
         'stock_basis_id': ?stockBasisId,
         'stock_basis_ids': ?stockBasisIds,
         'stocktake_type': ?stocktakeType,
         'supplier_ref': ?supplierRef,
         'target_quantity': ?targetQuantity,
         'unit_cost_minor': ?unitCostMinor,
         'variant_ref': ?variantRef,
       });
  PostShopBulkOperationsJobIdApplyRequest.fromJson(super.json);
  String? get action => value('action') as String?;
  String? get actualMeasureId => value('actual_measure_id') as String?;
  int? get actualQuantity => value('actual_quantity') as int?;
  String? get allocationPolicy => value('allocation_policy') as String?;
  String? get availableFrom => value('available_from') as String?;
  String? get batchNumber => value('batch_number') as String?;
  String? get bestBeforeAt => value('best_before_at') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get consumptionMode => value('consumption_mode') as String?;
  int? get countedQuantity => value('counted_quantity') as int?;
  String? get cutoffAt => value('cutoff_at') as String?;
  int? get damagedQuantity => value('damaged_quantity') as int?;
  String? get destinationBranchId => value('destination_branch_id') as String?;
  String? get destinationStockBasisId =>
      value('destination_stock_basis_id') as String?;
  bool? get dryRun => value('dry_run') as bool?;
  int? get expectedCatalogueRevision =>
      value('expected_catalogue_revision') as int?;
  int? get expectedInventoryRevision =>
      value('expected_inventory_revision') as int?;
  String? get expectedRestockAt => value('expected_restock_at') as String?;
  int? get expectedRevision => value('expected_revision') as int?;
  int? get expectedVersion => value('expected_version') as int?;
  String? get expiresAt => value('expires_at') as String?;
  int? get expiringSoonHours => value('expiring_soon_hours') as int?;
  int? get leadTimeDays => value('lead_time_days') as int?;
  String? get lineId => value('line_id') as String?;
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
  String? get listingId => value('listing_id') as String?;
  String? get listingRef => value('listing_ref') as String?;
  String? get lotNumber => value('lot_number') as String?;
  bool? get lotTracking => value('lot_tracking') as bool?;
  String? get manufacturedAt => value('manufactured_at') as String?;
  int? get maximumOutstandingQuantity =>
      value('maximum_outstanding_quantity') as int?;
  int? get maximumPerRequest => value('maximum_per_request') as int?;
  String? get measureId => value('measure_id') as String?;
  String? get measuredAt => value('measured_at') as String?;
  String? get measuredBy => value('measured_by') as String?;
  int? get minimumQuantity => value('minimum_quantity') as int?;
  int? get missingQuantity => value('missing_quantity') as int?;
  String? get mode => value('mode') as String?;
  String? get note => value('note') as String?;
  String? get notes => value('notes') as String?;
  String? get operationType => value('operation_type') as String?;
  int? get quantity => value('quantity') as int?;
  int? get quantityDelta => value('quantity_delta') as int?;
  String? get reasonCode => value('reason_code') as String?;
  String? get receivedAt => value('received_at') as String?;
  int? get receivedQuantity => value('received_quantity') as int?;
  String? get reservationLineId => value('reservation_line_id') as String?;
  List<dynamic>? get rows => value('rows') as List<dynamic>?;
  String? get salesMode => value('sales_mode') as String?;
  String? get sellUnitRef => value('sell_unit_ref') as String?;
  String? get sourceBranchId => value('source_branch_id') as String?;
  String? get sourceRef => value('source_ref') as String?;
  String? get sourceStockBasisId => value('source_stock_basis_id') as String?;
  String? get sourceType => value('source_type') as String?;
  String? get stockBasisId => value('stock_basis_id') as String?;
  List<dynamic>? get stockBasisIds =>
      value('stock_basis_ids') as List<dynamic>?;
  String? get stocktakeType => value('stocktake_type') as String?;
  String? get supplierRef => value('supplier_ref') as String?;
  int? get targetQuantity => value('target_quantity') as int?;
  int? get unitCostMinor => value('unit_cost_minor') as int?;
  String? get variantRef => value('variant_ref') as String?;
}

class PostShopCategoriesRequest extends VendorPayload {
  PostShopCategoriesRequest({
    String? parentId,
    String? name,
    String? slug,
    String? description,
    int? position,
  }) : super({
         'parent_id': ?parentId,
         'name': ?name,
         'slug': ?slug,
         'description': ?description,
         'position': ?position,
       });
  PostShopCategoriesRequest.fromJson(super.json);
  String? get parentId => value('parent_id') as String?;
  String? get name => value('name') as String?;
  String? get slug => value('slug') as String?;
  String? get description => value('description') as String?;
  int? get position => value('position') as int?;
}

class PatchShopCategoriesCategoryIdRequest extends VendorPayload {
  PatchShopCategoriesCategoryIdRequest({
    String? name,
    String? slug,
    String? description,
    int? position,
    String? parentId,
    int? expectedVersion,
  }) : super({
         'name': ?name,
         'slug': ?slug,
         'description': ?description,
         'position': ?position,
         'parent_id': ?parentId,
         'expected_version': ?expectedVersion,
       });
  PatchShopCategoriesCategoryIdRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  String? get slug => value('slug') as String?;
  String? get description => value('description') as String?;
  int? get position => value('position') as int?;
  String? get parentId => value('parent_id') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostShopCategoriesCategoryIdArchiveRequest extends VendorPayload {
  PostShopCategoriesCategoryIdArchiveRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostShopCategoriesCategoryIdArchiveRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostShopCategoriesCategoryIdRestoreRequest extends VendorPayload {
  PostShopCategoriesCategoryIdRestoreRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostShopCategoriesCategoryIdRestoreRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostShopInventoryAdjustmentsRequest extends VendorPayload {
  PostShopInventoryAdjustmentsRequest({
    String? stockBasisId,
    int? quantityDelta,
    String? reasonCode,
    String? note,
    int? expectedRevision,
  }) : super({
         'stock_basis_id': ?stockBasisId,
         'quantity_delta': ?quantityDelta,
         'reason_code': ?reasonCode,
         'note': ?note,
         'expected_revision': ?expectedRevision,
       });
  PostShopInventoryAdjustmentsRequest.fromJson(super.json);
  String? get stockBasisId => value('stock_basis_id') as String?;
  int? get quantityDelta => value('quantity_delta') as int?;
  String? get reasonCode => value('reason_code') as String?;
  String? get note => value('note') as String?;
  int? get expectedRevision => value('expected_revision') as int?;
}

class PostShopInventoryAlertsAlertIdAcknowledgeRequest extends VendorPayload {
  PostShopInventoryAlertsAlertIdAcknowledgeRequest({
    String? action,
    String? actualMeasureId,
    int? actualQuantity,
    String? allocationPolicy,
    String? availableFrom,
    String? batchNumber,
    String? bestBeforeAt,
    String? branchId,
    String? consumptionMode,
    int? countedQuantity,
    String? cutoffAt,
    int? damagedQuantity,
    String? destinationBranchId,
    String? destinationStockBasisId,
    bool? dryRun,
    int? expectedCatalogueRevision,
    int? expectedInventoryRevision,
    String? expectedRestockAt,
    int? expectedRevision,
    int? expectedVersion,
    String? expiresAt,
    int? expiringSoonHours,
    int? leadTimeDays,
    String? lineId,
    List<dynamic>? lines,
    String? listingId,
    String? listingRef,
    String? lotNumber,
    bool? lotTracking,
    String? manufacturedAt,
    int? maximumOutstandingQuantity,
    int? maximumPerRequest,
    String? measureId,
    String? measuredAt,
    String? measuredBy,
    int? minimumQuantity,
    int? missingQuantity,
    String? mode,
    String? note,
    String? notes,
    String? operationType,
    int? quantity,
    int? quantityDelta,
    String? reasonCode,
    String? receivedAt,
    int? receivedQuantity,
    String? reservationLineId,
    List<dynamic>? rows,
    String? salesMode,
    String? sellUnitRef,
    String? sourceBranchId,
    String? sourceRef,
    String? sourceStockBasisId,
    String? sourceType,
    String? stockBasisId,
    List<dynamic>? stockBasisIds,
    String? stocktakeType,
    String? supplierRef,
    int? targetQuantity,
    int? unitCostMinor,
    String? variantRef,
  }) : super({
         'action': ?action,
         'actual_measure_id': ?actualMeasureId,
         'actual_quantity': ?actualQuantity,
         'allocation_policy': ?allocationPolicy,
         'available_from': ?availableFrom,
         'batch_number': ?batchNumber,
         'best_before_at': ?bestBeforeAt,
         'branch_id': ?branchId,
         'consumption_mode': ?consumptionMode,
         'counted_quantity': ?countedQuantity,
         'cutoff_at': ?cutoffAt,
         'damaged_quantity': ?damagedQuantity,
         'destination_branch_id': ?destinationBranchId,
         'destination_stock_basis_id': ?destinationStockBasisId,
         'dry_run': ?dryRun,
         'expected_catalogue_revision': ?expectedCatalogueRevision,
         'expected_inventory_revision': ?expectedInventoryRevision,
         'expected_restock_at': ?expectedRestockAt,
         'expected_revision': ?expectedRevision,
         'expected_version': ?expectedVersion,
         'expires_at': ?expiresAt,
         'expiring_soon_hours': ?expiringSoonHours,
         'lead_time_days': ?leadTimeDays,
         'line_id': ?lineId,
         'lines': ?lines,
         'listing_id': ?listingId,
         'listing_ref': ?listingRef,
         'lot_number': ?lotNumber,
         'lot_tracking': ?lotTracking,
         'manufactured_at': ?manufacturedAt,
         'maximum_outstanding_quantity': ?maximumOutstandingQuantity,
         'maximum_per_request': ?maximumPerRequest,
         'measure_id': ?measureId,
         'measured_at': ?measuredAt,
         'measured_by': ?measuredBy,
         'minimum_quantity': ?minimumQuantity,
         'missing_quantity': ?missingQuantity,
         'mode': ?mode,
         'note': ?note,
         'notes': ?notes,
         'operation_type': ?operationType,
         'quantity': ?quantity,
         'quantity_delta': ?quantityDelta,
         'reason_code': ?reasonCode,
         'received_at': ?receivedAt,
         'received_quantity': ?receivedQuantity,
         'reservation_line_id': ?reservationLineId,
         'rows': ?rows,
         'sales_mode': ?salesMode,
         'sell_unit_ref': ?sellUnitRef,
         'source_branch_id': ?sourceBranchId,
         'source_ref': ?sourceRef,
         'source_stock_basis_id': ?sourceStockBasisId,
         'source_type': ?sourceType,
         'stock_basis_id': ?stockBasisId,
         'stock_basis_ids': ?stockBasisIds,
         'stocktake_type': ?stocktakeType,
         'supplier_ref': ?supplierRef,
         'target_quantity': ?targetQuantity,
         'unit_cost_minor': ?unitCostMinor,
         'variant_ref': ?variantRef,
       });
  PostShopInventoryAlertsAlertIdAcknowledgeRequest.fromJson(super.json);
  String? get action => value('action') as String?;
  String? get actualMeasureId => value('actual_measure_id') as String?;
  int? get actualQuantity => value('actual_quantity') as int?;
  String? get allocationPolicy => value('allocation_policy') as String?;
  String? get availableFrom => value('available_from') as String?;
  String? get batchNumber => value('batch_number') as String?;
  String? get bestBeforeAt => value('best_before_at') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get consumptionMode => value('consumption_mode') as String?;
  int? get countedQuantity => value('counted_quantity') as int?;
  String? get cutoffAt => value('cutoff_at') as String?;
  int? get damagedQuantity => value('damaged_quantity') as int?;
  String? get destinationBranchId => value('destination_branch_id') as String?;
  String? get destinationStockBasisId =>
      value('destination_stock_basis_id') as String?;
  bool? get dryRun => value('dry_run') as bool?;
  int? get expectedCatalogueRevision =>
      value('expected_catalogue_revision') as int?;
  int? get expectedInventoryRevision =>
      value('expected_inventory_revision') as int?;
  String? get expectedRestockAt => value('expected_restock_at') as String?;
  int? get expectedRevision => value('expected_revision') as int?;
  int? get expectedVersion => value('expected_version') as int?;
  String? get expiresAt => value('expires_at') as String?;
  int? get expiringSoonHours => value('expiring_soon_hours') as int?;
  int? get leadTimeDays => value('lead_time_days') as int?;
  String? get lineId => value('line_id') as String?;
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
  String? get listingId => value('listing_id') as String?;
  String? get listingRef => value('listing_ref') as String?;
  String? get lotNumber => value('lot_number') as String?;
  bool? get lotTracking => value('lot_tracking') as bool?;
  String? get manufacturedAt => value('manufactured_at') as String?;
  int? get maximumOutstandingQuantity =>
      value('maximum_outstanding_quantity') as int?;
  int? get maximumPerRequest => value('maximum_per_request') as int?;
  String? get measureId => value('measure_id') as String?;
  String? get measuredAt => value('measured_at') as String?;
  String? get measuredBy => value('measured_by') as String?;
  int? get minimumQuantity => value('minimum_quantity') as int?;
  int? get missingQuantity => value('missing_quantity') as int?;
  String? get mode => value('mode') as String?;
  String? get note => value('note') as String?;
  String? get notes => value('notes') as String?;
  String? get operationType => value('operation_type') as String?;
  int? get quantity => value('quantity') as int?;
  int? get quantityDelta => value('quantity_delta') as int?;
  String? get reasonCode => value('reason_code') as String?;
  String? get receivedAt => value('received_at') as String?;
  int? get receivedQuantity => value('received_quantity') as int?;
  String? get reservationLineId => value('reservation_line_id') as String?;
  List<dynamic>? get rows => value('rows') as List<dynamic>?;
  String? get salesMode => value('sales_mode') as String?;
  String? get sellUnitRef => value('sell_unit_ref') as String?;
  String? get sourceBranchId => value('source_branch_id') as String?;
  String? get sourceRef => value('source_ref') as String?;
  String? get sourceStockBasisId => value('source_stock_basis_id') as String?;
  String? get sourceType => value('source_type') as String?;
  String? get stockBasisId => value('stock_basis_id') as String?;
  List<dynamic>? get stockBasisIds =>
      value('stock_basis_ids') as List<dynamic>?;
  String? get stocktakeType => value('stocktake_type') as String?;
  String? get supplierRef => value('supplier_ref') as String?;
  int? get targetQuantity => value('target_quantity') as int?;
  int? get unitCostMinor => value('unit_cost_minor') as int?;
  String? get variantRef => value('variant_ref') as String?;
}

class PostShopInventoryAlertsAlertIdResolveRequest extends VendorPayload {
  PostShopInventoryAlertsAlertIdResolveRequest({
    String? action,
    String? actualMeasureId,
    int? actualQuantity,
    String? allocationPolicy,
    String? availableFrom,
    String? batchNumber,
    String? bestBeforeAt,
    String? branchId,
    String? consumptionMode,
    int? countedQuantity,
    String? cutoffAt,
    int? damagedQuantity,
    String? destinationBranchId,
    String? destinationStockBasisId,
    bool? dryRun,
    int? expectedCatalogueRevision,
    int? expectedInventoryRevision,
    String? expectedRestockAt,
    int? expectedRevision,
    int? expectedVersion,
    String? expiresAt,
    int? expiringSoonHours,
    int? leadTimeDays,
    String? lineId,
    List<dynamic>? lines,
    String? listingId,
    String? listingRef,
    String? lotNumber,
    bool? lotTracking,
    String? manufacturedAt,
    int? maximumOutstandingQuantity,
    int? maximumPerRequest,
    String? measureId,
    String? measuredAt,
    String? measuredBy,
    int? minimumQuantity,
    int? missingQuantity,
    String? mode,
    String? note,
    String? notes,
    String? operationType,
    int? quantity,
    int? quantityDelta,
    String? reasonCode,
    String? receivedAt,
    int? receivedQuantity,
    String? reservationLineId,
    List<dynamic>? rows,
    String? salesMode,
    String? sellUnitRef,
    String? sourceBranchId,
    String? sourceRef,
    String? sourceStockBasisId,
    String? sourceType,
    String? stockBasisId,
    List<dynamic>? stockBasisIds,
    String? stocktakeType,
    String? supplierRef,
    int? targetQuantity,
    int? unitCostMinor,
    String? variantRef,
  }) : super({
         'action': ?action,
         'actual_measure_id': ?actualMeasureId,
         'actual_quantity': ?actualQuantity,
         'allocation_policy': ?allocationPolicy,
         'available_from': ?availableFrom,
         'batch_number': ?batchNumber,
         'best_before_at': ?bestBeforeAt,
         'branch_id': ?branchId,
         'consumption_mode': ?consumptionMode,
         'counted_quantity': ?countedQuantity,
         'cutoff_at': ?cutoffAt,
         'damaged_quantity': ?damagedQuantity,
         'destination_branch_id': ?destinationBranchId,
         'destination_stock_basis_id': ?destinationStockBasisId,
         'dry_run': ?dryRun,
         'expected_catalogue_revision': ?expectedCatalogueRevision,
         'expected_inventory_revision': ?expectedInventoryRevision,
         'expected_restock_at': ?expectedRestockAt,
         'expected_revision': ?expectedRevision,
         'expected_version': ?expectedVersion,
         'expires_at': ?expiresAt,
         'expiring_soon_hours': ?expiringSoonHours,
         'lead_time_days': ?leadTimeDays,
         'line_id': ?lineId,
         'lines': ?lines,
         'listing_id': ?listingId,
         'listing_ref': ?listingRef,
         'lot_number': ?lotNumber,
         'lot_tracking': ?lotTracking,
         'manufactured_at': ?manufacturedAt,
         'maximum_outstanding_quantity': ?maximumOutstandingQuantity,
         'maximum_per_request': ?maximumPerRequest,
         'measure_id': ?measureId,
         'measured_at': ?measuredAt,
         'measured_by': ?measuredBy,
         'minimum_quantity': ?minimumQuantity,
         'missing_quantity': ?missingQuantity,
         'mode': ?mode,
         'note': ?note,
         'notes': ?notes,
         'operation_type': ?operationType,
         'quantity': ?quantity,
         'quantity_delta': ?quantityDelta,
         'reason_code': ?reasonCode,
         'received_at': ?receivedAt,
         'received_quantity': ?receivedQuantity,
         'reservation_line_id': ?reservationLineId,
         'rows': ?rows,
         'sales_mode': ?salesMode,
         'sell_unit_ref': ?sellUnitRef,
         'source_branch_id': ?sourceBranchId,
         'source_ref': ?sourceRef,
         'source_stock_basis_id': ?sourceStockBasisId,
         'source_type': ?sourceType,
         'stock_basis_id': ?stockBasisId,
         'stock_basis_ids': ?stockBasisIds,
         'stocktake_type': ?stocktakeType,
         'supplier_ref': ?supplierRef,
         'target_quantity': ?targetQuantity,
         'unit_cost_minor': ?unitCostMinor,
         'variant_ref': ?variantRef,
       });
  PostShopInventoryAlertsAlertIdResolveRequest.fromJson(super.json);
  String? get action => value('action') as String?;
  String? get actualMeasureId => value('actual_measure_id') as String?;
  int? get actualQuantity => value('actual_quantity') as int?;
  String? get allocationPolicy => value('allocation_policy') as String?;
  String? get availableFrom => value('available_from') as String?;
  String? get batchNumber => value('batch_number') as String?;
  String? get bestBeforeAt => value('best_before_at') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get consumptionMode => value('consumption_mode') as String?;
  int? get countedQuantity => value('counted_quantity') as int?;
  String? get cutoffAt => value('cutoff_at') as String?;
  int? get damagedQuantity => value('damaged_quantity') as int?;
  String? get destinationBranchId => value('destination_branch_id') as String?;
  String? get destinationStockBasisId =>
      value('destination_stock_basis_id') as String?;
  bool? get dryRun => value('dry_run') as bool?;
  int? get expectedCatalogueRevision =>
      value('expected_catalogue_revision') as int?;
  int? get expectedInventoryRevision =>
      value('expected_inventory_revision') as int?;
  String? get expectedRestockAt => value('expected_restock_at') as String?;
  int? get expectedRevision => value('expected_revision') as int?;
  int? get expectedVersion => value('expected_version') as int?;
  String? get expiresAt => value('expires_at') as String?;
  int? get expiringSoonHours => value('expiring_soon_hours') as int?;
  int? get leadTimeDays => value('lead_time_days') as int?;
  String? get lineId => value('line_id') as String?;
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
  String? get listingId => value('listing_id') as String?;
  String? get listingRef => value('listing_ref') as String?;
  String? get lotNumber => value('lot_number') as String?;
  bool? get lotTracking => value('lot_tracking') as bool?;
  String? get manufacturedAt => value('manufactured_at') as String?;
  int? get maximumOutstandingQuantity =>
      value('maximum_outstanding_quantity') as int?;
  int? get maximumPerRequest => value('maximum_per_request') as int?;
  String? get measureId => value('measure_id') as String?;
  String? get measuredAt => value('measured_at') as String?;
  String? get measuredBy => value('measured_by') as String?;
  int? get minimumQuantity => value('minimum_quantity') as int?;
  int? get missingQuantity => value('missing_quantity') as int?;
  String? get mode => value('mode') as String?;
  String? get note => value('note') as String?;
  String? get notes => value('notes') as String?;
  String? get operationType => value('operation_type') as String?;
  int? get quantity => value('quantity') as int?;
  int? get quantityDelta => value('quantity_delta') as int?;
  String? get reasonCode => value('reason_code') as String?;
  String? get receivedAt => value('received_at') as String?;
  int? get receivedQuantity => value('received_quantity') as int?;
  String? get reservationLineId => value('reservation_line_id') as String?;
  List<dynamic>? get rows => value('rows') as List<dynamic>?;
  String? get salesMode => value('sales_mode') as String?;
  String? get sellUnitRef => value('sell_unit_ref') as String?;
  String? get sourceBranchId => value('source_branch_id') as String?;
  String? get sourceRef => value('source_ref') as String?;
  String? get sourceStockBasisId => value('source_stock_basis_id') as String?;
  String? get sourceType => value('source_type') as String?;
  String? get stockBasisId => value('stock_basis_id') as String?;
  List<dynamic>? get stockBasisIds =>
      value('stock_basis_ids') as List<dynamic>?;
  String? get stocktakeType => value('stocktake_type') as String?;
  String? get supplierRef => value('supplier_ref') as String?;
  int? get targetQuantity => value('target_quantity') as int?;
  int? get unitCostMinor => value('unit_cost_minor') as int?;
  String? get variantRef => value('variant_ref') as String?;
}

class PostShopInventoryFulfilmentMeasurementsRequest extends VendorPayload {
  PostShopInventoryFulfilmentMeasurementsRequest({
    String? action,
    String? actualMeasureId,
    int? actualQuantity,
    String? allocationPolicy,
    String? availableFrom,
    String? batchNumber,
    String? bestBeforeAt,
    String? branchId,
    String? consumptionMode,
    int? countedQuantity,
    String? cutoffAt,
    int? damagedQuantity,
    String? destinationBranchId,
    String? destinationStockBasisId,
    bool? dryRun,
    int? expectedCatalogueRevision,
    int? expectedInventoryRevision,
    String? expectedRestockAt,
    int? expectedRevision,
    int? expectedVersion,
    String? expiresAt,
    int? expiringSoonHours,
    int? leadTimeDays,
    String? lineId,
    List<dynamic>? lines,
    String? listingId,
    String? listingRef,
    String? lotNumber,
    bool? lotTracking,
    String? manufacturedAt,
    int? maximumOutstandingQuantity,
    int? maximumPerRequest,
    String? measureId,
    String? measuredAt,
    String? measuredBy,
    int? minimumQuantity,
    int? missingQuantity,
    String? mode,
    String? note,
    String? notes,
    String? operationType,
    int? quantity,
    int? quantityDelta,
    String? reasonCode,
    String? receivedAt,
    int? receivedQuantity,
    String? reservationLineId,
    List<dynamic>? rows,
    String? salesMode,
    String? sellUnitRef,
    String? sourceBranchId,
    String? sourceRef,
    String? sourceStockBasisId,
    String? sourceType,
    String? stockBasisId,
    List<dynamic>? stockBasisIds,
    String? stocktakeType,
    String? supplierRef,
    int? targetQuantity,
    int? unitCostMinor,
    String? variantRef,
  }) : super({
         'action': ?action,
         'actual_measure_id': ?actualMeasureId,
         'actual_quantity': ?actualQuantity,
         'allocation_policy': ?allocationPolicy,
         'available_from': ?availableFrom,
         'batch_number': ?batchNumber,
         'best_before_at': ?bestBeforeAt,
         'branch_id': ?branchId,
         'consumption_mode': ?consumptionMode,
         'counted_quantity': ?countedQuantity,
         'cutoff_at': ?cutoffAt,
         'damaged_quantity': ?damagedQuantity,
         'destination_branch_id': ?destinationBranchId,
         'destination_stock_basis_id': ?destinationStockBasisId,
         'dry_run': ?dryRun,
         'expected_catalogue_revision': ?expectedCatalogueRevision,
         'expected_inventory_revision': ?expectedInventoryRevision,
         'expected_restock_at': ?expectedRestockAt,
         'expected_revision': ?expectedRevision,
         'expected_version': ?expectedVersion,
         'expires_at': ?expiresAt,
         'expiring_soon_hours': ?expiringSoonHours,
         'lead_time_days': ?leadTimeDays,
         'line_id': ?lineId,
         'lines': ?lines,
         'listing_id': ?listingId,
         'listing_ref': ?listingRef,
         'lot_number': ?lotNumber,
         'lot_tracking': ?lotTracking,
         'manufactured_at': ?manufacturedAt,
         'maximum_outstanding_quantity': ?maximumOutstandingQuantity,
         'maximum_per_request': ?maximumPerRequest,
         'measure_id': ?measureId,
         'measured_at': ?measuredAt,
         'measured_by': ?measuredBy,
         'minimum_quantity': ?minimumQuantity,
         'missing_quantity': ?missingQuantity,
         'mode': ?mode,
         'note': ?note,
         'notes': ?notes,
         'operation_type': ?operationType,
         'quantity': ?quantity,
         'quantity_delta': ?quantityDelta,
         'reason_code': ?reasonCode,
         'received_at': ?receivedAt,
         'received_quantity': ?receivedQuantity,
         'reservation_line_id': ?reservationLineId,
         'rows': ?rows,
         'sales_mode': ?salesMode,
         'sell_unit_ref': ?sellUnitRef,
         'source_branch_id': ?sourceBranchId,
         'source_ref': ?sourceRef,
         'source_stock_basis_id': ?sourceStockBasisId,
         'source_type': ?sourceType,
         'stock_basis_id': ?stockBasisId,
         'stock_basis_ids': ?stockBasisIds,
         'stocktake_type': ?stocktakeType,
         'supplier_ref': ?supplierRef,
         'target_quantity': ?targetQuantity,
         'unit_cost_minor': ?unitCostMinor,
         'variant_ref': ?variantRef,
       });
  PostShopInventoryFulfilmentMeasurementsRequest.fromJson(super.json);
  String? get action => value('action') as String?;
  String? get actualMeasureId => value('actual_measure_id') as String?;
  int? get actualQuantity => value('actual_quantity') as int?;
  String? get allocationPolicy => value('allocation_policy') as String?;
  String? get availableFrom => value('available_from') as String?;
  String? get batchNumber => value('batch_number') as String?;
  String? get bestBeforeAt => value('best_before_at') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get consumptionMode => value('consumption_mode') as String?;
  int? get countedQuantity => value('counted_quantity') as int?;
  String? get cutoffAt => value('cutoff_at') as String?;
  int? get damagedQuantity => value('damaged_quantity') as int?;
  String? get destinationBranchId => value('destination_branch_id') as String?;
  String? get destinationStockBasisId =>
      value('destination_stock_basis_id') as String?;
  bool? get dryRun => value('dry_run') as bool?;
  int? get expectedCatalogueRevision =>
      value('expected_catalogue_revision') as int?;
  int? get expectedInventoryRevision =>
      value('expected_inventory_revision') as int?;
  String? get expectedRestockAt => value('expected_restock_at') as String?;
  int? get expectedRevision => value('expected_revision') as int?;
  int? get expectedVersion => value('expected_version') as int?;
  String? get expiresAt => value('expires_at') as String?;
  int? get expiringSoonHours => value('expiring_soon_hours') as int?;
  int? get leadTimeDays => value('lead_time_days') as int?;
  String? get lineId => value('line_id') as String?;
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
  String? get listingId => value('listing_id') as String?;
  String? get listingRef => value('listing_ref') as String?;
  String? get lotNumber => value('lot_number') as String?;
  bool? get lotTracking => value('lot_tracking') as bool?;
  String? get manufacturedAt => value('manufactured_at') as String?;
  int? get maximumOutstandingQuantity =>
      value('maximum_outstanding_quantity') as int?;
  int? get maximumPerRequest => value('maximum_per_request') as int?;
  String? get measureId => value('measure_id') as String?;
  String? get measuredAt => value('measured_at') as String?;
  String? get measuredBy => value('measured_by') as String?;
  int? get minimumQuantity => value('minimum_quantity') as int?;
  int? get missingQuantity => value('missing_quantity') as int?;
  String? get mode => value('mode') as String?;
  String? get note => value('note') as String?;
  String? get notes => value('notes') as String?;
  String? get operationType => value('operation_type') as String?;
  int? get quantity => value('quantity') as int?;
  int? get quantityDelta => value('quantity_delta') as int?;
  String? get reasonCode => value('reason_code') as String?;
  String? get receivedAt => value('received_at') as String?;
  int? get receivedQuantity => value('received_quantity') as int?;
  String? get reservationLineId => value('reservation_line_id') as String?;
  List<dynamic>? get rows => value('rows') as List<dynamic>?;
  String? get salesMode => value('sales_mode') as String?;
  String? get sellUnitRef => value('sell_unit_ref') as String?;
  String? get sourceBranchId => value('source_branch_id') as String?;
  String? get sourceRef => value('source_ref') as String?;
  String? get sourceStockBasisId => value('source_stock_basis_id') as String?;
  String? get sourceType => value('source_type') as String?;
  String? get stockBasisId => value('stock_basis_id') as String?;
  List<dynamic>? get stockBasisIds =>
      value('stock_basis_ids') as List<dynamic>?;
  String? get stocktakeType => value('stocktake_type') as String?;
  String? get supplierRef => value('supplier_ref') as String?;
  int? get targetQuantity => value('target_quantity') as int?;
  int? get unitCostMinor => value('unit_cost_minor') as int?;
  String? get variantRef => value('variant_ref') as String?;
}

class PostShopInventoryQuarantinesRequest extends VendorPayload {
  PostShopInventoryQuarantinesRequest({
    String? stockBasisId,
    int? quantity,
    String? reasonCode,
    String? note,
  }) : super({
         'stock_basis_id': ?stockBasisId,
         'quantity': ?quantity,
         'reason_code': ?reasonCode,
         'note': ?note,
       });
  PostShopInventoryQuarantinesRequest.fromJson(super.json);
  String? get stockBasisId => value('stock_basis_id') as String?;
  int? get quantity => value('quantity') as int?;
  String? get reasonCode => value('reason_code') as String?;
  String? get note => value('note') as String?;
}

class PostShopInventoryQuarantinesQuarantineIdResolveRequest
    extends VendorPayload {
  PostShopInventoryQuarantinesQuarantineIdResolveRequest({String? action})
    : super({'action': ?action});
  PostShopInventoryQuarantinesQuarantineIdResolveRequest.fromJson(super.json);
  String? get action => value('action') as String?;
}

class PostShopInventoryReceiptsRequest extends VendorPayload {
  PostShopInventoryReceiptsRequest({
    String? branchId,
    String? sourceRef,
    String? supplierRef,
    String? notes,
    String? receivedAt,
    List<dynamic>? lines,
  }) : super({
         'branch_id': ?branchId,
         'source_ref': ?sourceRef,
         'supplier_ref': ?supplierRef,
         'notes': ?notes,
         'received_at': ?receivedAt,
         'lines': ?lines,
       });
  PostShopInventoryReceiptsRequest.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  String? get sourceRef => value('source_ref') as String?;
  String? get supplierRef => value('supplier_ref') as String?;
  String? get notes => value('notes') as String?;
  String? get receivedAt => value('received_at') as String?;
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
}

class PostShopInventoryReceiptsReceiptIdCancelRequest extends VendorPayload {
  PostShopInventoryReceiptsReceiptIdCancelRequest({
    String? action,
    String? actualMeasureId,
    int? actualQuantity,
    String? allocationPolicy,
    String? availableFrom,
    String? batchNumber,
    String? bestBeforeAt,
    String? branchId,
    String? consumptionMode,
    int? countedQuantity,
    String? cutoffAt,
    int? damagedQuantity,
    String? destinationBranchId,
    String? destinationStockBasisId,
    bool? dryRun,
    int? expectedCatalogueRevision,
    int? expectedInventoryRevision,
    String? expectedRestockAt,
    int? expectedRevision,
    int? expectedVersion,
    String? expiresAt,
    int? expiringSoonHours,
    int? leadTimeDays,
    String? lineId,
    List<dynamic>? lines,
    String? listingId,
    String? listingRef,
    String? lotNumber,
    bool? lotTracking,
    String? manufacturedAt,
    int? maximumOutstandingQuantity,
    int? maximumPerRequest,
    String? measureId,
    String? measuredAt,
    String? measuredBy,
    int? minimumQuantity,
    int? missingQuantity,
    String? mode,
    String? note,
    String? notes,
    String? operationType,
    int? quantity,
    int? quantityDelta,
    String? reasonCode,
    String? receivedAt,
    int? receivedQuantity,
    String? reservationLineId,
    List<dynamic>? rows,
    String? salesMode,
    String? sellUnitRef,
    String? sourceBranchId,
    String? sourceRef,
    String? sourceStockBasisId,
    String? sourceType,
    String? stockBasisId,
    List<dynamic>? stockBasisIds,
    String? stocktakeType,
    String? supplierRef,
    int? targetQuantity,
    int? unitCostMinor,
    String? variantRef,
  }) : super({
         'action': ?action,
         'actual_measure_id': ?actualMeasureId,
         'actual_quantity': ?actualQuantity,
         'allocation_policy': ?allocationPolicy,
         'available_from': ?availableFrom,
         'batch_number': ?batchNumber,
         'best_before_at': ?bestBeforeAt,
         'branch_id': ?branchId,
         'consumption_mode': ?consumptionMode,
         'counted_quantity': ?countedQuantity,
         'cutoff_at': ?cutoffAt,
         'damaged_quantity': ?damagedQuantity,
         'destination_branch_id': ?destinationBranchId,
         'destination_stock_basis_id': ?destinationStockBasisId,
         'dry_run': ?dryRun,
         'expected_catalogue_revision': ?expectedCatalogueRevision,
         'expected_inventory_revision': ?expectedInventoryRevision,
         'expected_restock_at': ?expectedRestockAt,
         'expected_revision': ?expectedRevision,
         'expected_version': ?expectedVersion,
         'expires_at': ?expiresAt,
         'expiring_soon_hours': ?expiringSoonHours,
         'lead_time_days': ?leadTimeDays,
         'line_id': ?lineId,
         'lines': ?lines,
         'listing_id': ?listingId,
         'listing_ref': ?listingRef,
         'lot_number': ?lotNumber,
         'lot_tracking': ?lotTracking,
         'manufactured_at': ?manufacturedAt,
         'maximum_outstanding_quantity': ?maximumOutstandingQuantity,
         'maximum_per_request': ?maximumPerRequest,
         'measure_id': ?measureId,
         'measured_at': ?measuredAt,
         'measured_by': ?measuredBy,
         'minimum_quantity': ?minimumQuantity,
         'missing_quantity': ?missingQuantity,
         'mode': ?mode,
         'note': ?note,
         'notes': ?notes,
         'operation_type': ?operationType,
         'quantity': ?quantity,
         'quantity_delta': ?quantityDelta,
         'reason_code': ?reasonCode,
         'received_at': ?receivedAt,
         'received_quantity': ?receivedQuantity,
         'reservation_line_id': ?reservationLineId,
         'rows': ?rows,
         'sales_mode': ?salesMode,
         'sell_unit_ref': ?sellUnitRef,
         'source_branch_id': ?sourceBranchId,
         'source_ref': ?sourceRef,
         'source_stock_basis_id': ?sourceStockBasisId,
         'source_type': ?sourceType,
         'stock_basis_id': ?stockBasisId,
         'stock_basis_ids': ?stockBasisIds,
         'stocktake_type': ?stocktakeType,
         'supplier_ref': ?supplierRef,
         'target_quantity': ?targetQuantity,
         'unit_cost_minor': ?unitCostMinor,
         'variant_ref': ?variantRef,
       });
  PostShopInventoryReceiptsReceiptIdCancelRequest.fromJson(super.json);
  String? get action => value('action') as String?;
  String? get actualMeasureId => value('actual_measure_id') as String?;
  int? get actualQuantity => value('actual_quantity') as int?;
  String? get allocationPolicy => value('allocation_policy') as String?;
  String? get availableFrom => value('available_from') as String?;
  String? get batchNumber => value('batch_number') as String?;
  String? get bestBeforeAt => value('best_before_at') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get consumptionMode => value('consumption_mode') as String?;
  int? get countedQuantity => value('counted_quantity') as int?;
  String? get cutoffAt => value('cutoff_at') as String?;
  int? get damagedQuantity => value('damaged_quantity') as int?;
  String? get destinationBranchId => value('destination_branch_id') as String?;
  String? get destinationStockBasisId =>
      value('destination_stock_basis_id') as String?;
  bool? get dryRun => value('dry_run') as bool?;
  int? get expectedCatalogueRevision =>
      value('expected_catalogue_revision') as int?;
  int? get expectedInventoryRevision =>
      value('expected_inventory_revision') as int?;
  String? get expectedRestockAt => value('expected_restock_at') as String?;
  int? get expectedRevision => value('expected_revision') as int?;
  int? get expectedVersion => value('expected_version') as int?;
  String? get expiresAt => value('expires_at') as String?;
  int? get expiringSoonHours => value('expiring_soon_hours') as int?;
  int? get leadTimeDays => value('lead_time_days') as int?;
  String? get lineId => value('line_id') as String?;
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
  String? get listingId => value('listing_id') as String?;
  String? get listingRef => value('listing_ref') as String?;
  String? get lotNumber => value('lot_number') as String?;
  bool? get lotTracking => value('lot_tracking') as bool?;
  String? get manufacturedAt => value('manufactured_at') as String?;
  int? get maximumOutstandingQuantity =>
      value('maximum_outstanding_quantity') as int?;
  int? get maximumPerRequest => value('maximum_per_request') as int?;
  String? get measureId => value('measure_id') as String?;
  String? get measuredAt => value('measured_at') as String?;
  String? get measuredBy => value('measured_by') as String?;
  int? get minimumQuantity => value('minimum_quantity') as int?;
  int? get missingQuantity => value('missing_quantity') as int?;
  String? get mode => value('mode') as String?;
  String? get note => value('note') as String?;
  String? get notes => value('notes') as String?;
  String? get operationType => value('operation_type') as String?;
  int? get quantity => value('quantity') as int?;
  int? get quantityDelta => value('quantity_delta') as int?;
  String? get reasonCode => value('reason_code') as String?;
  String? get receivedAt => value('received_at') as String?;
  int? get receivedQuantity => value('received_quantity') as int?;
  String? get reservationLineId => value('reservation_line_id') as String?;
  List<dynamic>? get rows => value('rows') as List<dynamic>?;
  String? get salesMode => value('sales_mode') as String?;
  String? get sellUnitRef => value('sell_unit_ref') as String?;
  String? get sourceBranchId => value('source_branch_id') as String?;
  String? get sourceRef => value('source_ref') as String?;
  String? get sourceStockBasisId => value('source_stock_basis_id') as String?;
  String? get sourceType => value('source_type') as String?;
  String? get stockBasisId => value('stock_basis_id') as String?;
  List<dynamic>? get stockBasisIds =>
      value('stock_basis_ids') as List<dynamic>?;
  String? get stocktakeType => value('stocktake_type') as String?;
  String? get supplierRef => value('supplier_ref') as String?;
  int? get targetQuantity => value('target_quantity') as int?;
  int? get unitCostMinor => value('unit_cost_minor') as int?;
  String? get variantRef => value('variant_ref') as String?;
}

class PostShopInventoryReceiptsReceiptIdPostRequest extends VendorPayload {
  PostShopInventoryReceiptsReceiptIdPostRequest({
    String? action,
    String? actualMeasureId,
    int? actualQuantity,
    String? allocationPolicy,
    String? availableFrom,
    String? batchNumber,
    String? bestBeforeAt,
    String? branchId,
    String? consumptionMode,
    int? countedQuantity,
    String? cutoffAt,
    int? damagedQuantity,
    String? destinationBranchId,
    String? destinationStockBasisId,
    bool? dryRun,
    int? expectedCatalogueRevision,
    int? expectedInventoryRevision,
    String? expectedRestockAt,
    int? expectedRevision,
    int? expectedVersion,
    String? expiresAt,
    int? expiringSoonHours,
    int? leadTimeDays,
    String? lineId,
    List<dynamic>? lines,
    String? listingId,
    String? listingRef,
    String? lotNumber,
    bool? lotTracking,
    String? manufacturedAt,
    int? maximumOutstandingQuantity,
    int? maximumPerRequest,
    String? measureId,
    String? measuredAt,
    String? measuredBy,
    int? minimumQuantity,
    int? missingQuantity,
    String? mode,
    String? note,
    String? notes,
    String? operationType,
    int? quantity,
    int? quantityDelta,
    String? reasonCode,
    String? receivedAt,
    int? receivedQuantity,
    String? reservationLineId,
    List<dynamic>? rows,
    String? salesMode,
    String? sellUnitRef,
    String? sourceBranchId,
    String? sourceRef,
    String? sourceStockBasisId,
    String? sourceType,
    String? stockBasisId,
    List<dynamic>? stockBasisIds,
    String? stocktakeType,
    String? supplierRef,
    int? targetQuantity,
    int? unitCostMinor,
    String? variantRef,
  }) : super({
         'action': ?action,
         'actual_measure_id': ?actualMeasureId,
         'actual_quantity': ?actualQuantity,
         'allocation_policy': ?allocationPolicy,
         'available_from': ?availableFrom,
         'batch_number': ?batchNumber,
         'best_before_at': ?bestBeforeAt,
         'branch_id': ?branchId,
         'consumption_mode': ?consumptionMode,
         'counted_quantity': ?countedQuantity,
         'cutoff_at': ?cutoffAt,
         'damaged_quantity': ?damagedQuantity,
         'destination_branch_id': ?destinationBranchId,
         'destination_stock_basis_id': ?destinationStockBasisId,
         'dry_run': ?dryRun,
         'expected_catalogue_revision': ?expectedCatalogueRevision,
         'expected_inventory_revision': ?expectedInventoryRevision,
         'expected_restock_at': ?expectedRestockAt,
         'expected_revision': ?expectedRevision,
         'expected_version': ?expectedVersion,
         'expires_at': ?expiresAt,
         'expiring_soon_hours': ?expiringSoonHours,
         'lead_time_days': ?leadTimeDays,
         'line_id': ?lineId,
         'lines': ?lines,
         'listing_id': ?listingId,
         'listing_ref': ?listingRef,
         'lot_number': ?lotNumber,
         'lot_tracking': ?lotTracking,
         'manufactured_at': ?manufacturedAt,
         'maximum_outstanding_quantity': ?maximumOutstandingQuantity,
         'maximum_per_request': ?maximumPerRequest,
         'measure_id': ?measureId,
         'measured_at': ?measuredAt,
         'measured_by': ?measuredBy,
         'minimum_quantity': ?minimumQuantity,
         'missing_quantity': ?missingQuantity,
         'mode': ?mode,
         'note': ?note,
         'notes': ?notes,
         'operation_type': ?operationType,
         'quantity': ?quantity,
         'quantity_delta': ?quantityDelta,
         'reason_code': ?reasonCode,
         'received_at': ?receivedAt,
         'received_quantity': ?receivedQuantity,
         'reservation_line_id': ?reservationLineId,
         'rows': ?rows,
         'sales_mode': ?salesMode,
         'sell_unit_ref': ?sellUnitRef,
         'source_branch_id': ?sourceBranchId,
         'source_ref': ?sourceRef,
         'source_stock_basis_id': ?sourceStockBasisId,
         'source_type': ?sourceType,
         'stock_basis_id': ?stockBasisId,
         'stock_basis_ids': ?stockBasisIds,
         'stocktake_type': ?stocktakeType,
         'supplier_ref': ?supplierRef,
         'target_quantity': ?targetQuantity,
         'unit_cost_minor': ?unitCostMinor,
         'variant_ref': ?variantRef,
       });
  PostShopInventoryReceiptsReceiptIdPostRequest.fromJson(super.json);
  String? get action => value('action') as String?;
  String? get actualMeasureId => value('actual_measure_id') as String?;
  int? get actualQuantity => value('actual_quantity') as int?;
  String? get allocationPolicy => value('allocation_policy') as String?;
  String? get availableFrom => value('available_from') as String?;
  String? get batchNumber => value('batch_number') as String?;
  String? get bestBeforeAt => value('best_before_at') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get consumptionMode => value('consumption_mode') as String?;
  int? get countedQuantity => value('counted_quantity') as int?;
  String? get cutoffAt => value('cutoff_at') as String?;
  int? get damagedQuantity => value('damaged_quantity') as int?;
  String? get destinationBranchId => value('destination_branch_id') as String?;
  String? get destinationStockBasisId =>
      value('destination_stock_basis_id') as String?;
  bool? get dryRun => value('dry_run') as bool?;
  int? get expectedCatalogueRevision =>
      value('expected_catalogue_revision') as int?;
  int? get expectedInventoryRevision =>
      value('expected_inventory_revision') as int?;
  String? get expectedRestockAt => value('expected_restock_at') as String?;
  int? get expectedRevision => value('expected_revision') as int?;
  int? get expectedVersion => value('expected_version') as int?;
  String? get expiresAt => value('expires_at') as String?;
  int? get expiringSoonHours => value('expiring_soon_hours') as int?;
  int? get leadTimeDays => value('lead_time_days') as int?;
  String? get lineId => value('line_id') as String?;
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
  String? get listingId => value('listing_id') as String?;
  String? get listingRef => value('listing_ref') as String?;
  String? get lotNumber => value('lot_number') as String?;
  bool? get lotTracking => value('lot_tracking') as bool?;
  String? get manufacturedAt => value('manufactured_at') as String?;
  int? get maximumOutstandingQuantity =>
      value('maximum_outstanding_quantity') as int?;
  int? get maximumPerRequest => value('maximum_per_request') as int?;
  String? get measureId => value('measure_id') as String?;
  String? get measuredAt => value('measured_at') as String?;
  String? get measuredBy => value('measured_by') as String?;
  int? get minimumQuantity => value('minimum_quantity') as int?;
  int? get missingQuantity => value('missing_quantity') as int?;
  String? get mode => value('mode') as String?;
  String? get note => value('note') as String?;
  String? get notes => value('notes') as String?;
  String? get operationType => value('operation_type') as String?;
  int? get quantity => value('quantity') as int?;
  int? get quantityDelta => value('quantity_delta') as int?;
  String? get reasonCode => value('reason_code') as String?;
  String? get receivedAt => value('received_at') as String?;
  int? get receivedQuantity => value('received_quantity') as int?;
  String? get reservationLineId => value('reservation_line_id') as String?;
  List<dynamic>? get rows => value('rows') as List<dynamic>?;
  String? get salesMode => value('sales_mode') as String?;
  String? get sellUnitRef => value('sell_unit_ref') as String?;
  String? get sourceBranchId => value('source_branch_id') as String?;
  String? get sourceRef => value('source_ref') as String?;
  String? get sourceStockBasisId => value('source_stock_basis_id') as String?;
  String? get sourceType => value('source_type') as String?;
  String? get stockBasisId => value('stock_basis_id') as String?;
  List<dynamic>? get stockBasisIds =>
      value('stock_basis_ids') as List<dynamic>?;
  String? get stocktakeType => value('stocktake_type') as String?;
  String? get supplierRef => value('supplier_ref') as String?;
  int? get targetQuantity => value('target_quantity') as int?;
  int? get unitCostMinor => value('unit_cost_minor') as int?;
  String? get variantRef => value('variant_ref') as String?;
}

class PostShopInventoryReservationsRequest extends VendorPayload {
  PostShopInventoryReservationsRequest({
    String? action,
    String? actualMeasureId,
    int? actualQuantity,
    String? allocationPolicy,
    String? availableFrom,
    String? batchNumber,
    String? bestBeforeAt,
    String? branchId,
    String? consumptionMode,
    int? countedQuantity,
    String? cutoffAt,
    int? damagedQuantity,
    String? destinationBranchId,
    String? destinationStockBasisId,
    bool? dryRun,
    int? expectedCatalogueRevision,
    int? expectedInventoryRevision,
    String? expectedRestockAt,
    int? expectedRevision,
    int? expectedVersion,
    String? expiresAt,
    int? expiringSoonHours,
    int? leadTimeDays,
    String? lineId,
    List<dynamic>? lines,
    String? listingId,
    String? listingRef,
    String? lotNumber,
    bool? lotTracking,
    String? manufacturedAt,
    int? maximumOutstandingQuantity,
    int? maximumPerRequest,
    String? measureId,
    String? measuredAt,
    String? measuredBy,
    int? minimumQuantity,
    int? missingQuantity,
    String? mode,
    String? note,
    String? notes,
    String? operationType,
    int? quantity,
    int? quantityDelta,
    String? reasonCode,
    String? receivedAt,
    int? receivedQuantity,
    String? reservationLineId,
    List<dynamic>? rows,
    String? salesMode,
    String? sellUnitRef,
    String? sourceBranchId,
    String? sourceRef,
    String? sourceStockBasisId,
    String? sourceType,
    String? stockBasisId,
    List<dynamic>? stockBasisIds,
    String? stocktakeType,
    String? supplierRef,
    int? targetQuantity,
    int? unitCostMinor,
    String? variantRef,
  }) : super({
         'action': ?action,
         'actual_measure_id': ?actualMeasureId,
         'actual_quantity': ?actualQuantity,
         'allocation_policy': ?allocationPolicy,
         'available_from': ?availableFrom,
         'batch_number': ?batchNumber,
         'best_before_at': ?bestBeforeAt,
         'branch_id': ?branchId,
         'consumption_mode': ?consumptionMode,
         'counted_quantity': ?countedQuantity,
         'cutoff_at': ?cutoffAt,
         'damaged_quantity': ?damagedQuantity,
         'destination_branch_id': ?destinationBranchId,
         'destination_stock_basis_id': ?destinationStockBasisId,
         'dry_run': ?dryRun,
         'expected_catalogue_revision': ?expectedCatalogueRevision,
         'expected_inventory_revision': ?expectedInventoryRevision,
         'expected_restock_at': ?expectedRestockAt,
         'expected_revision': ?expectedRevision,
         'expected_version': ?expectedVersion,
         'expires_at': ?expiresAt,
         'expiring_soon_hours': ?expiringSoonHours,
         'lead_time_days': ?leadTimeDays,
         'line_id': ?lineId,
         'lines': ?lines,
         'listing_id': ?listingId,
         'listing_ref': ?listingRef,
         'lot_number': ?lotNumber,
         'lot_tracking': ?lotTracking,
         'manufactured_at': ?manufacturedAt,
         'maximum_outstanding_quantity': ?maximumOutstandingQuantity,
         'maximum_per_request': ?maximumPerRequest,
         'measure_id': ?measureId,
         'measured_at': ?measuredAt,
         'measured_by': ?measuredBy,
         'minimum_quantity': ?minimumQuantity,
         'missing_quantity': ?missingQuantity,
         'mode': ?mode,
         'note': ?note,
         'notes': ?notes,
         'operation_type': ?operationType,
         'quantity': ?quantity,
         'quantity_delta': ?quantityDelta,
         'reason_code': ?reasonCode,
         'received_at': ?receivedAt,
         'received_quantity': ?receivedQuantity,
         'reservation_line_id': ?reservationLineId,
         'rows': ?rows,
         'sales_mode': ?salesMode,
         'sell_unit_ref': ?sellUnitRef,
         'source_branch_id': ?sourceBranchId,
         'source_ref': ?sourceRef,
         'source_stock_basis_id': ?sourceStockBasisId,
         'source_type': ?sourceType,
         'stock_basis_id': ?stockBasisId,
         'stock_basis_ids': ?stockBasisIds,
         'stocktake_type': ?stocktakeType,
         'supplier_ref': ?supplierRef,
         'target_quantity': ?targetQuantity,
         'unit_cost_minor': ?unitCostMinor,
         'variant_ref': ?variantRef,
       });
  PostShopInventoryReservationsRequest.fromJson(super.json);
  String? get action => value('action') as String?;
  String? get actualMeasureId => value('actual_measure_id') as String?;
  int? get actualQuantity => value('actual_quantity') as int?;
  String? get allocationPolicy => value('allocation_policy') as String?;
  String? get availableFrom => value('available_from') as String?;
  String? get batchNumber => value('batch_number') as String?;
  String? get bestBeforeAt => value('best_before_at') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get consumptionMode => value('consumption_mode') as String?;
  int? get countedQuantity => value('counted_quantity') as int?;
  String? get cutoffAt => value('cutoff_at') as String?;
  int? get damagedQuantity => value('damaged_quantity') as int?;
  String? get destinationBranchId => value('destination_branch_id') as String?;
  String? get destinationStockBasisId =>
      value('destination_stock_basis_id') as String?;
  bool? get dryRun => value('dry_run') as bool?;
  int? get expectedCatalogueRevision =>
      value('expected_catalogue_revision') as int?;
  int? get expectedInventoryRevision =>
      value('expected_inventory_revision') as int?;
  String? get expectedRestockAt => value('expected_restock_at') as String?;
  int? get expectedRevision => value('expected_revision') as int?;
  int? get expectedVersion => value('expected_version') as int?;
  String? get expiresAt => value('expires_at') as String?;
  int? get expiringSoonHours => value('expiring_soon_hours') as int?;
  int? get leadTimeDays => value('lead_time_days') as int?;
  String? get lineId => value('line_id') as String?;
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
  String? get listingId => value('listing_id') as String?;
  String? get listingRef => value('listing_ref') as String?;
  String? get lotNumber => value('lot_number') as String?;
  bool? get lotTracking => value('lot_tracking') as bool?;
  String? get manufacturedAt => value('manufactured_at') as String?;
  int? get maximumOutstandingQuantity =>
      value('maximum_outstanding_quantity') as int?;
  int? get maximumPerRequest => value('maximum_per_request') as int?;
  String? get measureId => value('measure_id') as String?;
  String? get measuredAt => value('measured_at') as String?;
  String? get measuredBy => value('measured_by') as String?;
  int? get minimumQuantity => value('minimum_quantity') as int?;
  int? get missingQuantity => value('missing_quantity') as int?;
  String? get mode => value('mode') as String?;
  String? get note => value('note') as String?;
  String? get notes => value('notes') as String?;
  String? get operationType => value('operation_type') as String?;
  int? get quantity => value('quantity') as int?;
  int? get quantityDelta => value('quantity_delta') as int?;
  String? get reasonCode => value('reason_code') as String?;
  String? get receivedAt => value('received_at') as String?;
  int? get receivedQuantity => value('received_quantity') as int?;
  String? get reservationLineId => value('reservation_line_id') as String?;
  List<dynamic>? get rows => value('rows') as List<dynamic>?;
  String? get salesMode => value('sales_mode') as String?;
  String? get sellUnitRef => value('sell_unit_ref') as String?;
  String? get sourceBranchId => value('source_branch_id') as String?;
  String? get sourceRef => value('source_ref') as String?;
  String? get sourceStockBasisId => value('source_stock_basis_id') as String?;
  String? get sourceType => value('source_type') as String?;
  String? get stockBasisId => value('stock_basis_id') as String?;
  List<dynamic>? get stockBasisIds =>
      value('stock_basis_ids') as List<dynamic>?;
  String? get stocktakeType => value('stocktake_type') as String?;
  String? get supplierRef => value('supplier_ref') as String?;
  int? get targetQuantity => value('target_quantity') as int?;
  int? get unitCostMinor => value('unit_cost_minor') as int?;
  String? get variantRef => value('variant_ref') as String?;
}

class PostShopInventoryReservationsReservationIdCommitRequest
    extends VendorPayload {
  PostShopInventoryReservationsReservationIdCommitRequest({
    String? action,
    String? actualMeasureId,
    int? actualQuantity,
    String? allocationPolicy,
    String? availableFrom,
    String? batchNumber,
    String? bestBeforeAt,
    String? branchId,
    String? consumptionMode,
    int? countedQuantity,
    String? cutoffAt,
    int? damagedQuantity,
    String? destinationBranchId,
    String? destinationStockBasisId,
    bool? dryRun,
    int? expectedCatalogueRevision,
    int? expectedInventoryRevision,
    String? expectedRestockAt,
    int? expectedRevision,
    int? expectedVersion,
    String? expiresAt,
    int? expiringSoonHours,
    int? leadTimeDays,
    String? lineId,
    List<dynamic>? lines,
    String? listingId,
    String? listingRef,
    String? lotNumber,
    bool? lotTracking,
    String? manufacturedAt,
    int? maximumOutstandingQuantity,
    int? maximumPerRequest,
    String? measureId,
    String? measuredAt,
    String? measuredBy,
    int? minimumQuantity,
    int? missingQuantity,
    String? mode,
    String? note,
    String? notes,
    String? operationType,
    int? quantity,
    int? quantityDelta,
    String? reasonCode,
    String? receivedAt,
    int? receivedQuantity,
    String? reservationLineId,
    List<dynamic>? rows,
    String? salesMode,
    String? sellUnitRef,
    String? sourceBranchId,
    String? sourceRef,
    String? sourceStockBasisId,
    String? sourceType,
    String? stockBasisId,
    List<dynamic>? stockBasisIds,
    String? stocktakeType,
    String? supplierRef,
    int? targetQuantity,
    int? unitCostMinor,
    String? variantRef,
  }) : super({
         'action': ?action,
         'actual_measure_id': ?actualMeasureId,
         'actual_quantity': ?actualQuantity,
         'allocation_policy': ?allocationPolicy,
         'available_from': ?availableFrom,
         'batch_number': ?batchNumber,
         'best_before_at': ?bestBeforeAt,
         'branch_id': ?branchId,
         'consumption_mode': ?consumptionMode,
         'counted_quantity': ?countedQuantity,
         'cutoff_at': ?cutoffAt,
         'damaged_quantity': ?damagedQuantity,
         'destination_branch_id': ?destinationBranchId,
         'destination_stock_basis_id': ?destinationStockBasisId,
         'dry_run': ?dryRun,
         'expected_catalogue_revision': ?expectedCatalogueRevision,
         'expected_inventory_revision': ?expectedInventoryRevision,
         'expected_restock_at': ?expectedRestockAt,
         'expected_revision': ?expectedRevision,
         'expected_version': ?expectedVersion,
         'expires_at': ?expiresAt,
         'expiring_soon_hours': ?expiringSoonHours,
         'lead_time_days': ?leadTimeDays,
         'line_id': ?lineId,
         'lines': ?lines,
         'listing_id': ?listingId,
         'listing_ref': ?listingRef,
         'lot_number': ?lotNumber,
         'lot_tracking': ?lotTracking,
         'manufactured_at': ?manufacturedAt,
         'maximum_outstanding_quantity': ?maximumOutstandingQuantity,
         'maximum_per_request': ?maximumPerRequest,
         'measure_id': ?measureId,
         'measured_at': ?measuredAt,
         'measured_by': ?measuredBy,
         'minimum_quantity': ?minimumQuantity,
         'missing_quantity': ?missingQuantity,
         'mode': ?mode,
         'note': ?note,
         'notes': ?notes,
         'operation_type': ?operationType,
         'quantity': ?quantity,
         'quantity_delta': ?quantityDelta,
         'reason_code': ?reasonCode,
         'received_at': ?receivedAt,
         'received_quantity': ?receivedQuantity,
         'reservation_line_id': ?reservationLineId,
         'rows': ?rows,
         'sales_mode': ?salesMode,
         'sell_unit_ref': ?sellUnitRef,
         'source_branch_id': ?sourceBranchId,
         'source_ref': ?sourceRef,
         'source_stock_basis_id': ?sourceStockBasisId,
         'source_type': ?sourceType,
         'stock_basis_id': ?stockBasisId,
         'stock_basis_ids': ?stockBasisIds,
         'stocktake_type': ?stocktakeType,
         'supplier_ref': ?supplierRef,
         'target_quantity': ?targetQuantity,
         'unit_cost_minor': ?unitCostMinor,
         'variant_ref': ?variantRef,
       });
  PostShopInventoryReservationsReservationIdCommitRequest.fromJson(super.json);
  String? get action => value('action') as String?;
  String? get actualMeasureId => value('actual_measure_id') as String?;
  int? get actualQuantity => value('actual_quantity') as int?;
  String? get allocationPolicy => value('allocation_policy') as String?;
  String? get availableFrom => value('available_from') as String?;
  String? get batchNumber => value('batch_number') as String?;
  String? get bestBeforeAt => value('best_before_at') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get consumptionMode => value('consumption_mode') as String?;
  int? get countedQuantity => value('counted_quantity') as int?;
  String? get cutoffAt => value('cutoff_at') as String?;
  int? get damagedQuantity => value('damaged_quantity') as int?;
  String? get destinationBranchId => value('destination_branch_id') as String?;
  String? get destinationStockBasisId =>
      value('destination_stock_basis_id') as String?;
  bool? get dryRun => value('dry_run') as bool?;
  int? get expectedCatalogueRevision =>
      value('expected_catalogue_revision') as int?;
  int? get expectedInventoryRevision =>
      value('expected_inventory_revision') as int?;
  String? get expectedRestockAt => value('expected_restock_at') as String?;
  int? get expectedRevision => value('expected_revision') as int?;
  int? get expectedVersion => value('expected_version') as int?;
  String? get expiresAt => value('expires_at') as String?;
  int? get expiringSoonHours => value('expiring_soon_hours') as int?;
  int? get leadTimeDays => value('lead_time_days') as int?;
  String? get lineId => value('line_id') as String?;
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
  String? get listingId => value('listing_id') as String?;
  String? get listingRef => value('listing_ref') as String?;
  String? get lotNumber => value('lot_number') as String?;
  bool? get lotTracking => value('lot_tracking') as bool?;
  String? get manufacturedAt => value('manufactured_at') as String?;
  int? get maximumOutstandingQuantity =>
      value('maximum_outstanding_quantity') as int?;
  int? get maximumPerRequest => value('maximum_per_request') as int?;
  String? get measureId => value('measure_id') as String?;
  String? get measuredAt => value('measured_at') as String?;
  String? get measuredBy => value('measured_by') as String?;
  int? get minimumQuantity => value('minimum_quantity') as int?;
  int? get missingQuantity => value('missing_quantity') as int?;
  String? get mode => value('mode') as String?;
  String? get note => value('note') as String?;
  String? get notes => value('notes') as String?;
  String? get operationType => value('operation_type') as String?;
  int? get quantity => value('quantity') as int?;
  int? get quantityDelta => value('quantity_delta') as int?;
  String? get reasonCode => value('reason_code') as String?;
  String? get receivedAt => value('received_at') as String?;
  int? get receivedQuantity => value('received_quantity') as int?;
  String? get reservationLineId => value('reservation_line_id') as String?;
  List<dynamic>? get rows => value('rows') as List<dynamic>?;
  String? get salesMode => value('sales_mode') as String?;
  String? get sellUnitRef => value('sell_unit_ref') as String?;
  String? get sourceBranchId => value('source_branch_id') as String?;
  String? get sourceRef => value('source_ref') as String?;
  String? get sourceStockBasisId => value('source_stock_basis_id') as String?;
  String? get sourceType => value('source_type') as String?;
  String? get stockBasisId => value('stock_basis_id') as String?;
  List<dynamic>? get stockBasisIds =>
      value('stock_basis_ids') as List<dynamic>?;
  String? get stocktakeType => value('stocktake_type') as String?;
  String? get supplierRef => value('supplier_ref') as String?;
  int? get targetQuantity => value('target_quantity') as int?;
  int? get unitCostMinor => value('unit_cost_minor') as int?;
  String? get variantRef => value('variant_ref') as String?;
}

class PostShopInventoryReservationsReservationIdConfirmRequest
    extends VendorPayload {
  PostShopInventoryReservationsReservationIdConfirmRequest({
    String? action,
    String? actualMeasureId,
    int? actualQuantity,
    String? allocationPolicy,
    String? availableFrom,
    String? batchNumber,
    String? bestBeforeAt,
    String? branchId,
    String? consumptionMode,
    int? countedQuantity,
    String? cutoffAt,
    int? damagedQuantity,
    String? destinationBranchId,
    String? destinationStockBasisId,
    bool? dryRun,
    int? expectedCatalogueRevision,
    int? expectedInventoryRevision,
    String? expectedRestockAt,
    int? expectedRevision,
    int? expectedVersion,
    String? expiresAt,
    int? expiringSoonHours,
    int? leadTimeDays,
    String? lineId,
    List<dynamic>? lines,
    String? listingId,
    String? listingRef,
    String? lotNumber,
    bool? lotTracking,
    String? manufacturedAt,
    int? maximumOutstandingQuantity,
    int? maximumPerRequest,
    String? measureId,
    String? measuredAt,
    String? measuredBy,
    int? minimumQuantity,
    int? missingQuantity,
    String? mode,
    String? note,
    String? notes,
    String? operationType,
    int? quantity,
    int? quantityDelta,
    String? reasonCode,
    String? receivedAt,
    int? receivedQuantity,
    String? reservationLineId,
    List<dynamic>? rows,
    String? salesMode,
    String? sellUnitRef,
    String? sourceBranchId,
    String? sourceRef,
    String? sourceStockBasisId,
    String? sourceType,
    String? stockBasisId,
    List<dynamic>? stockBasisIds,
    String? stocktakeType,
    String? supplierRef,
    int? targetQuantity,
    int? unitCostMinor,
    String? variantRef,
  }) : super({
         'action': ?action,
         'actual_measure_id': ?actualMeasureId,
         'actual_quantity': ?actualQuantity,
         'allocation_policy': ?allocationPolicy,
         'available_from': ?availableFrom,
         'batch_number': ?batchNumber,
         'best_before_at': ?bestBeforeAt,
         'branch_id': ?branchId,
         'consumption_mode': ?consumptionMode,
         'counted_quantity': ?countedQuantity,
         'cutoff_at': ?cutoffAt,
         'damaged_quantity': ?damagedQuantity,
         'destination_branch_id': ?destinationBranchId,
         'destination_stock_basis_id': ?destinationStockBasisId,
         'dry_run': ?dryRun,
         'expected_catalogue_revision': ?expectedCatalogueRevision,
         'expected_inventory_revision': ?expectedInventoryRevision,
         'expected_restock_at': ?expectedRestockAt,
         'expected_revision': ?expectedRevision,
         'expected_version': ?expectedVersion,
         'expires_at': ?expiresAt,
         'expiring_soon_hours': ?expiringSoonHours,
         'lead_time_days': ?leadTimeDays,
         'line_id': ?lineId,
         'lines': ?lines,
         'listing_id': ?listingId,
         'listing_ref': ?listingRef,
         'lot_number': ?lotNumber,
         'lot_tracking': ?lotTracking,
         'manufactured_at': ?manufacturedAt,
         'maximum_outstanding_quantity': ?maximumOutstandingQuantity,
         'maximum_per_request': ?maximumPerRequest,
         'measure_id': ?measureId,
         'measured_at': ?measuredAt,
         'measured_by': ?measuredBy,
         'minimum_quantity': ?minimumQuantity,
         'missing_quantity': ?missingQuantity,
         'mode': ?mode,
         'note': ?note,
         'notes': ?notes,
         'operation_type': ?operationType,
         'quantity': ?quantity,
         'quantity_delta': ?quantityDelta,
         'reason_code': ?reasonCode,
         'received_at': ?receivedAt,
         'received_quantity': ?receivedQuantity,
         'reservation_line_id': ?reservationLineId,
         'rows': ?rows,
         'sales_mode': ?salesMode,
         'sell_unit_ref': ?sellUnitRef,
         'source_branch_id': ?sourceBranchId,
         'source_ref': ?sourceRef,
         'source_stock_basis_id': ?sourceStockBasisId,
         'source_type': ?sourceType,
         'stock_basis_id': ?stockBasisId,
         'stock_basis_ids': ?stockBasisIds,
         'stocktake_type': ?stocktakeType,
         'supplier_ref': ?supplierRef,
         'target_quantity': ?targetQuantity,
         'unit_cost_minor': ?unitCostMinor,
         'variant_ref': ?variantRef,
       });
  PostShopInventoryReservationsReservationIdConfirmRequest.fromJson(super.json);
  String? get action => value('action') as String?;
  String? get actualMeasureId => value('actual_measure_id') as String?;
  int? get actualQuantity => value('actual_quantity') as int?;
  String? get allocationPolicy => value('allocation_policy') as String?;
  String? get availableFrom => value('available_from') as String?;
  String? get batchNumber => value('batch_number') as String?;
  String? get bestBeforeAt => value('best_before_at') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get consumptionMode => value('consumption_mode') as String?;
  int? get countedQuantity => value('counted_quantity') as int?;
  String? get cutoffAt => value('cutoff_at') as String?;
  int? get damagedQuantity => value('damaged_quantity') as int?;
  String? get destinationBranchId => value('destination_branch_id') as String?;
  String? get destinationStockBasisId =>
      value('destination_stock_basis_id') as String?;
  bool? get dryRun => value('dry_run') as bool?;
  int? get expectedCatalogueRevision =>
      value('expected_catalogue_revision') as int?;
  int? get expectedInventoryRevision =>
      value('expected_inventory_revision') as int?;
  String? get expectedRestockAt => value('expected_restock_at') as String?;
  int? get expectedRevision => value('expected_revision') as int?;
  int? get expectedVersion => value('expected_version') as int?;
  String? get expiresAt => value('expires_at') as String?;
  int? get expiringSoonHours => value('expiring_soon_hours') as int?;
  int? get leadTimeDays => value('lead_time_days') as int?;
  String? get lineId => value('line_id') as String?;
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
  String? get listingId => value('listing_id') as String?;
  String? get listingRef => value('listing_ref') as String?;
  String? get lotNumber => value('lot_number') as String?;
  bool? get lotTracking => value('lot_tracking') as bool?;
  String? get manufacturedAt => value('manufactured_at') as String?;
  int? get maximumOutstandingQuantity =>
      value('maximum_outstanding_quantity') as int?;
  int? get maximumPerRequest => value('maximum_per_request') as int?;
  String? get measureId => value('measure_id') as String?;
  String? get measuredAt => value('measured_at') as String?;
  String? get measuredBy => value('measured_by') as String?;
  int? get minimumQuantity => value('minimum_quantity') as int?;
  int? get missingQuantity => value('missing_quantity') as int?;
  String? get mode => value('mode') as String?;
  String? get note => value('note') as String?;
  String? get notes => value('notes') as String?;
  String? get operationType => value('operation_type') as String?;
  int? get quantity => value('quantity') as int?;
  int? get quantityDelta => value('quantity_delta') as int?;
  String? get reasonCode => value('reason_code') as String?;
  String? get receivedAt => value('received_at') as String?;
  int? get receivedQuantity => value('received_quantity') as int?;
  String? get reservationLineId => value('reservation_line_id') as String?;
  List<dynamic>? get rows => value('rows') as List<dynamic>?;
  String? get salesMode => value('sales_mode') as String?;
  String? get sellUnitRef => value('sell_unit_ref') as String?;
  String? get sourceBranchId => value('source_branch_id') as String?;
  String? get sourceRef => value('source_ref') as String?;
  String? get sourceStockBasisId => value('source_stock_basis_id') as String?;
  String? get sourceType => value('source_type') as String?;
  String? get stockBasisId => value('stock_basis_id') as String?;
  List<dynamic>? get stockBasisIds =>
      value('stock_basis_ids') as List<dynamic>?;
  String? get stocktakeType => value('stocktake_type') as String?;
  String? get supplierRef => value('supplier_ref') as String?;
  int? get targetQuantity => value('target_quantity') as int?;
  int? get unitCostMinor => value('unit_cost_minor') as int?;
  String? get variantRef => value('variant_ref') as String?;
}

class PostShopInventoryReservationsReservationIdReleaseRequest
    extends VendorPayload {
  PostShopInventoryReservationsReservationIdReleaseRequest({
    String? action,
    String? actualMeasureId,
    int? actualQuantity,
    String? allocationPolicy,
    String? availableFrom,
    String? batchNumber,
    String? bestBeforeAt,
    String? branchId,
    String? consumptionMode,
    int? countedQuantity,
    String? cutoffAt,
    int? damagedQuantity,
    String? destinationBranchId,
    String? destinationStockBasisId,
    bool? dryRun,
    int? expectedCatalogueRevision,
    int? expectedInventoryRevision,
    String? expectedRestockAt,
    int? expectedRevision,
    int? expectedVersion,
    String? expiresAt,
    int? expiringSoonHours,
    int? leadTimeDays,
    String? lineId,
    List<dynamic>? lines,
    String? listingId,
    String? listingRef,
    String? lotNumber,
    bool? lotTracking,
    String? manufacturedAt,
    int? maximumOutstandingQuantity,
    int? maximumPerRequest,
    String? measureId,
    String? measuredAt,
    String? measuredBy,
    int? minimumQuantity,
    int? missingQuantity,
    String? mode,
    String? note,
    String? notes,
    String? operationType,
    int? quantity,
    int? quantityDelta,
    String? reasonCode,
    String? receivedAt,
    int? receivedQuantity,
    String? reservationLineId,
    List<dynamic>? rows,
    String? salesMode,
    String? sellUnitRef,
    String? sourceBranchId,
    String? sourceRef,
    String? sourceStockBasisId,
    String? sourceType,
    String? stockBasisId,
    List<dynamic>? stockBasisIds,
    String? stocktakeType,
    String? supplierRef,
    int? targetQuantity,
    int? unitCostMinor,
    String? variantRef,
  }) : super({
         'action': ?action,
         'actual_measure_id': ?actualMeasureId,
         'actual_quantity': ?actualQuantity,
         'allocation_policy': ?allocationPolicy,
         'available_from': ?availableFrom,
         'batch_number': ?batchNumber,
         'best_before_at': ?bestBeforeAt,
         'branch_id': ?branchId,
         'consumption_mode': ?consumptionMode,
         'counted_quantity': ?countedQuantity,
         'cutoff_at': ?cutoffAt,
         'damaged_quantity': ?damagedQuantity,
         'destination_branch_id': ?destinationBranchId,
         'destination_stock_basis_id': ?destinationStockBasisId,
         'dry_run': ?dryRun,
         'expected_catalogue_revision': ?expectedCatalogueRevision,
         'expected_inventory_revision': ?expectedInventoryRevision,
         'expected_restock_at': ?expectedRestockAt,
         'expected_revision': ?expectedRevision,
         'expected_version': ?expectedVersion,
         'expires_at': ?expiresAt,
         'expiring_soon_hours': ?expiringSoonHours,
         'lead_time_days': ?leadTimeDays,
         'line_id': ?lineId,
         'lines': ?lines,
         'listing_id': ?listingId,
         'listing_ref': ?listingRef,
         'lot_number': ?lotNumber,
         'lot_tracking': ?lotTracking,
         'manufactured_at': ?manufacturedAt,
         'maximum_outstanding_quantity': ?maximumOutstandingQuantity,
         'maximum_per_request': ?maximumPerRequest,
         'measure_id': ?measureId,
         'measured_at': ?measuredAt,
         'measured_by': ?measuredBy,
         'minimum_quantity': ?minimumQuantity,
         'missing_quantity': ?missingQuantity,
         'mode': ?mode,
         'note': ?note,
         'notes': ?notes,
         'operation_type': ?operationType,
         'quantity': ?quantity,
         'quantity_delta': ?quantityDelta,
         'reason_code': ?reasonCode,
         'received_at': ?receivedAt,
         'received_quantity': ?receivedQuantity,
         'reservation_line_id': ?reservationLineId,
         'rows': ?rows,
         'sales_mode': ?salesMode,
         'sell_unit_ref': ?sellUnitRef,
         'source_branch_id': ?sourceBranchId,
         'source_ref': ?sourceRef,
         'source_stock_basis_id': ?sourceStockBasisId,
         'source_type': ?sourceType,
         'stock_basis_id': ?stockBasisId,
         'stock_basis_ids': ?stockBasisIds,
         'stocktake_type': ?stocktakeType,
         'supplier_ref': ?supplierRef,
         'target_quantity': ?targetQuantity,
         'unit_cost_minor': ?unitCostMinor,
         'variant_ref': ?variantRef,
       });
  PostShopInventoryReservationsReservationIdReleaseRequest.fromJson(super.json);
  String? get action => value('action') as String?;
  String? get actualMeasureId => value('actual_measure_id') as String?;
  int? get actualQuantity => value('actual_quantity') as int?;
  String? get allocationPolicy => value('allocation_policy') as String?;
  String? get availableFrom => value('available_from') as String?;
  String? get batchNumber => value('batch_number') as String?;
  String? get bestBeforeAt => value('best_before_at') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get consumptionMode => value('consumption_mode') as String?;
  int? get countedQuantity => value('counted_quantity') as int?;
  String? get cutoffAt => value('cutoff_at') as String?;
  int? get damagedQuantity => value('damaged_quantity') as int?;
  String? get destinationBranchId => value('destination_branch_id') as String?;
  String? get destinationStockBasisId =>
      value('destination_stock_basis_id') as String?;
  bool? get dryRun => value('dry_run') as bool?;
  int? get expectedCatalogueRevision =>
      value('expected_catalogue_revision') as int?;
  int? get expectedInventoryRevision =>
      value('expected_inventory_revision') as int?;
  String? get expectedRestockAt => value('expected_restock_at') as String?;
  int? get expectedRevision => value('expected_revision') as int?;
  int? get expectedVersion => value('expected_version') as int?;
  String? get expiresAt => value('expires_at') as String?;
  int? get expiringSoonHours => value('expiring_soon_hours') as int?;
  int? get leadTimeDays => value('lead_time_days') as int?;
  String? get lineId => value('line_id') as String?;
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
  String? get listingId => value('listing_id') as String?;
  String? get listingRef => value('listing_ref') as String?;
  String? get lotNumber => value('lot_number') as String?;
  bool? get lotTracking => value('lot_tracking') as bool?;
  String? get manufacturedAt => value('manufactured_at') as String?;
  int? get maximumOutstandingQuantity =>
      value('maximum_outstanding_quantity') as int?;
  int? get maximumPerRequest => value('maximum_per_request') as int?;
  String? get measureId => value('measure_id') as String?;
  String? get measuredAt => value('measured_at') as String?;
  String? get measuredBy => value('measured_by') as String?;
  int? get minimumQuantity => value('minimum_quantity') as int?;
  int? get missingQuantity => value('missing_quantity') as int?;
  String? get mode => value('mode') as String?;
  String? get note => value('note') as String?;
  String? get notes => value('notes') as String?;
  String? get operationType => value('operation_type') as String?;
  int? get quantity => value('quantity') as int?;
  int? get quantityDelta => value('quantity_delta') as int?;
  String? get reasonCode => value('reason_code') as String?;
  String? get receivedAt => value('received_at') as String?;
  int? get receivedQuantity => value('received_quantity') as int?;
  String? get reservationLineId => value('reservation_line_id') as String?;
  List<dynamic>? get rows => value('rows') as List<dynamic>?;
  String? get salesMode => value('sales_mode') as String?;
  String? get sellUnitRef => value('sell_unit_ref') as String?;
  String? get sourceBranchId => value('source_branch_id') as String?;
  String? get sourceRef => value('source_ref') as String?;
  String? get sourceStockBasisId => value('source_stock_basis_id') as String?;
  String? get sourceType => value('source_type') as String?;
  String? get stockBasisId => value('stock_basis_id') as String?;
  List<dynamic>? get stockBasisIds =>
      value('stock_basis_ids') as List<dynamic>?;
  String? get stocktakeType => value('stocktake_type') as String?;
  String? get supplierRef => value('supplier_ref') as String?;
  int? get targetQuantity => value('target_quantity') as int?;
  int? get unitCostMinor => value('unit_cost_minor') as int?;
  String? get variantRef => value('variant_ref') as String?;
}

class PutShopInventoryStockBasesStockBasisIdReorderRuleRequest
    extends VendorPayload {
  PutShopInventoryStockBasesStockBasisIdReorderRuleRequest({
    int? minimumQuantity,
    int? targetQuantity,
    int? expiringSoonHours,
  }) : super({
         'minimum_quantity': ?minimumQuantity,
         'target_quantity': ?targetQuantity,
         'expiring_soon_hours': ?expiringSoonHours,
       });
  PutShopInventoryStockBasesStockBasisIdReorderRuleRequest.fromJson(super.json);
  int? get minimumQuantity => value('minimum_quantity') as int?;
  int? get targetQuantity => value('target_quantity') as int?;
  int? get expiringSoonHours => value('expiring_soon_hours') as int?;
}

class PutShopInventoryStocktakesStocktakeIdLinesStockBasisIdRequest
    extends VendorPayload {
  PutShopInventoryStocktakesStocktakeIdLinesStockBasisIdRequest({
    int? countedQuantity,
  }) : super({'counted_quantity': ?countedQuantity});
  PutShopInventoryStocktakesStocktakeIdLinesStockBasisIdRequest.fromJson(
    super.json,
  );
  int? get countedQuantity => value('counted_quantity') as int?;
}

class PostShopInventoryStocktakesStocktakeIdPostRequest extends VendorPayload {
  PostShopInventoryStocktakesStocktakeIdPostRequest({
    String? action,
    String? actualMeasureId,
    int? actualQuantity,
    String? allocationPolicy,
    String? availableFrom,
    String? batchNumber,
    String? bestBeforeAt,
    String? branchId,
    String? consumptionMode,
    int? countedQuantity,
    String? cutoffAt,
    int? damagedQuantity,
    String? destinationBranchId,
    String? destinationStockBasisId,
    bool? dryRun,
    int? expectedCatalogueRevision,
    int? expectedInventoryRevision,
    String? expectedRestockAt,
    int? expectedRevision,
    int? expectedVersion,
    String? expiresAt,
    int? expiringSoonHours,
    int? leadTimeDays,
    String? lineId,
    List<dynamic>? lines,
    String? listingId,
    String? listingRef,
    String? lotNumber,
    bool? lotTracking,
    String? manufacturedAt,
    int? maximumOutstandingQuantity,
    int? maximumPerRequest,
    String? measureId,
    String? measuredAt,
    String? measuredBy,
    int? minimumQuantity,
    int? missingQuantity,
    String? mode,
    String? note,
    String? notes,
    String? operationType,
    int? quantity,
    int? quantityDelta,
    String? reasonCode,
    String? receivedAt,
    int? receivedQuantity,
    String? reservationLineId,
    List<dynamic>? rows,
    String? salesMode,
    String? sellUnitRef,
    String? sourceBranchId,
    String? sourceRef,
    String? sourceStockBasisId,
    String? sourceType,
    String? stockBasisId,
    List<dynamic>? stockBasisIds,
    String? stocktakeType,
    String? supplierRef,
    int? targetQuantity,
    int? unitCostMinor,
    String? variantRef,
  }) : super({
         'action': ?action,
         'actual_measure_id': ?actualMeasureId,
         'actual_quantity': ?actualQuantity,
         'allocation_policy': ?allocationPolicy,
         'available_from': ?availableFrom,
         'batch_number': ?batchNumber,
         'best_before_at': ?bestBeforeAt,
         'branch_id': ?branchId,
         'consumption_mode': ?consumptionMode,
         'counted_quantity': ?countedQuantity,
         'cutoff_at': ?cutoffAt,
         'damaged_quantity': ?damagedQuantity,
         'destination_branch_id': ?destinationBranchId,
         'destination_stock_basis_id': ?destinationStockBasisId,
         'dry_run': ?dryRun,
         'expected_catalogue_revision': ?expectedCatalogueRevision,
         'expected_inventory_revision': ?expectedInventoryRevision,
         'expected_restock_at': ?expectedRestockAt,
         'expected_revision': ?expectedRevision,
         'expected_version': ?expectedVersion,
         'expires_at': ?expiresAt,
         'expiring_soon_hours': ?expiringSoonHours,
         'lead_time_days': ?leadTimeDays,
         'line_id': ?lineId,
         'lines': ?lines,
         'listing_id': ?listingId,
         'listing_ref': ?listingRef,
         'lot_number': ?lotNumber,
         'lot_tracking': ?lotTracking,
         'manufactured_at': ?manufacturedAt,
         'maximum_outstanding_quantity': ?maximumOutstandingQuantity,
         'maximum_per_request': ?maximumPerRequest,
         'measure_id': ?measureId,
         'measured_at': ?measuredAt,
         'measured_by': ?measuredBy,
         'minimum_quantity': ?minimumQuantity,
         'missing_quantity': ?missingQuantity,
         'mode': ?mode,
         'note': ?note,
         'notes': ?notes,
         'operation_type': ?operationType,
         'quantity': ?quantity,
         'quantity_delta': ?quantityDelta,
         'reason_code': ?reasonCode,
         'received_at': ?receivedAt,
         'received_quantity': ?receivedQuantity,
         'reservation_line_id': ?reservationLineId,
         'rows': ?rows,
         'sales_mode': ?salesMode,
         'sell_unit_ref': ?sellUnitRef,
         'source_branch_id': ?sourceBranchId,
         'source_ref': ?sourceRef,
         'source_stock_basis_id': ?sourceStockBasisId,
         'source_type': ?sourceType,
         'stock_basis_id': ?stockBasisId,
         'stock_basis_ids': ?stockBasisIds,
         'stocktake_type': ?stocktakeType,
         'supplier_ref': ?supplierRef,
         'target_quantity': ?targetQuantity,
         'unit_cost_minor': ?unitCostMinor,
         'variant_ref': ?variantRef,
       });
  PostShopInventoryStocktakesStocktakeIdPostRequest.fromJson(super.json);
  String? get action => value('action') as String?;
  String? get actualMeasureId => value('actual_measure_id') as String?;
  int? get actualQuantity => value('actual_quantity') as int?;
  String? get allocationPolicy => value('allocation_policy') as String?;
  String? get availableFrom => value('available_from') as String?;
  String? get batchNumber => value('batch_number') as String?;
  String? get bestBeforeAt => value('best_before_at') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get consumptionMode => value('consumption_mode') as String?;
  int? get countedQuantity => value('counted_quantity') as int?;
  String? get cutoffAt => value('cutoff_at') as String?;
  int? get damagedQuantity => value('damaged_quantity') as int?;
  String? get destinationBranchId => value('destination_branch_id') as String?;
  String? get destinationStockBasisId =>
      value('destination_stock_basis_id') as String?;
  bool? get dryRun => value('dry_run') as bool?;
  int? get expectedCatalogueRevision =>
      value('expected_catalogue_revision') as int?;
  int? get expectedInventoryRevision =>
      value('expected_inventory_revision') as int?;
  String? get expectedRestockAt => value('expected_restock_at') as String?;
  int? get expectedRevision => value('expected_revision') as int?;
  int? get expectedVersion => value('expected_version') as int?;
  String? get expiresAt => value('expires_at') as String?;
  int? get expiringSoonHours => value('expiring_soon_hours') as int?;
  int? get leadTimeDays => value('lead_time_days') as int?;
  String? get lineId => value('line_id') as String?;
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
  String? get listingId => value('listing_id') as String?;
  String? get listingRef => value('listing_ref') as String?;
  String? get lotNumber => value('lot_number') as String?;
  bool? get lotTracking => value('lot_tracking') as bool?;
  String? get manufacturedAt => value('manufactured_at') as String?;
  int? get maximumOutstandingQuantity =>
      value('maximum_outstanding_quantity') as int?;
  int? get maximumPerRequest => value('maximum_per_request') as int?;
  String? get measureId => value('measure_id') as String?;
  String? get measuredAt => value('measured_at') as String?;
  String? get measuredBy => value('measured_by') as String?;
  int? get minimumQuantity => value('minimum_quantity') as int?;
  int? get missingQuantity => value('missing_quantity') as int?;
  String? get mode => value('mode') as String?;
  String? get note => value('note') as String?;
  String? get notes => value('notes') as String?;
  String? get operationType => value('operation_type') as String?;
  int? get quantity => value('quantity') as int?;
  int? get quantityDelta => value('quantity_delta') as int?;
  String? get reasonCode => value('reason_code') as String?;
  String? get receivedAt => value('received_at') as String?;
  int? get receivedQuantity => value('received_quantity') as int?;
  String? get reservationLineId => value('reservation_line_id') as String?;
  List<dynamic>? get rows => value('rows') as List<dynamic>?;
  String? get salesMode => value('sales_mode') as String?;
  String? get sellUnitRef => value('sell_unit_ref') as String?;
  String? get sourceBranchId => value('source_branch_id') as String?;
  String? get sourceRef => value('source_ref') as String?;
  String? get sourceStockBasisId => value('source_stock_basis_id') as String?;
  String? get sourceType => value('source_type') as String?;
  String? get stockBasisId => value('stock_basis_id') as String?;
  List<dynamic>? get stockBasisIds =>
      value('stock_basis_ids') as List<dynamic>?;
  String? get stocktakeType => value('stocktake_type') as String?;
  String? get supplierRef => value('supplier_ref') as String?;
  int? get targetQuantity => value('target_quantity') as int?;
  int? get unitCostMinor => value('unit_cost_minor') as int?;
  String? get variantRef => value('variant_ref') as String?;
}

class PostShopInventoryTransfersRequest extends VendorPayload {
  PostShopInventoryTransfersRequest({
    String? sourceBranchId,
    String? destinationBranchId,
    String? notes,
    List<dynamic>? lines,
  }) : super({
         'source_branch_id': ?sourceBranchId,
         'destination_branch_id': ?destinationBranchId,
         'notes': ?notes,
         'lines': ?lines,
       });
  PostShopInventoryTransfersRequest.fromJson(super.json);
  String? get sourceBranchId => value('source_branch_id') as String?;
  String? get destinationBranchId => value('destination_branch_id') as String?;
  String? get notes => value('notes') as String?;
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
}

class PostShopInventoryTransfersTransferIdCancelRequest extends VendorPayload {
  PostShopInventoryTransfersTransferIdCancelRequest({
    String? action,
    String? actualMeasureId,
    int? actualQuantity,
    String? allocationPolicy,
    String? availableFrom,
    String? batchNumber,
    String? bestBeforeAt,
    String? branchId,
    String? consumptionMode,
    int? countedQuantity,
    String? cutoffAt,
    int? damagedQuantity,
    String? destinationBranchId,
    String? destinationStockBasisId,
    bool? dryRun,
    int? expectedCatalogueRevision,
    int? expectedInventoryRevision,
    String? expectedRestockAt,
    int? expectedRevision,
    int? expectedVersion,
    String? expiresAt,
    int? expiringSoonHours,
    int? leadTimeDays,
    String? lineId,
    List<dynamic>? lines,
    String? listingId,
    String? listingRef,
    String? lotNumber,
    bool? lotTracking,
    String? manufacturedAt,
    int? maximumOutstandingQuantity,
    int? maximumPerRequest,
    String? measureId,
    String? measuredAt,
    String? measuredBy,
    int? minimumQuantity,
    int? missingQuantity,
    String? mode,
    String? note,
    String? notes,
    String? operationType,
    int? quantity,
    int? quantityDelta,
    String? reasonCode,
    String? receivedAt,
    int? receivedQuantity,
    String? reservationLineId,
    List<dynamic>? rows,
    String? salesMode,
    String? sellUnitRef,
    String? sourceBranchId,
    String? sourceRef,
    String? sourceStockBasisId,
    String? sourceType,
    String? stockBasisId,
    List<dynamic>? stockBasisIds,
    String? stocktakeType,
    String? supplierRef,
    int? targetQuantity,
    int? unitCostMinor,
    String? variantRef,
  }) : super({
         'action': ?action,
         'actual_measure_id': ?actualMeasureId,
         'actual_quantity': ?actualQuantity,
         'allocation_policy': ?allocationPolicy,
         'available_from': ?availableFrom,
         'batch_number': ?batchNumber,
         'best_before_at': ?bestBeforeAt,
         'branch_id': ?branchId,
         'consumption_mode': ?consumptionMode,
         'counted_quantity': ?countedQuantity,
         'cutoff_at': ?cutoffAt,
         'damaged_quantity': ?damagedQuantity,
         'destination_branch_id': ?destinationBranchId,
         'destination_stock_basis_id': ?destinationStockBasisId,
         'dry_run': ?dryRun,
         'expected_catalogue_revision': ?expectedCatalogueRevision,
         'expected_inventory_revision': ?expectedInventoryRevision,
         'expected_restock_at': ?expectedRestockAt,
         'expected_revision': ?expectedRevision,
         'expected_version': ?expectedVersion,
         'expires_at': ?expiresAt,
         'expiring_soon_hours': ?expiringSoonHours,
         'lead_time_days': ?leadTimeDays,
         'line_id': ?lineId,
         'lines': ?lines,
         'listing_id': ?listingId,
         'listing_ref': ?listingRef,
         'lot_number': ?lotNumber,
         'lot_tracking': ?lotTracking,
         'manufactured_at': ?manufacturedAt,
         'maximum_outstanding_quantity': ?maximumOutstandingQuantity,
         'maximum_per_request': ?maximumPerRequest,
         'measure_id': ?measureId,
         'measured_at': ?measuredAt,
         'measured_by': ?measuredBy,
         'minimum_quantity': ?minimumQuantity,
         'missing_quantity': ?missingQuantity,
         'mode': ?mode,
         'note': ?note,
         'notes': ?notes,
         'operation_type': ?operationType,
         'quantity': ?quantity,
         'quantity_delta': ?quantityDelta,
         'reason_code': ?reasonCode,
         'received_at': ?receivedAt,
         'received_quantity': ?receivedQuantity,
         'reservation_line_id': ?reservationLineId,
         'rows': ?rows,
         'sales_mode': ?salesMode,
         'sell_unit_ref': ?sellUnitRef,
         'source_branch_id': ?sourceBranchId,
         'source_ref': ?sourceRef,
         'source_stock_basis_id': ?sourceStockBasisId,
         'source_type': ?sourceType,
         'stock_basis_id': ?stockBasisId,
         'stock_basis_ids': ?stockBasisIds,
         'stocktake_type': ?stocktakeType,
         'supplier_ref': ?supplierRef,
         'target_quantity': ?targetQuantity,
         'unit_cost_minor': ?unitCostMinor,
         'variant_ref': ?variantRef,
       });
  PostShopInventoryTransfersTransferIdCancelRequest.fromJson(super.json);
  String? get action => value('action') as String?;
  String? get actualMeasureId => value('actual_measure_id') as String?;
  int? get actualQuantity => value('actual_quantity') as int?;
  String? get allocationPolicy => value('allocation_policy') as String?;
  String? get availableFrom => value('available_from') as String?;
  String? get batchNumber => value('batch_number') as String?;
  String? get bestBeforeAt => value('best_before_at') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get consumptionMode => value('consumption_mode') as String?;
  int? get countedQuantity => value('counted_quantity') as int?;
  String? get cutoffAt => value('cutoff_at') as String?;
  int? get damagedQuantity => value('damaged_quantity') as int?;
  String? get destinationBranchId => value('destination_branch_id') as String?;
  String? get destinationStockBasisId =>
      value('destination_stock_basis_id') as String?;
  bool? get dryRun => value('dry_run') as bool?;
  int? get expectedCatalogueRevision =>
      value('expected_catalogue_revision') as int?;
  int? get expectedInventoryRevision =>
      value('expected_inventory_revision') as int?;
  String? get expectedRestockAt => value('expected_restock_at') as String?;
  int? get expectedRevision => value('expected_revision') as int?;
  int? get expectedVersion => value('expected_version') as int?;
  String? get expiresAt => value('expires_at') as String?;
  int? get expiringSoonHours => value('expiring_soon_hours') as int?;
  int? get leadTimeDays => value('lead_time_days') as int?;
  String? get lineId => value('line_id') as String?;
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
  String? get listingId => value('listing_id') as String?;
  String? get listingRef => value('listing_ref') as String?;
  String? get lotNumber => value('lot_number') as String?;
  bool? get lotTracking => value('lot_tracking') as bool?;
  String? get manufacturedAt => value('manufactured_at') as String?;
  int? get maximumOutstandingQuantity =>
      value('maximum_outstanding_quantity') as int?;
  int? get maximumPerRequest => value('maximum_per_request') as int?;
  String? get measureId => value('measure_id') as String?;
  String? get measuredAt => value('measured_at') as String?;
  String? get measuredBy => value('measured_by') as String?;
  int? get minimumQuantity => value('minimum_quantity') as int?;
  int? get missingQuantity => value('missing_quantity') as int?;
  String? get mode => value('mode') as String?;
  String? get note => value('note') as String?;
  String? get notes => value('notes') as String?;
  String? get operationType => value('operation_type') as String?;
  int? get quantity => value('quantity') as int?;
  int? get quantityDelta => value('quantity_delta') as int?;
  String? get reasonCode => value('reason_code') as String?;
  String? get receivedAt => value('received_at') as String?;
  int? get receivedQuantity => value('received_quantity') as int?;
  String? get reservationLineId => value('reservation_line_id') as String?;
  List<dynamic>? get rows => value('rows') as List<dynamic>?;
  String? get salesMode => value('sales_mode') as String?;
  String? get sellUnitRef => value('sell_unit_ref') as String?;
  String? get sourceBranchId => value('source_branch_id') as String?;
  String? get sourceRef => value('source_ref') as String?;
  String? get sourceStockBasisId => value('source_stock_basis_id') as String?;
  String? get sourceType => value('source_type') as String?;
  String? get stockBasisId => value('stock_basis_id') as String?;
  List<dynamic>? get stockBasisIds =>
      value('stock_basis_ids') as List<dynamic>?;
  String? get stocktakeType => value('stocktake_type') as String?;
  String? get supplierRef => value('supplier_ref') as String?;
  int? get targetQuantity => value('target_quantity') as int?;
  int? get unitCostMinor => value('unit_cost_minor') as int?;
  String? get variantRef => value('variant_ref') as String?;
}

class PostShopInventoryTransfersTransferIdDispatchRequest
    extends VendorPayload {
  PostShopInventoryTransfersTransferIdDispatchRequest({
    String? action,
    String? actualMeasureId,
    int? actualQuantity,
    String? allocationPolicy,
    String? availableFrom,
    String? batchNumber,
    String? bestBeforeAt,
    String? branchId,
    String? consumptionMode,
    int? countedQuantity,
    String? cutoffAt,
    int? damagedQuantity,
    String? destinationBranchId,
    String? destinationStockBasisId,
    bool? dryRun,
    int? expectedCatalogueRevision,
    int? expectedInventoryRevision,
    String? expectedRestockAt,
    int? expectedRevision,
    int? expectedVersion,
    String? expiresAt,
    int? expiringSoonHours,
    int? leadTimeDays,
    String? lineId,
    List<dynamic>? lines,
    String? listingId,
    String? listingRef,
    String? lotNumber,
    bool? lotTracking,
    String? manufacturedAt,
    int? maximumOutstandingQuantity,
    int? maximumPerRequest,
    String? measureId,
    String? measuredAt,
    String? measuredBy,
    int? minimumQuantity,
    int? missingQuantity,
    String? mode,
    String? note,
    String? notes,
    String? operationType,
    int? quantity,
    int? quantityDelta,
    String? reasonCode,
    String? receivedAt,
    int? receivedQuantity,
    String? reservationLineId,
    List<dynamic>? rows,
    String? salesMode,
    String? sellUnitRef,
    String? sourceBranchId,
    String? sourceRef,
    String? sourceStockBasisId,
    String? sourceType,
    String? stockBasisId,
    List<dynamic>? stockBasisIds,
    String? stocktakeType,
    String? supplierRef,
    int? targetQuantity,
    int? unitCostMinor,
    String? variantRef,
  }) : super({
         'action': ?action,
         'actual_measure_id': ?actualMeasureId,
         'actual_quantity': ?actualQuantity,
         'allocation_policy': ?allocationPolicy,
         'available_from': ?availableFrom,
         'batch_number': ?batchNumber,
         'best_before_at': ?bestBeforeAt,
         'branch_id': ?branchId,
         'consumption_mode': ?consumptionMode,
         'counted_quantity': ?countedQuantity,
         'cutoff_at': ?cutoffAt,
         'damaged_quantity': ?damagedQuantity,
         'destination_branch_id': ?destinationBranchId,
         'destination_stock_basis_id': ?destinationStockBasisId,
         'dry_run': ?dryRun,
         'expected_catalogue_revision': ?expectedCatalogueRevision,
         'expected_inventory_revision': ?expectedInventoryRevision,
         'expected_restock_at': ?expectedRestockAt,
         'expected_revision': ?expectedRevision,
         'expected_version': ?expectedVersion,
         'expires_at': ?expiresAt,
         'expiring_soon_hours': ?expiringSoonHours,
         'lead_time_days': ?leadTimeDays,
         'line_id': ?lineId,
         'lines': ?lines,
         'listing_id': ?listingId,
         'listing_ref': ?listingRef,
         'lot_number': ?lotNumber,
         'lot_tracking': ?lotTracking,
         'manufactured_at': ?manufacturedAt,
         'maximum_outstanding_quantity': ?maximumOutstandingQuantity,
         'maximum_per_request': ?maximumPerRequest,
         'measure_id': ?measureId,
         'measured_at': ?measuredAt,
         'measured_by': ?measuredBy,
         'minimum_quantity': ?minimumQuantity,
         'missing_quantity': ?missingQuantity,
         'mode': ?mode,
         'note': ?note,
         'notes': ?notes,
         'operation_type': ?operationType,
         'quantity': ?quantity,
         'quantity_delta': ?quantityDelta,
         'reason_code': ?reasonCode,
         'received_at': ?receivedAt,
         'received_quantity': ?receivedQuantity,
         'reservation_line_id': ?reservationLineId,
         'rows': ?rows,
         'sales_mode': ?salesMode,
         'sell_unit_ref': ?sellUnitRef,
         'source_branch_id': ?sourceBranchId,
         'source_ref': ?sourceRef,
         'source_stock_basis_id': ?sourceStockBasisId,
         'source_type': ?sourceType,
         'stock_basis_id': ?stockBasisId,
         'stock_basis_ids': ?stockBasisIds,
         'stocktake_type': ?stocktakeType,
         'supplier_ref': ?supplierRef,
         'target_quantity': ?targetQuantity,
         'unit_cost_minor': ?unitCostMinor,
         'variant_ref': ?variantRef,
       });
  PostShopInventoryTransfersTransferIdDispatchRequest.fromJson(super.json);
  String? get action => value('action') as String?;
  String? get actualMeasureId => value('actual_measure_id') as String?;
  int? get actualQuantity => value('actual_quantity') as int?;
  String? get allocationPolicy => value('allocation_policy') as String?;
  String? get availableFrom => value('available_from') as String?;
  String? get batchNumber => value('batch_number') as String?;
  String? get bestBeforeAt => value('best_before_at') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get consumptionMode => value('consumption_mode') as String?;
  int? get countedQuantity => value('counted_quantity') as int?;
  String? get cutoffAt => value('cutoff_at') as String?;
  int? get damagedQuantity => value('damaged_quantity') as int?;
  String? get destinationBranchId => value('destination_branch_id') as String?;
  String? get destinationStockBasisId =>
      value('destination_stock_basis_id') as String?;
  bool? get dryRun => value('dry_run') as bool?;
  int? get expectedCatalogueRevision =>
      value('expected_catalogue_revision') as int?;
  int? get expectedInventoryRevision =>
      value('expected_inventory_revision') as int?;
  String? get expectedRestockAt => value('expected_restock_at') as String?;
  int? get expectedRevision => value('expected_revision') as int?;
  int? get expectedVersion => value('expected_version') as int?;
  String? get expiresAt => value('expires_at') as String?;
  int? get expiringSoonHours => value('expiring_soon_hours') as int?;
  int? get leadTimeDays => value('lead_time_days') as int?;
  String? get lineId => value('line_id') as String?;
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
  String? get listingId => value('listing_id') as String?;
  String? get listingRef => value('listing_ref') as String?;
  String? get lotNumber => value('lot_number') as String?;
  bool? get lotTracking => value('lot_tracking') as bool?;
  String? get manufacturedAt => value('manufactured_at') as String?;
  int? get maximumOutstandingQuantity =>
      value('maximum_outstanding_quantity') as int?;
  int? get maximumPerRequest => value('maximum_per_request') as int?;
  String? get measureId => value('measure_id') as String?;
  String? get measuredAt => value('measured_at') as String?;
  String? get measuredBy => value('measured_by') as String?;
  int? get minimumQuantity => value('minimum_quantity') as int?;
  int? get missingQuantity => value('missing_quantity') as int?;
  String? get mode => value('mode') as String?;
  String? get note => value('note') as String?;
  String? get notes => value('notes') as String?;
  String? get operationType => value('operation_type') as String?;
  int? get quantity => value('quantity') as int?;
  int? get quantityDelta => value('quantity_delta') as int?;
  String? get reasonCode => value('reason_code') as String?;
  String? get receivedAt => value('received_at') as String?;
  int? get receivedQuantity => value('received_quantity') as int?;
  String? get reservationLineId => value('reservation_line_id') as String?;
  List<dynamic>? get rows => value('rows') as List<dynamic>?;
  String? get salesMode => value('sales_mode') as String?;
  String? get sellUnitRef => value('sell_unit_ref') as String?;
  String? get sourceBranchId => value('source_branch_id') as String?;
  String? get sourceRef => value('source_ref') as String?;
  String? get sourceStockBasisId => value('source_stock_basis_id') as String?;
  String? get sourceType => value('source_type') as String?;
  String? get stockBasisId => value('stock_basis_id') as String?;
  List<dynamic>? get stockBasisIds =>
      value('stock_basis_ids') as List<dynamic>?;
  String? get stocktakeType => value('stocktake_type') as String?;
  String? get supplierRef => value('supplier_ref') as String?;
  int? get targetQuantity => value('target_quantity') as int?;
  int? get unitCostMinor => value('unit_cost_minor') as int?;
  String? get variantRef => value('variant_ref') as String?;
}

class PostShopInventoryTransfersTransferIdReceiveRequest extends VendorPayload {
  PostShopInventoryTransfersTransferIdReceiveRequest({List<dynamic>? lines})
    : super({'lines': ?lines});
  PostShopInventoryTransfersTransferIdReceiveRequest.fromJson(super.json);
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
}

class PatchShopListingsListingIdRequest extends VendorPayload {
  PatchShopListingsListingIdRequest({
    String? operatingModel,
    bool? retailEnabled,
    bool? wholesaleEnabled,
    int? retailOverride,
    int? wholesaleOverride,
    int? minimumOrderQty,
    int? orderMultiple,
    int? maximumOrderQty,
    String? preorderPolicy,
    String? backorderPolicy,
    bool? autoHideUnavailable,
    bool? logisticsAvailable,
    List<dynamic>? sellUnitIds,
    String? status,
    int? expectedVersion,
  }) : super({
         'operating_model': ?operatingModel,
         'retail_enabled': ?retailEnabled,
         'wholesale_enabled': ?wholesaleEnabled,
         'retail_override': ?retailOverride,
         'wholesale_override': ?wholesaleOverride,
         'minimum_order_qty': ?minimumOrderQty,
         'order_multiple': ?orderMultiple,
         'maximum_order_qty': ?maximumOrderQty,
         'preorder_policy': ?preorderPolicy,
         'backorder_policy': ?backorderPolicy,
         'auto_hide_unavailable': ?autoHideUnavailable,
         'logistics_available': ?logisticsAvailable,
         'sell_unit_ids': ?sellUnitIds,
         'status': ?status,
         'expected_version': ?expectedVersion,
       });
  PatchShopListingsListingIdRequest.fromJson(super.json);
  String? get operatingModel => value('operating_model') as String?;
  bool? get retailEnabled => value('retail_enabled') as bool?;
  bool? get wholesaleEnabled => value('wholesale_enabled') as bool?;
  int? get retailOverride => value('retail_override') as int?;
  int? get wholesaleOverride => value('wholesale_override') as int?;
  int? get minimumOrderQty => value('minimum_order_qty') as int?;
  int? get orderMultiple => value('order_multiple') as int?;
  int? get maximumOrderQty => value('maximum_order_qty') as int?;
  String? get preorderPolicy => value('preorder_policy') as String?;
  String? get backorderPolicy => value('backorder_policy') as String?;
  bool? get autoHideUnavailable => value('auto_hide_unavailable') as bool?;
  bool? get logisticsAvailable => value('logistics_available') as bool?;
  List<dynamic>? get sellUnitIds => value('sell_unit_ids') as List<dynamic>?;
  String? get status => value('status') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostShopListingsListingIdArchiveRequest extends VendorPayload {
  PostShopListingsListingIdArchiveRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostShopListingsListingIdArchiveRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PutShopListingsListingIdInventoryPolicyRequest extends VendorPayload {
  PutShopListingsListingIdInventoryPolicyRequest({
    String? action,
    String? actualMeasureId,
    int? actualQuantity,
    String? allocationPolicy,
    String? availableFrom,
    String? batchNumber,
    String? bestBeforeAt,
    String? branchId,
    String? consumptionMode,
    int? countedQuantity,
    String? cutoffAt,
    int? damagedQuantity,
    String? destinationBranchId,
    String? destinationStockBasisId,
    bool? dryRun,
    int? expectedCatalogueRevision,
    int? expectedInventoryRevision,
    String? expectedRestockAt,
    int? expectedRevision,
    int? expectedVersion,
    String? expiresAt,
    int? expiringSoonHours,
    int? leadTimeDays,
    String? lineId,
    List<dynamic>? lines,
    String? listingId,
    String? listingRef,
    String? lotNumber,
    bool? lotTracking,
    String? manufacturedAt,
    int? maximumOutstandingQuantity,
    int? maximumPerRequest,
    String? measureId,
    String? measuredAt,
    String? measuredBy,
    int? minimumQuantity,
    int? missingQuantity,
    String? mode,
    String? note,
    String? notes,
    String? operationType,
    int? quantity,
    int? quantityDelta,
    String? reasonCode,
    String? receivedAt,
    int? receivedQuantity,
    String? reservationLineId,
    List<dynamic>? rows,
    String? salesMode,
    String? sellUnitRef,
    String? sourceBranchId,
    String? sourceRef,
    String? sourceStockBasisId,
    String? sourceType,
    String? stockBasisId,
    List<dynamic>? stockBasisIds,
    String? stocktakeType,
    String? supplierRef,
    int? targetQuantity,
    int? unitCostMinor,
    String? variantRef,
  }) : super({
         'action': ?action,
         'actual_measure_id': ?actualMeasureId,
         'actual_quantity': ?actualQuantity,
         'allocation_policy': ?allocationPolicy,
         'available_from': ?availableFrom,
         'batch_number': ?batchNumber,
         'best_before_at': ?bestBeforeAt,
         'branch_id': ?branchId,
         'consumption_mode': ?consumptionMode,
         'counted_quantity': ?countedQuantity,
         'cutoff_at': ?cutoffAt,
         'damaged_quantity': ?damagedQuantity,
         'destination_branch_id': ?destinationBranchId,
         'destination_stock_basis_id': ?destinationStockBasisId,
         'dry_run': ?dryRun,
         'expected_catalogue_revision': ?expectedCatalogueRevision,
         'expected_inventory_revision': ?expectedInventoryRevision,
         'expected_restock_at': ?expectedRestockAt,
         'expected_revision': ?expectedRevision,
         'expected_version': ?expectedVersion,
         'expires_at': ?expiresAt,
         'expiring_soon_hours': ?expiringSoonHours,
         'lead_time_days': ?leadTimeDays,
         'line_id': ?lineId,
         'lines': ?lines,
         'listing_id': ?listingId,
         'listing_ref': ?listingRef,
         'lot_number': ?lotNumber,
         'lot_tracking': ?lotTracking,
         'manufactured_at': ?manufacturedAt,
         'maximum_outstanding_quantity': ?maximumOutstandingQuantity,
         'maximum_per_request': ?maximumPerRequest,
         'measure_id': ?measureId,
         'measured_at': ?measuredAt,
         'measured_by': ?measuredBy,
         'minimum_quantity': ?minimumQuantity,
         'missing_quantity': ?missingQuantity,
         'mode': ?mode,
         'note': ?note,
         'notes': ?notes,
         'operation_type': ?operationType,
         'quantity': ?quantity,
         'quantity_delta': ?quantityDelta,
         'reason_code': ?reasonCode,
         'received_at': ?receivedAt,
         'received_quantity': ?receivedQuantity,
         'reservation_line_id': ?reservationLineId,
         'rows': ?rows,
         'sales_mode': ?salesMode,
         'sell_unit_ref': ?sellUnitRef,
         'source_branch_id': ?sourceBranchId,
         'source_ref': ?sourceRef,
         'source_stock_basis_id': ?sourceStockBasisId,
         'source_type': ?sourceType,
         'stock_basis_id': ?stockBasisId,
         'stock_basis_ids': ?stockBasisIds,
         'stocktake_type': ?stocktakeType,
         'supplier_ref': ?supplierRef,
         'target_quantity': ?targetQuantity,
         'unit_cost_minor': ?unitCostMinor,
         'variant_ref': ?variantRef,
       });
  PutShopListingsListingIdInventoryPolicyRequest.fromJson(super.json);
  String? get action => value('action') as String?;
  String? get actualMeasureId => value('actual_measure_id') as String?;
  int? get actualQuantity => value('actual_quantity') as int?;
  String? get allocationPolicy => value('allocation_policy') as String?;
  String? get availableFrom => value('available_from') as String?;
  String? get batchNumber => value('batch_number') as String?;
  String? get bestBeforeAt => value('best_before_at') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get consumptionMode => value('consumption_mode') as String?;
  int? get countedQuantity => value('counted_quantity') as int?;
  String? get cutoffAt => value('cutoff_at') as String?;
  int? get damagedQuantity => value('damaged_quantity') as int?;
  String? get destinationBranchId => value('destination_branch_id') as String?;
  String? get destinationStockBasisId =>
      value('destination_stock_basis_id') as String?;
  bool? get dryRun => value('dry_run') as bool?;
  int? get expectedCatalogueRevision =>
      value('expected_catalogue_revision') as int?;
  int? get expectedInventoryRevision =>
      value('expected_inventory_revision') as int?;
  String? get expectedRestockAt => value('expected_restock_at') as String?;
  int? get expectedRevision => value('expected_revision') as int?;
  int? get expectedVersion => value('expected_version') as int?;
  String? get expiresAt => value('expires_at') as String?;
  int? get expiringSoonHours => value('expiring_soon_hours') as int?;
  int? get leadTimeDays => value('lead_time_days') as int?;
  String? get lineId => value('line_id') as String?;
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
  String? get listingId => value('listing_id') as String?;
  String? get listingRef => value('listing_ref') as String?;
  String? get lotNumber => value('lot_number') as String?;
  bool? get lotTracking => value('lot_tracking') as bool?;
  String? get manufacturedAt => value('manufactured_at') as String?;
  int? get maximumOutstandingQuantity =>
      value('maximum_outstanding_quantity') as int?;
  int? get maximumPerRequest => value('maximum_per_request') as int?;
  String? get measureId => value('measure_id') as String?;
  String? get measuredAt => value('measured_at') as String?;
  String? get measuredBy => value('measured_by') as String?;
  int? get minimumQuantity => value('minimum_quantity') as int?;
  int? get missingQuantity => value('missing_quantity') as int?;
  String? get mode => value('mode') as String?;
  String? get note => value('note') as String?;
  String? get notes => value('notes') as String?;
  String? get operationType => value('operation_type') as String?;
  int? get quantity => value('quantity') as int?;
  int? get quantityDelta => value('quantity_delta') as int?;
  String? get reasonCode => value('reason_code') as String?;
  String? get receivedAt => value('received_at') as String?;
  int? get receivedQuantity => value('received_quantity') as int?;
  String? get reservationLineId => value('reservation_line_id') as String?;
  List<dynamic>? get rows => value('rows') as List<dynamic>?;
  String? get salesMode => value('sales_mode') as String?;
  String? get sellUnitRef => value('sell_unit_ref') as String?;
  String? get sourceBranchId => value('source_branch_id') as String?;
  String? get sourceRef => value('source_ref') as String?;
  String? get sourceStockBasisId => value('source_stock_basis_id') as String?;
  String? get sourceType => value('source_type') as String?;
  String? get stockBasisId => value('stock_basis_id') as String?;
  List<dynamic>? get stockBasisIds =>
      value('stock_basis_ids') as List<dynamic>?;
  String? get stocktakeType => value('stocktake_type') as String?;
  String? get supplierRef => value('supplier_ref') as String?;
  int? get targetQuantity => value('target_quantity') as int?;
  int? get unitCostMinor => value('unit_cost_minor') as int?;
  String? get variantRef => value('variant_ref') as String?;
}

class PostShopListingsListingIdQuantityBreaksRequest extends VendorPayload {
  PostShopListingsListingIdQuantityBreaksRequest({
    String? sellUnitId,
    int? minQuantity,
    int? maxQuantity,
    int? unitPriceMinor,
    bool? wholesaleOnly,
    int? leadTimeDays,
  }) : super({
         'sell_unit_id': ?sellUnitId,
         'min_quantity': ?minQuantity,
         'max_quantity': ?maxQuantity,
         'unit_price_minor': ?unitPriceMinor,
         'wholesale_only': ?wholesaleOnly,
         'lead_time_days': ?leadTimeDays,
       });
  PostShopListingsListingIdQuantityBreaksRequest.fromJson(super.json);
  String? get sellUnitId => value('sell_unit_id') as String?;
  int? get minQuantity => value('min_quantity') as int?;
  int? get maxQuantity => value('max_quantity') as int?;
  int? get unitPriceMinor => value('unit_price_minor') as int?;
  bool? get wholesaleOnly => value('wholesale_only') as bool?;
  int? get leadTimeDays => value('lead_time_days') as int?;
}

class PatchShopListingsListingIdQuantityBreaksBreakIdRequest
    extends VendorPayload {
  PatchShopListingsListingIdQuantityBreaksBreakIdRequest({
    int? minQuantity,
    int? maxQuantity,
    int? unitPriceMinor,
    bool? wholesaleOnly,
    int? leadTimeDays,
    int? expectedVersion,
  }) : super({
         'min_quantity': ?minQuantity,
         'max_quantity': ?maxQuantity,
         'unit_price_minor': ?unitPriceMinor,
         'wholesale_only': ?wholesaleOnly,
         'lead_time_days': ?leadTimeDays,
         'expected_version': ?expectedVersion,
       });
  PatchShopListingsListingIdQuantityBreaksBreakIdRequest.fromJson(super.json);
  int? get minQuantity => value('min_quantity') as int?;
  int? get maxQuantity => value('max_quantity') as int?;
  int? get unitPriceMinor => value('unit_price_minor') as int?;
  bool? get wholesaleOnly => value('wholesale_only') as bool?;
  int? get leadTimeDays => value('lead_time_days') as int?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostShopListingsListingIdQuantityBreaksBreakIdArchiveRequest
    extends VendorPayload {
  PostShopListingsListingIdQuantityBreaksBreakIdArchiveRequest({
    int? expectedVersion,
  }) : super({'expected_version': ?expectedVersion});
  PostShopListingsListingIdQuantityBreaksBreakIdArchiveRequest.fromJson(
    super.json,
  );
  int? get expectedVersion => value('expected_version') as int?;
}

class PutShopListingsListingIdSellUnitsListingUnitIdRequest
    extends VendorPayload {
  PutShopListingsListingIdSellUnitsListingUnitIdRequest({
    bool? enabled,
    String? calibrationId,
    int? priceOverride,
    int? wholesaleOverride,
    int? expectedVersion,
  }) : super({
         'enabled': ?enabled,
         'calibration_id': ?calibrationId,
         'price_override': ?priceOverride,
         'wholesale_override': ?wholesaleOverride,
         'expected_version': ?expectedVersion,
       });
  PutShopListingsListingIdSellUnitsListingUnitIdRequest.fromJson(super.json);
  bool? get enabled => value('enabled') as bool?;
  String? get calibrationId => value('calibration_id') as String?;
  int? get priceOverride => value('price_override') as int?;
  int? get wholesaleOverride => value('wholesale_override') as int?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostShopMeasurementsRequest extends VendorPayload {
  PostShopMeasurementsRequest({
    String? code,
    String? displayName,
    String? family,
    String? regionCode,
    int? baseQuantity,
  }) : super({
         'code': ?code,
         'display_name': ?displayName,
         'family': ?family,
         'region_code': ?regionCode,
         'base_quantity': ?baseQuantity,
       });
  PostShopMeasurementsRequest.fromJson(super.json);
  String? get code => value('code') as String?;
  String? get displayName => value('display_name') as String?;
  String? get family => value('family') as String?;
  String? get regionCode => value('region_code') as String?;
  int? get baseQuantity => value('base_quantity') as int?;
}

class PatchShopMeasurementsMeasureIdRequest extends VendorPayload {
  PatchShopMeasurementsMeasureIdRequest({
    String? displayName,
    String? regionCode,
    int? expectedVersion,
  }) : super({
         'display_name': ?displayName,
         'region_code': ?regionCode,
         'expected_version': ?expectedVersion,
       });
  PatchShopMeasurementsMeasureIdRequest.fromJson(super.json);
  String? get displayName => value('display_name') as String?;
  String? get regionCode => value('region_code') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostShopMeasurementsMeasureIdAliasesRequest extends VendorPayload {
  PostShopMeasurementsMeasureIdAliasesRequest({
    String? alias,
    String? regionCode,
  }) : super({'alias': ?alias, 'region_code': ?regionCode});
  PostShopMeasurementsMeasureIdAliasesRequest.fromJson(super.json);
  String? get alias => value('alias') as String?;
  String? get regionCode => value('region_code') as String?;
}

class DeleteShopMeasurementsMeasureIdAliasesAliasIdRequest
    extends VendorPayload {
  DeleteShopMeasurementsMeasureIdAliasesAliasIdRequest() : super({});
  DeleteShopMeasurementsMeasureIdAliasesAliasIdRequest.fromJson(super.json);
}

class PostShopMeasurementsMeasureIdArchiveRequest extends VendorPayload {
  PostShopMeasurementsMeasureIdArchiveRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostShopMeasurementsMeasureIdArchiveRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostShopProductsRequest extends VendorPayload {
  PostShopProductsRequest({
    String? categoryId,
    String? name,
    String? description,
    String? brand,
    String? manufacturer,
    String? originCountry,
    String? productClass,
    String? regulatoryReference,
    String? manufacturerCode,
    List<dynamic>? operatingModels,
    List<dynamic>? mediaRefs,
  }) : super({
         'category_id': ?categoryId,
         'name': ?name,
         'description': ?description,
         'brand': ?brand,
         'manufacturer': ?manufacturer,
         'origin_country': ?originCountry,
         'product_class': ?productClass,
         'regulatory_reference': ?regulatoryReference,
         'manufacturer_code': ?manufacturerCode,
         'operating_models': ?operatingModels,
         'media_refs': ?mediaRefs,
       });
  PostShopProductsRequest.fromJson(super.json);
  String? get categoryId => value('category_id') as String?;
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  String? get brand => value('brand') as String?;
  String? get manufacturer => value('manufacturer') as String?;
  String? get originCountry => value('origin_country') as String?;
  String? get productClass => value('product_class') as String?;
  String? get regulatoryReference => value('regulatory_reference') as String?;
  String? get manufacturerCode => value('manufacturer_code') as String?;
  List<dynamic>? get operatingModels =>
      value('operating_models') as List<dynamic>?;
  List<dynamic>? get mediaRefs => value('media_refs') as List<dynamic>?;
}

class PatchShopProductsProductIdRequest extends VendorPayload {
  PatchShopProductsProductIdRequest({
    String? categoryId,
    String? name,
    String? description,
    String? brand,
    String? manufacturer,
    String? originCountry,
    String? productClass,
    String? regulatoryReference,
    String? manufacturerCode,
    List<dynamic>? operatingModels,
    List<dynamic>? mediaRefs,
    String? status,
    int? expectedVersion,
  }) : super({
         'category_id': ?categoryId,
         'name': ?name,
         'description': ?description,
         'brand': ?brand,
         'manufacturer': ?manufacturer,
         'origin_country': ?originCountry,
         'product_class': ?productClass,
         'regulatory_reference': ?regulatoryReference,
         'manufacturer_code': ?manufacturerCode,
         'operating_models': ?operatingModels,
         'media_refs': ?mediaRefs,
         'status': ?status,
         'expected_version': ?expectedVersion,
       });
  PatchShopProductsProductIdRequest.fromJson(super.json);
  String? get categoryId => value('category_id') as String?;
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  String? get brand => value('brand') as String?;
  String? get manufacturer => value('manufacturer') as String?;
  String? get originCountry => value('origin_country') as String?;
  String? get productClass => value('product_class') as String?;
  String? get regulatoryReference => value('regulatory_reference') as String?;
  String? get manufacturerCode => value('manufacturer_code') as String?;
  List<dynamic>? get operatingModels =>
      value('operating_models') as List<dynamic>?;
  List<dynamic>? get mediaRefs => value('media_refs') as List<dynamic>?;
  String? get status => value('status') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostShopProductsProductIdArchiveRequest extends VendorPayload {
  PostShopProductsProductIdArchiveRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostShopProductsProductIdArchiveRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PutShopProductsProductIdFreshAttributesRequest extends VendorPayload {
  PutShopProductsProductIdFreshAttributesRequest({
    String? attributeType,
    bool? requiredValue,
    int? position,
    List<dynamic>? options,
    int? expectedVersion,
  }) : super({
         'attribute_type': ?attributeType,
         'required': ?requiredValue,
         'position': ?position,
         'options': ?options,
         'expected_version': ?expectedVersion,
       });
  PutShopProductsProductIdFreshAttributesRequest.fromJson(super.json);
  String? get attributeType => value('attribute_type') as String?;
  bool? get requiredValue => value('required') as bool?;
  int? get position => value('position') as int?;
  List<dynamic>? get options => value('options') as List<dynamic>?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostShopProductsProductIdFreshAttributesAttributeIdArchiveRequest
    extends VendorPayload {
  PostShopProductsProductIdFreshAttributesAttributeIdArchiveRequest({
    int? expectedVersion,
  }) : super({'expected_version': ?expectedVersion});
  PostShopProductsProductIdFreshAttributesAttributeIdArchiveRequest.fromJson(
    super.json,
  );
  int? get expectedVersion => value('expected_version') as int?;
}

class PostShopProductsProductIdMeasureCalibrationsRequest
    extends VendorPayload {
  PostShopProductsProductIdMeasureCalibrationsRequest({
    String? branchId,
    String? variantId,
    String? measureId,
    int? weightGrams,
    int? volumeMl,
    int? toleranceBps,
    String? source,
  }) : super({
         'branch_id': ?branchId,
         'variant_id': ?variantId,
         'measure_id': ?measureId,
         'weight_grams': ?weightGrams,
         'volume_ml': ?volumeMl,
         'tolerance_bps': ?toleranceBps,
         'source': ?source,
       });
  PostShopProductsProductIdMeasureCalibrationsRequest.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  String? get variantId => value('variant_id') as String?;
  String? get measureId => value('measure_id') as String?;
  int? get weightGrams => value('weight_grams') as int?;
  int? get volumeMl => value('volume_ml') as int?;
  int? get toleranceBps => value('tolerance_bps') as int?;
  String? get source => value('source') as String?;
}

class PatchShopProductsProductIdMeasureCalibrationsCalibrationIdRequest
    extends VendorPayload {
  PatchShopProductsProductIdMeasureCalibrationsCalibrationIdRequest({
    int? weightGrams,
    int? volumeMl,
    int? toleranceBps,
    String? source,
    int? expectedVersion,
  }) : super({
         'weight_grams': ?weightGrams,
         'volume_ml': ?volumeMl,
         'tolerance_bps': ?toleranceBps,
         'source': ?source,
         'expected_version': ?expectedVersion,
       });
  PatchShopProductsProductIdMeasureCalibrationsCalibrationIdRequest.fromJson(
    super.json,
  );
  int? get weightGrams => value('weight_grams') as int?;
  int? get volumeMl => value('volume_ml') as int?;
  int? get toleranceBps => value('tolerance_bps') as int?;
  String? get source => value('source') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostShopProductsProductIdMeasureCalibrationsCalibrationIdArchiveRequest
    extends VendorPayload {
  PostShopProductsProductIdMeasureCalibrationsCalibrationIdArchiveRequest({
    int? expectedVersion,
  }) : super({'expected_version': ?expectedVersion});
  PostShopProductsProductIdMeasureCalibrationsCalibrationIdArchiveRequest.fromJson(
    super.json,
  );
  int? get expectedVersion => value('expected_version') as int?;
}

class PostShopProductsProductIdPackagingRequest extends VendorPayload {
  PostShopProductsProductIdPackagingRequest({
    String? parentVariantId,
    String? childVariantId,
    int? unitsPerParent,
  }) : super({
         'parent_variant_id': ?parentVariantId,
         'child_variant_id': ?childVariantId,
         'units_per_parent': ?unitsPerParent,
       });
  PostShopProductsProductIdPackagingRequest.fromJson(super.json);
  String? get parentVariantId => value('parent_variant_id') as String?;
  String? get childVariantId => value('child_variant_id') as String?;
  int? get unitsPerParent => value('units_per_parent') as int?;
}

class PostShopProductsProductIdPackagingRelationIdArchiveRequest
    extends VendorPayload {
  PostShopProductsProductIdPackagingRelationIdArchiveRequest({
    int? expectedVersion,
  }) : super({'expected_version': ?expectedVersion});
  PostShopProductsProductIdPackagingRelationIdArchiveRequest.fromJson(
    super.json,
  );
  int? get expectedVersion => value('expected_version') as int?;
}

class PostShopProductsProductIdSellUnitsRequest extends VendorPayload {
  PostShopProductsProductIdSellUnitsRequest({
    String? variantId,
    String? measureId,
    int? fractionNumerator,
    int? fractionDenominator,
    String? displayName,
    String? measurementMode,
    String? calibrationId,
    int? estimatedBaseQty,
    int? underfillBps,
    int? overfillBps,
    bool? measurementRequired,
    int? displayPrecision,
    int? priceMinor,
    int? wholesalePriceMinor,
    bool? retailEnabled,
    bool? wholesaleEnabled,
    int? minimumOrderQty,
    int? orderMultiple,
    int? maximumOrderQty,
  }) : super({
         'variant_id': ?variantId,
         'measure_id': ?measureId,
         'fraction_numerator': ?fractionNumerator,
         'fraction_denominator': ?fractionDenominator,
         'display_name': ?displayName,
         'measurement_mode': ?measurementMode,
         'calibration_id': ?calibrationId,
         'estimated_base_qty': ?estimatedBaseQty,
         'underfill_bps': ?underfillBps,
         'overfill_bps': ?overfillBps,
         'measurement_required': ?measurementRequired,
         'display_precision': ?displayPrecision,
         'price_minor': ?priceMinor,
         'wholesale_price_minor': ?wholesalePriceMinor,
         'retail_enabled': ?retailEnabled,
         'wholesale_enabled': ?wholesaleEnabled,
         'minimum_order_qty': ?minimumOrderQty,
         'order_multiple': ?orderMultiple,
         'maximum_order_qty': ?maximumOrderQty,
       });
  PostShopProductsProductIdSellUnitsRequest.fromJson(super.json);
  String? get variantId => value('variant_id') as String?;
  String? get measureId => value('measure_id') as String?;
  int? get fractionNumerator => value('fraction_numerator') as int?;
  int? get fractionDenominator => value('fraction_denominator') as int?;
  String? get displayName => value('display_name') as String?;
  String? get measurementMode => value('measurement_mode') as String?;
  String? get calibrationId => value('calibration_id') as String?;
  int? get estimatedBaseQty => value('estimated_base_qty') as int?;
  int? get underfillBps => value('underfill_bps') as int?;
  int? get overfillBps => value('overfill_bps') as int?;
  bool? get measurementRequired => value('measurement_required') as bool?;
  int? get displayPrecision => value('display_precision') as int?;
  int? get priceMinor => value('price_minor') as int?;
  int? get wholesalePriceMinor => value('wholesale_price_minor') as int?;
  bool? get retailEnabled => value('retail_enabled') as bool?;
  bool? get wholesaleEnabled => value('wholesale_enabled') as bool?;
  int? get minimumOrderQty => value('minimum_order_qty') as int?;
  int? get orderMultiple => value('order_multiple') as int?;
  int? get maximumOrderQty => value('maximum_order_qty') as int?;
}

class PatchShopProductsProductIdSellUnitsSellUnitIdRequest
    extends VendorPayload {
  PatchShopProductsProductIdSellUnitsSellUnitIdRequest({
    String? displayName,
    String? measurementMode,
    String? calibrationId,
    int? estimatedBaseQty,
    int? underfillBps,
    int? overfillBps,
    bool? measurementRequired,
    int? displayPrecision,
    int? priceMinor,
    int? wholesalePriceMinor,
    bool? retailEnabled,
    bool? wholesaleEnabled,
    int? minimumOrderQty,
    int? orderMultiple,
    int? maximumOrderQty,
    String? status,
    int? expectedVersion,
  }) : super({
         'display_name': ?displayName,
         'measurement_mode': ?measurementMode,
         'calibration_id': ?calibrationId,
         'estimated_base_qty': ?estimatedBaseQty,
         'underfill_bps': ?underfillBps,
         'overfill_bps': ?overfillBps,
         'measurement_required': ?measurementRequired,
         'display_precision': ?displayPrecision,
         'price_minor': ?priceMinor,
         'wholesale_price_minor': ?wholesalePriceMinor,
         'retail_enabled': ?retailEnabled,
         'wholesale_enabled': ?wholesaleEnabled,
         'minimum_order_qty': ?minimumOrderQty,
         'order_multiple': ?orderMultiple,
         'maximum_order_qty': ?maximumOrderQty,
         'status': ?status,
         'expected_version': ?expectedVersion,
       });
  PatchShopProductsProductIdSellUnitsSellUnitIdRequest.fromJson(super.json);
  String? get displayName => value('display_name') as String?;
  String? get measurementMode => value('measurement_mode') as String?;
  String? get calibrationId => value('calibration_id') as String?;
  int? get estimatedBaseQty => value('estimated_base_qty') as int?;
  int? get underfillBps => value('underfill_bps') as int?;
  int? get overfillBps => value('overfill_bps') as int?;
  bool? get measurementRequired => value('measurement_required') as bool?;
  int? get displayPrecision => value('display_precision') as int?;
  int? get priceMinor => value('price_minor') as int?;
  int? get wholesalePriceMinor => value('wholesale_price_minor') as int?;
  bool? get retailEnabled => value('retail_enabled') as bool?;
  bool? get wholesaleEnabled => value('wholesale_enabled') as bool?;
  int? get minimumOrderQty => value('minimum_order_qty') as int?;
  int? get orderMultiple => value('order_multiple') as int?;
  int? get maximumOrderQty => value('maximum_order_qty') as int?;
  String? get status => value('status') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostShopProductsProductIdSellUnitsSellUnitIdArchiveRequest
    extends VendorPayload {
  PostShopProductsProductIdSellUnitsSellUnitIdArchiveRequest({
    int? expectedVersion,
  }) : super({'expected_version': ?expectedVersion});
  PostShopProductsProductIdSellUnitsSellUnitIdArchiveRequest.fromJson(
    super.json,
  );
  int? get expectedVersion => value('expected_version') as int?;
}

class PostShopProductsProductIdVariantsRequest extends VendorPayload {
  PostShopProductsProductIdVariantsRequest({
    String? sku,
    String? name,
    String? packageLevel,
    int? packQuantity,
    int? netWeightGrams,
    int? netVolumeMl,
    int? lengthMm,
    int? widthMm,
    int? heightMm,
    int? retailPriceMinor,
    int? wholesalePriceMinor,
    String? temperatureZone,
    String? consolidationClass,
    String? separationGroup,
    bool? stackable,
    bool? largeLoad,
    List<dynamic>? handlingFlags,
  }) : super({
         'sku': ?sku,
         'name': ?name,
         'package_level': ?packageLevel,
         'pack_quantity': ?packQuantity,
         'net_weight_grams': ?netWeightGrams,
         'net_volume_ml': ?netVolumeMl,
         'length_mm': ?lengthMm,
         'width_mm': ?widthMm,
         'height_mm': ?heightMm,
         'retail_price_minor': ?retailPriceMinor,
         'wholesale_price_minor': ?wholesalePriceMinor,
         'temperature_zone': ?temperatureZone,
         'consolidation_class': ?consolidationClass,
         'separation_group': ?separationGroup,
         'stackable': ?stackable,
         'large_load': ?largeLoad,
         'handling_flags': ?handlingFlags,
       });
  PostShopProductsProductIdVariantsRequest.fromJson(super.json);
  String? get sku => value('sku') as String?;
  String? get name => value('name') as String?;
  String? get packageLevel => value('package_level') as String?;
  int? get packQuantity => value('pack_quantity') as int?;
  int? get netWeightGrams => value('net_weight_grams') as int?;
  int? get netVolumeMl => value('net_volume_ml') as int?;
  int? get lengthMm => value('length_mm') as int?;
  int? get widthMm => value('width_mm') as int?;
  int? get heightMm => value('height_mm') as int?;
  int? get retailPriceMinor => value('retail_price_minor') as int?;
  int? get wholesalePriceMinor => value('wholesale_price_minor') as int?;
  String? get temperatureZone => value('temperature_zone') as String?;
  String? get consolidationClass => value('consolidation_class') as String?;
  String? get separationGroup => value('separation_group') as String?;
  bool? get stackable => value('stackable') as bool?;
  bool? get largeLoad => value('large_load') as bool?;
  List<dynamic>? get handlingFlags => value('handling_flags') as List<dynamic>?;
}

class PatchShopProductsProductIdVariantsVariantIdRequest extends VendorPayload {
  PatchShopProductsProductIdVariantsVariantIdRequest({
    String? sku,
    String? name,
    String? packageLevel,
    int? packQuantity,
    int? netWeightGrams,
    int? netVolumeMl,
    int? lengthMm,
    int? widthMm,
    int? heightMm,
    int? retailPriceMinor,
    int? wholesalePriceMinor,
    String? temperatureZone,
    String? consolidationClass,
    String? separationGroup,
    bool? stackable,
    bool? largeLoad,
    List<dynamic>? handlingFlags,
    String? status,
    int? expectedVersion,
  }) : super({
         'sku': ?sku,
         'name': ?name,
         'package_level': ?packageLevel,
         'pack_quantity': ?packQuantity,
         'net_weight_grams': ?netWeightGrams,
         'net_volume_ml': ?netVolumeMl,
         'length_mm': ?lengthMm,
         'width_mm': ?widthMm,
         'height_mm': ?heightMm,
         'retail_price_minor': ?retailPriceMinor,
         'wholesale_price_minor': ?wholesalePriceMinor,
         'temperature_zone': ?temperatureZone,
         'consolidation_class': ?consolidationClass,
         'separation_group': ?separationGroup,
         'stackable': ?stackable,
         'large_load': ?largeLoad,
         'handling_flags': ?handlingFlags,
         'status': ?status,
         'expected_version': ?expectedVersion,
       });
  PatchShopProductsProductIdVariantsVariantIdRequest.fromJson(super.json);
  String? get sku => value('sku') as String?;
  String? get name => value('name') as String?;
  String? get packageLevel => value('package_level') as String?;
  int? get packQuantity => value('pack_quantity') as int?;
  int? get netWeightGrams => value('net_weight_grams') as int?;
  int? get netVolumeMl => value('net_volume_ml') as int?;
  int? get lengthMm => value('length_mm') as int?;
  int? get widthMm => value('width_mm') as int?;
  int? get heightMm => value('height_mm') as int?;
  int? get retailPriceMinor => value('retail_price_minor') as int?;
  int? get wholesalePriceMinor => value('wholesale_price_minor') as int?;
  String? get temperatureZone => value('temperature_zone') as String?;
  String? get consolidationClass => value('consolidation_class') as String?;
  String? get separationGroup => value('separation_group') as String?;
  bool? get stackable => value('stackable') as bool?;
  bool? get largeLoad => value('large_load') as bool?;
  List<dynamic>? get handlingFlags => value('handling_flags') as List<dynamic>?;
  String? get status => value('status') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostShopProductsProductIdVariantsVariantIdArchiveRequest
    extends VendorPayload {
  PostShopProductsProductIdVariantsVariantIdArchiveRequest({
    int? expectedVersion,
  }) : super({'expected_version': ?expectedVersion});
  PostShopProductsProductIdVariantsVariantIdArchiveRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostShopVariantsVariantIdIdentifiersRequest extends VendorPayload {
  PostShopVariantsVariantIdIdentifiersRequest({
    String? identifierType,
    String? identifierValue,
    String? packageLevel,
  }) : super({
         'identifier_type': ?identifierType,
         'identifier_value': ?identifierValue,
         'package_level': ?packageLevel,
       });
  PostShopVariantsVariantIdIdentifiersRequest.fromJson(super.json);
  String? get identifierType => value('identifier_type') as String?;
  String? get identifierValue => value('identifier_value') as String?;
  String? get packageLevel => value('package_level') as String?;
}

class PostShopVariantsVariantIdIdentifiersIdentifierIdArchiveRequest
    extends VendorPayload {
  PostShopVariantsVariantIdIdentifiersIdentifierIdArchiveRequest({
    int? expectedVersion,
  }) : super({'expected_version': ?expectedVersion});
  PostShopVariantsVariantIdIdentifiersIdentifierIdArchiveRequest.fromJson(
    super.json,
  );
  int? get expectedVersion => value('expected_version') as int?;
}

class ShopCatalogueSection12aShopCatalogueAliasCommand extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueAliasCommand.fromJson(super.json);
  String? get alias => value('alias') as String?;
  String? get regionCode => value('region_code') as String?;
}

class ShopCatalogueSection12aShopCatalogueBranchListing extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueBranchListing.fromJson(super.json);
  bool? get autoHideWhenUnavailable =>
      value('auto_hide_when_unavailable') as bool?;
  String? get backorderPolicy => value('backorder_policy') as String?;
  String? get branchRef => value('branch_ref') as String?;
  String? get id => value('id') as String?;
  String? get inventoryAuthority => value('inventory_authority') as String?;
  bool? get logisticsAvailable => value('logistics_available') as bool?;
  int? get maximumOrderQuantity => value('maximum_order_quantity') as int?;
  int? get minimumOrderQuantity => value('minimum_order_quantity') as int?;
  String? get operatingModel => value('operating_model') as String?;
  int? get orderMultiple => value('order_multiple') as int?;
  String? get preorderPolicy => value('preorder_policy') as String?;
  String? get productRef => value('product_ref') as String?;
  bool? get retailEnabled => value('retail_enabled') as bool?;
  int? get retailPriceOverrideMinor =>
      value('retail_price_override_minor') as int?;
  String? get status => value('status') as String?;
  String? get variantRef => value('variant_ref') as String?;
  int? get version => value('version') as int?;
  bool? get wholesaleEnabled => value('wholesale_enabled') as bool?;
  int? get wholesalePriceOverrideMinor =>
      value('wholesale_price_override_minor') as int?;
}

class ShopCatalogueSection12aShopCatalogueCalibration extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueCalibration.fromJson(super.json);
  bool? get approximate => value('approximate') as bool?;
  int? get approximateVolumeMl => value('approximate_volume_ml') as int?;
  int? get approximateWeightGrams => value('approximate_weight_grams') as int?;
  String? get branchRef => value('branch_ref') as String?;
  String? get calibrationSource => value('calibration_source') as String?;
  String? get id => value('id') as String?;
  String? get measureRef => value('measure_ref') as String?;
  String? get productRef => value('product_ref') as String?;
  String? get status => value('status') as String?;
  int? get toleranceBps => value('tolerance_bps') as int?;
  String? get variantRef => value('variant_ref') as String?;
  int? get version => value('version') as int?;
}

class ShopCatalogueSection12aShopCatalogueCatalogueRevision
    extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueCatalogueRevision.fromJson(super.json);
  String? get domain => value('domain') as String?;
  int? get revision => value('revision') as int?;
  String? get updatedAt => value('updated_at') as String?;
}

class ShopCatalogueSection12aShopCatalogueCategory extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueCategory.fromJson(super.json);
  int? get depth => value('depth') as int?;
  String? get description => value('description') as String?;
  String? get id => value('id') as String?;
  String? get name => value('name') as String?;
  String? get parentRef => value('parent_ref') as String?;
  int? get position => value('position') as int?;
  String? get slug => value('slug') as String?;
  String? get status => value('status') as String?;
  int? get version => value('version') as int?;
}

class ShopCatalogueSection12aShopCatalogueCreateCalibrationCommand
    extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueCreateCalibrationCommand.fromJson(
    super.json,
  );
  String? get branchId => value('branch_id') as String?;
  String? get measureId => value('measure_id') as String?;
  String? get source => value('source') as String?;
  int? get toleranceBps => value('tolerance_bps') as int?;
  String? get variantId => value('variant_id') as String?;
  int? get volumeMl => value('volume_ml') as int?;
  int? get weightGrams => value('weight_grams') as int?;
}

class ShopCatalogueSection12aShopCatalogueCreateCategoryCommand
    extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueCreateCategoryCommand.fromJson(
    super.json,
  );
  String? get description => value('description') as String?;
  String? get name => value('name') as String?;
  String? get parentId => value('parent_id') as String?;
  int? get position => value('position') as int?;
  String? get slug => value('slug') as String?;
}

class ShopCatalogueSection12aShopCatalogueCreateListingCommand
    extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueCreateListingCommand.fromJson(super.json);
  bool? get autoHideUnavailable => value('auto_hide_unavailable') as bool?;
  String? get backorderPolicy => value('backorder_policy') as String?;
  bool? get logisticsAvailable => value('logistics_available') as bool?;
  int? get maximumOrderQty => value('maximum_order_qty') as int?;
  int? get minimumOrderQty => value('minimum_order_qty') as int?;
  String? get operatingModel => value('operating_model') as String?;
  int? get orderMultiple => value('order_multiple') as int?;
  String? get preorderPolicy => value('preorder_policy') as String?;
  bool? get retailEnabled => value('retail_enabled') as bool?;
  int? get retailOverride => value('retail_override') as int?;
  List<dynamic>? get sellUnitIds => value('sell_unit_ids') as List<dynamic>?;
  String? get variantId => value('variant_id') as String?;
  bool? get wholesaleEnabled => value('wholesale_enabled') as bool?;
  int? get wholesaleOverride => value('wholesale_override') as int?;
}

class ShopCatalogueSection12aShopCatalogueCreateMeasureCommand
    extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueCreateMeasureCommand.fromJson(super.json);
  int? get baseQuantity => value('base_quantity') as int?;
  String? get code => value('code') as String?;
  String? get displayName => value('display_name') as String?;
  String? get family => value('family') as String?;
  String? get regionCode => value('region_code') as String?;
}

class ShopCatalogueSection12aShopCatalogueCreateProductCommand
    extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueCreateProductCommand.fromJson(super.json);
  String? get brand => value('brand') as String?;
  String? get categoryId => value('category_id') as String?;
  String? get description => value('description') as String?;
  String? get manufacturer => value('manufacturer') as String?;
  String? get manufacturerCode => value('manufacturer_code') as String?;
  List<dynamic>? get mediaRefs => value('media_refs') as List<dynamic>?;
  String? get name => value('name') as String?;
  List<dynamic>? get operatingModels =>
      value('operating_models') as List<dynamic>?;
  String? get originCountry => value('origin_country') as String?;
  String? get productClass => value('product_class') as String?;
  String? get regulatoryReference => value('regulatory_reference') as String?;
}

class ShopCatalogueSection12aShopCatalogueCreateQuantityBreakCommand
    extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueCreateQuantityBreakCommand.fromJson(
    super.json,
  );
  int? get leadTimeDays => value('lead_time_days') as int?;
  int? get maxQuantity => value('max_quantity') as int?;
  int? get minQuantity => value('min_quantity') as int?;
  String? get sellUnitId => value('sell_unit_id') as String?;
  int? get unitPriceMinor => value('unit_price_minor') as int?;
  bool? get wholesaleOnly => value('wholesale_only') as bool?;
}

class ShopCatalogueSection12aShopCatalogueCreateSellUnitCommand
    extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueCreateSellUnitCommand.fromJson(
    super.json,
  );
  String? get calibrationId => value('calibration_id') as String?;
  String? get displayName => value('display_name') as String?;
  int? get displayPrecision => value('display_precision') as int?;
  int? get estimatedBaseQty => value('estimated_base_qty') as int?;
  int? get fractionDenominator => value('fraction_denominator') as int?;
  int? get fractionNumerator => value('fraction_numerator') as int?;
  int? get maximumOrderQty => value('maximum_order_qty') as int?;
  String? get measureId => value('measure_id') as String?;
  String? get measurementMode => value('measurement_mode') as String?;
  bool? get measurementRequired => value('measurement_required') as bool?;
  int? get minimumOrderQty => value('minimum_order_qty') as int?;
  int? get orderMultiple => value('order_multiple') as int?;
  int? get overfillBps => value('overfill_bps') as int?;
  int? get priceMinor => value('price_minor') as int?;
  bool? get retailEnabled => value('retail_enabled') as bool?;
  int? get underfillBps => value('underfill_bps') as int?;
  String? get variantId => value('variant_id') as String?;
  bool? get wholesaleEnabled => value('wholesale_enabled') as bool?;
  int? get wholesalePriceMinor => value('wholesale_price_minor') as int?;
}

class ShopCatalogueSection12aShopCatalogueCreateVariantCommand
    extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueCreateVariantCommand.fromJson(super.json);
  String? get consolidationClass => value('consolidation_class') as String?;
  List<dynamic>? get handlingFlags => value('handling_flags') as List<dynamic>?;
  int? get heightMm => value('height_mm') as int?;
  bool? get largeLoad => value('large_load') as bool?;
  int? get lengthMm => value('length_mm') as int?;
  String? get name => value('name') as String?;
  int? get netVolumeMl => value('net_volume_ml') as int?;
  int? get netWeightGrams => value('net_weight_grams') as int?;
  int? get packQuantity => value('pack_quantity') as int?;
  String? get packageLevel => value('package_level') as String?;
  int? get retailPriceMinor => value('retail_price_minor') as int?;
  String? get separationGroup => value('separation_group') as String?;
  String? get sku => value('sku') as String?;
  bool? get stackable => value('stackable') as bool?;
  String? get temperatureZone => value('temperature_zone') as String?;
  int? get wholesalePriceMinor => value('wholesale_price_minor') as int?;
  int? get widthMm => value('width_mm') as int?;
}

class ShopCatalogueSection12aShopCatalogueError extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueError.fromJson(super.json);
  JsonObject? get error =>
      value('error') == null ? null : jsonObject(value('error'));
  bool? get success => value('success') as bool?;
}

class ShopCatalogueSection12aShopCatalogueFraction extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueFraction.fromJson(super.json);
  int? get denominator => value('denominator') as int?;
  int? get numerator => value('numerator') as int?;
}

class ShopCatalogueSection12aShopCatalogueFreshAttribute extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueFreshAttribute.fromJson(super.json);
  String? get attributeType => value('attribute_type') as String?;
  String? get id => value('id') as String?;
  List<dynamic>? get options => value('options') as List<dynamic>?;
  int? get position => value('position') as int?;
  String? get productRef => value('product_ref') as String?;
  bool? get selectionRequired => value('selection_required') as bool?;
  String? get status => value('status') as String?;
  int? get version => value('version') as int?;
}

class ShopCatalogueSection12aShopCatalogueFreshAttributeCommand
    extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueFreshAttributeCommand.fromJson(
    super.json,
  );
  String? get attributeType => value('attribute_type') as String?;
  int? get expectedVersion => value('expected_version') as int?;
  List<dynamic>? get options => value('options') as List<dynamic>?;
  int? get position => value('position') as int?;
  bool? get requiredValue => value('required') as bool?;
}

class ShopCatalogueSection12aShopCatalogueIdentifier extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueIdentifier.fromJson(super.json);
  bool? get externallyVerified => value('externally_verified') as bool?;
  String? get id => value('id') as String?;
  String? get identifierType => value('identifier_type') as String?;
  String? get identifierValue => value('identifier_value') as String?;
  String? get packageLevel => value('package_level') as String?;
  String? get productRef => value('product_ref') as String?;
  String? get status => value('status') as String?;
  String? get variantRef => value('variant_ref') as String?;
  int? get version => value('version') as int?;
}

class ShopCatalogueSection12aShopCatalogueIdentifierCommand
    extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueIdentifierCommand.fromJson(super.json);
  String? get identifierType => value('identifier_type') as String?;
  String? get identifierValue => value('identifier_value') as String?;
  String? get packageLevel => value('package_level') as String?;
}

class ShopCatalogueSection12aShopCatalogueListingSellUnit
    extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueListingSellUnit.fromJson(super.json);
  String? get calibrationRef => value('calibration_ref') as String?;
  bool? get enabled => value('enabled') as bool?;
  String? get id => value('id') as String?;
  String? get listingRef => value('listing_ref') as String?;
  int? get position => value('position') as int?;
  int? get priceOverrideMinor => value('price_override_minor') as int?;
  String? get sellUnitRef => value('sell_unit_ref') as String?;
  int? get version => value('version') as int?;
  int? get wholesalePriceOverrideMinor =>
      value('wholesale_price_override_minor') as int?;
}

class ShopCatalogueSection12aShopCatalogueListingSellUnitCommand
    extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueListingSellUnitCommand.fromJson(
    super.json,
  );
  String? get calibrationId => value('calibration_id') as String?;
  bool? get enabled => value('enabled') as bool?;
  int? get expectedVersion => value('expected_version') as int?;
  int? get priceOverride => value('price_override') as int?;
  int? get wholesaleOverride => value('wholesale_override') as int?;
}

class ShopCatalogueSection12aShopCatalogueLogisticsProfile
    extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueLogisticsProfile.fromJson(super.json);
  String? get consolidationClass => value('consolidation_class') as String?;
  List<dynamic>? get handlingFlags => value('handling_flags') as List<dynamic>?;
  int? get heightMm => value('height_mm') as int?;
  bool? get largeLoad => value('large_load') as bool?;
  int? get lengthMm => value('length_mm') as int?;
  String? get separationGroup => value('separation_group') as String?;
  String? get temperatureZone => value('temperature_zone') as String?;
  int? get weightGrams => value('weight_grams') as int?;
  int? get widthMm => value('width_mm') as int?;
}

class ShopCatalogueSection12aShopCatalogueMeasureAlias extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueMeasureAlias.fromJson(super.json);
  String? get alias => value('alias') as String?;
  String? get id => value('id') as String?;
  String? get measureRef => value('measure_ref') as String?;
  String? get regionCode => value('region_code') as String?;
  String? get scope => value('scope') as String?;
}

class ShopCatalogueSection12aShopCatalogueMeasureDefinition
    extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueMeasureDefinition.fromJson(super.json);
  int? get baseQuantity => value('base_quantity') as int?;
  String? get baseUnit => value('base_unit') as String?;
  String? get code => value('code') as String?;
  bool? get convertsExactly => value('converts_exactly') as bool?;
  String? get displayName => value('display_name') as String?;
  String? get id => value('id') as String?;
  String? get measureOrigin => value('measure_origin') as String?;
  String? get measurementFamily => value('measurement_family') as String?;
  String? get regionCode => value('region_code') as String?;
  String? get scope => value('scope') as String?;
  String? get status => value('status') as String?;
  int? get version => value('version') as int?;
}

class ShopCatalogueSection12aShopCatalogueOperatingModelConfiguration
    extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueOperatingModelConfiguration.fromJson(
    super.json,
  );
  String? get branchRef => value('branch_ref') as String?;
  String? get id => value('id') as String?;
  List<dynamic>? get models => value('models') as List<dynamic>?;
  String? get primaryModel => value('primary_model') as String?;
  String? get status => value('status') as String?;
  int? get version => value('version') as int?;
}

class ShopCatalogueSection12aShopCatalogueOperatingModelsCommand
    extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueOperatingModelsCommand.fromJson(
    super.json,
  );
  int? get expectedVersion => value('expected_version') as int?;
  List<dynamic>? get models => value('models') as List<dynamic>?;
  String? get primaryModel => value('primary_model') as String?;
}

class ShopCatalogueSection12aShopCataloguePackageCommand extends VendorPayload {
  ShopCatalogueSection12aShopCataloguePackageCommand.fromJson(super.json);
  String? get childVariantId => value('child_variant_id') as String?;
  String? get parentVariantId => value('parent_variant_id') as String?;
  int? get unitsPerParent => value('units_per_parent') as int?;
}

class ShopCatalogueSection12aShopCataloguePackageRelation
    extends VendorPayload {
  ShopCatalogueSection12aShopCataloguePackageRelation.fromJson(super.json);
  String? get childVariantRef => value('child_variant_ref') as String?;
  String? get id => value('id') as String?;
  String? get parentVariantRef => value('parent_variant_ref') as String?;
  String? get productRef => value('product_ref') as String?;
  String? get status => value('status') as String?;
  int? get unitsPerParent => value('units_per_parent') as int?;
  int? get version => value('version') as int?;
}

class ShopCatalogueSection12aShopCataloguePriceVersion extends VendorPayload {
  ShopCatalogueSection12aShopCataloguePriceVersion.fromJson(super.json);
  String? get actorType => value('actor_type') as String?;
  int? get amountMinor => value('amount_minor') as int?;
  String? get branchRef => value('branch_ref') as String?;
  String? get changeReason => value('change_reason') as String?;
  String? get commandRef => value('command_ref') as String?;
  String? get currency => value('currency') as String?;
  String? get effectiveAt => value('effective_at') as String?;
  String? get id => value('id') as String?;
  bool? get inherited => value('inherited') as bool?;
  int? get previousAmountMinor => value('previous_amount_minor') as int?;
  String? get productRef => value('product_ref') as String?;
  String? get recordedAt => value('recorded_at') as String?;
  int? get revision => value('revision') as int?;
  String? get subjectRef => value('subject_ref') as String?;
  String? get subjectType => value('subject_type') as String?;
  int? get subjectVersion => value('subject_version') as int?;
  String? get variantRef => value('variant_ref') as String?;
}

class ShopCatalogueSection12aShopCatalogueProduct extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueProduct.fromJson(super.json);
  String? get brand => value('brand') as String?;
  String? get categoryRef => value('category_ref') as String?;
  String? get description => value('description') as String?;
  String? get id => value('id') as String?;
  String? get manufacturer => value('manufacturer') as String?;
  List<dynamic>? get mediaRefs => value('media_refs') as List<dynamic>?;
  String? get name => value('name') as String?;
  List<dynamic>? get operatingModels =>
      value('operating_models') as List<dynamic>?;
  String? get originCountry => value('origin_country') as String?;
  String? get productClass => value('product_class') as String?;
  String? get status => value('status') as String?;
  int? get version => value('version') as int?;
}

class ShopCatalogueSection12aShopCatalogueQuantityBreak extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueQuantityBreak.fromJson(super.json);
  String? get id => value('id') as String?;
  int? get leadTimeDays => value('lead_time_days') as int?;
  String? get listingRef => value('listing_ref') as String?;
  int? get maxQuantity => value('max_quantity') as int?;
  int? get minQuantity => value('min_quantity') as int?;
  String? get sellUnitRef => value('sell_unit_ref') as String?;
  String? get status => value('status') as String?;
  int? get unitPriceMinor => value('unit_price_minor') as int?;
  int? get version => value('version') as int?;
  bool? get wholesaleOnly => value('wholesale_only') as bool?;
}

class ShopCatalogueSection12aShopCatalogueResolvedPrice extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueResolvedPrice.fromJson(super.json);
  String? get availability => value('availability') as String?;
  String? get branchRef => value('branch_ref') as String?;
  String? get currency => value('currency') as String?;
  String? get listingRef => value('listing_ref') as String?;
  int? get maximumOrderQuantity => value('maximum_order_quantity') as int?;
  int? get minimumOrderQuantity => value('minimum_order_quantity') as int?;
  String? get mode => value('mode') as String?;
  int? get orderMultiple => value('order_multiple') as int?;
  bool? get orderRulesSatisfied => value('order_rules_satisfied') as bool?;
  String? get priceSource => value('price_source') as String?;
  int? get quantity => value('quantity') as int?;
  String? get quantityBreakRef => value('quantity_break_ref') as String?;
  String? get reason => value('reason') as String?;
  String? get sellUnitRef => value('sell_unit_ref') as String?;
  int? get totalPriceMinor => value('total_price_minor') as int?;
  int? get unitPriceMinor => value('unit_price_minor') as int?;
  String? get variantRef => value('variant_ref') as String?;
}

class ShopCatalogueSection12aShopCatalogueSellUnit extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueSellUnit.fromJson(super.json);
  bool? get actualMeasurementRequired =>
      value('actual_measurement_required') as bool?;
  String? get calibrationRef => value('calibration_ref') as String?;
  String? get conversionType => value('conversion_type') as String?;
  String? get displayName => value('display_name') as String?;
  int? get displayPrecision => value('display_precision') as int?;
  int? get estimatedBaseQuantity => value('estimated_base_quantity') as int?;
  String? get exactBaseFamily => value('exact_base_family') as String?;
  int? get exactBaseQuantity => value('exact_base_quantity') as int?;
  int? get fractionDenominator => value('fraction_denominator') as int?;
  int? get fractionNumerator => value('fraction_numerator') as int?;
  String? get id => value('id') as String?;
  int? get maximumOrderQuantity => value('maximum_order_quantity') as int?;
  String? get measureRef => value('measure_ref') as String?;
  String? get measurementMode => value('measurement_mode') as String?;
  int? get minimumOrderQuantity => value('minimum_order_quantity') as int?;
  int? get orderMultiple => value('order_multiple') as int?;
  int? get overfillToleranceBps => value('overfill_tolerance_bps') as int?;
  int? get position => value('position') as int?;
  int? get priceMinor => value('price_minor') as int?;
  String? get productRef => value('product_ref') as String?;
  bool? get retailEnabled => value('retail_enabled') as bool?;
  String? get status => value('status') as String?;
  int? get underfillToleranceBps => value('underfill_tolerance_bps') as int?;
  String? get variantRef => value('variant_ref') as String?;
  int? get version => value('version') as int?;
  bool? get wholesaleEnabled => value('wholesale_enabled') as bool?;
  int? get wholesalePriceMinor => value('wholesale_price_minor') as int?;
}

class ShopCatalogueSection12aShopCatalogueUpdateCalibrationCommand
    extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueUpdateCalibrationCommand.fromJson(
    super.json,
  );
  int? get expectedVersion => value('expected_version') as int?;
  String? get source => value('source') as String?;
  int? get toleranceBps => value('tolerance_bps') as int?;
  int? get volumeMl => value('volume_ml') as int?;
  int? get weightGrams => value('weight_grams') as int?;
}

class ShopCatalogueSection12aShopCatalogueUpdateCategoryCommand
    extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueUpdateCategoryCommand.fromJson(
    super.json,
  );
  String? get description => value('description') as String?;
  int? get expectedVersion => value('expected_version') as int?;
  String? get name => value('name') as String?;
  String? get parentId => value('parent_id') as String?;
  int? get position => value('position') as int?;
  String? get slug => value('slug') as String?;
}

class ShopCatalogueSection12aShopCatalogueUpdateListingCommand
    extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueUpdateListingCommand.fromJson(super.json);
  bool? get autoHideUnavailable => value('auto_hide_unavailable') as bool?;
  String? get backorderPolicy => value('backorder_policy') as String?;
  int? get expectedVersion => value('expected_version') as int?;
  bool? get logisticsAvailable => value('logistics_available') as bool?;
  int? get maximumOrderQty => value('maximum_order_qty') as int?;
  int? get minimumOrderQty => value('minimum_order_qty') as int?;
  String? get operatingModel => value('operating_model') as String?;
  int? get orderMultiple => value('order_multiple') as int?;
  String? get preorderPolicy => value('preorder_policy') as String?;
  bool? get retailEnabled => value('retail_enabled') as bool?;
  int? get retailOverride => value('retail_override') as int?;
  List<dynamic>? get sellUnitIds => value('sell_unit_ids') as List<dynamic>?;
  String? get status => value('status') as String?;
  bool? get wholesaleEnabled => value('wholesale_enabled') as bool?;
  int? get wholesaleOverride => value('wholesale_override') as int?;
}

class ShopCatalogueSection12aShopCatalogueUpdateMeasureCommand
    extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueUpdateMeasureCommand.fromJson(super.json);
  String? get displayName => value('display_name') as String?;
  int? get expectedVersion => value('expected_version') as int?;
  String? get regionCode => value('region_code') as String?;
}

class ShopCatalogueSection12aShopCatalogueUpdateProductCommand
    extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueUpdateProductCommand.fromJson(super.json);
  String? get brand => value('brand') as String?;
  String? get categoryId => value('category_id') as String?;
  String? get description => value('description') as String?;
  int? get expectedVersion => value('expected_version') as int?;
  String? get manufacturer => value('manufacturer') as String?;
  String? get manufacturerCode => value('manufacturer_code') as String?;
  List<dynamic>? get mediaRefs => value('media_refs') as List<dynamic>?;
  String? get name => value('name') as String?;
  List<dynamic>? get operatingModels =>
      value('operating_models') as List<dynamic>?;
  String? get originCountry => value('origin_country') as String?;
  String? get productClass => value('product_class') as String?;
  String? get regulatoryReference => value('regulatory_reference') as String?;
  String? get status => value('status') as String?;
}

class ShopCatalogueSection12aShopCatalogueUpdateQuantityBreakCommand
    extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueUpdateQuantityBreakCommand.fromJson(
    super.json,
  );
  int? get expectedVersion => value('expected_version') as int?;
  int? get leadTimeDays => value('lead_time_days') as int?;
  int? get maxQuantity => value('max_quantity') as int?;
  int? get minQuantity => value('min_quantity') as int?;
  int? get unitPriceMinor => value('unit_price_minor') as int?;
  bool? get wholesaleOnly => value('wholesale_only') as bool?;
}

class ShopCatalogueSection12aShopCatalogueUpdateSellUnitCommand
    extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueUpdateSellUnitCommand.fromJson(
    super.json,
  );
  String? get calibrationId => value('calibration_id') as String?;
  String? get displayName => value('display_name') as String?;
  int? get displayPrecision => value('display_precision') as int?;
  int? get estimatedBaseQty => value('estimated_base_qty') as int?;
  int? get expectedVersion => value('expected_version') as int?;
  int? get maximumOrderQty => value('maximum_order_qty') as int?;
  String? get measurementMode => value('measurement_mode') as String?;
  bool? get measurementRequired => value('measurement_required') as bool?;
  int? get minimumOrderQty => value('minimum_order_qty') as int?;
  int? get orderMultiple => value('order_multiple') as int?;
  int? get overfillBps => value('overfill_bps') as int?;
  int? get priceMinor => value('price_minor') as int?;
  bool? get retailEnabled => value('retail_enabled') as bool?;
  String? get status => value('status') as String?;
  int? get underfillBps => value('underfill_bps') as int?;
  bool? get wholesaleEnabled => value('wholesale_enabled') as bool?;
  int? get wholesalePriceMinor => value('wholesale_price_minor') as int?;
}

class ShopCatalogueSection12aShopCatalogueUpdateVariantCommand
    extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueUpdateVariantCommand.fromJson(super.json);
  String? get consolidationClass => value('consolidation_class') as String?;
  int? get expectedVersion => value('expected_version') as int?;
  List<dynamic>? get handlingFlags => value('handling_flags') as List<dynamic>?;
  int? get heightMm => value('height_mm') as int?;
  bool? get largeLoad => value('large_load') as bool?;
  int? get lengthMm => value('length_mm') as int?;
  String? get name => value('name') as String?;
  int? get netVolumeMl => value('net_volume_ml') as int?;
  int? get netWeightGrams => value('net_weight_grams') as int?;
  int? get packQuantity => value('pack_quantity') as int?;
  String? get packageLevel => value('package_level') as String?;
  int? get retailPriceMinor => value('retail_price_minor') as int?;
  String? get separationGroup => value('separation_group') as String?;
  String? get sku => value('sku') as String?;
  bool? get stackable => value('stackable') as bool?;
  String? get status => value('status') as String?;
  String? get temperatureZone => value('temperature_zone') as String?;
  int? get wholesalePriceMinor => value('wholesale_price_minor') as int?;
  int? get widthMm => value('width_mm') as int?;
}

class ShopCatalogueSection12aShopCatalogueVariableMeasurement
    extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueVariableMeasurement.fromJson(super.json);
  bool? get actualMeasurementRequired =>
      value('actual_measurement_required') as bool?;
  String? get mode => value('mode') as String?;
  int? get overfillToleranceBps => value('overfill_tolerance_bps') as int?;
  int? get underfillToleranceBps => value('underfill_tolerance_bps') as int?;
}

class ShopCatalogueSection12aShopCatalogueVariant extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueVariant.fromJson(super.json);
  String? get consolidationClass => value('consolidation_class') as String?;
  List<dynamic>? get handlingFlags => value('handling_flags') as List<dynamic>?;
  int? get heightMm => value('height_mm') as int?;
  String? get id => value('id') as String?;
  bool? get largeLoad => value('large_load') as bool?;
  int? get lengthMm => value('length_mm') as int?;
  String? get name => value('name') as String?;
  int? get netVolumeMl => value('net_volume_ml') as int?;
  int? get netWeightGrams => value('net_weight_grams') as int?;
  int? get packQuantity => value('pack_quantity') as int?;
  String? get packageLevel => value('package_level') as String?;
  String? get productRef => value('product_ref') as String?;
  int? get retailPriceMinor => value('retail_price_minor') as int?;
  String? get separationGroup => value('separation_group') as String?;
  String? get sku => value('sku') as String?;
  bool? get stackable => value('stackable') as bool?;
  String? get status => value('status') as String?;
  String? get temperatureZone => value('temperature_zone') as String?;
  int? get version => value('version') as int?;
  int? get wholesalePriceMinor => value('wholesale_price_minor') as int?;
  int? get widthMm => value('width_mm') as int?;
}

class ShopCatalogueSection12aShopCatalogueVersionCommand extends VendorPayload {
  ShopCatalogueSection12aShopCatalogueVersionCommand.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class ShopCatalogueSection12bShopInventoryError extends VendorPayload {
  ShopCatalogueSection12bShopInventoryError.fromJson(super.json);
  JsonObject? get error =>
      value('error') == null ? null : jsonObject(value('error'));
  bool? get success => value('success') as bool?;
}

class ShopCatalogueSection12bShopInventoryInventoryBalance
    extends VendorPayload {
  ShopCatalogueSection12bShopInventoryInventoryBalance.fromJson(super.json);
  int? get inventoryRevision => value('inventory_revision') as int?;
  int? get onHandQuantity => value('on_hand_quantity') as int?;
  int? get physicalAvailable => value('physical_available') as int?;
  int? get quarantinedQuantity => value('quarantined_quantity') as int?;
  int? get reservedQuantity => value('reserved_quantity') as int?;
}

class ShopCatalogueSection12bShopInventoryInventoryCommand
    extends VendorPayload {
  ShopCatalogueSection12bShopInventoryInventoryCommand.fromJson(super.json);
  String? get action => value('action') as String?;
  String? get actualMeasureId => value('actual_measure_id') as String?;
  int? get actualQuantity => value('actual_quantity') as int?;
  String? get allocationPolicy => value('allocation_policy') as String?;
  String? get availableFrom => value('available_from') as String?;
  String? get batchNumber => value('batch_number') as String?;
  String? get bestBeforeAt => value('best_before_at') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get consumptionMode => value('consumption_mode') as String?;
  int? get countedQuantity => value('counted_quantity') as int?;
  String? get cutoffAt => value('cutoff_at') as String?;
  int? get damagedQuantity => value('damaged_quantity') as int?;
  String? get destinationBranchId => value('destination_branch_id') as String?;
  String? get destinationStockBasisId =>
      value('destination_stock_basis_id') as String?;
  bool? get dryRun => value('dry_run') as bool?;
  int? get expectedCatalogueRevision =>
      value('expected_catalogue_revision') as int?;
  int? get expectedInventoryRevision =>
      value('expected_inventory_revision') as int?;
  String? get expectedRestockAt => value('expected_restock_at') as String?;
  int? get expectedRevision => value('expected_revision') as int?;
  int? get expectedVersion => value('expected_version') as int?;
  String? get expiresAt => value('expires_at') as String?;
  int? get expiringSoonHours => value('expiring_soon_hours') as int?;
  int? get leadTimeDays => value('lead_time_days') as int?;
  String? get lineId => value('line_id') as String?;
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
  String? get listingId => value('listing_id') as String?;
  String? get listingRef => value('listing_ref') as String?;
  String? get lotNumber => value('lot_number') as String?;
  bool? get lotTracking => value('lot_tracking') as bool?;
  String? get manufacturedAt => value('manufactured_at') as String?;
  int? get maximumOutstandingQuantity =>
      value('maximum_outstanding_quantity') as int?;
  int? get maximumPerRequest => value('maximum_per_request') as int?;
  String? get measureId => value('measure_id') as String?;
  String? get measuredAt => value('measured_at') as String?;
  String? get measuredBy => value('measured_by') as String?;
  int? get minimumQuantity => value('minimum_quantity') as int?;
  int? get missingQuantity => value('missing_quantity') as int?;
  String? get mode => value('mode') as String?;
  String? get note => value('note') as String?;
  String? get notes => value('notes') as String?;
  String? get operationType => value('operation_type') as String?;
  int? get quantity => value('quantity') as int?;
  int? get quantityDelta => value('quantity_delta') as int?;
  String? get reasonCode => value('reason_code') as String?;
  String? get receivedAt => value('received_at') as String?;
  int? get receivedQuantity => value('received_quantity') as int?;
  String? get reservationLineId => value('reservation_line_id') as String?;
  List<dynamic>? get rows => value('rows') as List<dynamic>?;
  String? get salesMode => value('sales_mode') as String?;
  String? get sellUnitRef => value('sell_unit_ref') as String?;
  String? get sourceBranchId => value('source_branch_id') as String?;
  String? get sourceRef => value('source_ref') as String?;
  String? get sourceStockBasisId => value('source_stock_basis_id') as String?;
  String? get sourceType => value('source_type') as String?;
  String? get stockBasisId => value('stock_basis_id') as String?;
  List<dynamic>? get stockBasisIds =>
      value('stock_basis_ids') as List<dynamic>?;
  String? get stocktakeType => value('stocktake_type') as String?;
  String? get supplierRef => value('supplier_ref') as String?;
  int? get targetQuantity => value('target_quantity') as int?;
  int? get unitCostMinor => value('unit_cost_minor') as int?;
  String? get variantRef => value('variant_ref') as String?;
}

class ShopCatalogueSection12bShopInventoryReservation extends VendorPayload {
  ShopCatalogueSection12bShopInventoryReservation.fromJson(super.json);
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
  String? get status => value('status') as String?;
}

class ShopCatalogueSection12bShopInventorySellability extends VendorPayload {
  ShopCatalogueSection12bShopInventorySellability.fromJson(super.json);
  int? get catalogueRevision => value('catalogue_revision') as int?;
  int? get inventoryRevision => value('inventory_revision') as int?;
  String? get reasonCode => value('reason_code') as String?;
  bool? get sellable => value('sellable') as bool?;
}

class ShopCatalogueSection12bShopInventorySuccessEnvelope
    extends VendorPayload {
  ShopCatalogueSection12bShopInventorySuccessEnvelope.fromJson(super.json);
  JsonObject? get data =>
      value('data') == null ? null : jsonObject(value('data'));
  bool? get success => value('success') as bool?;
}
