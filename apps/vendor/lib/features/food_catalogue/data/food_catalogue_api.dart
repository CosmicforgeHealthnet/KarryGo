// GENERATED. Methods correspond to exact public Vendor contracts.
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_api_client.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/network/vendor_response.dart';
import '../models/food_catalogue_models.dart';
import 'food_catalogue_routes.dart';

class VendorFoodCatalogueApi extends VendorFeatureApi {
  VendorFoodCatalogueApi(VendorApiClient client)
    : super(client, foodCatalogueOperations);

  /// Source: section9a-food-catalogue.openapi.json
  Future<VendorResponse> getFoodItemsItemId({
    required String itemId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/food/items/{itemId}',
    pathParameters: {'itemId': itemId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/food/http.go#UpdateItem
  Future<VendorResponse> patchFoodItemsItemId({
    required String itemId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchFoodItemsItemIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/food/items/{itemId}',
    pathParameters: {'itemId': itemId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/food/http.go#ArchiveItem
  Future<VendorResponse> postFoodItemsItemIdArchive({
    required String itemId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostFoodItemsItemIdArchiveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/food/items/{itemId}/archive',
    pathParameters: {'itemId': itemId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section9b-food-operations.openapi.json
  Future<VendorResponse> getFoodItemsItemIdAvailability({
    required String itemId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/food/items/{itemId}/availability',
    pathParameters: {'itemId': itemId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/food/operations_http.go#SetItemAvailability
  Future<VendorResponse> putFoodItemsItemIdAvailability({
    required String itemId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutFoodItemsItemIdAvailabilityRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/food/items/{itemId}/availability',
    pathParameters: {'itemId': itemId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/food/operations_http.go#ClearAvailabilitySource
  Future<VendorResponse> deleteFoodItemsItemIdAvailabilitySource({
    required String itemId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required DeleteFoodItemsItemIdAvailabilitySourceRequest body,
  }) => request(
    'DELETE',
    '/api/v1/vendor/food/items/{itemId}/availability-source',
    pathParameters: {'itemId': itemId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section9b-food-operations.openapi.json
  Future<VendorResponse> getFoodItemsItemIdAvailabilitySource({
    required String itemId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/food/items/{itemId}/availability-source',
    pathParameters: {'itemId': itemId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/food/operations_http.go#SetAvailabilitySource
  Future<VendorResponse> putFoodItemsItemIdAvailabilitySource({
    required String itemId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutFoodItemsItemIdAvailabilitySourceRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/food/items/{itemId}/availability-source',
    pathParameters: {'itemId': itemId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section9b-food-operations.openapi.json
  Future<VendorResponse> getFoodItemsItemIdAvailabilityEffective({
    required String itemId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/food/items/{itemId}/availability/effective',
    pathParameters: {'itemId': itemId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/food/operations_http.go#ClearBranchOverride
  Future<VendorResponse> deleteFoodItemsItemIdBranchOverride({
    required String itemId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required DeleteFoodItemsItemIdBranchOverrideRequest body,
  }) => request(
    'DELETE',
    '/api/v1/vendor/food/items/{itemId}/branch-override',
    pathParameters: {'itemId': itemId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section9b-food-operations.openapi.json
  Future<VendorResponse> getFoodItemsItemIdBranchOverride({
    required String itemId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/food/items/{itemId}/branch-override',
    pathParameters: {'itemId': itemId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/food/operations_http.go#SetBranchOverride
  Future<VendorResponse> putFoodItemsItemIdBranchOverride({
    required String itemId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutFoodItemsItemIdBranchOverrideRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/food/items/{itemId}/branch-override',
    pathParameters: {'itemId': itemId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/food/operations_http.go#ClearLogisticsProfile
  Future<VendorResponse> deleteFoodItemsItemIdLogisticsProfile({
    required String itemId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required DeleteFoodItemsItemIdLogisticsProfileRequest body,
  }) => request(
    'DELETE',
    '/api/v1/vendor/food/items/{itemId}/logistics-profile',
    pathParameters: {'itemId': itemId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section9b-food-operations.openapi.json
  Future<VendorResponse> getFoodItemsItemIdLogisticsProfile({
    required String itemId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/food/items/{itemId}/logistics-profile',
    pathParameters: {'itemId': itemId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/food/operations_http.go#SetLogisticsProfile
  Future<VendorResponse> putFoodItemsItemIdLogisticsProfile({
    required String itemId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutFoodItemsItemIdLogisticsProfileRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/food/items/{itemId}/logistics-profile',
    pathParameters: {'itemId': itemId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section9a-food-catalogue.openapi.json
  Future<VendorResponse> getFoodItemsItemIdMedia({
    required String itemId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/food/items/{itemId}/media',
    pathParameters: {'itemId': itemId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/food/http.go#ReplaceItemMedia
  Future<VendorResponse> putFoodItemsItemIdMedia({
    required String itemId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutFoodItemsItemIdMediaRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/food/items/{itemId}/media',
    pathParameters: {'itemId': itemId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section9a-food-catalogue.openapi.json
  Future<VendorResponse> getFoodItemsItemIdModifierGroups({
    required String itemId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/food/items/{itemId}/modifier-groups',
    pathParameters: {'itemId': itemId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/food/http.go#ReorderAttachedModifierGroups
  Future<VendorResponse> postFoodItemsItemIdModifierGroupsReorder({
    required String itemId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostFoodItemsItemIdModifierGroupsReorderRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/food/items/{itemId}/modifier-groups/reorder',
    pathParameters: {'itemId': itemId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section9a-food-catalogue.openapi.json
  Future<VendorResponse> deleteFoodItemsItemIdModifierGroupsGroupId({
    required String itemId,
    required String groupId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    VendorPayload? body,
  }) => request(
    'DELETE',
    '/api/v1/vendor/food/items/{itemId}/modifier-groups/{groupId}',
    pathParameters: {'itemId': itemId, 'groupId': groupId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section9a-food-catalogue.openapi.json
  Future<VendorResponse> putFoodItemsItemIdModifierGroupsGroupId({
    required String itemId,
    required String groupId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    VendorPayload? body,
  }) => request(
    'PUT',
    '/api/v1/vendor/food/items/{itemId}/modifier-groups/{groupId}',
    pathParameters: {'itemId': itemId, 'groupId': groupId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/food/operations_http.go#ClearPrepProfile
  Future<VendorResponse> deleteFoodItemsItemIdPrepProfile({
    required String itemId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required DeleteFoodItemsItemIdPrepProfileRequest body,
  }) => request(
    'DELETE',
    '/api/v1/vendor/food/items/{itemId}/prep-profile',
    pathParameters: {'itemId': itemId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section9b-food-operations.openapi.json
  Future<VendorResponse> getFoodItemsItemIdPrepProfile({
    required String itemId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/food/items/{itemId}/prep-profile',
    pathParameters: {'itemId': itemId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/food/operations_http.go#SetPrepProfile
  Future<VendorResponse> putFoodItemsItemIdPrepProfile({
    required String itemId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutFoodItemsItemIdPrepProfileRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/food/items/{itemId}/prep-profile',
    pathParameters: {'itemId': itemId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section9b-food-operations.openapi.json
  Future<VendorResponse> getFoodItemsItemIdPricingHistory({
    required String itemId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/food/items/{itemId}/pricing/history',
    pathParameters: {'itemId': itemId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/food/operations_http.go#ResolvePrice
  Future<VendorResponse> postFoodItemsItemIdPricingResolve({
    required String itemId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostFoodItemsItemIdPricingResolveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/food/items/{itemId}/pricing/resolve',
    pathParameters: {'itemId': itemId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section9a-food-catalogue.openapi.json
  Future<VendorResponse> getFoodItemsItemIdVariants({
    required String itemId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/food/items/{itemId}/variants',
    pathParameters: {'itemId': itemId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/food/http.go#CreateVariant
  Future<VendorResponse> postFoodItemsItemIdVariants({
    required String itemId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostFoodItemsItemIdVariantsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/food/items/{itemId}/variants',
    pathParameters: {'itemId': itemId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/food/http.go#ReorderVariants
  Future<VendorResponse> postFoodItemsItemIdVariantsReorder({
    required String itemId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostFoodItemsItemIdVariantsReorderRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/food/items/{itemId}/variants/reorder',
    pathParameters: {'itemId': itemId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/food/http.go#UpdateVariant
  Future<VendorResponse> patchFoodItemsItemIdVariantsVariantId({
    required String itemId,
    required String variantId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchFoodItemsItemIdVariantsVariantIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/food/items/{itemId}/variants/{variantId}',
    pathParameters: {'itemId': itemId, 'variantId': variantId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/food/http.go#ArchiveVariant
  Future<VendorResponse> postFoodItemsItemIdVariantsVariantIdArchive({
    required String itemId,
    required String variantId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostFoodItemsItemIdVariantsVariantIdArchiveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/food/items/{itemId}/variants/{variantId}/archive',
    pathParameters: {'itemId': itemId, 'variantId': variantId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section9a-food-catalogue.openapi.json
  Future<VendorResponse> getFoodMenus({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/food/menus',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/food/http.go#CreateMenu
  Future<VendorResponse> postFoodMenus({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostFoodMenusRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/food/menus',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section9a-food-catalogue.openapi.json
  Future<VendorResponse> getFoodMenusMenuId({
    required String menuId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/food/menus/{menuId}',
    pathParameters: {'menuId': menuId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/food/http.go#UpdateMenu
  Future<VendorResponse> patchFoodMenusMenuId({
    required String menuId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchFoodMenusMenuIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/food/menus/{menuId}',
    pathParameters: {'menuId': menuId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/food/http.go#menuStatusCommand
  Future<VendorResponse> postFoodMenusMenuIdActivate({
    required String menuId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostFoodMenusMenuIdActivateRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/food/menus/{menuId}/activate',
    pathParameters: {'menuId': menuId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/food/http.go#menuStatusCommand
  Future<VendorResponse> postFoodMenusMenuIdArchive({
    required String menuId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostFoodMenusMenuIdArchiveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/food/menus/{menuId}/archive',
    pathParameters: {'menuId': menuId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/food/http.go#menuStatusCommand
  Future<VendorResponse> postFoodMenusMenuIdPause({
    required String menuId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostFoodMenusMenuIdPauseRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/food/menus/{menuId}/pause',
    pathParameters: {'menuId': menuId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section9b-food-operations.openapi.json
  Future<VendorResponse> getFoodMenusMenuIdSchedules({
    required String menuId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/food/menus/{menuId}/schedules',
    pathParameters: {'menuId': menuId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/food/operations_http.go#CreateSchedule
  Future<VendorResponse> postFoodMenusMenuIdSchedules({
    required String menuId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostFoodMenusMenuIdSchedulesRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/food/menus/{menuId}/schedules',
    pathParameters: {'menuId': menuId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section9b-food-operations.openapi.json
  Future<VendorResponse> getFoodMenusMenuIdSchedulesScheduleId({
    required String menuId,
    required String scheduleId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/food/menus/{menuId}/schedules/{scheduleId}',
    pathParameters: {'menuId': menuId, 'scheduleId': scheduleId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/food/operations_http.go#UpdateSchedule
  Future<VendorResponse> patchFoodMenusMenuIdSchedulesScheduleId({
    required String menuId,
    required String scheduleId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchFoodMenusMenuIdSchedulesScheduleIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/food/menus/{menuId}/schedules/{scheduleId}',
    pathParameters: {'menuId': menuId, 'scheduleId': scheduleId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/food/operations_http.go#ArchiveSchedule
  Future<VendorResponse> postFoodMenusMenuIdSchedulesScheduleIdArchive({
    required String menuId,
    required String scheduleId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostFoodMenusMenuIdSchedulesScheduleIdArchiveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/food/menus/{menuId}/schedules/{scheduleId}/archive',
    pathParameters: {'menuId': menuId, 'scheduleId': scheduleId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section9a-food-catalogue.openapi.json
  Future<VendorResponse> getFoodMenusMenuIdSections({
    required String menuId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/food/menus/{menuId}/sections',
    pathParameters: {'menuId': menuId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/food/http.go#CreateSection
  Future<VendorResponse> postFoodMenusMenuIdSections({
    required String menuId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostFoodMenusMenuIdSectionsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/food/menus/{menuId}/sections',
    pathParameters: {'menuId': menuId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/food/http.go#ReorderSections
  Future<VendorResponse> postFoodMenusMenuIdSectionsReorder({
    required String menuId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostFoodMenusMenuIdSectionsReorderRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/food/menus/{menuId}/sections/reorder',
    pathParameters: {'menuId': menuId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/food/http.go#UpdateSection
  Future<VendorResponse> patchFoodMenusMenuIdSectionsSectionId({
    required String menuId,
    required String sectionId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchFoodMenusMenuIdSectionsSectionIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/food/menus/{menuId}/sections/{sectionId}',
    pathParameters: {'menuId': menuId, 'sectionId': sectionId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/food/http.go#ArchiveSection
  Future<VendorResponse> postFoodMenusMenuIdSectionsSectionIdArchive({
    required String menuId,
    required String sectionId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostFoodMenusMenuIdSectionsSectionIdArchiveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/food/menus/{menuId}/sections/{sectionId}/archive',
    pathParameters: {'menuId': menuId, 'sectionId': sectionId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section9a-food-catalogue.openapi.json
  Future<VendorResponse> getFoodMenusMenuIdSectionsSectionIdItems({
    required String menuId,
    required String sectionId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/food/menus/{menuId}/sections/{sectionId}/items',
    pathParameters: {'menuId': menuId, 'sectionId': sectionId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/food/http.go#CreateItem
  Future<VendorResponse> postFoodMenusMenuIdSectionsSectionIdItems({
    required String menuId,
    required String sectionId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostFoodMenusMenuIdSectionsSectionIdItemsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/food/menus/{menuId}/sections/{sectionId}/items',
    pathParameters: {'menuId': menuId, 'sectionId': sectionId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/food/http.go#ReorderItems
  Future<VendorResponse> postFoodMenusMenuIdSectionsSectionIdItemsReorder({
    required String menuId,
    required String sectionId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostFoodMenusMenuIdSectionsSectionIdItemsReorderRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/food/menus/{menuId}/sections/{sectionId}/items/reorder',
    pathParameters: {'menuId': menuId, 'sectionId': sectionId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section9a-food-catalogue.openapi.json
  Future<VendorResponse> getFoodModifierGroups({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/food/modifier-groups',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/food/http.go#CreateModifierGroup
  Future<VendorResponse> postFoodModifierGroups({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostFoodModifierGroupsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/food/modifier-groups',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section9a-food-catalogue.openapi.json
  Future<VendorResponse> getFoodModifierGroupsGroupId({
    required String groupId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/food/modifier-groups/{groupId}',
    pathParameters: {'groupId': groupId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/food/http.go#UpdateModifierGroup
  Future<VendorResponse> patchFoodModifierGroupsGroupId({
    required String groupId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchFoodModifierGroupsGroupIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/food/modifier-groups/{groupId}',
    pathParameters: {'groupId': groupId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/food/http.go#ArchiveModifierGroup
  Future<VendorResponse> postFoodModifierGroupsGroupIdArchive({
    required String groupId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostFoodModifierGroupsGroupIdArchiveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/food/modifier-groups/{groupId}/archive',
    pathParameters: {'groupId': groupId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section9a-food-catalogue.openapi.json
  Future<VendorResponse> getFoodModifierGroupsGroupIdModifiers({
    required String groupId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/food/modifier-groups/{groupId}/modifiers',
    pathParameters: {'groupId': groupId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/food/http.go#CreateModifier
  Future<VendorResponse> postFoodModifierGroupsGroupIdModifiers({
    required String groupId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostFoodModifierGroupsGroupIdModifiersRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/food/modifier-groups/{groupId}/modifiers',
    pathParameters: {'groupId': groupId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/food/http.go#ReorderModifiers
  Future<VendorResponse> postFoodModifierGroupsGroupIdModifiersReorder({
    required String groupId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostFoodModifierGroupsGroupIdModifiersReorderRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/food/modifier-groups/{groupId}/modifiers/reorder',
    pathParameters: {'groupId': groupId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/food/http.go#UpdateModifier
  Future<VendorResponse> patchFoodModifierGroupsGroupIdModifiersModifierId({
    required String groupId,
    required String modifierId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchFoodModifierGroupsGroupIdModifiersModifierIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/food/modifier-groups/{groupId}/modifiers/{modifierId}',
    pathParameters: {'groupId': groupId, 'modifierId': modifierId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/food/http.go#ArchiveModifier
  Future<VendorResponse>
  postFoodModifierGroupsGroupIdModifiersModifierIdArchive({
    required String groupId,
    required String modifierId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostFoodModifierGroupsGroupIdModifiersModifierIdArchiveRequest
    body,
  }) => request(
    'POST',
    '/api/v1/vendor/food/modifier-groups/{groupId}/modifiers/{modifierId}/archive',
    pathParameters: {'groupId': groupId, 'modifierId': modifierId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );
}
