import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import 'delivery_attempt.dart';

const riderActions = <String, List<String>>{
  'ASSIGNED': ['acknowledge'],
  'ACKNOWLEDGED': ['start-pickup'],
  'EN_ROUTE_PICKUP': ['arrive-pickup'],
  'ARRIVED_PICKUP': ['complete-pickup'],
  'IN_CUSTODY': ['start-receiver'],
  'EN_ROUTE_RECEIVER': ['arrive-receiver'],
  'ARRIVED_RECEIVER': ['complete-delivery', 'receiver-unavailable'],
  'RECEIVER_UNAVAILABLE': ['start-receiver', 'return'],
  'COMPLETED': [],
  'RETURNED': [],
  'PLANNED': [],
  'RETURNING': [],
  'FAILED': [],
  'FALLBACK_STARTED': [],
  'FALLBACK_COMPLETED': [],
};

class RiderWorkspaceController extends ChangeNotifier {
  RiderWorkspaceController(this.runtime, {this.id})
    : attempt = DeliveryAttempt(runtime, 'rider:${id ?? 'availability'}');
  final VendorRuntime runtime;
  final String? id;
  final DeliveryAttempt attempt;
  List<JsonObject> missions = [], branches = [];
  JsonObject? record, availability;
  bool loading = false,
      busy = false,
      stale = true,
      disposed = false,
      initialized = false;
  String? error, branchError;
  List<String> get actions =>
      loading || busy || stale || attempt.blocked || attempt.pending != null
      ? []
      : riderActions[record?['stage']] ?? [];
  void emit() {
    if (!disposed) notifyListeners();
  }

  void validateRuntime(JsonObject value) {
    final mission = jsonObject(value['mission']);
    if (!riderActions.containsKey(value['stage']) ||
        value['runtime_version'] is! int ||
        mission['version'] is! int ||
        mission['business_id'] != attempt.app.scope.vendorId ||
        mission['assigned_staff_id'] != attempt.app.auth.session?.staffId ||
        id != null && mission['vendor_mission_id'] != id) {
      throw const FormatException('Incomplete rider mission response');
    }
  }

  void validateAvailability(JsonObject value) {
    if (value['business_id'] != attempt.app.scope.vendorId ||
        value['staff_id'] != attempt.app.auth.session?.staffId ||
        value['version'] is! int ||
        value['version'] < 1 ||
        ![
          'AVAILABLE',
          'OFFLINE',
          'BUSY',
          'SUSPENDED',
        ].contains(value['state'])) {
      throw const FormatException('Incomplete availability response');
    }
  }

  Future<void> load() async {
    if (loading || busy || disposed) return;
    loading = true;
    stale = true;
    error = null;
    emit();
    try {
      if (!initialized) {
        await attempt.restore();
        initialized = true;
      }
      if (id != null) {
        final r = await attempt.app.deliveryApi.request(
          'GET',
          '/api/v1/vendor/delivery/rider/missions/{missionId}',
          pathParameters: {'missionId': id!},
        );
        validateRuntime(r.object);
        record = r.object;
      } else {
        final r = await attempt.app.deliveryApi.request(
          'GET',
          '/api/v1/vendor/delivery/rider/missions',
          query: {'limit': '200'},
        );
        final rows = r.object['vendor_missions'];
        if (rows is! List) {
          throw const FormatException('Incomplete rider mission list');
        }
        missions = rows.map(jsonObject).toList();
        for (final value in missions) {
          validateRuntime(value);
        }
        try {
          availability = (await attempt.app.deliveryApi.request(
            'GET',
            '/api/v1/vendor/delivery/rider/availability',
          )).object;
          validateAvailability(availability!);
        } on ApiException catch (e) {
          if (e.statusCode == 404 && e.code == 'FULFILMENT_NOT_FOUND') {
            availability = null;
          } else {
            rethrow;
          }
        }
        branchError = null;
        try {
          branches = [];
          var offset = 0;
          while (true) {
            final result = await attempt.app.deliveryApi.request(
              'GET',
              '/api/v1/vendor/delivery/rider/branches',
              query: {'limit': '200', 'offset': '$offset'},
            );
            final data = result.object;
            if (data['items'] is! List) {
              throw const FormatException('Incomplete rider branches');
            }
            branches.addAll((data['items'] as List).map(jsonObject));
            if (data['has_more'] != true) break;
            final next = data['next_offset'];
            if (next is! int || next <= offset) {
              throw const FormatException('Incomplete rider page');
            }
            offset = next;
          }
          if (branches.isEmpty) {
            branchError =
                'No active branches are available for your rider role. Ask your manager to check your branch access.';
          }
        } catch (_) {
          branchError =
              'Could not load your rider branches. Refresh to try again.';
          branches = [];
        }
      }
      stale = false;
    } catch (e) {
      error = e is ApiException
          ? e.message
          : e is FormatException
          ? e.message
          : 'Could not connect. Refresh to continue.';
    } finally {
      loading = false;
      emit();
    }
  }

  Future<bool> execute(
    String action, {
    String? evidence,
    String? method,
    String? credential,
    String? reason,
  }) async {
    if (busy || loading || attempt.blocked || id == null) return false;
    if (attempt.pending == null && !actions.contains(action)) return false;
    if (attempt.pending == null &&
        ['complete-delivery', 'return'].contains(action) &&
        (evidence?.trim().isNotEmpty != true ||
            !['OTP', 'QR'].contains(method) ||
            credential?.isNotEmpty != true)) {
      return false;
    }
    if (attempt.pending == null &&
        action == 'receiver-unavailable' &&
        reason?.trim().isNotEmpty != true) {
      return false;
    }
    busy = true;
    error = null;
    emit();
    final value = await attempt.send(
      method: 'POST',
      path:
          '/api/v1/vendor/delivery/rider/missions/{missionId}/actions/{action}',
      params: {'missionId': id!, 'action': action},
      version: jsonObject(record?['mission'] ?? {})['version'] as int?,
      credential: credential,
      validateResponse: validateRuntime,
      body: {
        if (evidence?.trim().isNotEmpty == true)
          'evidence_ref': evidence!.trim(),
        'method': ?method,
        if (reason?.trim().isNotEmpty == true) 'reason_code': reason!.trim(),
      },
    );
    var success = false;
    if (value != null) {
      try {
        validateRuntime(value);
        record = value;
        success = true;
      } catch (_) {
        error = 'The action was accepted. Refresh to load its latest status.';
      }
    }
    error ??= attempt.error;
    busy = false;
    stale = !success;
    emit();
    return success;
  }

  Future<bool> setAvailability(String branch, String state) async {
    if (busy ||
        loading ||
        stale && attempt.pending == null ||
        attempt.blocked ||
        !['AVAILABLE', 'OFFLINE'].contains(state)) {
      return false;
    }
    if (attempt.pending == null && !branches.any((b) => b['id'] == branch)) {
      return false;
    }
    busy = true;
    error = null;
    emit();
    final value = await attempt.send(
      method: 'PUT',
      path: '/api/v1/vendor/delivery/rider/availability',
      params: {},
      version: availability?['version'] as int? ?? 0,
      body: {'branch_id': branch, 'state': state},
      validateResponse: validateAvailability,
    );
    final success = value != null;
    if (success) availability = value;
    error = attempt.error;
    busy = false;
    stale = !success;
    emit();
    return success;
  }

  @override
  void dispose() {
    disposed = true;
    super.dispose();
  }
}
