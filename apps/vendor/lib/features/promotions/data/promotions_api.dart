// GENERATED. Methods correspond to exact public Vendor contracts.
import '../../../core/network/vendor_api_client.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/network/vendor_response.dart';
import '../models/promotions_models.dart';
import 'promotions_routes.dart';

class VendorPromotionsApi extends VendorFeatureApi {
  VendorPromotionsApi(VendorApiClient client)
    : super(client, promotionsOperations);

  /// Source: promotions-delivery-funding.openapi.json
  Future<VendorResponse> getPromotions({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/promotions',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/promotions/http.go#create
  Future<VendorResponse> postPromotions({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPromotionsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/promotions',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: promotions-delivery-funding.openapi.json
  Future<VendorResponse> getPromotionsPromotionId({
    required String promotionId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/promotions/{promotionId}',
    pathParameters: {'promotionId': promotionId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/promotions/http.go#update
  Future<VendorResponse> patchPromotionsPromotionId({
    required String promotionId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchPromotionsPromotionIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/promotions/{promotionId}',
    pathParameters: {'promotionId': promotionId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/promotions/http.go#coupon
  Future<VendorResponse> postPromotionsPromotionIdCoupons({
    required String promotionId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPromotionsPromotionIdCouponsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/promotions/{promotionId}/coupons',
    pathParameters: {'promotionId': promotionId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/promotions/http.go#status
  Future<VendorResponse> postPromotionsPromotionIdStatus({
    required String promotionId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostPromotionsPromotionIdStatusRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/promotions/{promotionId}/status',
    pathParameters: {'promotionId': promotionId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: promotions-delivery-funding.openapi.json
  Future<VendorResponse> getPromotionsPromotionIdUsage({
    required String promotionId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/promotions/{promotionId}/usage',
    pathParameters: {'promotionId': promotionId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );
}
