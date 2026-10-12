// GENERATED. Methods correspond to exact public Vendor contracts.
import '../../../core/network/vendor_api_client.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/network/vendor_response.dart';
import '../models/shop_catalogue_models.dart';
import 'shop_catalogue_routes.dart';

class VendorShopCatalogueApi extends VendorFeatureApi {
  VendorShopCatalogueApi(VendorApiClient client)
    : super(client, shopCatalogueOperations);

  /// Source: section12b-shop-inventory.openapi.json
  Future<VendorResponse> getShopBranchesBranchIdInventoryAlerts({
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/branches/{branchId}/inventory/alerts',
    pathParameters: {'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section12b-shop-inventory.openapi.json
  Future<VendorResponse> getShopBranchesBranchIdInventoryBalances({
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/branches/{branchId}/inventory/balances',
    pathParameters: {'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section12b-shop-inventory.openapi.json
  Future<VendorResponse> getShopBranchesBranchIdInventoryBalancesStockBasisId({
    required String branchId,
    required String stockBasisId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/branches/{branchId}/inventory/balances/{stockBasisId}',
    pathParameters: {'branchId': branchId, 'stockBasisId': stockBasisId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section12b-shop-inventory.openapi.json
  Future<VendorResponse> getShopBranchesBranchIdInventoryMovements({
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/branches/{branchId}/inventory/movements',
    pathParameters: {'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section12b-shop-inventory.openapi.json
  Future<VendorResponse> getShopBranchesBranchIdInventoryQuarantines({
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/branches/{branchId}/inventory/quarantines',
    pathParameters: {'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section12b-shop-inventory.openapi.json
  Future<VendorResponse> getShopBranchesBranchIdInventoryReceipts({
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/branches/{branchId}/inventory/receipts',
    pathParameters: {'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section12b-shop-inventory.openapi.json
  Future<VendorResponse> getShopBranchesBranchIdInventoryReorderRules({
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/branches/{branchId}/inventory/reorder-rules',
    pathParameters: {'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section12b-shop-inventory.openapi.json
  Future<VendorResponse> getShopBranchesBranchIdInventoryRevision({
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/branches/{branchId}/inventory/revision',
    pathParameters: {'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/shop/inventory_http.go
  Future<VendorResponse> putShopBranchesBranchIdInventoryStockBases({
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutShopBranchesBranchIdInventoryStockBasesRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/shop/branches/{branchId}/inventory/stock-bases',
    pathParameters: {'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12b-shop-inventory.openapi.json
  Future<VendorResponse> getShopBranchesBranchIdInventoryStocktakes({
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/branches/{branchId}/inventory/stocktakes',
    pathParameters: {'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/shop/inventory_http.go
  Future<VendorResponse> postShopBranchesBranchIdInventoryStocktakes({
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopBranchesBranchIdInventoryStocktakesRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/branches/{branchId}/inventory/stocktakes',
    pathParameters: {'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12b-shop-inventory.openapi.json
  Future<VendorResponse> getShopBranchesBranchIdInventoryTransfers({
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/branches/{branchId}/inventory/transfers',
    pathParameters: {'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section12b-shop-inventory.openapi.json
  Future<VendorResponse>
  getShopBranchesBranchIdInventoryVariantsVariantIdAvailability({
    required String branchId,
    required String variantId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/branches/{branchId}/inventory/variants/{variantId}/availability',
    pathParameters: {'branchId': branchId, 'variantId': variantId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section12b-shop-inventory.openapi.json
  Future<VendorResponse>
  getShopBranchesBranchIdInventoryVariantsVariantIdSellability({
    required String branchId,
    required String variantId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/branches/{branchId}/inventory/variants/{variantId}/sellability',
    pathParameters: {'branchId': branchId, 'variantId': variantId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section12a-shop-catalogue.openapi.json
  Future<VendorResponse> getShopBranchesBranchIdListings({
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/branches/{branchId}/listings',
    pathParameters: {'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#createListing
  Future<VendorResponse> postShopBranchesBranchIdListings({
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopBranchesBranchIdListingsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/branches/{branchId}/listings',
    pathParameters: {'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12a-shop-catalogue.openapi.json
  Future<VendorResponse> getShopBranchesBranchIdOperatingModels({
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/branches/{branchId}/operating-models',
    pathParameters: {'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#putOperatingModels
  Future<VendorResponse> putShopBranchesBranchIdOperatingModels({
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutShopBranchesBranchIdOperatingModelsRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/shop/branches/{branchId}/operating-models',
    pathParameters: {'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#archiveOperatingModels
  Future<VendorResponse> postShopBranchesBranchIdOperatingModelsArchive({
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopBranchesBranchIdOperatingModelsArchiveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/branches/{branchId}/operating-models/archive',
    pathParameters: {'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12b-shop-inventory.openapi.json
  Future<VendorResponse> postShopBulkOperations({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopBulkOperationsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/bulk-operations',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12b-shop-inventory.openapi.json
  Future<VendorResponse> getShopBulkOperationsJobId({
    required String jobId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/bulk-operations/{jobId}',
    pathParameters: {'jobId': jobId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section12b-shop-inventory.openapi.json
  Future<VendorResponse> postShopBulkOperationsJobIdApply({
    required String jobId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopBulkOperationsJobIdApplyRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/bulk-operations/{jobId}/apply',
    pathParameters: {'jobId': jobId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12a-shop-catalogue.openapi.json
  Future<VendorResponse> getShopCatalogueRevision({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/catalogue-revision',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section12a-shop-catalogue.openapi.json
  Future<VendorResponse> getShopCategories({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/categories',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#createCategory
  Future<VendorResponse> postShopCategories({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopCategoriesRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/categories',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12a-shop-catalogue.openapi.json
  Future<VendorResponse> getShopCategoriesCategoryId({
    required String categoryId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/categories/{categoryId}',
    pathParameters: {'categoryId': categoryId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#updateCategory
  Future<VendorResponse> patchShopCategoriesCategoryId({
    required String categoryId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchShopCategoriesCategoryIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/shop/categories/{categoryId}',
    pathParameters: {'categoryId': categoryId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#categoryLifecycle
  Future<VendorResponse> postShopCategoriesCategoryIdArchive({
    required String categoryId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopCategoriesCategoryIdArchiveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/categories/{categoryId}/archive',
    pathParameters: {'categoryId': categoryId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#categoryLifecycle
  Future<VendorResponse> postShopCategoriesCategoryIdRestore({
    required String categoryId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopCategoriesCategoryIdRestoreRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/categories/{categoryId}/restore',
    pathParameters: {'categoryId': categoryId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12a-shop-catalogue.openapi.json
  Future<VendorResponse> getShopIdentifierLookup({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/identifier-lookup',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/shop/inventory_http.go
  Future<VendorResponse> postShopInventoryAdjustments({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopInventoryAdjustmentsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/inventory/adjustments',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12b-shop-inventory.openapi.json
  Future<VendorResponse> postShopInventoryAlertsAlertIdAcknowledge({
    required String alertId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopInventoryAlertsAlertIdAcknowledgeRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/inventory/alerts/{alertId}/acknowledge',
    pathParameters: {'alertId': alertId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12b-shop-inventory.openapi.json
  Future<VendorResponse> postShopInventoryAlertsAlertIdResolve({
    required String alertId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopInventoryAlertsAlertIdResolveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/inventory/alerts/{alertId}/resolve',
    pathParameters: {'alertId': alertId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12b-shop-inventory.openapi.json
  Future<VendorResponse> postShopInventoryFulfilmentMeasurements({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopInventoryFulfilmentMeasurementsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/inventory/fulfilment-measurements',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/shop/inventory_http.go
  Future<VendorResponse> postShopInventoryQuarantines({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopInventoryQuarantinesRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/inventory/quarantines',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/shop/inventory_http.go
  Future<VendorResponse> postShopInventoryQuarantinesQuarantineIdResolve({
    required String quarantineId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopInventoryQuarantinesQuarantineIdResolveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/inventory/quarantines/{quarantineId}/resolve',
    pathParameters: {'quarantineId': quarantineId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/shop/inventory_http.go
  Future<VendorResponse> postShopInventoryReceipts({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopInventoryReceiptsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/inventory/receipts',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12b-shop-inventory.openapi.json
  Future<VendorResponse> getShopInventoryReceiptsReceiptId({
    required String receiptId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/inventory/receipts/{receiptId}',
    pathParameters: {'receiptId': receiptId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section12b-shop-inventory.openapi.json
  Future<VendorResponse> postShopInventoryReceiptsReceiptIdCancel({
    required String receiptId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopInventoryReceiptsReceiptIdCancelRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/inventory/receipts/{receiptId}/cancel',
    pathParameters: {'receiptId': receiptId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12b-shop-inventory.openapi.json
  Future<VendorResponse> postShopInventoryReceiptsReceiptIdPost({
    required String receiptId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopInventoryReceiptsReceiptIdPostRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/inventory/receipts/{receiptId}/post',
    pathParameters: {'receiptId': receiptId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12b-shop-inventory.openapi.json
  Future<VendorResponse> postShopInventoryReservations({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopInventoryReservationsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/inventory/reservations',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12b-shop-inventory.openapi.json
  Future<VendorResponse> getShopInventoryReservationsReservationId({
    required String reservationId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/inventory/reservations/{reservationId}',
    pathParameters: {'reservationId': reservationId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section12b-shop-inventory.openapi.json
  Future<VendorResponse> postShopInventoryReservationsReservationIdCommit({
    required String reservationId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopInventoryReservationsReservationIdCommitRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/inventory/reservations/{reservationId}/commit',
    pathParameters: {'reservationId': reservationId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12b-shop-inventory.openapi.json
  Future<VendorResponse> postShopInventoryReservationsReservationIdConfirm({
    required String reservationId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopInventoryReservationsReservationIdConfirmRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/inventory/reservations/{reservationId}/confirm',
    pathParameters: {'reservationId': reservationId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12b-shop-inventory.openapi.json
  Future<VendorResponse> postShopInventoryReservationsReservationIdRelease({
    required String reservationId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopInventoryReservationsReservationIdReleaseRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/inventory/reservations/{reservationId}/release',
    pathParameters: {'reservationId': reservationId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/shop/inventory_http.go
  Future<VendorResponse> putShopInventoryStockBasesStockBasisIdReorderRule({
    required String stockBasisId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutShopInventoryStockBasesStockBasisIdReorderRuleRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/shop/inventory/stock-bases/{stockBasisId}/reorder-rule',
    pathParameters: {'stockBasisId': stockBasisId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12b-shop-inventory.openapi.json
  Future<VendorResponse> getShopInventoryStocktakesStocktakeId({
    required String stocktakeId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/inventory/stocktakes/{stocktakeId}',
    pathParameters: {'stocktakeId': stocktakeId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/shop/inventory_http.go
  Future<VendorResponse>
  putShopInventoryStocktakesStocktakeIdLinesStockBasisId({
    required String stocktakeId,
    required String stockBasisId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutShopInventoryStocktakesStocktakeIdLinesStockBasisIdRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/shop/inventory/stocktakes/{stocktakeId}/lines/{stockBasisId}',
    pathParameters: {'stocktakeId': stocktakeId, 'stockBasisId': stockBasisId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12b-shop-inventory.openapi.json
  Future<VendorResponse> postShopInventoryStocktakesStocktakeIdPost({
    required String stocktakeId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopInventoryStocktakesStocktakeIdPostRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/inventory/stocktakes/{stocktakeId}/post',
    pathParameters: {'stocktakeId': stocktakeId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/shop/inventory_http.go
  Future<VendorResponse> postShopInventoryTransfers({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopInventoryTransfersRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/inventory/transfers',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12b-shop-inventory.openapi.json
  Future<VendorResponse> getShopInventoryTransfersTransferId({
    required String transferId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/inventory/transfers/{transferId}',
    pathParameters: {'transferId': transferId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section12b-shop-inventory.openapi.json
  Future<VendorResponse> postShopInventoryTransfersTransferIdCancel({
    required String transferId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopInventoryTransfersTransferIdCancelRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/inventory/transfers/{transferId}/cancel',
    pathParameters: {'transferId': transferId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12b-shop-inventory.openapi.json
  Future<VendorResponse> postShopInventoryTransfersTransferIdDispatch({
    required String transferId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopInventoryTransfersTransferIdDispatchRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/inventory/transfers/{transferId}/dispatch',
    pathParameters: {'transferId': transferId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/shop/inventory_http.go
  Future<VendorResponse> postShopInventoryTransfersTransferIdReceive({
    required String transferId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopInventoryTransfersTransferIdReceiveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/inventory/transfers/{transferId}/receive',
    pathParameters: {'transferId': transferId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12a-shop-catalogue.openapi.json
  Future<VendorResponse> getShopListingsListingId({
    required String listingId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/listings/{listingId}',
    pathParameters: {'listingId': listingId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#updateListing
  Future<VendorResponse> patchShopListingsListingId({
    required String listingId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchShopListingsListingIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/shop/listings/{listingId}',
    pathParameters: {'listingId': listingId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#archiveListing
  Future<VendorResponse> postShopListingsListingIdArchive({
    required String listingId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopListingsListingIdArchiveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/listings/{listingId}/archive',
    pathParameters: {'listingId': listingId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12b-shop-inventory.openapi.json
  Future<VendorResponse> putShopListingsListingIdInventoryPolicy({
    required String listingId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutShopListingsListingIdInventoryPolicyRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/shop/listings/{listingId}/inventory-policy',
    pathParameters: {'listingId': listingId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12a-shop-catalogue.openapi.json
  Future<VendorResponse> getShopListingsListingIdQuantityBreaks({
    required String listingId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/listings/{listingId}/quantity-breaks',
    pathParameters: {'listingId': listingId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#createQuantityBreak
  Future<VendorResponse> postShopListingsListingIdQuantityBreaks({
    required String listingId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopListingsListingIdQuantityBreaksRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/listings/{listingId}/quantity-breaks',
    pathParameters: {'listingId': listingId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#updateQuantityBreak
  Future<VendorResponse> patchShopListingsListingIdQuantityBreaksBreakId({
    required String listingId,
    required String breakId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchShopListingsListingIdQuantityBreaksBreakIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/shop/listings/{listingId}/quantity-breaks/{breakId}',
    pathParameters: {'listingId': listingId, 'breakId': breakId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#archiveQuantityBreak
  Future<VendorResponse> postShopListingsListingIdQuantityBreaksBreakIdArchive({
    required String listingId,
    required String breakId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopListingsListingIdQuantityBreaksBreakIdArchiveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/listings/{listingId}/quantity-breaks/{breakId}/archive',
    pathParameters: {'listingId': listingId, 'breakId': breakId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12a-shop-catalogue.openapi.json
  Future<VendorResponse> getShopListingsListingIdResolvedPrice({
    required String listingId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/listings/{listingId}/resolved-price',
    pathParameters: {'listingId': listingId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#setListingUnitPrice
  Future<VendorResponse> putShopListingsListingIdSellUnitsListingUnitId({
    required String listingId,
    required String listingUnitId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutShopListingsListingIdSellUnitsListingUnitIdRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/shop/listings/{listingId}/sell-units/{listingUnitId}',
    pathParameters: {'listingId': listingId, 'listingUnitId': listingUnitId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12a-shop-catalogue.openapi.json
  Future<VendorResponse> getShopMeasurements({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/measurements',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#createMeasure
  Future<VendorResponse> postShopMeasurements({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopMeasurementsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/measurements',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12a-shop-catalogue.openapi.json
  Future<VendorResponse> getShopMeasurementsMeasureId({
    required String measureId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/measurements/{measureId}',
    pathParameters: {'measureId': measureId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#updateMeasure
  Future<VendorResponse> patchShopMeasurementsMeasureId({
    required String measureId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchShopMeasurementsMeasureIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/shop/measurements/{measureId}',
    pathParameters: {'measureId': measureId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12a-shop-catalogue.openapi.json
  Future<VendorResponse> getShopMeasurementsMeasureIdAliases({
    required String measureId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/measurements/{measureId}/aliases',
    pathParameters: {'measureId': measureId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#addAlias
  Future<VendorResponse> postShopMeasurementsMeasureIdAliases({
    required String measureId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopMeasurementsMeasureIdAliasesRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/measurements/{measureId}/aliases',
    pathParameters: {'measureId': measureId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12a-shop-catalogue.openapi.json
  Future<VendorResponse> deleteShopMeasurementsMeasureIdAliasesAliasId({
    required String measureId,
    required String aliasId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required DeleteShopMeasurementsMeasureIdAliasesAliasIdRequest body,
  }) => request(
    'DELETE',
    '/api/v1/vendor/shop/measurements/{measureId}/aliases/{aliasId}',
    pathParameters: {'measureId': measureId, 'aliasId': aliasId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#archiveMeasure
  Future<VendorResponse> postShopMeasurementsMeasureIdArchive({
    required String measureId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopMeasurementsMeasureIdArchiveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/measurements/{measureId}/archive',
    pathParameters: {'measureId': measureId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12a-shop-catalogue.openapi.json
  Future<VendorResponse> getShopPriceHistory({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/price-history',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section12a-shop-catalogue.openapi.json
  Future<VendorResponse> getShopProducts({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/products',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#createProduct
  Future<VendorResponse> postShopProducts({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopProductsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/products',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12a-shop-catalogue.openapi.json
  Future<VendorResponse> getShopProductsProductId({
    required String productId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/products/{productId}',
    pathParameters: {'productId': productId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#updateProduct
  Future<VendorResponse> patchShopProductsProductId({
    required String productId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchShopProductsProductIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/shop/products/{productId}',
    pathParameters: {'productId': productId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#archiveProduct
  Future<VendorResponse> postShopProductsProductIdArchive({
    required String productId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopProductsProductIdArchiveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/products/{productId}/archive',
    pathParameters: {'productId': productId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12a-shop-catalogue.openapi.json
  Future<VendorResponse> getShopProductsProductIdFreshAttributes({
    required String productId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/products/{productId}/fresh-attributes',
    pathParameters: {'productId': productId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#configureFresh
  Future<VendorResponse> putShopProductsProductIdFreshAttributes({
    required String productId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutShopProductsProductIdFreshAttributesRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/shop/products/{productId}/fresh-attributes',
    pathParameters: {'productId': productId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#archiveFresh
  Future<VendorResponse>
  postShopProductsProductIdFreshAttributesAttributeIdArchive({
    required String productId,
    required String attributeId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopProductsProductIdFreshAttributesAttributeIdArchiveRequest
    body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/products/{productId}/fresh-attributes/{attributeId}/archive',
    pathParameters: {'productId': productId, 'attributeId': attributeId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12a-shop-catalogue.openapi.json
  Future<VendorResponse> getShopProductsProductIdMeasureCalibrations({
    required String productId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/products/{productId}/measure-calibrations',
    pathParameters: {'productId': productId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#createCalibration
  Future<VendorResponse> postShopProductsProductIdMeasureCalibrations({
    required String productId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopProductsProductIdMeasureCalibrationsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/products/{productId}/measure-calibrations',
    pathParameters: {'productId': productId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#updateCalibration
  Future<VendorResponse>
  patchShopProductsProductIdMeasureCalibrationsCalibrationId({
    required String productId,
    required String calibrationId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchShopProductsProductIdMeasureCalibrationsCalibrationIdRequest
    body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/shop/products/{productId}/measure-calibrations/{calibrationId}',
    pathParameters: {'productId': productId, 'calibrationId': calibrationId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#archiveCalibration
  Future<VendorResponse>
  postShopProductsProductIdMeasureCalibrationsCalibrationIdArchive({
    required String productId,
    required String calibrationId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopProductsProductIdMeasureCalibrationsCalibrationIdArchiveRequest
    body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/products/{productId}/measure-calibrations/{calibrationId}/archive',
    pathParameters: {'productId': productId, 'calibrationId': calibrationId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12a-shop-catalogue.openapi.json
  Future<VendorResponse> getShopProductsProductIdPackaging({
    required String productId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/products/{productId}/packaging',
    pathParameters: {'productId': productId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#addPackage
  Future<VendorResponse> postShopProductsProductIdPackaging({
    required String productId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopProductsProductIdPackagingRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/products/{productId}/packaging',
    pathParameters: {'productId': productId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#archivePackage
  Future<VendorResponse> postShopProductsProductIdPackagingRelationIdArchive({
    required String productId,
    required String relationId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopProductsProductIdPackagingRelationIdArchiveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/products/{productId}/packaging/{relationId}/archive',
    pathParameters: {'productId': productId, 'relationId': relationId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12a-shop-catalogue.openapi.json
  Future<VendorResponse> getShopProductsProductIdSellUnits({
    required String productId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/products/{productId}/sell-units',
    pathParameters: {'productId': productId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#createSellUnit
  Future<VendorResponse> postShopProductsProductIdSellUnits({
    required String productId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopProductsProductIdSellUnitsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/products/{productId}/sell-units',
    pathParameters: {'productId': productId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#updateSellUnit
  Future<VendorResponse> patchShopProductsProductIdSellUnitsSellUnitId({
    required String productId,
    required String sellUnitId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchShopProductsProductIdSellUnitsSellUnitIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/shop/products/{productId}/sell-units/{sellUnitId}',
    pathParameters: {'productId': productId, 'sellUnitId': sellUnitId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#archiveSellUnit
  Future<VendorResponse> postShopProductsProductIdSellUnitsSellUnitIdArchive({
    required String productId,
    required String sellUnitId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopProductsProductIdSellUnitsSellUnitIdArchiveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/products/{productId}/sell-units/{sellUnitId}/archive',
    pathParameters: {'productId': productId, 'sellUnitId': sellUnitId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12a-shop-catalogue.openapi.json
  Future<VendorResponse> getShopProductsProductIdVariants({
    required String productId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/products/{productId}/variants',
    pathParameters: {'productId': productId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#createVariant
  Future<VendorResponse> postShopProductsProductIdVariants({
    required String productId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopProductsProductIdVariantsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/products/{productId}/variants',
    pathParameters: {'productId': productId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#updateVariant
  Future<VendorResponse> patchShopProductsProductIdVariantsVariantId({
    required String productId,
    required String variantId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchShopProductsProductIdVariantsVariantIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/shop/products/{productId}/variants/{variantId}',
    pathParameters: {'productId': productId, 'variantId': variantId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#archiveVariant
  Future<VendorResponse> postShopProductsProductIdVariantsVariantIdArchive({
    required String productId,
    required String variantId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopProductsProductIdVariantsVariantIdArchiveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/products/{productId}/variants/{variantId}/archive',
    pathParameters: {'productId': productId, 'variantId': variantId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section12a-shop-catalogue.openapi.json
  Future<VendorResponse> getShopVariantsVariantId({
    required String variantId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/variants/{variantId}',
    pathParameters: {'variantId': variantId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section12a-shop-catalogue.openapi.json
  Future<VendorResponse> getShopVariantsVariantIdIdentifiers({
    required String variantId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/shop/variants/{variantId}/identifiers',
    pathParameters: {'variantId': variantId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#addIdentifier
  Future<VendorResponse> postShopVariantsVariantIdIdentifiers({
    required String variantId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopVariantsVariantIdIdentifiersRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/variants/{variantId}/identifiers',
    pathParameters: {'variantId': variantId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/shop/http.go#archiveIdentifier
  Future<VendorResponse>
  postShopVariantsVariantIdIdentifiersIdentifierIdArchive({
    required String variantId,
    required String identifierId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostShopVariantsVariantIdIdentifiersIdentifierIdArchiveRequest
    body,
  }) => request(
    'POST',
    '/api/v1/vendor/shop/variants/{variantId}/identifiers/{identifierId}/archive',
    pathParameters: {'variantId': variantId, 'identifierId': identifierId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );
}
