// GENERATED. Methods correspond to exact public Vendor contracts.
import '../../../core/network/vendor_api_client.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/network/vendor_response.dart';
import '../models/business_models.dart';
import 'business_routes.dart';

class VendorBusinessApi extends VendorFeatureApi {
  VendorBusinessApi(VendorApiClient client) : super(client, businessOperations);

  /// Source: section6-vendor-business.openapi.json
  Future<VendorResponse> getBusinesses({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/businesses',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/vendors/http.go#CreateBusiness
  Future<VendorResponse> postBusinesses({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostBusinessesRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/businesses',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section6-vendor-business.openapi.json
  Future<VendorResponse> getBusinessesBusinessId({
    required String businessId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/businesses/{businessId}',
    pathParameters: {'businessId': businessId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/vendors/http.go#UpdateBusiness
  Future<VendorResponse> patchBusinessesBusinessId({
    required String businessId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchBusinessesBusinessIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/businesses/{businessId}',
    pathParameters: {'businessId': businessId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section6-vendor-business.openapi.json
  Future<VendorResponse> getBusinessesBusinessIdBranches({
    required String businessId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/businesses/{businessId}/branches',
    pathParameters: {'businessId': businessId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/vendors/http.go#CreateBranch
  Future<VendorResponse> postBusinessesBusinessIdBranches({
    required String businessId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostBusinessesBusinessIdBranchesRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/businesses/{businessId}/branches',
    pathParameters: {'businessId': businessId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section6-vendor-business.openapi.json
  Future<VendorResponse> getBusinessesBusinessIdBranchesBranchId({
    required String businessId,
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/businesses/{businessId}/branches/{branchId}',
    pathParameters: {'businessId': businessId, 'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/vendors/http.go#UpdateBranch
  Future<VendorResponse> patchBusinessesBusinessIdBranchesBranchId({
    required String businessId,
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchBusinessesBusinessIdBranchesBranchIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/businesses/{businessId}/branches/{branchId}',
    pathParameters: {'businessId': businessId, 'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section6-vendor-business.openapi.json
  Future<VendorResponse> getBusinessesBusinessIdBranchesBranchIdCapabilities({
    required String businessId,
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/businesses/{businessId}/branches/{branchId}/capabilities',
    pathParameters: {'businessId': businessId, 'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/vendors/http.go#PutBranchCapability
  Future<VendorResponse>
  putBusinessesBusinessIdBranchesBranchIdCapabilitiesCapabilityId({
    required String businessId,
    required String branchId,
    required String capabilityId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutBusinessesBusinessIdBranchesBranchIdCapabilitiesCapabilityIdRequest
    body,
  }) => request(
    'PUT',
    '/api/v1/vendor/businesses/{businessId}/branches/{branchId}/capabilities/{capabilityId}',
    pathParameters: {
      'businessId': businessId,
      'branchId': branchId,
      'capabilityId': capabilityId,
    },
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section6-vendor-business.openapi.json
  Future<VendorResponse>
  postBusinessesBusinessIdBranchesBranchIdCapabilitiesCapabilityIdPause({
    required String businessId,
    required String branchId,
    required String capabilityId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostBusinessesBusinessIdBranchesBranchIdCapabilitiesCapabilityIdPauseRequest
    body,
  }) => request(
    'POST',
    '/api/v1/vendor/businesses/{businessId}/branches/{branchId}/capabilities/{capabilityId}/pause',
    pathParameters: {
      'businessId': businessId,
      'branchId': branchId,
      'capabilityId': capabilityId,
    },
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section6-vendor-business.openapi.json
  Future<VendorResponse>
  postBusinessesBusinessIdBranchesBranchIdCapabilitiesCapabilityIdResume({
    required String businessId,
    required String branchId,
    required String capabilityId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostBusinessesBusinessIdBranchesBranchIdCapabilitiesCapabilityIdResumeRequest
    body,
  }) => request(
    'POST',
    '/api/v1/vendor/businesses/{businessId}/branches/{branchId}/capabilities/{capabilityId}/resume',
    pathParameters: {
      'businessId': businessId,
      'branchId': branchId,
      'capabilityId': capabilityId,
    },
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section6-vendor-business.openapi.json
  Future<VendorResponse> getBusinessesBusinessIdBranchesBranchIdHours({
    required String businessId,
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/businesses/{businessId}/branches/{branchId}/hours',
    pathParameters: {'businessId': businessId, 'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/vendors/http.go#PutHours
  Future<VendorResponse> putBusinessesBusinessIdBranchesBranchIdHours({
    required String businessId,
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutBusinessesBusinessIdBranchesBranchIdHoursRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/businesses/{businessId}/branches/{branchId}/hours',
    pathParameters: {'businessId': businessId, 'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section6-vendor-business.openapi.json
  Future<VendorResponse>
  getBusinessesBusinessIdBranchesBranchIdOperationsSettings({
    required String businessId,
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/businesses/{businessId}/branches/{branchId}/operations-settings',
    pathParameters: {'businessId': businessId, 'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/vendors/http.go#PutSettings
  Future<VendorResponse>
  putBusinessesBusinessIdBranchesBranchIdOperationsSettings({
    required String businessId,
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutBusinessesBusinessIdBranchesBranchIdOperationsSettingsRequest
    body,
  }) => request(
    'PUT',
    '/api/v1/vendor/businesses/{businessId}/branches/{branchId}/operations-settings',
    pathParameters: {'businessId': businessId, 'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section6-vendor-business.openapi.json
  Future<VendorResponse> postBusinessesBusinessIdBranchesBranchIdPause({
    required String businessId,
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostBusinessesBusinessIdBranchesBranchIdPauseRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/businesses/{businessId}/branches/{branchId}/pause',
    pathParameters: {'businessId': businessId, 'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section6-vendor-business.openapi.json
  Future<VendorResponse> postBusinessesBusinessIdBranchesBranchIdResume({
    required String businessId,
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostBusinessesBusinessIdBranchesBranchIdResumeRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/businesses/{businessId}/branches/{branchId}/resume',
    pathParameters: {'businessId': businessId, 'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section6-vendor-business.openapi.json
  Future<VendorResponse> getBusinessesBusinessIdBranchesBranchIdServiceArea({
    required String businessId,
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/businesses/{businessId}/branches/{branchId}/service-area',
    pathParameters: {'businessId': businessId, 'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/vendors/http.go#PutServiceArea
  Future<VendorResponse> putBusinessesBusinessIdBranchesBranchIdServiceArea({
    required String businessId,
    required String branchId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutBusinessesBusinessIdBranchesBranchIdServiceAreaRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/businesses/{businessId}/branches/{branchId}/service-area',
    pathParameters: {'businessId': businessId, 'branchId': branchId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section6-vendor-business.openapi.json
  Future<VendorResponse> getBusinessesBusinessIdBrands({
    required String businessId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/businesses/{businessId}/brands',
    pathParameters: {'businessId': businessId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/vendors/http.go#CreateBrand
  Future<VendorResponse> postBusinessesBusinessIdBrands({
    required String businessId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostBusinessesBusinessIdBrandsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/businesses/{businessId}/brands',
    pathParameters: {'businessId': businessId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section6-vendor-business.openapi.json
  Future<VendorResponse> getBusinessesBusinessIdBrandsBrandId({
    required String businessId,
    required String brandId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/businesses/{businessId}/brands/{brandId}',
    pathParameters: {'businessId': businessId, 'brandId': brandId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/vendors/http.go#UpdateBrand
  Future<VendorResponse> patchBusinessesBusinessIdBrandsBrandId({
    required String businessId,
    required String brandId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchBusinessesBusinessIdBrandsBrandIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/businesses/{businessId}/brands/{brandId}',
    pathParameters: {'businessId': businessId, 'brandId': brandId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section6-vendor-business.openapi.json
  Future<VendorResponse> getBusinessesBusinessIdCapabilities({
    required String businessId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/businesses/{businessId}/capabilities',
    pathParameters: {'businessId': businessId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/vendors/http.go#CreateCapability
  Future<VendorResponse> postBusinessesBusinessIdCapabilities({
    required String businessId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostBusinessesBusinessIdCapabilitiesRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/businesses/{businessId}/capabilities',
    pathParameters: {'businessId': businessId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section6-vendor-business.openapi.json
  Future<VendorResponse> getBusinessesBusinessIdCapabilitiesCapabilityId({
    required String businessId,
    required String capabilityId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/businesses/{businessId}/capabilities/{capabilityId}',
    pathParameters: {'businessId': businessId, 'capabilityId': capabilityId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section6-vendor-business.openapi.json
  Future<VendorResponse> postBusinessesBusinessIdCapabilitiesCapabilityIdPause({
    required String businessId,
    required String capabilityId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostBusinessesBusinessIdCapabilitiesCapabilityIdPauseRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/businesses/{businessId}/capabilities/{capabilityId}/pause',
    pathParameters: {'businessId': businessId, 'capabilityId': capabilityId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section6-vendor-business.openapi.json
  Future<VendorResponse>
  postBusinessesBusinessIdCapabilitiesCapabilityIdResume({
    required String businessId,
    required String capabilityId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostBusinessesBusinessIdCapabilitiesCapabilityIdResumeRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/businesses/{businessId}/capabilities/{capabilityId}/resume',
    pathParameters: {'businessId': businessId, 'capabilityId': capabilityId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section6-vendor-business.openapi.json
  Future<VendorResponse> postBusinessesBusinessIdPause({
    required String businessId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostBusinessesBusinessIdPauseRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/businesses/{businessId}/pause',
    pathParameters: {'businessId': businessId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section6-vendor-business.openapi.json
  Future<VendorResponse> postBusinessesBusinessIdResume({
    required String businessId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostBusinessesBusinessIdResumeRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/businesses/{businessId}/resume',
    pathParameters: {'businessId': businessId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section6-vendor-business.openapi.json
  Future<VendorResponse> getCapabilityTypes({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/capability-types',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );
}
