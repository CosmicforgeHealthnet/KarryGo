// GENERATED from actual request schemas/Go JSON fields.
import '../../../core/contracts/vendor_contract.dart';

class PostCommerceSection21RecurringSchedulesRequest extends VendorPayload {
  PostCommerceSection21RecurringSchedulesRequest({
    String? businessID,
    String? requesterID,
    String? corporateRef,
    String? branchID,
    List<dynamic>? weekdays,
    String? mealTime,
    String? timezone,
    int? headcount,
    int? allowanceMinor,
    JsonObject? destinationSnapshot,
    JsonObject? menuRotation,
    JsonObject? productionRequirement,
    String? tentativeVehicleClass,
    int? advanceHorizonDays,
    String? startsOn,
    String? endsOn,
    String? idempotencyKey,
  }) : super({
         'BusinessID': ?businessID,
         'RequesterID': ?requesterID,
         'CorporateRef': ?corporateRef,
         'BranchID': ?branchID,
         'Weekdays': ?weekdays,
         'MealTime': ?mealTime,
         'Timezone': ?timezone,
         'Headcount': ?headcount,
         'AllowanceMinor': ?allowanceMinor,
         'DestinationSnapshot': ?destinationSnapshot,
         'MenuRotation': ?menuRotation,
         'ProductionRequirement': ?productionRequirement,
         'TentativeVehicleClass': ?tentativeVehicleClass,
         'AdvanceHorizonDays': ?advanceHorizonDays,
         'StartsOn': ?startsOn,
         'EndsOn': ?endsOn,
         'IdempotencyKey': ?idempotencyKey,
       });
  PostCommerceSection21RecurringSchedulesRequest.fromJson(super.json);
  String? get businessID => value('BusinessID') as String?;
  String? get requesterID => value('RequesterID') as String?;
  String? get corporateRef => value('CorporateRef') as String?;
  String? get branchID => value('BranchID') as String?;
  List<dynamic>? get weekdays => value('Weekdays') as List<dynamic>?;
  String? get mealTime => value('MealTime') as String?;
  String? get timezone => value('Timezone') as String?;
  int? get headcount => value('Headcount') as int?;
  int? get allowanceMinor => value('AllowanceMinor') as int?;
  JsonObject? get destinationSnapshot => value('DestinationSnapshot') == null
      ? null
      : jsonObject(value('DestinationSnapshot'));
  JsonObject? get menuRotation =>
      value('MenuRotation') == null ? null : jsonObject(value('MenuRotation'));
  JsonObject? get productionRequirement =>
      value('ProductionRequirement') == null
      ? null
      : jsonObject(value('ProductionRequirement'));
  String? get tentativeVehicleClass =>
      value('TentativeVehicleClass') as String?;
  int? get advanceHorizonDays => value('AdvanceHorizonDays') as int?;
  String? get startsOn => value('StartsOn') as String?;
  String? get endsOn => value('EndsOn') as String?;
  String? get idempotencyKey => value('IdempotencyKey') as String?;
}

class PutCommerceSection21RecurringSchedulesScheduleIdRequest
    extends VendorPayload {
  PutCommerceSection21RecurringSchedulesScheduleIdRequest({
    String? businessID,
    String? requesterID,
    String? corporateRef,
    String? branchID,
    List<dynamic>? weekdays,
    String? mealTime,
    String? timezone,
    int? headcount,
    int? allowanceMinor,
    JsonObject? destinationSnapshot,
    JsonObject? menuRotation,
    JsonObject? productionRequirement,
    String? tentativeVehicleClass,
    int? advanceHorizonDays,
    String? startsOn,
    String? endsOn,
    String? idempotencyKey,
  }) : super({
         'BusinessID': ?businessID,
         'RequesterID': ?requesterID,
         'CorporateRef': ?corporateRef,
         'BranchID': ?branchID,
         'Weekdays': ?weekdays,
         'MealTime': ?mealTime,
         'Timezone': ?timezone,
         'Headcount': ?headcount,
         'AllowanceMinor': ?allowanceMinor,
         'DestinationSnapshot': ?destinationSnapshot,
         'MenuRotation': ?menuRotation,
         'ProductionRequirement': ?productionRequirement,
         'TentativeVehicleClass': ?tentativeVehicleClass,
         'AdvanceHorizonDays': ?advanceHorizonDays,
         'StartsOn': ?startsOn,
         'EndsOn': ?endsOn,
         'IdempotencyKey': ?idempotencyKey,
       });
  PutCommerceSection21RecurringSchedulesScheduleIdRequest.fromJson(super.json);
  String? get businessID => value('BusinessID') as String?;
  String? get requesterID => value('RequesterID') as String?;
  String? get corporateRef => value('CorporateRef') as String?;
  String? get branchID => value('BranchID') as String?;
  List<dynamic>? get weekdays => value('Weekdays') as List<dynamic>?;
  String? get mealTime => value('MealTime') as String?;
  String? get timezone => value('Timezone') as String?;
  int? get headcount => value('Headcount') as int?;
  int? get allowanceMinor => value('AllowanceMinor') as int?;
  JsonObject? get destinationSnapshot => value('DestinationSnapshot') == null
      ? null
      : jsonObject(value('DestinationSnapshot'));
  JsonObject? get menuRotation =>
      value('MenuRotation') == null ? null : jsonObject(value('MenuRotation'));
  JsonObject? get productionRequirement =>
      value('ProductionRequirement') == null
      ? null
      : jsonObject(value('ProductionRequirement'));
  String? get tentativeVehicleClass =>
      value('TentativeVehicleClass') as String?;
  int? get advanceHorizonDays => value('AdvanceHorizonDays') as int?;
  String? get startsOn => value('StartsOn') as String?;
  String? get endsOn => value('EndsOn') as String?;
  String? get idempotencyKey => value('IdempotencyKey') as String?;
}

