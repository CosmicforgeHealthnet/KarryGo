import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_command.dart';

const schedulingBase = '/api/v1/vendor/commerce/section21';

class SchedulingWorkspaceController extends ChangeNotifier {
  SchedulingWorkspaceController(
    this.runtime, {
    this.recurring = false,
    this.id,
  });
  final VendorRuntime runtime;
  final bool recurring;
  final String? id;
  late final app = runtime.backend;
  List<JsonObject> rows = [], occurrences = [];
  JsonObject? record;
  bool loading = false,
      busy = false,
      stale = true,
      hasMore = false,
      disposed = false;
  String? error, occurrenceError;
  String view = 'ALL';

  Future<void> selectView(String next) async {
    if (loading || busy || disposed || next == view) return;
    view = next;
    rows = [];
    hasMore = false;
    await load();
  }

  String get collection =>
      '$schedulingBase/${recurring ? 'recurring-schedules' : 'scheduled-orders'}';
  String get detail => '$collection/{${recurring ? 'scheduleId' : 'id'}}';
  Map<String, String> get params => {recurring ? 'scheduleId' : 'id': id!};
  String get status =>
      record?[recurring ? 'Status' : 'SchedulingStatus']?.toString() ?? '';
  bool get writable =>
      !loading &&
      !busy &&
      !stale &&
      record != null &&
      !(recurring ? ['CANCELLED', 'ENDED'] : ['CANCELLED']).contains(status);
  void emit() {
    if (!disposed) notifyListeners();
  }

  void validate(JsonObject value) {
    if (value['ID'] is! String ||
        value['BusinessID'] != app.scope.vendorId ||
        value['BranchID'] is! String ||
        id != null && value['ID'] != id ||
        recurring && value['Version'] is! int) {
      throw const FormatException(
        'Incomplete or out-of-scope schedule response',
      );
    }
  }

  Future<void> load({bool more = false}) async {
    if (loading || busy || disposed) return;
    loading = true;
    stale = true;
    error = null;
    emit();
    try {
      if (id == null) {
        final response = await app.scheduledCommerceApi.request(
          'GET',
          collection,
          query: {
            'limit': '50',
            'offset': more ? '${rows.length}' : '0',
            'view': view,
            if (app.scope.branchId != null) 'branch_id': app.scope.branchId!,
          },
        );
        final list = response.object['items'];
        final pagination = response.object['pagination'];
        if (list is! List ||
            pagination is! Map ||
            pagination['has_more'] is! bool) {
          throw const FormatException('Incomplete schedule list');
        }
        final next = list.map(jsonObject).toList();
        for (final value in next) {
          validate(value);
        }
        rows = more ? [...rows, ...next] : next;
        hasMore = pagination['has_more'];
      } else {
        final response = await app.scheduledCommerceApi.request(
          'GET',
          detail,
          pathParameters: params,
        );
        validate(response.object);
        record = response.object;
        occurrenceError = null;
        if (recurring) {
          try {
            final result = await app.scheduledCommerceApi.request(
              'GET',
              '$detail/occurrences',
              pathParameters: params,
            );
            final list = result.data;
            if (list is! List) {
              throw const FormatException('Incomplete occurrence list');
            }
            occurrences = list.map(jsonObject).toList();
            if (occurrences.any(
              (o) =>
                  o['ScheduleID'] != id ||
                  o['BusinessID'] != app.scope.vendorId,
            )) {
              throw const FormatException('Out-of-scope occurrence response');
            }
          } catch (e) {
            occurrenceError = message(e);
            occurrences = [];
          }
        }
      }
      stale = false;
    } catch (e) {
      error = message(e);
      if (e is ApiException && [403, 404].contains(e.statusCode)) {
        rows = [];
        record = null;
        occurrences = [];
      }
    } finally {
      loading = false;
      emit();
    }
  }

  Future<bool> changeStatus(String target) async {
    if (!writable) return false;
    if (recurring &&
            !['ACTIVE', 'PAUSED', 'CANCELLED', 'ENDED'].contains(target) ||
        !recurring && target != 'CANCELLED') {
      return false;
    }
    if (target == status) return true;
    return write(
      'POST',
      '$detail/${recurring ? 'status' : 'cancel'}',
      recurring
          ? {'status': target, 'expected_version': record!['Version']}
          : null,
    );
  }

  Future<bool> edit(JsonObject changes) async {
    if (!recurring || !writable) return false;
    final current = record!;
    final body = <String, dynamic>{
      for (final name in [
        'RequesterID',
        'CorporateRef',
        'BranchID',
        'Weekdays',
        'MealTime',
        'Timezone',
        'Headcount',
        'AllowanceMinor',
        'DestinationSnapshot',
        'MenuRotation',
        'ProductionRequirement',
        'TentativeVehicleClass',
        'AdvanceHorizonDays',
        'StartsOn',
        'EndsOn',
      ])
        if (current.containsKey(name)) name: current[name],
      ...changes,
      'IdempotencyKey': VendorCommand.create().idempotencyKey,
    };
    return write('PUT', detail, body, version: current['Version'] as int);
  }

  Future<bool> write(
    String method,
    String path,
    JsonObject? body, {
    int? version,
  }) async {
    busy = true;
    error = null;
    emit();
    var accepted = false;
    try {
      await app.scheduledCommerceApi.request(
        method,
        path,
        pathParameters: params,
        body: body == null ? null : VendorPayload(body),
        command: version == null ? null : VendorCommand(ifMatch: '$version'),
      );
      accepted = true;
    } catch (e) {
      error = message(e);
      stale = true;
    } finally {
      busy = false;
      emit();
    }
    if (accepted) {
      await load();
      if (stale) {
        error =
            'The change was accepted, but the latest schedule could not be loaded. Refresh before continuing.';
        emit();
      }
    }
    return accepted && !stale;
  }

  static String message(Object e) => e is ApiException
      ? e.message
      : e is FormatException
      ? e.message
      : 'Could not connect. Refresh to check the latest state before trying again.';
  @override
  void dispose() {
    disposed = true;
    super.dispose();
  }
}
