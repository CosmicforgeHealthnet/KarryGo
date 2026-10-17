import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import 'delivery_attempt.dart';

class FulfilmentWorkspaceController extends ChangeNotifier {
  FulfilmentWorkspaceController(
    VendorRuntime runtime,
    this.groupId, {
    this.parcelId,
  }) : attempt = DeliveryAttempt(
         runtime,
         'fulfilment:$groupId:${parcelId ?? 'group'}',
       );
  final String groupId;
  final String? parcelId;
  final DeliveryAttempt attempt;
  JsonObject? view, parcel;
  List<JsonObject> custody = [], plans = [];
  bool loading = false,
      busy = false,
      stale = true,
      initialized = false,
      disposed = false;
  String? error, historyError;
  void emit() {
    if (!disposed) notifyListeners();
  }

  List<JsonObject> get units =>
      (view?['group']?['units'] as List? ?? []).map(jsonObject).toList();
  List<JsonObject> get parcels =>
      (view?['group']?['parcels'] as List? ?? []).map(jsonObject).toList();
  bool get canWrite =>
      !busy &&
      !loading &&
      !stale &&
      !attempt.blocked &&
      attempt.pending == null;
  bool get sealed =>
      parcel?['manifest']?['sealed_at'] != null ||
      !['CREATED', 'PACKED', 'READY', 'CANCELLED'].contains(parcel?['status']);
  Future<void> load() async {
    if (loading || busy) return;
    loading = true;
    stale = true;
    error = null;
    historyError = null;
    emit();
    try {
      if (!initialized) {
        await attempt.restore();
        initialized = true;
      }
      if (parcelId != null) {
        final r = await attempt.app.deliveryApi.request(
          'GET',
          '/api/v1/vendor/fulfilment/parcels/{parcelId}',
          pathParameters: {'parcelId': parcelId!},
        );
        if (r.object['parcel_id'] != parcelId ||
            r.object['business_id'] != attempt.app.scope.vendorId ||
            r.object['version'] is! int) {
          throw const FormatException('Incomplete parcel response');
        }
        parcel = r.object;
        try {
          final r = await attempt.app.deliveryApi.request(
            'GET',
            '/api/v1/vendor/fulfilment/parcels/{parcelId}/custody',
            pathParameters: {'parcelId': parcelId!},
          );
          final rows = r.object['custody_events'];
          if (rows is! List) {
            throw const FormatException('Incomplete custody history');
          }
          custody = rows.map(jsonObject).toList();
        } catch (_) {
          historyError = 'Custody history could not be loaded.';
        }
      } else {
        final r = await attempt.app.deliveryApi.request(
          'GET',
          '/api/v1/vendor/fulfilment/groups/{groupId}',
          pathParameters: {'groupId': groupId},
        );
        if (r.object['group']?['fulfilment_group_id'] != groupId) {
          throw const FormatException('Incomplete fulfilment response');
        }
        view = r.object;
        if (units.any((u) => u['business_id'] != attempt.app.scope.vendorId) ||
            parcels.any(
              (p) => p['business_id'] != attempt.app.scope.vendorId,
            )) {
          throw const FormatException('Unexpected fulfilment scope');
        }
        try {
          final r = await attempt.app.deliveryApi.request(
            'GET',
            '/api/v1/vendor/fulfilment/groups/{groupId}/plans',
            pathParameters: {'groupId': groupId},
          );
          final rows = r.object['plans'];
          if (rows is! List) {
            throw const FormatException('Incomplete plan history');
          }
          plans = rows.map(jsonObject).toList();
        } catch (_) {
          historyError = 'Plan history could not be loaded.';
        }
      }
      stale = false;
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

  Future<JsonObject> order(String id) async =>
      (await attempt.app.ordersApi.getCommerceOrdersVendorOrderId(
        vendorOrderId: id,
      )).object;
  Future<bool> command({
    required String method,
    required String path,
    required Map<String, String> params,
    required JsonObject body,
    int version = 0,
  }) async {
    if (busy ||
        loading ||
        attempt.blocked ||
        attempt.pending == null && !canWrite) {
      return false;
    }
    busy = true;
    error = null;
    emit();
    final r = await attempt.send(
      method: method,
      path: path,
      params: params,
      body: body,
      version: version,
      validateResponse: (v) {
        if (v['version'] is! int) {
          throw const FormatException('Incomplete fulfilment update response');
        }
      },
    );
    final ok = r != null;
    error = attempt.error;
    busy = false;
    stale = !ok;
    emit();
    if (ok) await load();
    return ok;
  }

  Future<bool> retry() async {
    final pending = attempt.pending;
    if (pending == null) return false;
    return command(
      method: pending['method'] as String,
      path: pending['path'] as String,
      params: Map<String, String>.from(pending['params'] as Map),
      body: jsonObject(pending['body']),
      version: pending['version'] as int,
    );
  }

  @override
  void dispose() {
    disposed = true;
    super.dispose();
  }
}