class PostCommerceSection21RecurringSchedulesScheduleIdStatusRequest
    extends VendorPayload {
  PostCommerceSection21RecurringSchedulesScheduleIdStatusRequest({
    String? status,
    int? expectedVersion,
  }) : super({'status': ?status, 'expected_version': ?expectedVersion});
  PostCommerceSection21RecurringSchedulesScheduleIdStatusRequest.fromJson(
    super.json,
  );
  String? get status => value('status') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCommerceSection21RfqsRfqIdQuotesRequest extends VendorPayload {
  PostCommerceSection21RfqsRfqIdQuotesRequest({
    String? vendorId,
    String? branchId,
    int? priceMinor,
    String? currency,
    JsonObject? snapshot,
    JsonObject? capacity,
    String? expiresAt,
    String? idempotencyKey,
  }) : super({
         'vendor_id': ?vendorId,
         'branch_id': ?branchId,
         'price_minor': ?priceMinor,
         'currency': ?currency,
         'snapshot': ?snapshot,
         'capacity': ?capacity,
         'expires_at': ?expiresAt,
         'idempotency_key': ?idempotencyKey,
       });
  PostCommerceSection21RfqsRfqIdQuotesRequest.fromJson(super.json);
  String? get vendorId => value('vendor_id') as String?;
  String? get branchId => value('branch_id') as String?;
  int? get priceMinor => value('price_minor') as int?;
  String? get currency => value('currency') as String?;
  JsonObject? get snapshot =>
      value('snapshot') == null ? null : jsonObject(value('snapshot'));
  JsonObject? get capacity =>
      value('capacity') == null ? null : jsonObject(value('capacity'));
  String? get expiresAt => value('expires_at') as String?;
  String? get idempotencyKey => value('idempotency_key') as String?;
}

class PostCommerceSection21ScheduledOrdersRequest extends VendorPayload {
  PostCommerceSection21ScheduledOrdersRequest({
    String? businessID,
    String? customerID,
    String? quoteID,
    String? vendorOrderID,
    String? branchID,
    String? desiredDeliveryAt,
    String? deliveryWindowStart,
    String? deliveryWindowEnd,
    String? productionReadyBy,
    String? expectedReadyAt,
    String? fulfilmentPlanID,
    String? tentativeVehicleClass,
    JsonObject? bulkRequirement,
    JsonObject? capacity,
    String? sourceRef,
    int? sourceVersion,
    String? idempotencyKey,
  }) : super({
         'BusinessID': ?businessID,
         'CustomerID': ?customerID,
         'QuoteID': ?quoteID,
         'VendorOrderID': ?vendorOrderID,
         'BranchID': ?branchID,
         'DesiredDeliveryAt': ?desiredDeliveryAt,
         'DeliveryWindowStart': ?deliveryWindowStart,
         'DeliveryWindowEnd': ?deliveryWindowEnd,
         'ProductionReadyBy': ?productionReadyBy,
         'ExpectedReadyAt': ?expectedReadyAt,
         'FulfilmentPlanID': ?fulfilmentPlanID,
         'TentativeVehicleClass': ?tentativeVehicleClass,
         'BulkRequirement': ?bulkRequirement,
         'Capacity': ?capacity,
         'SourceRef': ?sourceRef,
         'SourceVersion': ?sourceVersion,
         'IdempotencyKey': ?idempotencyKey,
       });
  PostCommerceSection21ScheduledOrdersRequest.fromJson(super.json);
  String? get businessID => value('BusinessID') as String?;
  String? get customerID => value('CustomerID') as String?;
  String? get quoteID => value('QuoteID') as String?;
  String? get vendorOrderID => value('VendorOrderID') as String?;
  String? get branchID => value('BranchID') as String?;
  String? get desiredDeliveryAt => value('DesiredDeliveryAt') as String?;
  String? get deliveryWindowStart => value('DeliveryWindowStart') as String?;
  String? get deliveryWindowEnd => value('DeliveryWindowEnd') as String?;
  String? get productionReadyBy => value('ProductionReadyBy') as String?;
  String? get expectedReadyAt => value('ExpectedReadyAt') as String?;
  String? get fulfilmentPlanID => value('FulfilmentPlanID') as String?;
  String? get tentativeVehicleClass =>
      value('TentativeVehicleClass') as String?;
  JsonObject? get bulkRequirement => value('BulkRequirement') == null
      ? null
      : jsonObject(value('BulkRequirement'));
  JsonObject? get capacity =>
      value('Capacity') == null ? null : jsonObject(value('Capacity'));
  String? get sourceRef => value('SourceRef') as String?;
  int? get sourceVersion => value('SourceVersion') as int?;
  String? get idempotencyKey => value('IdempotencyKey') as String?;
}
