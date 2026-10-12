import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import 'delivery_attempt.dart';

const fallbackReasons = [
  'OWN_RIDER_FIRST',
  'DISTANCE_OVERFLOW',
  'TIME_OVERFLOW',
  'CAPACITY_OVERFLOW',
  'VEHICLE_OVERFLOW',
  'SLA_OVERFLOW',
  'PROVIDER_UNAVAILABLE',
  'VENDOR_UNAVAILABLE',
  'CUSTOMER_APPROVED_CHANGE',
  'MISSION_REPLACED',
  'RETURN',
  'TRANSFER',
];

class HybridFallbackController extends ChangeNotifier {
  HybridFallbackController(VendorRuntime runtime, this.mission)
    : attempt = DeliveryAttempt(
        runtime,
        'fallback:${mission['vendor_mission_id']}',
      );
  final JsonObject mission;
  final DeliveryAttempt attempt;
  JsonObject? record;
  bool loading = false, busy = false, initialized = false, disposed = false;
  String? error;
  bool get canRequest =>
      initialized &&
      error == null &&
      !loading &&
      !busy &&
      !attempt.blocked &&
      record == null &&
      attempt.pending == null &&
      ![
        'CUSTODY_TRANSFERRED',
        'OUT_FOR_DELIVERY',
        'RETURN_REQUIRED',
        'DELIVERED',
        'RETURNED',
      ].contains(mission['status']);
  void emit() {
    if (!disposed) notifyListeners();
  }

  void validate(JsonObject v) {
    if (v['BusinessID'] != mission['business_id'] ||
        v['VendorMissionID'] != mission['vendor_mission_id'] ||
        v['Version'] is! int ||
        ![
          'STARTED',
          'PLAN_ACTIVATED',
          'FUNDING_TRANSITIONED',
          'MISSION_LINKED',
          'COMPLETED',
          'FAILED',
        ].contains(v['State'])) {
      throw const FormatException('Incomplete fallback response');
    }
  }

  Future<void> load() async {
    if (loading || busy) return;
    loading = true;
    error = null;
    emit();
    try {
      if (!initialized) {
        await attempt.restore();
        initialized = true;
      }
      final operation =
          attempt.pending?['key'] as String? ??
          await attempt.storage.read('${attempt.key}.operation');
      if (operation != null && operation.isNotEmpty) {
        try {
          final r = await attempt.app.deliveryApi.request(
            'GET',
            '/api/v1/vendor/fulfilment/vendor-missions/fallbacks/{operationRef}',
            pathParameters: {'operationRef': operation},
          );
          validate(r.object);
          record = r.object;
          if (record!['State'] == 'COMPLETED') {
            await attempt.storage.write('${attempt.key}.operation', operation);
            await attempt.clear();
          }
        } on ApiException catch (e) {
          if (e.statusCode != 404 || attempt.pending == null) rethrow;
        }
      }
    } catch (e) {
      error = e is ApiException
          ? e.message
          : 'Could not restore fallback progress. Refresh before continuing.';
    } finally {
      loading = false;
      emit();
    }
  }

  Future<bool> request(String? reason) async {
    if (loading ||
        busy ||
        attempt.blocked ||
        attempt.pending == null && !canRequest ||
        attempt.pending == null && !fallbackReasons.contains(reason)) {
      return false;
    }
    busy = true;
    error = null;
    emit();
    final r = await attempt.send(
      method: 'POST',
      path: '/api/v1/vendor/fulfilment/vendor-missions/{missionId}/fallbacks',
      params: {'missionId': mission['vendor_mission_id'] as String},
      version: mission['version'] as int,
      body: {'reason_code': reason},
      validateResponse: validate,
      beforeClear: (v, key) async {
        await attempt.storage.write('${attempt.key}.operation', key);
      },
    );
    if (r != null) record = r;
    error = attempt.error;
    busy = false;
    emit();
    return r != null;
  }

  @override
  void dispose() {
    disposed = true;
    super.dispose();
  }
}
