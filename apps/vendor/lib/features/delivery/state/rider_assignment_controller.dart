import 'package:flutter/foundation.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../settings/state/settings_controller.dart';
import 'delivery_attempt.dart';

class RiderAssignmentController extends ChangeNotifier {
  RiderAssignmentController(VendorRuntime runtime, this.mission)
    : attempt = DeliveryAttempt(
        runtime,
        'assign:${mission['vendor_mission_id']}',
      );
  final JsonObject mission;
  final DeliveryAttempt attempt;
  List<JsonObject> riders = [];
  bool loading = false, busy = false, disposed = false, initialized = false;
  String? error;
  void emit() {
    if (!disposed) notifyListeners();
  }

  Future<void> load() async {
    if (loading || busy) return;
    loading = true;
    error = null;
    emit();
    final settings = SettingsController(attempt.app, SettingsKind.staff);
    try {
      if (!initialized) {
        await attempt.restore();
        initialized = true;
      }
      await settings.load();
      if (settings.error != null) throw StateError(settings.error!);
      final values = await Future.wait(
        settings.rows.where((s) => s['status'] == 'active').map((staff) async {
          final r = await attempt.app.accessApi.getStaffIdRoles(
            id: staff['id'] as String,
          );
          final assignments = SettingsController.objects(r.data, 'assignments');
          return assignments.any(
                (a) =>
                    a['role_id'] == 'role_vendor_rider' &&
                    a['revoked_at'] == null &&
                    (a['branch_id'] == null ||
                        a['branch_id'] == mission['branch_id']),
              )
              ? staff
              : null;
        }),
      );
      riders = values.whereType<JsonObject>().toList();
    } catch (e) {
      error = SettingsController.message(e);
      riders = [];
    } finally {
      settings.dispose();
      loading = false;
      emit();
    }
  }

  Future<bool> assign(String? staff) async {
    if (busy ||
        loading ||
        attempt.blocked ||
        attempt.pending == null && !riders.any((r) => r['id'] == staff)) {
      return false;
    }
    busy = true;
    error = null;
    emit();
    final response = await attempt.send(
      method: 'PUT',
      path: '/api/v1/vendor/fulfilment/vendor-missions/{missionId}/rider',
      params: {'missionId': mission['vendor_mission_id'] as String},
      version: mission['version'] as int,
      body: {'staff_id': staff},
      validateResponse: (v) {
        if (v['vendor_mission_id'] != mission['vendor_mission_id'] ||
            v['business_id'] != mission['business_id'] ||
            v['branch_id'] != mission['branch_id'] ||
            v['assigned_staff_id'] != attempt.pending!['body']['staff_id'] ||
            v['version'] is! int) {
          throw const FormatException('Incomplete assignment response');
        }
      },
    );
    error = attempt.error;
    busy = false;
    emit();
    return response != null;
  }

  @override
  void dispose() {
    disposed = true;
    super.dispose();
  }
}
