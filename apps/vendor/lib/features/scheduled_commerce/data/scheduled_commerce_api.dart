// GENERATED. Methods correspond to exact public Vendor contracts.
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_api_client.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/network/vendor_response.dart';
import '../models/scheduled_commerce_models.dart';
import 'scheduled_commerce_routes.dart';

class VendorScheduledCommerceApi extends VendorFeatureApi {
  VendorScheduledCommerceApi(VendorApiClient client)
    : super(client, scheduledCommerceOperations);

  /// Source: scheduled_group_rfq_recurring_commerce.openapi.json
  Future<VendorResponse> getCommerceSection21RecurringSchedules({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/commerce/section21/recurring-schedules',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/orders/commerce_orders_http.go#createRecurringSchedule
  Future<VendorResponse> postCommerceSection21RecurringSchedules({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCommerceSection21RecurringSchedulesRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/commerce/section21/recurring-schedules',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: scheduled_group_rfq_recurring_commerce.openapi.json
  Future<VendorResponse> getCommerceSection21RecurringSchedulesScheduleId({
    required String scheduleId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/commerce/section21/recurring-schedules/{scheduleId}',
    pathParameters: {'scheduleId': scheduleId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/orders/commerce_orders_http.go#updateRecurringSchedule
  Future<VendorResponse> putCommerceSection21RecurringSchedulesScheduleId({
    required String scheduleId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutCommerceSection21RecurringSchedulesScheduleIdRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/commerce/section21/recurring-schedules/{scheduleId}',
    pathParameters: {'scheduleId': scheduleId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: scheduled_group_rfq_recurring_commerce.openapi.json
  Future<VendorResponse>
  getCommerceSection21RecurringSchedulesScheduleIdOccurrences({
    required String scheduleId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/commerce/section21/recurring-schedules/{scheduleId}/occurrences',
    pathParameters: {'scheduleId': scheduleId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/orders/commerce_orders_http.go#setRecurringScheduleStatus
  Future<VendorResponse>
  postCommerceSection21RecurringSchedulesScheduleIdStatus({
    required String scheduleId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCommerceSection21RecurringSchedulesScheduleIdStatusRequest
    body,
  }) => request(
    'POST',
    '/api/v1/vendor/commerce/section21/recurring-schedules/{scheduleId}/status',
    pathParameters: {'scheduleId': scheduleId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/orders/commerce_orders_http.go#quoteRFQ
  Future<VendorResponse> postCommerceSection21RfqsRfqIdQuotes({
    required String rfqId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCommerceSection21RfqsRfqIdQuotesRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/commerce/section21/rfqs/{rfqId}/quotes',
    pathParameters: {'rfqId': rfqId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: scheduled_group_rfq_recurring_commerce.openapi.json
  Future<VendorResponse> getCommerceSection21ScheduledOrders({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/commerce/section21/scheduled-orders',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/orders/commerce_orders_http.go#createScheduled
  Future<VendorResponse> postCommerceSection21ScheduledOrders({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostCommerceSection21ScheduledOrdersRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/commerce/section21/scheduled-orders',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: scheduled_group_rfq_recurring_commerce.openapi.json
  Future<VendorResponse> getCommerceSection21ScheduledOrdersId({
    required String id,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/commerce/section21/scheduled-orders/{id}',
    pathParameters: {'id': id},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: scheduled_group_rfq_recurring_commerce.openapi.json
  Future<VendorResponse> postCommerceSection21ScheduledOrdersIdCancel({
    required String id,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    VendorPayload? body,
  }) => request(
    'POST',
    '/api/v1/vendor/commerce/section21/scheduled-orders/{id}/cancel',
    pathParameters: {'id': id},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );
  Future<VendorResponse> getVendorRFQs({
    Map<String, String> query = const {},
  }) => request('GET', '/api/v1/vendor/commerce/section21/rfqs', query: query);
  Future<VendorResponse> getVendorRFQ({required String rfqId}) => request(
    'GET',
    '/api/v1/vendor/commerce/section21/rfqs/{rfqId}',
    pathParameters: {'rfqId': rfqId},
  );
  Future<VendorResponse> getVendorRFQQuotes({required String rfqId}) => request(
    'GET',
    '/api/v1/vendor/commerce/section21/rfqs/{rfqId}/quotes',
    pathParameters: {'rfqId': rfqId},
  );
  Future<VendorResponse> getSchedulingQuotes({
    Map<String, String> query = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/commerce/section21/scheduling-quotes',
    query: query,
  );
  Future<VendorResponse> getSchedulingQuote({
    required String quoteId,
    required String branchId,
  }) => request(
    'GET',
    '/api/v1/vendor/commerce/section21/scheduling-quotes/{quoteId}',
    pathParameters: {'quoteId': quoteId},
    query: {'branch_id': branchId},
  );
  Future<VendorResponse> getRecurringSetupContexts({
    Map<String, String> query = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/commerce/section21/recurring-setup-contexts',
    query: query,
  );
}
