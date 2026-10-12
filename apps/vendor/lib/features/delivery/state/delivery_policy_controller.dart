import 'package:flutter/foundation.dart';
import 'dart:convert';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import 'delivery_attempt.dart';
import 'delivery_policy_rules.dart';

class DeliveryPolicyController extends ChangeNotifier {
  DeliveryPolicyController(VendorRuntime runtime, this.branch)
    : attempt = DeliveryAttempt(runtime, 'policy:${branch['id']}');
  final JsonObject branch;
  final DeliveryAttempt attempt;
  JsonObject? record, business;
  bool loading = false,
      busy = false,
      stale = true,
      initialized = false,
      disposed = false;
  String? error;
  static const path = '/api/v1/vendor/delivery/branches/{branchId}/policy';
  bool get canWrite =>
      !loading &&
      !busy &&
      !stale &&
      !attempt.blocked &&
      attempt.pending == null &&
      (record?['hybrid_fallback_funding_sponsor'] == null ||
          record?['hybrid_fallback_funding_sponsor'] == 'VENDOR');
  void emit() {
    if (!disposed) notifyListeners();
  }

  void validate(JsonObject v) {
    if (v['branch_id'] != branch['id'] ||
        v['version'] is! int ||
        !['PLATFORM', 'VENDOR', 'HYBRID'].contains(v['delivery_mode']) ||
        v['currency'] is! String) {
      throw const FormatException('Incomplete delivery policy response');
    }
    if (validateAdvancedPolicy(v) != null) {
      throw const FormatException(
        'Incomplete advanced delivery policy response',
      );
    }
  }

  Future<void> load() async {
    if (loading || busy) return;
    loading = true;
    stale = true;
    error = null;
    emit();
    try {
      if (!initialized) {
        await attempt.restore();
        initialized = true;
      }
      try {
        final r = await attempt.app.deliveryApi.request(
          'GET',
          path,
          pathParameters: {'branchId': branch['id'] as String},
        );
        validate(r.object);
        record = r.object;
      } on ApiException catch (e) {
        if (e.statusCode == 404 && e.code == 'DELIVERY_POLICY_NOT_FOUND') {
          record = null;
        } else {
          rethrow;
        }
      }
      if (record == null) {
        try {
          business = (await attempt.app.businessApi.request(
            'GET',
            '/api/v1/vendor/businesses/{businessId}',
            pathParameters: {'businessId': attempt.app.scope.vendorId!},
          )).object;
        } catch (_) {
          business = null;
        }
      }
      stale = false;
    } catch (e) {
      error = e is ApiException
          ? e.message
          : 'Delivery policy could not be loaded. Refresh to continue.';
    } finally {
      loading = false;
      emit();
    }
  }

  JsonObject bodyWith(JsonObject values) {
    final props = jsonObject(
      attempt.app.deliveryApi
          .operation('PUT', path)
          .requestSchema!['properties'],
    );
    return jsonObject(
      jsonDecode(
        jsonEncode({
          if (record != null)
            for (final e in record!.entries)
              if (props.containsKey(e.key)) e.key: e.value,
          ...values,
        }),
      ),
    );
  }

  Future<bool> save(JsonObject values) async {
    if (loading ||
        busy ||
        attempt.blocked ||
        attempt.pending == null && !canWrite) {
      return false;
    }
    final payload = bodyWith(values);
    if (attempt.pending == null) {
      final validation = validateDeliveryPolicy(payload);
      if (validation != null) {
        error = validation;
        emit();
        return false;
      }
    }
    busy = true;
    error = null;
    emit();
    final r = await attempt.send(
      method: 'PUT',
      path: path,
      params: {'branchId': branch['id'] as String},
      version: record?['version'] as int? ?? 0,
      body: payload,
      validateResponse: validate,
    );
    if (r != null) record = r;
    error = attempt.error;
    busy = false;
    stale = r == null;
    emit();
    return r != null && !attempt.blocked;
  }

  @override
  void dispose() {
    disposed = true;
    super.dispose();
  }
}
