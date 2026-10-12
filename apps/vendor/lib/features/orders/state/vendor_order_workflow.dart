import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/network/vendor_response.dart';

/// Sends actual server commands; it does not implement a local state machine.
class VendorOrderWorkflow {
  VendorOrderWorkflow(this.api);
  final VendorFeatureApi api;
  Future<VendorResponse> read(String vendorOrderId) => api.request(
    'GET',
    '/api/v1/vendor/commerce/orders/{vendorOrderId}',
    pathParameters: {'vendorOrderId': vendorOrderId},
  );
  Future<VendorResponse> execute({
    required String action,
    required String vendorOrderId,
    required VendorCommand command,
    String? reasonCode,
  }) {
    const actions = {
      'accept',
      'ready',
      'handover',
      'deliver',
      'complete',
      'cancel',
      'reject',
      'preparing',
      'picking',
      'pack',
      'ready-for-pickup',
      'pickup',
      'delivering',
      'delivered',
    };
    if (!actions.contains(action)) {
      throw const VendorContractException('Unknown backend order action');
    }
    return api.request(
      'POST',
      '/api/v1/vendor/commerce/orders/{vendorOrderId}/$action',
      pathParameters: {'vendorOrderId': vendorOrderId},
      command: command,
      body: VendorPayload({'reason_code': ?reasonCode}),
    );
  }
}
