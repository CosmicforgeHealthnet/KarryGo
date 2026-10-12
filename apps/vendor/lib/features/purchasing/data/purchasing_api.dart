// GENERATED. Methods correspond to exact public Vendor contracts.
import '../../../core/network/vendor_api_client.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/network/vendor_response.dart';
import '../models/purchasing_models.dart';
import 'purchasing_routes.dart';

class VendorPurchasingApi extends VendorFeatureApi {
  VendorPurchasingApi(VendorApiClient client)
    : super(client, purchasingOperations);

  /// Source: section15-vendor-purchasing.openapi.json
  Future<VendorResponse> getPurchasingAlerts({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/purchasing/alerts',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section15-vendor-purchasing.openapi.json
  Future<VendorResponse> postPurchasingAlertsAlertIdAcknowledge({
    required String alertId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPurchasingAlertsAlertIdAcknowledgeRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/purchasing/alerts/{alertId}/acknowledge',
    pathParameters: {'alertId': alertId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section15-vendor-purchasing.openapi.json
  Future<VendorResponse> postPurchasingAlertsAlertIdResolve({
    required String alertId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPurchasingAlertsAlertIdResolveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/purchasing/alerts/{alertId}/resolve',
    pathParameters: {'alertId': alertId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section15-vendor-purchasing.openapi.json
  Future<VendorResponse> getPurchasingApprovalPolicy({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/purchasing/approval-policy',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/purchasing/purchase_order_http.go#replaceApprovalPolicy
  Future<VendorResponse> putPurchasingApprovalPolicy({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutPurchasingApprovalPolicyRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/purchasing/approval-policy',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section15-vendor-purchasing.openapi.json
  Future<VendorResponse> getPurchasingDashboard({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/purchasing/dashboard',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section15-vendor-purchasing.openapi.json
  Future<VendorResponse> getPurchasingGoodsReceipts({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/purchasing/goods-receipts',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/purchasing/goods_receiving_http.go#createReceipt
  Future<VendorResponse> postPurchasingGoodsReceipts({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPurchasingGoodsReceiptsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/purchasing/goods-receipts',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section15-vendor-purchasing.openapi.json
  Future<VendorResponse> getPurchasingGoodsReceiptsReceiptId({
    required String receiptId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/purchasing/goods-receipts/{receiptId}',
    pathParameters: {'receiptId': receiptId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section15-vendor-purchasing.openapi.json
  Future<VendorResponse> postPurchasingGoodsReceiptsReceiptIdConfirm({
    required String receiptId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPurchasingGoodsReceiptsReceiptIdConfirmRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/purchasing/goods-receipts/{receiptId}/confirm',
    pathParameters: {'receiptId': receiptId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section15-vendor-purchasing.openapi.json
  Future<VendorResponse> getPurchasingGoodsReceiptsReceiptIdDocuments({
    required String receiptId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/purchasing/goods-receipts/{receiptId}/documents',
    pathParameters: {'receiptId': receiptId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/purchasing/goods_receiving_http.go#attachDocument
  Future<VendorResponse> postPurchasingGoodsReceiptsReceiptIdDocuments({
    required String receiptId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPurchasingGoodsReceiptsReceiptIdDocumentsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/purchasing/goods-receipts/{receiptId}/documents',
    pathParameters: {'receiptId': receiptId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/purchasing/goods_receiving_http.go#inspectReceipt
  Future<VendorResponse> putPurchasingGoodsReceiptsReceiptIdInspection({
    required String receiptId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutPurchasingGoodsReceiptsReceiptIdInspectionRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/purchasing/goods-receipts/{receiptId}/inspection',
    pathParameters: {'receiptId': receiptId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/purchasing/goods_receiving_http.go#reverseReceipt
  Future<VendorResponse> postPurchasingGoodsReceiptsReceiptIdReverse({
    required String receiptId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPurchasingGoodsReceiptsReceiptIdReverseRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/purchasing/goods-receipts/{receiptId}/reverse',
    pathParameters: {'receiptId': receiptId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section15-vendor-purchasing.openapi.json
  Future<VendorResponse> getPurchasingOutstandingDeliveries({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/purchasing/outstanding-deliveries',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section15-vendor-purchasing.openapi.json
  Future<VendorResponse> getPurchasingPurchaseCostHistory({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/purchasing/purchase-cost-history',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section15-vendor-purchasing.openapi.json
  Future<VendorResponse> getPurchasingPurchaseOrders({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/purchasing/purchase-orders',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/purchasing/purchase_order_http.go#createPurchaseOrder
  Future<VendorResponse> postPurchasingPurchaseOrders({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPurchasingPurchaseOrdersRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/purchasing/purchase-orders',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section15-vendor-purchasing.openapi.json
  Future<VendorResponse> getPurchasingPurchaseOrdersPurchaseOrderId({
    required String purchaseOrderId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/purchasing/purchase-orders/{purchaseOrderId}',
    pathParameters: {'purchaseOrderId': purchaseOrderId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/purchasing/purchase_order_http.go#updatePurchaseOrder
  Future<VendorResponse> patchPurchasingPurchaseOrdersPurchaseOrderId({
    required String purchaseOrderId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchPurchasingPurchaseOrdersPurchaseOrderIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/purchasing/purchase-orders/{purchaseOrderId}',
    pathParameters: {'purchaseOrderId': purchaseOrderId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section15-vendor-purchasing.openapi.json
  Future<VendorResponse> getPurchasingPurchaseOrdersPurchaseOrderIdApprovals({
    required String purchaseOrderId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/purchasing/purchase-orders/{purchaseOrderId}/approvals',
    pathParameters: {'purchaseOrderId': purchaseOrderId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/purchasing/purchase_order_http.go#approval
  Future<VendorResponse> postPurchasingPurchaseOrdersPurchaseOrderIdApprove({
    required String purchaseOrderId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPurchasingPurchaseOrdersPurchaseOrderIdApproveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/purchasing/purchase-orders/{purchaseOrderId}/approve',
    pathParameters: {'purchaseOrderId': purchaseOrderId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/purchasing/purchase_order_http.go#transition
  Future<VendorResponse> postPurchasingPurchaseOrdersPurchaseOrderIdCancel({
    required String purchaseOrderId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPurchasingPurchaseOrdersPurchaseOrderIdCancelRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/purchasing/purchase-orders/{purchaseOrderId}/cancel',
    pathParameters: {'purchaseOrderId': purchaseOrderId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/purchasing/purchase_order_http.go#transition
  Future<VendorResponse> postPurchasingPurchaseOrdersPurchaseOrderIdClose({
    required String purchaseOrderId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPurchasingPurchaseOrdersPurchaseOrderIdCloseRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/purchasing/purchase-orders/{purchaseOrderId}/close',
    pathParameters: {'purchaseOrderId': purchaseOrderId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/purchasing/purchase_order_http.go#replacePurchaseOrderLines
  Future<VendorResponse> putPurchasingPurchaseOrdersPurchaseOrderIdLines({
    required String purchaseOrderId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutPurchasingPurchaseOrdersPurchaseOrderIdLinesRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/purchasing/purchase-orders/{purchaseOrderId}/lines',
    pathParameters: {'purchaseOrderId': purchaseOrderId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/purchasing/purchase_order_http.go#transition
  Future<VendorResponse> postPurchasingPurchaseOrdersPurchaseOrderIdOrder({
    required String purchaseOrderId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPurchasingPurchaseOrdersPurchaseOrderIdOrderRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/purchasing/purchase-orders/{purchaseOrderId}/order',
    pathParameters: {'purchaseOrderId': purchaseOrderId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/purchasing/purchase_order_http.go#approval
  Future<VendorResponse> postPurchasingPurchaseOrdersPurchaseOrderIdReject({
    required String purchaseOrderId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPurchasingPurchaseOrdersPurchaseOrderIdRejectRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/purchasing/purchase-orders/{purchaseOrderId}/reject',
    pathParameters: {'purchaseOrderId': purchaseOrderId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/purchasing/purchase_order_http.go#transition
  Future<VendorResponse> postPurchasingPurchaseOrdersPurchaseOrderIdSubmit({
    required String purchaseOrderId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPurchasingPurchaseOrdersPurchaseOrderIdSubmitRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/purchasing/purchase-orders/{purchaseOrderId}/submit',
    pathParameters: {'purchaseOrderId': purchaseOrderId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section15-vendor-purchasing.openapi.json
  Future<VendorResponse> getPurchasingReceivingPoliciesBranchesBranchId({
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/purchasing/receiving-policies/branches/{branchId}',
    pathParameters: {'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/purchasing/goods_receiving_http.go#putPolicy
  Future<VendorResponse> putPurchasingReceivingPoliciesBranchesBranchId({
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutPurchasingReceivingPoliciesBranchesBranchIdRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/purchasing/receiving-policies/branches/{branchId}',
    pathParameters: {'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section15-vendor-purchasing.openapi.json
  Future<VendorResponse> getPurchasingSupplierCategories({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/purchasing/supplier-categories',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/purchasing/http.go#createCategory
  Future<VendorResponse> postPurchasingSupplierCategories({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPurchasingSupplierCategoriesRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/purchasing/supplier-categories',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/purchasing/http.go#updateCategory
  Future<VendorResponse> patchPurchasingSupplierCategoriesCategoryId({
    required String categoryId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchPurchasingSupplierCategoriesCategoryIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/purchasing/supplier-categories/{categoryId}',
    pathParameters: {'categoryId': categoryId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/purchasing/http.go#archiveCategory
  Future<VendorResponse> postPurchasingSupplierCategoriesCategoryIdArchive({
    required String categoryId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPurchasingSupplierCategoriesCategoryIdArchiveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/purchasing/supplier-categories/{categoryId}/archive',
    pathParameters: {'categoryId': categoryId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section15-vendor-purchasing.openapi.json
  Future<VendorResponse> getPurchasingSupplierDiscrepancies({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/purchasing/supplier-discrepancies',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section15-vendor-purchasing.openapi.json
  Future<VendorResponse> getPurchasingSupplierDiscrepanciesDiscrepancyId({
    required String discrepancyId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/purchasing/supplier-discrepancies/{discrepancyId}',
    pathParameters: {'discrepancyId': discrepancyId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/purchasing/goods_receiving_http.go#discrepancyTransition
  Future<VendorResponse>
  postPurchasingSupplierDiscrepanciesDiscrepancyIdApprove({
    required String discrepancyId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPurchasingSupplierDiscrepanciesDiscrepancyIdApproveRequest
    body,
  }) => request(
    'POST',
    '/api/v1/vendor/purchasing/supplier-discrepancies/{discrepancyId}/approve',
    pathParameters: {'discrepancyId': discrepancyId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/purchasing/goods_receiving_http.go#discrepancyTransition
  Future<VendorResponse>
  postPurchasingSupplierDiscrepanciesDiscrepancyIdReject({
    required String discrepancyId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPurchasingSupplierDiscrepanciesDiscrepancyIdRejectRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/purchasing/supplier-discrepancies/{discrepancyId}/reject',
    pathParameters: {'discrepancyId': discrepancyId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/purchasing/goods_receiving_http.go#discrepancyTransition
  Future<VendorResponse>
  postPurchasingSupplierDiscrepanciesDiscrepancyIdResolve({
    required String discrepancyId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPurchasingSupplierDiscrepanciesDiscrepancyIdResolveRequest
    body,
  }) => request(
    'POST',
    '/api/v1/vendor/purchasing/supplier-discrepancies/{discrepancyId}/resolve',
    pathParameters: {'discrepancyId': discrepancyId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section15-vendor-purchasing.openapi.json
  Future<VendorResponse> getPurchasingSuppliers({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/purchasing/suppliers',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/purchasing/http.go#createSupplier
  Future<VendorResponse> postPurchasingSuppliers({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPurchasingSuppliersRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/purchasing/suppliers',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section15-vendor-purchasing.openapi.json
  Future<VendorResponse> getPurchasingSuppliersSupplierId({
    required String supplierId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/purchasing/suppliers/{supplierId}',
    pathParameters: {'supplierId': supplierId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/purchasing/http.go#updateSupplier
  Future<VendorResponse> patchPurchasingSuppliersSupplierId({
    required String supplierId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchPurchasingSuppliersSupplierIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/purchasing/suppliers/{supplierId}',
    pathParameters: {'supplierId': supplierId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/purchasing/http.go#archiveSupplier
  Future<VendorResponse> postPurchasingSuppliersSupplierIdArchive({
    required String supplierId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPurchasingSuppliersSupplierIdArchiveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/purchasing/suppliers/{supplierId}/archive',
    pathParameters: {'supplierId': supplierId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section15-vendor-purchasing.openapi.json
  Future<VendorResponse> getPurchasingSuppliersSupplierIdContacts({
    required String supplierId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/purchasing/suppliers/{supplierId}/contacts',
    pathParameters: {'supplierId': supplierId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/purchasing/http.go#createContact
  Future<VendorResponse> postPurchasingSuppliersSupplierIdContacts({
    required String supplierId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPurchasingSuppliersSupplierIdContactsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/purchasing/suppliers/{supplierId}/contacts',
    pathParameters: {'supplierId': supplierId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/purchasing/http.go#updateContact
  Future<VendorResponse> patchPurchasingSuppliersSupplierIdContactsContactId({
    required String supplierId,
    required String contactId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchPurchasingSuppliersSupplierIdContactsContactIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/purchasing/suppliers/{supplierId}/contacts/{contactId}',
    pathParameters: {'supplierId': supplierId, 'contactId': contactId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/purchasing/http.go#archiveContact
  Future<VendorResponse>
  postPurchasingSuppliersSupplierIdContactsContactIdArchive({
    required String supplierId,
    required String contactId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPurchasingSuppliersSupplierIdContactsContactIdArchiveRequest
    body,
  }) => request(
    'POST',
    '/api/v1/vendor/purchasing/suppliers/{supplierId}/contacts/{contactId}/archive',
    pathParameters: {'supplierId': supplierId, 'contactId': contactId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section15-vendor-purchasing.openapi.json
  Future<VendorResponse> getPurchasingSuppliersSupplierIdPerformance({
    required String supplierId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/purchasing/suppliers/{supplierId}/performance',
    pathParameters: {'supplierId': supplierId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section15-vendor-purchasing.openapi.json
  Future<VendorResponse> getPurchasingSuppliersSupplierIdProducts({
    required String supplierId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/purchasing/suppliers/{supplierId}/products',
    pathParameters: {'supplierId': supplierId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/purchasing/http.go#createSupplierProduct
  Future<VendorResponse> postPurchasingSuppliersSupplierIdProducts({
    required String supplierId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPurchasingSuppliersSupplierIdProductsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/purchasing/suppliers/{supplierId}/products',
    pathParameters: {'supplierId': supplierId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section15-vendor-purchasing.openapi.json
  Future<VendorResponse>
  getPurchasingSuppliersSupplierIdProductsSupplierProductId({
    required String supplierId,
    required String supplierProductId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/purchasing/suppliers/{supplierId}/products/{supplierProductId}',
    pathParameters: {
      'supplierId': supplierId,
      'supplierProductId': supplierProductId,
    },
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/purchasing/http.go#updateSupplierProduct
  Future<VendorResponse>
  patchPurchasingSuppliersSupplierIdProductsSupplierProductId({
    required String supplierId,
    required String supplierProductId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchPurchasingSuppliersSupplierIdProductsSupplierProductIdRequest
    body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/purchasing/suppliers/{supplierId}/products/{supplierProductId}',
    pathParameters: {
      'supplierId': supplierId,
      'supplierProductId': supplierProductId,
    },
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/purchasing/http.go#archiveSupplierProduct
  Future<VendorResponse>
  postPurchasingSuppliersSupplierIdProductsSupplierProductIdArchive({
    required String supplierId,
    required String supplierProductId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPurchasingSuppliersSupplierIdProductsSupplierProductIdArchiveRequest
    body,
  }) => request(
    'POST',
    '/api/v1/vendor/purchasing/suppliers/{supplierId}/products/{supplierProductId}/archive',
    pathParameters: {
      'supplierId': supplierId,
      'supplierProductId': supplierProductId,
    },
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section15-vendor-purchasing.openapi.json
  Future<VendorResponse> getPurchasingSuppliersSupplierIdTerms({
    required String supplierId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/purchasing/suppliers/{supplierId}/terms',
    pathParameters: {'supplierId': supplierId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/purchasing/http.go#upsertTerms
  Future<VendorResponse> putPurchasingSuppliersSupplierIdTerms({
    required String supplierId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutPurchasingSuppliersSupplierIdTermsRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/purchasing/suppliers/{supplierId}/terms',
    pathParameters: {'supplierId': supplierId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );
}
