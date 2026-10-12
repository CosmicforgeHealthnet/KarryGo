import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';

const orchestrationBase =
    '/api/v1/vendor/fulfilment/groups/{groupId}/orchestration';
const executionAuthorities = ['DISPATCH', 'TAXI', 'HAULING'];
const orchestrationStates = [
  'PENDING_REQUIREMENTS',
  'PLANNED',
  'WAITING_READINESS',
  'WARM_MATCH',
  'COMMIT_DUE',
  'COMMITTING',
  'COMMITTED',
  'IN_EXECUTION',
  'COMPLETED',
  'RECOVERING',
  'OPS_REVIEW',
  'CANCELLED',
  'FAILED',
];

class OrchestrationWorkspaceController extends ChangeNotifier {
  OrchestrationWorkspaceController(this.runtime, this.groupId);
  final VendorRuntime runtime;
  final String groupId;
  late final app = runtime.backend;
  late final storage = runtime.mode == VendorDataMode.mock
      ? runtime.mockSupportPreferences
      : runtime.preferences;
  late final recoveryKey =
      'vendor.orchestration.${base64Url.encode(utf8.encode('${runtime.mode.name}|${runtime.mode == VendorDataMode.mock ? runtime.revision : app.client.config.origin}|${app.scope.vendorId}|${app.auth.session?.staffId}|$groupId'))}';
  JsonObject? record, pending;
  List<JsonObject> pickups = [], decisions = [];
  bool loading = false,
      busy = false,
      stale = true,
      blocked = false,
      denied = false,
      disposed = false;
  String? error, errorCode, historyError, feedback;
  bool get canOverride =>
      !loading &&
      !busy &&
      !stale &&
      !blocked &&
      !denied &&
      historyError == null &&
      pending == null &&
      record != null &&
      !(record!['current_mission_ref'] is String &&
          (record!['current_mission_ref'] as String).isNotEmpty) &&
      ![
        'COMMITTING',
        'COMMITTED',
        'IN_EXECUTION',
        'COMPLETED',
        'RECOVERING',
        'CANCELLED',
        'FAILED',
      ].contains(record!['state']);
  void emit() {
    if (!disposed) notifyListeners();
  }

  void validate(JsonObject value) {
    if (value['id'] is! String ||
        value['fulfilment_group_ref'] != groupId ||
        value['version'] is! int ||
        value['version'] < 1 ||
        !orchestrationStates.contains(value['state'])) {
      throw const FormatException('Incomplete orchestration response');
    }
  }

  Future<void> restore() async {
    try {
      final saved = await storage.read(recoveryKey);
      if (saved == null || saved == '{}') return;
      final value = jsonObject(jsonDecode(saved));
      if (value['body'] is! Map ||
          value['event_ids'] is! List ||
          value['actor'] is! String ||
          value['orchestration_id'] is! String) {
        throw const FormatException('Invalid recovery record');
      }
      pending = value;
    } catch (_) {
      blocked = true;
      error =
          'The previous override could not be restored. Reopen this screen before making changes.';
    }
  }

  Future<void> read() async {
    final response = await app.orchestrationApi
        .getFulfilmentGroupsGroupIdOrchestration(groupId: groupId);
    final next = jsonObject(response.object['orchestration']);
    validate(next);
    final list = response.object['pickups'];
    if (list is! List) throw const FormatException('Missing pickup evidence');
    final owned = list.map(jsonObject).toList();
    if (owned.isEmpty ||
        owned.any(
          (p) =>
              p['business_id'] != app.scope.vendorId ||
              p['branch_id'] is! String,
        )) {
      throw const FormatException('Out-of-scope pickup evidence');
    }
    record = next;
    pickups = owned;
    historyError = null;
    try {
      final history = await app.orchestrationApi
          .getFulfilmentGroupsGroupIdOrchestrationDecisions(groupId: groupId);
      final events = history.object['decisions'];
      if (events != null && events is! List) {
        throw const FormatException('Missing decision evidence');
      }
      final nextEvents = (events as List? ?? []).map(jsonObject).toList();
      if (nextEvents.any(
        (e) => e['id'] is! String || e['orchestration_id'] != next['id'],
      )) {
        throw const FormatException('Out-of-scope decisions');
      }
      decisions = nextEvents;
    } catch (e) {
      decisions = [];
      historyError = e is ApiException
          ? e.message
          : 'Decision history could not be loaded.';
    }
    if (pending != null && historyError == null) await reconcile();
  }

