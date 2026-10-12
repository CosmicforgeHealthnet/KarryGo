// GENERATED. Methods correspond to exact public Vendor contracts.
import '../../../core/network/vendor_api_client.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/network/vendor_response.dart';
import '../models/handover_models.dart';
import 'handover_routes.dart';

class VendorHandoverApi extends VendorFeatureApi {
  VendorHandoverApi(VendorApiClient client) : super(client, handoverOperations);

  /// Source: section32-handover.openapi.json
  Future<VendorResponse>
  postCommerceOrdersVendorOrderIdMissionsMissionIdHandoverPickupConfirm({
    required String vendorOrderId,
    required String missionId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCommerceOrdersVendorOrderIdMissionsMissionIdHandoverPickupConfirmRequest
    body,
  }) => request(
    'POST',
    '/api/v1/vendor/commerce/orders/{vendorOrderId}/missions/{missionId}/handover/pickup/confirm',
    pathParameters: {'vendorOrderId': vendorOrderId, 'missionId': missionId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section32-handover.openapi.json
  Future<VendorResponse>
  getCommerceOrdersVendorOrderIdMissionsMissionIdHandoverStage({
    required String vendorOrderId,
    required String missionId,
    required String stage,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/commerce/orders/{vendorOrderId}/missions/{missionId}/handover/{stage}',
    pathParameters: {
      'vendorOrderId': vendorOrderId,
      'missionId': missionId,
      'stage': stage,
    },
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );
}
