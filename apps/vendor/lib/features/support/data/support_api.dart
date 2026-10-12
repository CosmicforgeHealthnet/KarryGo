// GENERATED. Methods correspond to exact public Vendor contracts.
import '../../../core/network/vendor_api_client.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/network/vendor_response.dart';
import '../models/support_models.dart';
import 'support_routes.dart';

class VendorSupportApi extends VendorFeatureApi {
  VendorSupportApi(VendorApiClient client) : super(client, supportOperations);

  /// Source: commerce-support-cases.openapi.json
  Future<VendorResponse> getCommerceOrdersVendorOrderIdSupportCases({
    required String vendorOrderId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/commerce/orders/{vendorOrderId}/support-cases',
    pathParameters: {'vendorOrderId': vendorOrderId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/orders/support_cases_http.go#createVendorSupportCase
  Future<VendorResponse> postCommerceOrdersVendorOrderIdSupportCases({
    required String vendorOrderId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCommerceOrdersVendorOrderIdSupportCasesRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/commerce/orders/{vendorOrderId}/support-cases',
    pathParameters: {'vendorOrderId': vendorOrderId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );
}
