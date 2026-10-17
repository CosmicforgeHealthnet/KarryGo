import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../app/vendor_app.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_command.dart';

const readinessTargets = {
  'estimate': 'PREPARING',
  'ready-soon': 'READY_SOON',
  'ready': 'READY',
  'delay': 'DELAYED',
  'block': 'BLOCKED',
};
const readinessTransitions = <String, List<String>>{
  'NOT_STARTED': ['ACCEPTED', 'CANCELLED', 'BLOCKED'],
  'ACCEPTED': ['PREPARING', 'BLOCKED', 'DELAYED', 'CANCELLED'],
  'PREPARING': ['READY_SOON', 'READY', 'BLOCKED', 'DELAYED', 'CANCELLED'],
  'READY_SOON': ['READY', 'BLOCKED', 'DELAYED', 'CANCELLED'],
  'READY': ['DELAYED', 'BLOCKED', 'CANCELLED'],
  'BLOCKED': [
    'ACCEPTED',
    'PREPARING',
    'READY_SOON',
    'READY',
    'DELAYED',
    'CANCELLED',
  ],
  'DELAYED': ['PREPARING', 'READY_SOON', 'READY', 'BLOCKED', 'CANCELLED'],
  'CANCELLED': [],
};
const readinessSubstates = <String, List<String>>{
  'RESTAURANT': ['COOKING', 'ASSEMBLING', 'PACKING'],
  'FAST_FOOD': ['COOKING', 'ASSEMBLING', 'PACKING'],
  'CLOUD_KITCHEN': ['QUEUE', 'PRODUCTION', 'BATCH_COMPLETION', 'PACKING'],
  'PASTRY_BAKERY': [
    'PRODUCTION',
    'COOLING_CHILLING',
    'DECORATING',
    'PACKAGING',
  ],
  'SHOP_GROCERY': [
    'PICKING',
    'SUBSTITUTION_REQUIRED',
    'CUSTOMER_DECISION_WAIT',
    'PACKING',
  ],
  'MARKET': ['PICKING', 'WEIGHING', 'CUSTOMER_APPROVAL_WAIT', 'PACKING'],
  'WHOLESALE': ['PICKING', 'STAGING', 'LOADING_PREP', 'LOAD_READY'],
  'CATERING': ['PRODUCTION_BATCHES', 'STAGING', 'LOAD_READY'],
  'MANUFACTURER': [
    'PRODUCTION_COMPLETE',
    'WAREHOUSE_RELEASE',
    'LOADING_BAY_READY',
  ],
};
bool readinessAllows(String from, String to) =>
    readinessTransitions.containsKey(from) &&
    (from == to || readinessTransitions[from]!.contains(to));

class OrderReadinessController extends ChangeNotifier {
  OrderReadinessController(this.app, this.order);
  final VendorApp app;
  final JsonObject order;
  JsonObject? record;
  List<JsonObject> history = [];
  List<String> types = [];
  bool loading = false,
      busy = false,
      disposed = false,
      stale = true,
      missing = false,
      denied = false;
  String? error, historyError, typeError;
  String get id => order['vendor_order_id'] as String;
  String get path => '/api/v1/vendor/commerce/orders/{vendorOrderId}/readiness';
  bool get enabled => !loading && !busy && !stale && !denied;
  List<String> get actions => !enabled
      ? []
      : record == null
      ? (missing && types.isNotEmpty ? ['start'] : [])
      : readinessTargets.entries
            .where((e) => readinessAllows(record!['state'] as String, e.value))
            .map((e) => e.key)
            .toList();
  void emit() {
    if (!disposed) notifyListeners();
  }

  void validate(JsonObject value) {
    if (value['vendor_order_id'] != id ||
        value['branch_id'] != order['branch_id'] ||
        value['business_id'] != order['business_id'] ||
        !readinessTransitions.containsKey(value['state']) ||
        value['readiness_version'] is! int ||
        value['readiness_version'] < 1 ||
        !readinessSubstates.containsKey(value['vendor_type'])) {
      throw const FormatException('Incomplete preparation response');
    }
  }

