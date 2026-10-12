// GENERATED from actual request schemas/Go JSON fields.
import '../../../core/contracts/vendor_contract.dart';

class PutInventoryItemsInventoryItemIdReorderRuleRequest extends VendorPayload {
  PutInventoryItemsInventoryItemIdReorderRuleRequest({
    String? branchId,
    String? inventoryItemId,
    int? minimumQuantity,
    int? targetQuantity,
    int? expiringSoonHours,
    String? supplierRef,
    String? status,
  }) : super({
         'branch_id': ?branchId,
         'inventory_item_id': ?inventoryItemId,
         'minimum_quantity': ?minimumQuantity,
         'target_quantity': ?targetQuantity,
         'expiring_soon_hours': ?expiringSoonHours,
         'supplier_ref': ?supplierRef,
         'status': ?status,
       });
  PutInventoryItemsInventoryItemIdReorderRuleRequest.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  String? get inventoryItemId => value('inventory_item_id') as String?;
  int? get minimumQuantity => value('minimum_quantity') as int?;
  int? get targetQuantity => value('target_quantity') as int?;
  int? get expiringSoonHours => value('expiring_soon_hours') as int?;
  String? get supplierRef => value('supplier_ref') as String?;
  String? get status => value('status') as String?;
}

class PostInventoryQuarantinesRequest extends VendorPayload {
  PostInventoryQuarantinesRequest({
    String? branchId,
    String? inventoryItemId,
    int? quantity,
    String? reasonCode,
    String? note,
  }) : super({
         'branch_id': ?branchId,
         'inventory_item_id': ?inventoryItemId,
         'quantity': ?quantity,
         'reason_code': ?reasonCode,
         'note': ?note,
       });
  PostInventoryQuarantinesRequest.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  String? get inventoryItemId => value('inventory_item_id') as String?;
  int? get quantity => value('quantity') as int?;
  String? get reasonCode => value('reason_code') as String?;
  String? get note => value('note') as String?;
}

class PostInventoryQuarantinesQuarantineIdDisposeRequest extends VendorPayload {
  PostInventoryQuarantinesQuarantineIdDisposeRequest({String? reasonCode})
    : super({'reason_code': ?reasonCode});
  PostInventoryQuarantinesQuarantineIdDisposeRequest.fromJson(super.json);
  String? get reasonCode => value('reason_code') as String?;
}

class PostInventoryQuarantinesQuarantineIdReleaseRequest extends VendorPayload {
  PostInventoryQuarantinesQuarantineIdReleaseRequest({String? reasonCode})
    : super({'reason_code': ?reasonCode});
  PostInventoryQuarantinesQuarantineIdReleaseRequest.fromJson(super.json);
  String? get reasonCode => value('reason_code') as String?;
}

class PostInventoryReservationsRequest extends VendorPayload {
  PostInventoryReservationsRequest({
    String? branchId,
    String? sourceType,
    String? sourceRef,
    String? expiresAt,
    List<dynamic>? lines,
  }) : super({
         'branch_id': ?branchId,
         'source_type': ?sourceType,
         'source_ref': ?sourceRef,
         'expires_at': ?expiresAt,
         'lines': ?lines,
       });
  PostInventoryReservationsRequest.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  String? get sourceType => value('source_type') as String?;
  String? get sourceRef => value('source_ref') as String?;
  String? get expiresAt => value('expires_at') as String?;
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
}

class PostInventoryStocktakesRequest extends VendorPayload {
  PostInventoryStocktakesRequest({
    String? branchId,
    String? stocktakeType,
    List<dynamic>? inventoryItemIds,
  }) : super({
         'branch_id': ?branchId,
         'stocktake_type': ?stocktakeType,
         'inventory_item_ids': ?inventoryItemIds,
       });
  PostInventoryStocktakesRequest.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  String? get stocktakeType => value('stocktake_type') as String?;
  List<dynamic>? get inventoryItemIds =>
      value('inventory_item_ids') as List<dynamic>?;
}

