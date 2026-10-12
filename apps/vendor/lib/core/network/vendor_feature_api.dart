import '../contracts/vendor_contract.dart';
import 'vendor_api_client.dart';
import 'vendor_command.dart';
import 'vendor_response.dart';

class VendorFeatureApi {
  VendorFeatureApi(this.client, this.operations);
  final VendorApiClient client;
  final List<VendorOperation> operations;
  VendorOperation operation(String method, String path) =>
      operations.firstWhere(
        (o) => o.method == method && o.path == path,
        orElse: () => throw const VendorContractException(
          'Operation is outside this feature contract',
        ),
      );
  Future<VendorResponse> request(
    String method,
    String path, {
    Map<String, String> pathParameters = const {},
    Map<String, String> query = const {},
    VendorPayload? body,
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    bool retryAuthentication = true,
  }) => client.call(
    operation(method, path),
    pathParameters: pathParameters,
    query: query,
    body: body,
    command: command,
    contractHeaders: contractHeaders,
    retryAuthentication: retryAuthentication,
  );
}
