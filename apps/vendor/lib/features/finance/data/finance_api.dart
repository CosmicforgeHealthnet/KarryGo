// GENERATED. Methods correspond to exact public Vendor contracts.
import '../../../core/network/vendor_api_client.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/network/vendor_response.dart';
import '../models/finance_models.dart';
import 'finance_routes.dart';

class VendorFinanceApi extends VendorFeatureApi {
  VendorFinanceApi(VendorApiClient client) : super(client, financeOperations);

  /// Source: vendor-finance.openapi.json
  Future<VendorResponse> getFinanceBankAccounts({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/finance/bank-accounts',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: vendor-finance.openapi.json
  Future<VendorResponse> getFinanceOverview({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/finance/overview',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: vendor-finance.openapi.json
  Future<VendorResponse> getFinanceTransactions({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/finance/transactions',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: vendor-finance.openapi.json
  Future<VendorResponse> getFinanceWithdrawals({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/finance/withdrawals',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/finance_projection/http.go#RequestWithdrawal
  Future<VendorResponse> postFinanceWithdrawals({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostFinanceWithdrawalsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/finance/withdrawals',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: vendor-finance.openapi.json
  Future<VendorResponse> getFinanceWithdrawalsReference({
    required String reference,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/finance/withdrawals/{reference}',
    pathParameters: {'reference': reference},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );
}