  Future<void> reconcile() async {
    final saved = pending!;
    final body = jsonObject(saved['body']);
    final match = decisions.any(
      (e) =>
          e['event_type'] == 'OVERRIDDEN' &&
          !(saved['event_ids'] as List).contains(e['id']) &&
          e['orchestration_id'] == saved['orchestration_id'] &&
          e['actor_ref'] == saved['actor'] &&
          e['candidate_authority'] == body['authority'] &&
          (e['candidate_product_code'] ?? '') == body['product_code'] &&
          e['override_reason'] == body['reason'],
    );
    if (!match) return;
    await storage.write(recoveryKey, '{}');
    pending = null;
    feedback =
        'The decision history confirms your override. The latest plan is shown below.';
  }

  Future<void> load() async {
    if (loading || busy || disposed) return;
    loading = true;
    stale = true;
    error = null;
    errorCode = null;
    emit();
    try {
      await restore();
      await read();
      stale = false;
    } catch (e) {
      error = e is ApiException
          ? e.message
          : 'Delivery planning could not be loaded. Refresh to continue.';
      errorCode = e is ApiException ? e.code : null;
    } finally {
      loading = false;
      emit();
    }
  }

  Future<bool> submitOverride(JsonObject body, int version) async {
    if (!canOverride ||
        historyError != null ||
        body['reason'] is! String ||
        (body['reason'] as String).trim().isEmpty ||
        !executionAuthorities.contains(body['authority'])) {
      return false;
    }
    busy = true;
    error = null;
    feedback = null;
    emit();
    var accepted = false;
    try {
      await read();
      if (record!['version'] != version ||
          historyError != null ||
          [
            'COMMITTING',
            'COMMITTED',
            'IN_EXECUTION',
            'COMPLETED',
            'RECOVERING',
            'CANCELLED',
            'FAILED',
          ].contains(record!['state']) ||
          (record!['current_mission_ref'] ?? '') != '') {
        error =
            'The plan changed while you were reviewing it. Review the latest decision before continuing.';
        return false;
      }
      final saved = <String, dynamic>{
        'body': jsonObject(jsonDecode(jsonEncode(body))),
        'event_ids': decisions.map((e) => e['id']).toList(),
        'actor': app.auth.session!.staffId,
        'orchestration_id': record!['id'],
      };
      await storage.write(recoveryKey, jsonEncode(saved));
      pending = saved;
      final response = await app.orchestrationApi.request(
        'POST',
        '$orchestrationBase/authority-override',
        pathParameters: {'groupId': groupId},
        body: VendorPayload(jsonObject(saved['body'])),
      );
      validate(response.object);
      accepted = true;
      record = response.object;
      await storage.write(recoveryKey, '{}');
      pending = null;
      await read();
      stale = false;
      return true;
    } catch (e) {
      error = accepted
          ? 'The override was saved. Refresh to load the latest plan and finish any pending recovery.'
          : e is ApiException
          ? e.message
          : pending == null
          ? 'The override could not be submitted. Refresh before continuing.'
          : 'The override result is uncertain. Refresh to check decision history; do not submit it again.';
      if (e is ApiException &&
          [400, 401, 403, 404, 409, 422].contains(e.statusCode)) {
        if (e.statusCode == 403) denied = true;
        try {
          await storage.write(recoveryKey, '{}');
          pending = null;
        } catch (_) {
          blocked = true;
        }
      }
      stale = true;
      return false;
    } finally {
      busy = false;
      emit();
    }
  }

  @override
  void dispose() {
    disposed = true;
    super.dispose();
  }
}
