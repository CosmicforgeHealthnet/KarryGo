import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/network/vendor_response.dart';

class VendorWithdrawalWorkflow {
  VendorWithdrawalWorkflow(this.api);
  final VendorFeatureApi api;
  Future<VendorResponse> request({
    required String bankAccountId,
    required int amountKobo,
    required String stepUpGrantId,
    required VendorCommand command,
    String? currency,
  }) => api.request(
    'POST',
    '/api/v1/vendor/finance/withdrawals',
    body: VendorPayload({
      'bank_account_id': bankAccountId,
      'amount_kobo': amountKobo,
      'step_up_grant_id': stepUpGrantId,
      'currency': ?currency,
    }),
    command: command,
    retryAuthentication: false,
  );
  Future<VendorResponse> read(String reference) => api.request(
    'GET',
    '/api/v1/vendor/finance/withdrawals/{reference}',
    pathParameters: {'reference': reference},
  );
}
