import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../app/vendor_app.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_command.dart';

const vendorMissionStaffTransitions = <String, List<String>>{
  'PLANNED': [],
  'ASSIGNED': ['READY_FOR_PICKUP', 'FAILED'],
  'READY_FOR_PICKUP': ['FAILED', 'RETURN_REQUIRED'],
  'CUSTODY_TRANSFERRED': ['OUT_FOR_DELIVERY', 'RETURN_REQUIRED'],
  'OUT_FOR_DELIVERY': ['FAILED', 'RETURN_REQUIRED'],
  'RETURN_REQUIRED': [],
  'DELIVERED': [],
  'FAILED': [],
  'RETURNED': [],
};

class VendorMissionsController extends ChangeNotifier {
  VendorMissionsController(this.app, {this.id});
  final VendorApp app;
  final String? id;
  static const path = '/api/v1/vendor/fulfilment/vendor-missions';
  List<JsonObject> missions = [];
  JsonObject? record;
  bool loading = false,
      busy = false,
      stale = true,
      disposed = false,
      denied = false;
  String? error;
  List<String> get actions =>
      loading || busy || stale || denied || record?['assigned_staff_id'] == null
      ? []
      : vendorMissionStaffTransitions[record?['status']] ?? [];
  void emit() {
    if (!disposed) notifyListeners();
  }

  void validate(JsonObject value) {
    if (value['vendor_mission_id'] is! String ||
        value['business_id'] != app.scope.vendorId ||
        value['version'] is! int ||
        value['version'] < 1 ||
        !vendorMissionStaffTransitions.containsKey(value['status']) ||
        id != null && value['vendor_mission_id'] != id) {
      throw const FormatException('Incomplete delivery response');
    }
  }

  Future<void> load() async {
    if (loading || busy || disposed) return;
    loading = true;
    stale = true;
    error = null;
    emit();
    try {
      final response = await app.deliveryApi.request(
        'GET',
        id == null ? path : '$path/{missionId}',
        pathParameters: id == null ? {} : {'missionId': id!},
        query: id == null ? {'limit': '200'} : {},
      );
      if (id == null) {
        final values = response.object['vendor_missions'];
        if (values is! List) {
          throw const FormatException('Incomplete delivery response');
        }
        final rows = values.map(jsonObject).toList();
        for (final row in rows) {
          validate(row);
        }
        missions = rows;
      } else {
        final value = response.object;
        validate(value);
        record = value;
      }
      stale = false;
      denied = false;
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

  Future<bool> update(String status, {String? reason}) async {
    if (!actions.contains(status) ||
        status == 'FAILED' && (reason == null || reason.trim().isEmpty)) {
      return false;
    }
    busy = true;
    error = null;
    emit();
    var success = false;
    try {
      final response = await app.deliveryApi.request(
        'PUT',
        '$path/{missionId}/status',
        pathParameters: {'missionId': id!},
        command: VendorCommand(ifMatch: record!['version'].toString()),
        body: VendorPayload({
          'status': status,
          if (status == 'FAILED') 'failure_reason': reason!.trim(),
        }),
      );
      final value = response.object;
      validate(value);
      record = value;
      success = true;
    } catch (e) {
      stale = true;
      denied = e is ApiException && e.statusCode == 403;
      error = e is ApiException
          ? e.message
          : 'The delivery update could not be confirmed. Refresh before continuing.';
    } finally {
      busy = false;
      emit();
    }
    return success;
  }

  @override
  void dispose() {
    disposed = true;
    super.dispose();
  }
}
