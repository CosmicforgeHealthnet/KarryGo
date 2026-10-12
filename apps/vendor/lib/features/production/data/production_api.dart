// GENERATED. Methods correspond to exact public Vendor contracts.
import '../../../core/network/vendor_api_client.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/network/vendor_response.dart';
import '../models/production_models.dart';
import 'production_routes.dart';

class VendorProductionApi extends VendorFeatureApi {
  VendorProductionApi(VendorApiClient client)
    : super(client, productionOperations);

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> getProductionAlerts({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/production/alerts',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/production/http.go#acknowledgeAlert
  Future<VendorResponse> postProductionAlertsAlertIdAcknowledge({
    required String alertId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostProductionAlertsAlertIdAcknowledgeRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/production/alerts/{alertId}/acknowledge',
    pathParameters: {'alertId': alertId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/production/http.go#resolveAlert
  Future<VendorResponse> postProductionAlertsAlertIdResolve({
    required String alertId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostProductionAlertsAlertIdResolveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/production/alerts/{alertId}/resolve',
    pathParameters: {'alertId': alertId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> getProductionBlackouts({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/production/blackouts',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/production/http.go#createBlackout
  Future<VendorResponse> postProductionBlackouts({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostProductionBlackoutsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/production/blackouts',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/production/http.go#cancelBlackout
  Future<VendorResponse> postProductionBlackoutsBlackoutIdCancel({
    required String blackoutId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostProductionBlackoutsBlackoutIdCancelRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/production/blackouts/{blackoutId}/cancel',
    pathParameters: {'blackoutId': blackoutId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/production/http.go#preview
  Future<VendorResponse> postProductionCapacityPreview({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostProductionCapacityPreviewRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/production/capacity/preview',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> getProductionCapacityUtilization({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/production/capacity/utilization',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> getProductionJobs({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/production/jobs',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/production/http.go#createJob
  Future<VendorResponse> postProductionJobs({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostProductionJobsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/production/jobs',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> getProductionJobsJobId({
    required String jobId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/production/jobs/{jobId}',
    pathParameters: {'jobId': jobId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> postProductionJobsJobIdComplete({
    required String jobId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    PostProductionJobsJobIdCompleteRequest? body,
  }) => request(
    'POST',
    '/api/v1/vendor/production/jobs/{jobId}/complete',
    pathParameters: {'jobId': jobId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/production/http.go#reportDelay
  Future<VendorResponse> postProductionJobsJobIdDelay({
    required String jobId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostProductionJobsJobIdDelayRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/production/jobs/{jobId}/delay',
    pathParameters: {'jobId': jobId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> getProductionJobsJobIdEvents({
    required String jobId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/production/jobs/{jobId}/events',
    pathParameters: {'jobId': jobId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> postProductionJobsJobIdReady({
    required String jobId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    PostProductionJobsJobIdReadyRequest? body,
  }) => request(
    'POST',
    '/api/v1/vendor/production/jobs/{jobId}/ready',
    pathParameters: {'jobId': jobId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> postProductionJobsJobIdStart({
    required String jobId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    PostProductionJobsJobIdStartRequest? body,
  }) => request(
    'POST',
    '/api/v1/vendor/production/jobs/{jobId}/start',
    pathParameters: {'jobId': jobId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> getProductionJobsJobIdSteps({
    required String jobId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/production/jobs/{jobId}/steps',
    pathParameters: {'jobId': jobId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/production/http.go#createStep
  Future<VendorResponse> postProductionJobsJobIdSteps({
    required String jobId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostProductionJobsJobIdStepsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/production/jobs/{jobId}/steps',
    pathParameters: {'jobId': jobId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/production/http.go#changeStep
  Future<VendorResponse> postProductionJobsJobIdStepsStepIdStatus({
    required String jobId,
    required String stepId,
    required String status,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostProductionJobsJobIdStepsStepIdStatusRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/production/jobs/{jobId}/steps/{stepId}/{status}',
    pathParameters: {'jobId': jobId, 'stepId': stepId, 'status': status},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/production/http.go#transitionJob
  Future<VendorResponse> postProductionJobsJobIdStatus({
    required String jobId,
    required String status,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostProductionJobsJobIdStatusRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/production/jobs/{jobId}/{status}',
    pathParameters: {'jobId': jobId, 'status': status},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> getProductionLines({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/production/lines',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/production/http.go#createLine
  Future<VendorResponse> postProductionLines({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostProductionLinesRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/production/lines',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> getProductionLinesLineId({
    required String lineId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/production/lines/{lineId}',
    pathParameters: {'lineId': lineId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/production/http.go#updateLine
  Future<VendorResponse> patchProductionLinesLineId({
    required String lineId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchProductionLinesLineIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/production/lines/{lineId}',
    pathParameters: {'lineId': lineId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> postProductionLinesLineIdArchive({
    required String lineId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    PostProductionLinesLineIdArchiveRequest? body,
  }) => request(
    'POST',
    '/api/v1/vendor/production/lines/{lineId}/archive',
    pathParameters: {'lineId': lineId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> getProductionPools({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/production/pools',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/production/http.go#createPool
  Future<VendorResponse> postProductionPools({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostProductionPoolsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/production/pools',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> getProductionPoolsPoolId({
    required String poolId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/production/pools/{poolId}',
    pathParameters: {'poolId': poolId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/production/http.go#updatePool
  Future<VendorResponse> patchProductionPoolsPoolId({
    required String poolId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchProductionPoolsPoolIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/production/pools/{poolId}',
    pathParameters: {'poolId': poolId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> postProductionPoolsPoolIdArchive({
    required String poolId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    PostProductionPoolsPoolIdArchiveRequest? body,
  }) => request(
    'POST',
    '/api/v1/vendor/production/pools/{poolId}/archive',
    pathParameters: {'poolId': poolId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/production/http.go#reserve
  Future<VendorResponse> postProductionReservations({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostProductionReservationsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/production/reservations',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> getProductionReservationsReservationId({
    required String reservationId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/production/reservations/{reservationId}',
    pathParameters: {'reservationId': reservationId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/production/http.go#confirm
  Future<VendorResponse> postProductionReservationsReservationIdConfirm({
    required String reservationId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostProductionReservationsReservationIdConfirmRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/production/reservations/{reservationId}/confirm',
    pathParameters: {'reservationId': reservationId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/production/http.go#release
  Future<VendorResponse> postProductionReservationsReservationIdRelease({
    required String reservationId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    PostProductionReservationsReservationIdReleaseRequest? body,
  }) => request(
    'POST',
    '/api/v1/vendor/production/reservations/{reservationId}/release',
    pathParameters: {'reservationId': reservationId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/production/http.go#reschedule
  Future<VendorResponse> postProductionReservationsReservationIdReschedule({
    required String reservationId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostProductionReservationsReservationIdRescheduleRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/production/reservations/{reservationId}/reschedule',
    pathParameters: {'reservationId': reservationId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> getProductionRules({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/production/rules',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/production/http.go#createRule
  Future<VendorResponse> postProductionRules({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostProductionRulesRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/production/rules',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> getProductionRulesRuleId({
    required String ruleId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/production/rules/{ruleId}',
    pathParameters: {'ruleId': ruleId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/production/http.go#updateRule
  Future<VendorResponse> patchProductionRulesRuleId({
    required String ruleId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchProductionRulesRuleIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/production/rules/{ruleId}',
    pathParameters: {'ruleId': ruleId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> postProductionRulesRuleIdArchive({
    required String ruleId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    PostProductionRulesRuleIdArchiveRequest? body,
  }) => request(
    'POST',
    '/api/v1/vendor/production/rules/{ruleId}/archive',
    pathParameters: {'ruleId': ruleId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> getProductionSlotTemplates({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/production/slot-templates',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/production/http.go#createSlotTemplate
  Future<VendorResponse> postProductionSlotTemplates({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostProductionSlotTemplatesRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/production/slot-templates',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> getProductionSlotTemplatesTemplateId({
    required String templateId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/production/slot-templates/{templateId}',
    pathParameters: {'templateId': templateId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/production/http.go#updateSlotTemplate
  Future<VendorResponse> patchProductionSlotTemplatesTemplateId({
    required String templateId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchProductionSlotTemplatesTemplateIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/production/slot-templates/{templateId}',
    pathParameters: {'templateId': templateId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> postProductionSlotTemplatesTemplateIdArchive({
    required String templateId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    PostProductionSlotTemplatesTemplateIdArchiveRequest? body,
  }) => request(
    'POST',
    '/api/v1/vendor/production/slot-templates/{templateId}/archive',
    pathParameters: {'templateId': templateId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> postProductionSlotTemplatesTemplateIdGenerate({
    required String templateId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    PostProductionSlotTemplatesTemplateIdGenerateRequest? body,
  }) => request(
    'POST',
    '/api/v1/vendor/production/slot-templates/{templateId}/generate',
    pathParameters: {'templateId': templateId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> getProductionSlots({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/production/slots',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/production/http.go#createSlot
  Future<VendorResponse> postProductionSlots({
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostProductionSlotsRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/production/slots',
    pathParameters: {},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: sections10-11-production.openapi.json
  Future<VendorResponse> getProductionSlotsSlotId({
    required String slotId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/production/slots/{slotId}',
    pathParameters: {'slotId': slotId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/production/http.go#updateSlot
  Future<VendorResponse> patchProductionSlotsSlotId({
    required String slotId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PatchProductionSlotsSlotIdRequest body,
  }) => request(
    'PATCH',
    '/api/v1/vendor/production/slots/{slotId}',
    pathParameters: {'slotId': slotId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/production/http.go#archiveSlot
  Future<VendorResponse> postProductionSlotsSlotIdArchive({
    required String slotId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostProductionSlotsSlotIdArchiveRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/production/slots/{slotId}/archive',
    pathParameters: {'slotId': slotId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/production/http.go#blackoutSlot
  Future<VendorResponse> postProductionSlotsSlotIdBlackout({
    required String slotId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostProductionSlotsSlotIdBlackoutRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/production/slots/{slotId}/blackout',
    pathParameters: {'slotId': slotId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/production/http.go#closeSlot
  Future<VendorResponse> postProductionSlotsSlotIdClose({
    required String slotId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostProductionSlotsSlotIdCloseRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/production/slots/{slotId}/close',
    pathParameters: {'slotId': slotId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: services/vendor-service/internal/features/production/http.go#openSlot
  Future<VendorResponse> postProductionSlotsSlotIdOpen({
    required String slotId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PostProductionSlotsSlotIdOpenRequest body,
  }) => request(
    'POST',
    '/api/v1/vendor/production/slots/{slotId}/open',
    pathParameters: {'slotId': slotId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );
}