class PutInventoryStocktakesStocktakeIdItemsInventoryItemIdCountRequest
    extends VendorPayload {
  PutInventoryStocktakesStocktakeIdItemsInventoryItemIdCountRequest({
    int? countedQuantity,
  }) : super({'counted_quantity': ?countedQuantity});
  PutInventoryStocktakesStocktakeIdItemsInventoryItemIdCountRequest.fromJson(
    super.json,
  );
  int? get countedQuantity => value('counted_quantity') as int?;
}

class PostInventoryTransfersRequest extends VendorPayload {
  PostInventoryTransfersRequest({
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
  PostInventoryTransfersRequest.fromJson(super.json);
  String? get sourceBranchId => value('source_branch_id') as String?;
  String? get destinationBranchId => value('destination_branch_id') as String?;
  String? get notes => value('notes') as String?;
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
}

class PostInventoryTransfersTransferIdReceiveRequest extends VendorPayload {
  PostInventoryTransfersTransferIdReceiveRequest({
    String? sourceRef,
    List<dynamic>? lines,
  }) : super({'source_ref': ?sourceRef, 'lines': ?lines});
  PostInventoryTransfersTransferIdReceiveRequest.fromJson(super.json);
  String? get sourceRef => value('source_ref') as String?;
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
}

class InventorySection14VendorInventoryAlert extends VendorPayload {
  InventorySection14VendorInventoryAlert.fromJson(super.json);
  String? get id => value('id') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get inventoryItemId => value('inventory_item_id') as String?;
  String? get alertType => value('alert_type') as String?;
  String? get severity => value('severity') as String?;
  String? get status => value('status') as String?;
  String? get dedupeKey => value('dedupe_key') as String?;
  String? get createdAt => value('created_at') as String?;
}

class InventorySection14VendorInventoryAlertEnvelope extends VendorPayload {
  InventorySection14VendorInventoryAlertEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get data =>
      value('data') == null ? null : jsonObject(value('data'));
}

class InventorySection14VendorInventoryAlertListEnvelope extends VendorPayload {
  InventorySection14VendorInventoryAlertListEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get data =>
      value('data') == null ? null : jsonObject(value('data'));
}

class InventorySection14VendorInventoryBalance extends VendorPayload {
  InventorySection14VendorInventoryBalance.fromJson(super.json);
  String? get businessId => value('business_id') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get inventoryItemId => value('inventory_item_id') as String?;
  int? get onHandQuantity => value('on_hand_quantity') as int?;
  int? get reservedQuantity => value('reserved_quantity') as int?;
  int? get quarantinedQuantity => value('quarantined_quantity') as int?;
  int? get inTransitQuantity => value('in_transit_quantity') as int?;
  int? get incomingQuantity => value('incoming_quantity') as int?;
  int? get backorderQuantity => value('backorder_quantity') as int?;
  int? get availableQuantity => value('available_quantity') as int?;
  int? get expiredQuantity => value('expired_quantity') as int?;
  int? get revision => value('revision') as int?;
}

class InventorySection14VendorInventoryBalanceEnvelope extends VendorPayload {
  InventorySection14VendorInventoryBalanceEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get data =>
      value('data') == null ? null : jsonObject(value('data'));
}

class InventorySection14VendorInventoryBalanceListEnvelope
    extends VendorPayload {
  InventorySection14VendorInventoryBalanceListEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get data =>
      value('data') == null ? null : jsonObject(value('data'));
}

class InventorySection14VendorInventoryCountStocktakeLineRequest
    extends VendorPayload {
  InventorySection14VendorInventoryCountStocktakeLineRequest.fromJson(
    super.json,
  );
  int? get countedQuantity => value('counted_quantity') as int?;
}

class InventorySection14VendorInventoryCreateQuarantineRequest
    extends VendorPayload {
  InventorySection14VendorInventoryCreateQuarantineRequest.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  String? get inventoryItemId => value('inventory_item_id') as String?;
  int? get quantity => value('quantity') as int?;
  String? get reasonCode => value('reason_code') as String?;
  String? get note => value('note') as String?;
}

class InventorySection14VendorInventoryCreateReservationRequest
    extends VendorPayload {
  InventorySection14VendorInventoryCreateReservationRequest.fromJson(
    super.json,
  );
  String? get branchId => value('branch_id') as String?;
  String? get sourceType => value('source_type') as String?;
  String? get sourceRef => value('source_ref') as String?;
  String? get expiresAt => value('expires_at') as String?;
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
}

class InventorySection14VendorInventoryCreateStocktakeRequest
    extends VendorPayload {
  InventorySection14VendorInventoryCreateStocktakeRequest.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  String? get type => value('type') as String?;
  List<dynamic>? get inventoryItemIds =>
      value('inventory_item_ids') as List<dynamic>?;
}

class InventorySection14VendorInventoryCreateTransferRequest
    extends VendorPayload {
  InventorySection14VendorInventoryCreateTransferRequest.fromJson(super.json);
  String? get sourceBranchId => value('source_branch_id') as String?;
  String? get destinationBranchId => value('destination_branch_id') as String?;
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
}

class InventorySection14VendorInventoryErrorEnvelope extends VendorPayload {
  InventorySection14VendorInventoryErrorEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get error =>
      value('error') == null ? null : jsonObject(value('error'));
}

class InventorySection14VendorInventoryMovement extends VendorPayload {
  InventorySection14VendorInventoryMovement.fromJson(super.json);
  String? get id => value('id') as String?;
  String? get inventoryItemId => value('inventory_item_id') as String?;
  String? get lotId => value('lot_id') as String?;
  String? get movementType => value('movement_type') as String?;
  int? get quantity => value('quantity') as int?;
  String? get sourceType => value('source_type') as String?;
  String? get sourceRef => value('source_ref') as String?;
  String? get reasonCode => value('reason_code') as String?;
  String? get occurredAt => value('occurred_at') as String?;
  int? get inventoryRevision => value('inventory_revision') as int?;
}

class InventorySection14VendorInventoryMovementListEnvelope
    extends VendorPayload {
  InventorySection14VendorInventoryMovementListEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get data =>
      value('data') == null ? null : jsonObject(value('data'));
}

class InventorySection14VendorInventoryQuarantine extends VendorPayload {
  InventorySection14VendorInventoryQuarantine.fromJson(super.json);
  String? get id => value('id') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get inventoryItemId => value('inventory_item_id') as String?;
  int? get quantity => value('quantity') as int?;
  String? get status => value('status') as String?;
  String? get reasonCode => value('reason_code') as String?;
  int? get version => value('version') as int?;
}

class InventorySection14VendorInventoryQuarantineEnvelope
    extends VendorPayload {
  InventorySection14VendorInventoryQuarantineEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get data =>
      value('data') == null ? null : jsonObject(value('data'));
}

class InventorySection14VendorInventoryReceiveTransferRequest
    extends VendorPayload {
  InventorySection14VendorInventoryReceiveTransferRequest.fromJson(super.json);
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
}

class InventorySection14VendorInventoryReorderRule extends VendorPayload {
  InventorySection14VendorInventoryReorderRule.fromJson(super.json);
  String? get id => value('id') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get inventoryItemId => value('inventory_item_id') as String?;
  int? get minimumQuantity => value('minimum_quantity') as int?;
  int? get targetQuantity => value('target_quantity') as int?;
  String? get status => value('status') as String?;
  int? get version => value('version') as int?;
}

class InventorySection14VendorInventoryReorderRuleEnvelope
    extends VendorPayload {
  InventorySection14VendorInventoryReorderRuleEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get data =>
      value('data') == null ? null : jsonObject(value('data'));
}

class InventorySection14VendorInventoryReservation extends VendorPayload {
  InventorySection14VendorInventoryReservation.fromJson(super.json);
  String? get id => value('id') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get status => value('status') as String?;
  String? get sourceType => value('source_type') as String?;
  String? get sourceRef => value('source_ref') as String?;
  String? get allocationPolicy => value('allocation_policy') as String?;
  String? get expiresAt => value('expires_at') as String?;
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
  int? get version => value('version') as int?;
}

class InventorySection14VendorInventoryReservationEnvelope
    extends VendorPayload {
  InventorySection14VendorInventoryReservationEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get data =>
      value('data') == null ? null : jsonObject(value('data'));
}

class InventorySection14VendorInventoryReservationLineInput
    extends VendorPayload {
  InventorySection14VendorInventoryReservationLineInput.fromJson(super.json);
  String? get inventoryItemId => value('inventory_item_id') as String?;
  int? get quantity => value('quantity') as int?;
}

class InventorySection14VendorInventoryRevisionEnvelope extends VendorPayload {
  InventorySection14VendorInventoryRevisionEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get data =>
      value('data') == null ? null : jsonObject(value('data'));
}

class InventorySection14VendorInventorySellability extends VendorPayload {
  InventorySection14VendorInventorySellability.fromJson(super.json);
  String? get inventoryItemId => value('inventory_item_id') as String?;
  bool? get sellable => value('sellable') as bool?;
  int? get availableQuantity => value('available_quantity') as int?;
  String? get reasonCode => value('reason_code') as String?;
}

class InventorySection14VendorInventorySellabilityEnvelope
    extends VendorPayload {
  InventorySection14VendorInventorySellabilityEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get data =>
      value('data') == null ? null : jsonObject(value('data'));
}

class InventorySection14VendorInventorySetReorderRuleRequest
    extends VendorPayload {
  InventorySection14VendorInventorySetReorderRuleRequest.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  int? get minimumQuantity => value('minimum_quantity') as int?;
  int? get targetQuantity => value('target_quantity') as int?;
  int? get expiringSoonHours => value('expiring_soon_hours') as int?;
  String? get supplierRef => value('supplier_ref') as String?;
  String? get status => value('status') as String?;
}

class InventorySection14VendorInventoryStocktake extends VendorPayload {
  InventorySection14VendorInventoryStocktake.fromJson(super.json);
  String? get id => value('id') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get type => value('type') as String?;
  String? get status => value('status') as String?;
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
  int? get version => value('version') as int?;
}

class InventorySection14VendorInventoryStocktakeEnvelope extends VendorPayload {
  InventorySection14VendorInventoryStocktakeEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get data =>
      value('data') == null ? null : jsonObject(value('data'));
}

class InventorySection14VendorInventorySuccessEnvelope extends VendorPayload {
  InventorySection14VendorInventorySuccessEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get data =>
      value('data') == null ? null : jsonObject(value('data'));
}

class InventorySection14VendorInventoryTransfer extends VendorPayload {
  InventorySection14VendorInventoryTransfer.fromJson(super.json);
  String? get id => value('id') as String?;
  String? get sourceBranchId => value('source_branch_id') as String?;
  String? get destinationBranchId => value('destination_branch_id') as String?;
  String? get status => value('status') as String?;
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
  int? get version => value('version') as int?;
}

class InventorySection14VendorInventoryTransferEnvelope extends VendorPayload {
  InventorySection14VendorInventoryTransferEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get data =>
      value('data') == null ? null : jsonObject(value('data'));
}

class InventorySection14VendorInventoryTransferLineInput extends VendorPayload {
  InventorySection14VendorInventoryTransferLineInput.fromJson(super.json);
  String? get sourceInventoryItemId =>
      value('source_inventory_item_id') as String?;
  String? get destinationInventoryItemId =>
      value('destination_inventory_item_id') as String?;
  int? get quantity => value('quantity') as int?;
}
