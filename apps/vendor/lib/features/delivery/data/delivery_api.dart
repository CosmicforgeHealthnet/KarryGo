// GENERATED. Methods correspond to exact public Vendor contracts.
import '../../../core/network/vendor_api_client.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/network/vendor_response.dart';
import '../models/delivery_models.dart';
import 'delivery_routes.dart';

class VendorDeliveryApi extends VendorFeatureApi {
  VendorDeliveryApi(VendorApiClient client) : super(client, deliveryOperations);
  /// Source: section17c-vendor-delivery-policy.openapi.json
  Future<VendorResponse> getDeliveryBranchesBranchIdPolicy({required String branchId, Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}}) => request('GET', '/api/v1/vendor/delivery/branches/{branchId}/policy', pathParameters: {'branchId': branchId}, query:query, command:command, contractHeaders:contractHeaders);
  /// Source: services/vendor-service/internal/features/delivery/http.go#upsertPolicy
  Future<VendorResponse> putDeliveryBranchesBranchIdPolicy({required String branchId, Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}, required PutDeliveryBranchesBranchIdPolicyRequest body}) => request('PUT', '/api/v1/vendor/delivery/branches/{branchId}/policy', pathParameters: {'branchId': branchId}, query:query, command:command, contractHeaders:contractHeaders, body:body);
  /// Source: vendor-owned-hybrid-delivery-runtime.openapi.json
  Future<VendorResponse> getDeliveryRiderAvailability({Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}}) => request('GET', '/api/v1/vendor/delivery/rider/availability', pathParameters: {}, query:query, command:command, contractHeaders:contractHeaders);
  /// Source: services/vendor-service/internal/features/fulfilmentgroup/http.go#setRiderAvailability
  Future<VendorResponse> putDeliveryRiderAvailability({Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}, required PutDeliveryRiderAvailabilityRequest body}) => request('PUT', '/api/v1/vendor/delivery/rider/availability', pathParameters: {}, query:query, command:command, contractHeaders:contractHeaders, body:body);
  /// Source: vendor-rider-branches.openapi.json
  Future<VendorResponse> getDeliveryRiderBranches({Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}}) => request('GET', '/api/v1/vendor/delivery/rider/branches', pathParameters: {}, query:query, command:command, contractHeaders:contractHeaders);
  /// Source: vendor-owned-hybrid-delivery-runtime.openapi.json
  Future<VendorResponse> getDeliveryRiderMissions({Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}}) => request('GET', '/api/v1/vendor/delivery/rider/missions', pathParameters: {}, query:query, command:command, contractHeaders:contractHeaders);
  /// Source: vendor-owned-hybrid-delivery-runtime.openapi.json
  Future<VendorResponse> getDeliveryRiderMissionsCurrent({Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}}) => request('GET', '/api/v1/vendor/delivery/rider/missions/current', pathParameters: {}, query:query, command:command, contractHeaders:contractHeaders);
  /// Source: vendor-owned-hybrid-delivery-runtime.openapi.json
  Future<VendorResponse> getDeliveryRiderMissionsMissionId({required String missionId, Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}}) => request('GET', '/api/v1/vendor/delivery/rider/missions/{missionId}', pathParameters: {'missionId': missionId}, query:query, command:command, contractHeaders:contractHeaders);
  /// Source: services/vendor-service/internal/features/fulfilmentgroup/http.go#executeRiderAction
  Future<VendorResponse> postDeliveryRiderMissionsMissionIdActionsAction({required String missionId, required String action, Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}, required PostDeliveryRiderMissionsMissionIdActionsActionRequest body}) => request('POST', '/api/v1/vendor/delivery/rider/missions/{missionId}/actions/{action}', pathParameters: {'missionId': missionId, 'action': action}, query:query, command:command, contractHeaders:contractHeaders, body:body);
  /// Source: fulfilment-group-parcel-mission.openapi.json
  Future<VendorResponse> getFulfilmentGroupsGroupId({required String groupId, Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}}) => request('GET', '/api/v1/vendor/fulfilment/groups/{groupId}', pathParameters: {'groupId': groupId}, query:query, command:command, contractHeaders:contractHeaders);
  /// Source: fulfilment-group-parcel-mission.openapi.json
  Future<VendorResponse> getFulfilmentGroupsGroupIdPlans({required String groupId, Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}}) => request('GET', '/api/v1/vendor/fulfilment/groups/{groupId}/plans', pathParameters: {'groupId': groupId}, query:query, command:command, contractHeaders:contractHeaders);
  /// Source: fulfilment-group-parcel-mission.openapi.json
  Future<VendorResponse> getFulfilmentParcelsParcelId({required String parcelId, Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}}) => request('GET', '/api/v1/vendor/fulfilment/parcels/{parcelId}', pathParameters: {'parcelId': parcelId}, query:query, command:command, contractHeaders:contractHeaders);
  /// Source: fulfilment-group-parcel-mission.openapi.json
  Future<VendorResponse> getFulfilmentParcelsParcelIdCustody({required String parcelId, Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}}) => request('GET', '/api/v1/vendor/fulfilment/parcels/{parcelId}/custody', pathParameters: {'parcelId': parcelId}, query:query, command:command, contractHeaders:contractHeaders);
  /// Source: services/vendor-service/internal/features/fulfilmentgroup/http.go#amendManifest
  Future<VendorResponse> putFulfilmentParcelsParcelIdManifest({required String parcelId, Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}, required PutFulfilmentParcelsParcelIdManifestRequest body}) => request('PUT', '/api/v1/vendor/fulfilment/parcels/{parcelId}/manifest', pathParameters: {'parcelId': parcelId}, query:query, command:command, contractHeaders:contractHeaders, body:body);
  /// Source: services/vendor-service/internal/features/fulfilmentgroup/http.go#initiateReturn
  Future<VendorResponse> postFulfilmentParcelsParcelIdReturns({required String parcelId, Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}, required PostFulfilmentParcelsParcelIdReturnsRequest body}) => request('POST', '/api/v1/vendor/fulfilment/parcels/{parcelId}/returns', pathParameters: {'parcelId': parcelId}, query:query, command:command, contractHeaders:contractHeaders, body:body);
  /// Source: fulfilment-group-parcel-mission.openapi.json
  Future<VendorResponse> getFulfilmentUnitsUnitIdParcels({required String unitId, Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}}) => request('GET', '/api/v1/vendor/fulfilment/units/{unitId}/parcels', pathParameters: {'unitId': unitId}, query:query, command:command, contractHeaders:contractHeaders);
  /// Source: services/vendor-service/internal/features/fulfilmentgroup/http.go#packParcel
  Future<VendorResponse> postFulfilmentUnitsUnitIdParcels({required String unitId, Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}, required PostFulfilmentUnitsUnitIdParcelsRequest body}) => request('POST', '/api/v1/vendor/fulfilment/units/{unitId}/parcels', pathParameters: {'unitId': unitId}, query:query, command:command, contractHeaders:contractHeaders, body:body);
  /// Source: services/vendor-service/internal/features/fulfilmentgroup/http.go#setUnitReadiness
  Future<VendorResponse> putFulfilmentUnitsUnitIdReadiness({required String unitId, Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}, required PutFulfilmentUnitsUnitIdReadinessRequest body}) => request('PUT', '/api/v1/vendor/fulfilment/units/{unitId}/readiness', pathParameters: {'unitId': unitId}, query:query, command:command, contractHeaders:contractHeaders, body:body);
  /// Source: fulfilment-group-parcel-mission.openapi.json
  Future<VendorResponse> getFulfilmentVendorMissions({Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}}) => request('GET', '/api/v1/vendor/fulfilment/vendor-missions', pathParameters: {}, query:query, command:command, contractHeaders:contractHeaders);
  /// Source: vendor-owned-hybrid-delivery-runtime.openapi.json
  Future<VendorResponse> getFulfilmentVendorMissionsFallbacksOperationRef({required String operationRef, Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}}) => request('GET', '/api/v1/vendor/fulfilment/vendor-missions/fallbacks/{operationRef}', pathParameters: {'operationRef': operationRef}, query:query, command:command, contractHeaders:contractHeaders);
  /// Source: fulfilment-group-parcel-mission.openapi.json
  Future<VendorResponse> getFulfilmentVendorMissionsMissionId({required String missionId, Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}}) => request('GET', '/api/v1/vendor/fulfilment/vendor-missions/{missionId}', pathParameters: {'missionId': missionId}, query:query, command:command, contractHeaders:contractHeaders);
  /// Source: services/vendor-service/internal/features/fulfilmentgroup/http.go#triggerHybridFallback
  Future<VendorResponse> postFulfilmentVendorMissionsMissionIdFallbacks({required String missionId, Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}, required PostFulfilmentVendorMissionsMissionIdFallbacksRequest body}) => request('POST', '/api/v1/vendor/fulfilment/vendor-missions/{missionId}/fallbacks', pathParameters: {'missionId': missionId}, query:query, command:command, contractHeaders:contractHeaders, body:body);
  /// Source: services/vendor-service/internal/features/fulfilmentgroup/http.go#assignRider
  Future<VendorResponse> putFulfilmentVendorMissionsMissionIdRider({required String missionId, Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}, required PutFulfilmentVendorMissionsMissionIdRiderRequest body}) => request('PUT', '/api/v1/vendor/fulfilment/vendor-missions/{missionId}/rider', pathParameters: {'missionId': missionId}, query:query, command:command, contractHeaders:contractHeaders, body:body);
  /// Source: services/vendor-service/internal/features/fulfilmentgroup/http.go#advanceVendorMission
  Future<VendorResponse> putFulfilmentVendorMissionsMissionIdStatus({required String missionId, Map<String,String> query = const {}, VendorCommand? command, Map<String,String> contractHeaders = const {}, required PutFulfilmentVendorMissionsMissionIdStatusRequest body}) => request('PUT', '/api/v1/vendor/fulfilment/vendor-missions/{missionId}/status', pathParameters: {'missionId': missionId}, query:query, command:command, contractHeaders:contractHeaders, body:body);
}
