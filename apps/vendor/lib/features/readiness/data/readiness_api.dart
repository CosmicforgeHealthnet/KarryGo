// GENERATED. Methods correspond to exact public Vendor contracts.
import '../../../core/network/vendor_api_client.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/network/vendor_response.dart';
import '../models/readiness_models.dart';
import 'readiness_routes.dart';

class VendorReadinessApi extends VendorFeatureApi {
  VendorReadinessApi(VendorApiClient client)
    : super(client, readinessOperations);

  /// Source: services/vendor-service/internal/features/fulfilment_readiness/http.go
  Future<VendorResponse> getCommerceOrdersVendorOrderIdReadiness({
    required String vendorOrderId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/commerce/orders/{vendorOrderId}/readiness',
    pathParameters: {'vendorOrderId': vendorOrderId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/fulfilment_readiness/http.go#block
  Future<VendorResponse> postCommerceOrdersVendorOrderIdReadinessBlock({
    required String vendorOrderId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCommerceOrdersVendorOrderIdReadinessBlockRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/commerce/orders/{vendorOrderId}/readiness/block',
    pathParameters: {'vendorOrderId': vendorOrderId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/fulfilment_readiness/http.go#delay
  Future<VendorResponse> postCommerceOrdersVendorOrderIdReadinessDelay({
    required String vendorOrderId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCommerceOrdersVendorOrderIdReadinessDelayRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/commerce/orders/{vendorOrderId}/readiness/delay',
    pathParameters: {'vendorOrderId': vendorOrderId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/fulfilment_readiness/http.go#estimate
  Future<VendorResponse> postCommerceOrdersVendorOrderIdReadinessEstimate({
    required String vendorOrderId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCommerceOrdersVendorOrderIdReadinessEstimateRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/commerce/orders/{vendorOrderId}/readiness/estimate',
    pathParameters: {'vendorOrderId': vendorOrderId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/fulfilment_readiness/http.go
  Future<VendorResponse> getCommerceOrdersVendorOrderIdReadinessHistory({
    required String vendorOrderId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/commerce/orders/{vendorOrderId}/readiness/history',
    pathParameters: {'vendorOrderId': vendorOrderId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/fulfilment_readiness/http.go#ready
  Future<VendorResponse> postCommerceOrdersVendorOrderIdReadinessReady({
    required String vendorOrderId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCommerceOrdersVendorOrderIdReadinessReadyRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/commerce/orders/{vendorOrderId}/readiness/ready',
    pathParameters: {'vendorOrderId': vendorOrderId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/fulfilment_readiness/http.go#readySoon
  Future<VendorResponse> postCommerceOrdersVendorOrderIdReadinessReadySoon({
    required String vendorOrderId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCommerceOrdersVendorOrderIdReadinessReadySoonRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/commerce/orders/{vendorOrderId}/readiness/ready-soon',
    pathParameters: {'vendorOrderId': vendorOrderId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/fulfilment_readiness/http.go#start
  Future<VendorResponse> postCommerceOrdersVendorOrderIdReadinessStart({
    required String vendorOrderId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCommerceOrdersVendorOrderIdReadinessStartRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/commerce/orders/{vendorOrderId}/readiness/start',
    pathParameters: {'vendorOrderId': vendorOrderId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );
}
