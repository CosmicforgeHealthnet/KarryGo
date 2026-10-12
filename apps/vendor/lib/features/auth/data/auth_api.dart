// GENERATED. Methods correspond to exact public Vendor contracts.
import '../../../core/network/vendor_api_client.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/network/vendor_response.dart';
import '../models/auth_models.dart';
import 'auth_routes.dart';

class VendorAuthApi extends VendorFeatureApi {
  VendorAuthApi(VendorApiClient client) : super(client, authOperations);

  /// Source: services/vendor-service/internal/features/auth/http/handler.go#Logout
  Future<VendorResponse> postAuthLogout({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostAuthLogoutRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/auth/logout',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/auth/http/handler.go#Refresh
  Future<VendorResponse> postAuthRefresh({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostAuthRefreshRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/auth/refresh',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/auth/http/handler.go#Resend
  Future<VendorResponse> postAuthResend({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostAuthResendRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/auth/resend',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/auth/http/handler.go#Start
  Future<VendorResponse> postAuthStart({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostAuthStartRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/auth/start',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/auth/http/handler.go#StepUpStart
  Future<VendorResponse> postAuthStepUpStart({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostAuthStepUpStartRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/auth/step-up/start',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/auth/http/handler.go#StepUpVerify
  Future<VendorResponse> postAuthStepUpVerify({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostAuthStepUpVerifyRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/auth/step-up/verify',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/auth/http/handler.go#Verify
  Future<VendorResponse> postAuthVerify({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostAuthVerifyRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/auth/verify',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );
}
