// GENERATED from actual request schemas/Go JSON fields.
import '../../../core/contracts/vendor_contract.dart';

class PostFinanceWithdrawalsRequest extends VendorPayload {
  PostFinanceWithdrawalsRequest({
    required String bankAccountId,
    required int amountKobo,
    String? currency,
    required String stepUpGrantId,
  }) : super({
         'bank_account_id': bankAccountId,
         'amount_kobo': amountKobo,
         'currency': ?currency,
         'step_up_grant_id': stepUpGrantId,
       });
  PostFinanceWithdrawalsRequest.fromJson(super.json);
  String? get bankAccountId => value('bank_account_id') as String?;
  int? get amountKobo => value('amount_kobo') as int?;
  String? get currency => value('currency') as String?;
  String? get stepUpGrantId => value('step_up_grant_id') as String?;
}
