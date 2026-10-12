import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/network/vendor_response.dart';

class VendorPickupWorkflow {
  VendorPickupWorkflow(this.api);
  final VendorFeatureApi api;
  Future<VendorResponse> read({
    required String vendorOrderId,
    required String missionId,
    required String stage,
  }) => api.request(
    'GET',
    '/api/v1/vendor/commerce/orders/{vendorOrderId}/missions/{missionId}/handover/{stage}',
    pathParameters: {
      'vendorOrderId': vendorOrderId,
      'missionId': missionId,
      'stage': stage,
    },
  );
  Future<VendorResponse> confirm({
    required String vendorOrderId,
    required String missionId,
    required String handoverRef,
    required String method,
    required String credential,
    required VendorCommand command,
  }) => api.request(
    'POST',
    '/api/v1/vendor/commerce/orders/{vendorOrderId}/missions/{missionId}/handover/pickup/confirm',
    pathParameters: {'vendorOrderId': vendorOrderId, 'missionId': missionId},
    body: VendorPayload({
      'handover_ref': handoverRef,
      'method': method,
      'credential': credential,
    }),
    command: command,
  );
  // Call read after an unknown outcome. Credentials are never retained here.
}
