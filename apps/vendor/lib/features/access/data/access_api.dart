// GENERATED. Methods correspond to exact public Vendor contracts.
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_api_client.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/network/vendor_response.dart';
import '../models/access_models.dart';
import 'access_routes.dart';

class VendorAccessApi extends VendorFeatureApi {
  VendorAccessApi(VendorApiClient client) : super(client, accessOperations);
  /// Source: section5-auth-staff-iam.openapi.json
  Future<VendorResponse> deleteAssignmentsId({required String id, Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}, VendorPayload? body}) => request('DELETE', '/api/v1/vendor/assignments/{id}', pathParameters: {'id': id}, query:query, command:command, contractHeaders:contractHeaders, body:body);
  /// Source: section5-auth-staff-iam.openapi.json
  Future<VendorResponse> getPermissions({Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}}) => request('GET', '/api/v1/vendor/permissions', pathParameters: {}, query:query, command:command, contractHeaders:contractHeaders);
  /// Source: section5-auth-staff-iam.openapi.json
  Future<VendorResponse> getRoles({Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}}) => request('GET', '/api/v1/vendor/roles', pathParameters: {}, query:query, command:command, contractHeaders:contractHeaders);
  /// Source: services/vendor-service/internal/features/staff/http/roles_handler.go#CreateRole
  Future<VendorResponse> postRoles({Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}, required PostRolesRequest body}) => request('POST', '/api/v1/vendor/roles', pathParameters: {}, query:query, command:command, contractHeaders:contractHeaders, body:body);
  /// Source: section5-auth-staff-iam.openapi.json
  Future<VendorResponse> getRolesId({required String id, Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}}) => request('GET', '/api/v1/vendor/roles/{id}', pathParameters: {'id': id}, query:query, command:command, contractHeaders:contractHeaders);
  /// Source: services/vendor-service/internal/features/staff/http/roles_handler.go#AssignRole
  Future<VendorResponse> postRolesIdAssign({required String id, Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}, required PostRolesIdAssignRequest body}) => request('POST', '/api/v1/vendor/roles/{id}/assign', pathParameters: {'id': id}, query:query, command:command, contractHeaders:contractHeaders, body:body);
  /// Source: services/vendor-service/internal/features/staff/http/roles_handler.go#SetRolePermissions
  Future<VendorResponse> putRolesIdPermissions({required String id, Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}, required PutRolesIdPermissionsRequest body}) => request('PUT', '/api/v1/vendor/roles/{id}/permissions', pathParameters: {'id': id}, query:query, command:command, contractHeaders:contractHeaders, body:body);
  /// Source: section5-auth-staff-iam.openapi.json
  Future<VendorResponse> getStaff({Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}}) => request('GET', '/api/v1/vendor/staff', pathParameters: {}, query:query, command:command, contractHeaders:contractHeaders);
  /// Source: services/vendor-service/internal/features/staff/http/handler.go#InviteStaff
  Future<VendorResponse> postStaff({Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}, required PostStaffRequest body}) => request('POST', '/api/v1/vendor/staff', pathParameters: {}, query:query, command:command, contractHeaders:contractHeaders, body:body);
  /// Source: vendor-current-access.openapi.json
  Future<VendorResponse> getStaffMeAccess({Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}}) => request('GET', '/api/v1/vendor/staff/me/access', pathParameters: {}, query:query, command:command, contractHeaders:contractHeaders);
  /// Source: section5-auth-staff-iam.openapi.json
  Future<VendorResponse> getStaffId({required String id, Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}}) => request('GET', '/api/v1/vendor/staff/{id}', pathParameters: {'id': id}, query:query, command:command, contractHeaders:contractHeaders);
  /// Source: services/vendor-service/internal/features/staff/http/handler.go#ReactivateStaff
  Future<VendorResponse> putStaffIdReactivate({required String id, Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}, required PutStaffIdReactivateRequest body}) => request('PUT', '/api/v1/vendor/staff/{id}/reactivate', pathParameters: {'id': id}, query:query, command:command, contractHeaders:contractHeaders, body:body);
  /// Source: section5-auth-staff-iam.openapi.json
  Future<VendorResponse> getStaffIdRoles({required String id, Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}}) => request('GET', '/api/v1/vendor/staff/{id}/roles', pathParameters: {'id': id}, query:query, command:command, contractHeaders:contractHeaders);
  /// Source: services/vendor-service/internal/features/staff/http/handler.go#SuspendStaff
  Future<VendorResponse> putStaffIdSuspend({required String id, Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}, required PutStaffIdSuspendRequest body}) => request('PUT', '/api/v1/vendor/staff/{id}/suspend', pathParameters: {'id': id}, query:query, command:command, contractHeaders:contractHeaders, body:body);
}
