// GENERATED. Methods correspond to exact public Vendor contracts.
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_api_client.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/network/vendor_response.dart';
import '../models/inventory_models.dart';
import 'inventory_routes.dart';

class VendorInventoryApi extends VendorFeatureApi {
  VendorInventoryApi(VendorApiClient client)
    : super(client, inventoryOperations);

  /// Source: section14-vendor-inventory.openapi.json
  Future<VendorResponse> postInventoryAlertsAlertIdAcknowledge({
    required String alertId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    VendorPayload? body,
  }) => request(
    'POST',
    '/api/v1/vendor/inventory/alerts/{alertId}/acknowledge',
    pathParameters: {'alertId': alertId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section14-vendor-inventory.openapi.json
  Future<VendorResponse> postInventoryAlertsAlertIdResolve({
    required String alertId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    VendorPayload? body,
  }) => request(
    'POST',
    '/api/v1/vendor/inventory/alerts/{alertId}/resolve',
    pathParameters: {'alertId': alertId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section14-vendor-inventory.openapi.json
  Future<VendorResponse> getInventoryBranchesBranchIdAlerts({
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/inventory/branches/{branchId}/alerts',
    pathParameters: {'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section14-vendor-inventory.openapi.json
  Future<VendorResponse> getInventoryBranchesBranchIdBalances({
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/inventory/branches/{branchId}/balances',
    pathParameters: {'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section14-vendor-inventory.openapi.json
  Future<VendorResponse> getInventoryBranchesBranchIdMovements({
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/inventory/branches/{branchId}/movements',
    pathParameters: {'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section14-vendor-inventory.openapi.json
  Future<VendorResponse> getInventoryBranchesBranchIdRevision({
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/inventory/branches/{branchId}/revision',
    pathParameters: {'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section14-vendor-inventory.openapi.json
  Future<VendorResponse> getInventoryItemsInventoryItemIdAvailability({
    required String inventoryItemId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/inventory/items/{inventoryItemId}/availability',
    pathParameters: {'inventoryItemId': inventoryItemId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/inventory/operations_http.go#setReorderRule
  Future<VendorResponse> putInventoryItemsInventoryItemIdReorderRule({
    required String inventoryItemId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutInventoryItemsInventoryItemIdReorderRuleRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/inventory/items/{inventoryItemId}/reorder-rule',
    pathParameters: {'inventoryItemId': inventoryItemId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section14-vendor-inventory.openapi.json
  Future<VendorResponse> getInventoryItemsInventoryItemIdSellability({
    required String inventoryItemId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/inventory/items/{inventoryItemId}/sellability',
    pathParameters: {'inventoryItemId': inventoryItemId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/inventory/operations_http.go#createQuarantine
  Future<VendorResponse> postInventoryQuarantines({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostInventoryQuarantinesRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/inventory/quarantines',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/inventory/operations_http.go#quarantineTransition
  Future<VendorResponse> postInventoryQuarantinesQuarantineIdDispose({
    required String quarantineId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostInventoryQuarantinesQuarantineIdDisposeRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/inventory/quarantines/{quarantineId}/dispose',
    pathParameters: {'quarantineId': quarantineId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/inventory/operations_http.go#quarantineTransition
  Future<VendorResponse> postInventoryQuarantinesQuarantineIdRelease({
    required String quarantineId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostInventoryQuarantinesQuarantineIdReleaseRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/inventory/quarantines/{quarantineId}/release',
    pathParameters: {'quarantineId': quarantineId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/inventory/reservation_http.go#createReservation
  Future<VendorResponse> postInventoryReservations({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostInventoryReservationsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/inventory/reservations',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section14-vendor-inventory.openapi.json
  Future<VendorResponse> getInventoryReservationsReservationId({
    required String reservationId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/inventory/reservations/{reservationId}',
    pathParameters: {'reservationId': reservationId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section14-vendor-inventory.openapi.json
  Future<VendorResponse> postInventoryReservationsReservationIdCancel({
    required String reservationId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    VendorPayload? body,
  }) => request(
    'POST',
    '/api/v1/vendor/inventory/reservations/{reservationId}/cancel',
    pathParameters: {'reservationId': reservationId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section14-vendor-inventory.openapi.json
  Future<VendorResponse> postInventoryReservationsReservationIdCommit({
    required String reservationId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    VendorPayload? body,
  }) => request(
    'POST',
    '/api/v1/vendor/inventory/reservations/{reservationId}/commit',
    pathParameters: {'reservationId': reservationId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section14-vendor-inventory.openapi.json
  Future<VendorResponse> postInventoryReservationsReservationIdConfirm({
    required String reservationId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    VendorPayload? body,
  }) => request(
    'POST',
    '/api/v1/vendor/inventory/reservations/{reservationId}/confirm',
    pathParameters: {'reservationId': reservationId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section14-vendor-inventory.openapi.json
  Future<VendorResponse> postInventoryReservationsReservationIdRelease({
    required String reservationId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    VendorPayload? body,
  }) => request(
    'POST',
    '/api/v1/vendor/inventory/reservations/{reservationId}/release',
    pathParameters: {'reservationId': reservationId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/inventory/operations_http.go#createStocktake
  Future<VendorResponse> postInventoryStocktakes({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostInventoryStocktakesRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/inventory/stocktakes',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section14-vendor-inventory.openapi.json
  Future<VendorResponse> postInventoryStocktakesStocktakeIdCancel({
    required String stocktakeId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    VendorPayload? body,
  }) => request(
    'POST',
    '/api/v1/vendor/inventory/stocktakes/{stocktakeId}/cancel',
    pathParameters: {'stocktakeId': stocktakeId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/inventory/operations_http.go#countStocktake
  Future<VendorResponse>
  putInventoryStocktakesStocktakeIdItemsInventoryItemIdCount({
    required String stocktakeId,
    required String inventoryItemId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutInventoryStocktakesStocktakeIdItemsInventoryItemIdCountRequest
    body,
  }) => request(
    'PUT',
    '/api/v1/vendor/inventory/stocktakes/{stocktakeId}/items/{inventoryItemId}/count',
    pathParameters: {
      'stocktakeId': stocktakeId,
      'inventoryItemId': inventoryItemId,
    },
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section14-vendor-inventory.openapi.json
  Future<VendorResponse> postInventoryStocktakesStocktakeIdPost({
    required String stocktakeId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    VendorPayload? body,
  }) => request(
    'POST',
    '/api/v1/vendor/inventory/stocktakes/{stocktakeId}/post',
    pathParameters: {'stocktakeId': stocktakeId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/inventory/operations_http.go#createTransfer
  Future<VendorResponse> postInventoryTransfers({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostInventoryTransfersRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/inventory/transfers',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section14-vendor-inventory.openapi.json
  Future<VendorResponse> postInventoryTransfersTransferIdApprove({
    required String transferId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    VendorPayload? body,
  }) => request(
    'POST',
    '/api/v1/vendor/inventory/transfers/{transferId}/approve',
    pathParameters: {'transferId': transferId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section14-vendor-inventory.openapi.json
  Future<VendorResponse> postInventoryTransfersTransferIdCancel({
    required String transferId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    VendorPayload? body,
  }) => request(
    'POST',
    '/api/v1/vendor/inventory/transfers/{transferId}/cancel',
    pathParameters: {'transferId': transferId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section14-vendor-inventory.openapi.json
  Future<VendorResponse> postInventoryTransfersTransferIdDispatch({
    required String transferId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    VendorPayload? body,
  }) => request(
    'POST',
    '/api/v1/vendor/inventory/transfers/{transferId}/dispatch',
    pathParameters: {'transferId': transferId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/inventory/operations_http.go#receiveTransfer
  Future<VendorResponse> postInventoryTransfersTransferIdReceive({
    required String transferId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostInventoryTransfersTransferIdReceiveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/inventory/transfers/{transferId}/receive',
    pathParameters: {'transferId': transferId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );
}
