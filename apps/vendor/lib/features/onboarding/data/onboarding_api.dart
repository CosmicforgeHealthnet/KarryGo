// GENERATED. Methods correspond to exact public Vendor contracts.
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_api_client.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/network/vendor_response.dart';
import '../models/onboarding_models.dart';
import 'onboarding_routes.dart';

class VendorOnboardingApi extends VendorFeatureApi {
  VendorOnboardingApi(VendorApiClient client)
    : super(client, onboardingOperations);

  /// Source: section7-onboarding-verification.openapi.json
  Future<VendorResponse> getOnboarding({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/onboarding',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section7-onboarding-verification.openapi.json
  Future<VendorResponse> getOnboardingActivationReadiness({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/onboarding/activation-readiness',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section7-onboarding-verification.openapi.json
  Future<VendorResponse> getOnboardingBankProfile({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/onboarding/bank-profile',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/vendors/onboarding_http.go#PutBankProfile
  Future<VendorResponse> putOnboardingBankProfile({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutOnboardingBankProfileRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/onboarding/bank-profile',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section7-onboarding-verification.openapi.json
  Future<VendorResponse> getOnboardingBlockers({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/onboarding/blockers',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section7-onboarding-verification.openapi.json
  Future<VendorResponse> getOnboardingChecklist({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/onboarding/checklist',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/vendors/onboarding_http.go#CreateDocumentUpload
  Future<VendorResponse> postOnboardingDocumentUploads({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostOnboardingDocumentUploadsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/onboarding/document-uploads',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section7-onboarding-verification.openapi.json
  Future<VendorResponse> getOnboardingDocumentUploadsMediaRef({
    required String mediaRef,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/onboarding/document-uploads/{mediaRef}',
    pathParameters: {'mediaRef': mediaRef},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section7-onboarding-verification.openapi.json
  Future<VendorResponse> getOnboardingDocuments({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/onboarding/documents',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/vendors/onboarding_http.go#CreateDocument
  Future<VendorResponse> postOnboardingDocuments({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostOnboardingDocumentsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/onboarding/documents',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/vendors/onboarding_http.go#DeleteDocument
  Future<VendorResponse> deleteOnboardingDocumentsDocumentId({
    required String documentId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required DeleteOnboardingDocumentsDocumentIdRequest body,
  }) => request(
    'DELETE',
    '/api/v1/vendor/onboarding/documents/{documentId}',
    pathParameters: {'documentId': documentId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/vendors/onboarding_http.go#UpdateDocument
  Future<VendorResponse> patchOnboardingDocumentsDocumentId({
    required String documentId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchOnboardingDocumentsDocumentIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/onboarding/documents/{documentId}',
    pathParameters: {'documentId': documentId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section7-onboarding-verification.openapi.json
  Future<VendorResponse> getOnboardingProgress({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/onboarding/progress',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section7-onboarding-verification.openapi.json
  Future<VendorResponse> getOnboardingRepresentative({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/onboarding/representative',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/vendors/onboarding_http.go#PutRepresentative
  Future<VendorResponse> putOnboardingRepresentative({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutOnboardingRepresentativeRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/onboarding/representative',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section7-onboarding-verification.openapi.json
  Future<VendorResponse> postOnboardingRequestActivation({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    VendorPayload? body,
  }) => request(
    'POST',
    '/api/v1/vendor/onboarding/request-activation',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section7-onboarding-verification.openapi.json
  Future<VendorResponse> postOnboardingSubmit({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    VendorPayload? body,
  }) => request(
    'POST',
    '/api/v1/vendor/onboarding/submit',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section7-onboarding-verification.openapi.json
  Future<VendorResponse> getOnboardingVerifications({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/onboarding/verifications',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section7-onboarding-verification.openapi.json
  Future<VendorResponse> getOnboardingVerificationsDimension({
    required String dimension,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/onboarding/verifications/{dimension}',
    pathParameters: {'dimension': dimension},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/vendors/onboarding_http.go#SubmitVerification
  Future<VendorResponse> postOnboardingVerificationsDimensionResubmit({
    required String dimension,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostOnboardingVerificationsDimensionResubmitRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/onboarding/verifications/{dimension}/resubmit',
    pathParameters: {'dimension': dimension},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/vendors/onboarding_http.go#SubmitVerification
  Future<VendorResponse> postOnboardingVerificationsDimensionSubmit({
    required String dimension,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostOnboardingVerificationsDimensionSubmitRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/onboarding/verifications/{dimension}/submit',
    pathParameters: {'dimension': dimension},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );
}