  Future<void> load() async {
    if (busy || loading || disposed) return;
    loading = true;
    stale = true;
    error = null;
    historyError = null;
    missing = false;
    emit();
    try {
      try {
        final response = await app.readinessApi.request(
          'GET',
          path,
          pathParameters: {'vendorOrderId': id},
        );
        final value = response.object;
        validate(value);
        record = value;
      } on ApiException catch (e) {
        if (e.statusCode == 404 && e.code == 'READINESS_NOT_FOUND') {
          record = null;
          missing = true;
        } else {
          rethrow;
        }
      }
      if (missing) {
        types = [];
        typeError = null;
        try {
          final response = await app.businessApi
              .getBusinessesBusinessIdBranchesBranchIdCapabilities(
                businessId: order['business_id'] as String,
                branchId: order['branch_id'] as String,
              );
          final raw = response.data is List
              ? response.data
              : response.object['items'];
          if (raw is! List) {
            throw const FormatException('Incomplete branch capabilities');
          }
          types = raw
              .map(jsonObject)
              .where((v) => v['status'] == 'ACTIVE')
              .map((v) => v['capability_type'].toString())
              .where(readinessSubstates.containsKey)
              .toSet()
              .toList();
        } catch (_) {
          typeError =
              'Could not load the branch?s business types. Refresh to start preparation.';
        }
      } else {
        try {
          final response = await app.readinessApi.request(
            'GET',
            '$path/history',
            pathParameters: {'vendorOrderId': id},
          );
          final raw = response.object['history'];
          if (raw is! List) throw const FormatException('Incomplete history');
          history = raw.map(jsonObject).toList();
        } catch (_) {
          historyError = 'Preparation history could not be loaded.';
        }
      }
      stale = false;
      denied = false;
    } catch (e) {
      error = e is ApiException
          ? e.message
          : e is FormatException
          ? 'We could not read the latest details. Pull to refresh and try again.'
          : 'Could not connect. Refresh to continue.';
    } finally {
      loading = false;
      emit();
    }
  }

  Future<bool> save(
    String action, {
    String? type,
    String? substate,
    String? reason,
    DateTime? estimate,
  }) async {
    if (!actions.contains(action)) return false;
    if (action == 'start' &&
        (!types.contains(type) || order['parent_order_id'] is! String)) {
      return false;
    }
    if (['delay', 'block'].contains(action) &&
        (reason == null || reason.trim().isEmpty)) {
      return false;
    }
    if (substate != null &&
        !(readinessSubstates[record?['vendor_type']] ?? []).contains(
          substate,
        )) {
      return false;
    }
    busy = true;
    error = null;
    emit();
    var success = false;
    try {
      final response = await app.readinessApi.request(
        'POST',
        '$path/$action',
        pathParameters: {'vendorOrderId': id},
        command: action == 'start'
            ? null
            : VendorCommand(ifMatch: record!['readiness_version'].toString()),
        body: VendorPayload({
          if (action == 'start') ...{
            'parent_order_id': order['parent_order_id'],
            'branch_id': order['branch_id'],
            'vendor_type': type,
          },
          'substate': ?substate,
          if (reason != null && reason.trim().isNotEmpty)
            'reason_code': reason.trim(),
          if (estimate != null)
            'estimated_ready_at': estimate.toUtc().toIso8601String(),
        }),
      );
      final value = response.object;
      validate(value);
      record = value;
      missing = false;
      success = true;
    } catch (e) {
      stale = true;
      denied = e is ApiException && e.statusCode == 403;
      error = e is ApiException
          ? e.message
          : 'The result could not be confirmed. Refresh before making another update.';
    } finally {
      busy = false;
      emit();
    }
    if (success) await load();
    return success;
  }

  @override
  void dispose() {
    disposed = true;
    super.dispose();
  }
}
