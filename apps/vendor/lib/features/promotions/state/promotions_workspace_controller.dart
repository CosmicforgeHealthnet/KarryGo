import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_command.dart';
import 'promotion_rules.dart';

class PromotionsWorkspaceController extends ChangeNotifier {
  PromotionsWorkspaceController(this.runtime, {this.id});
  final VendorRuntime runtime;
  final String? id;
  late final app = runtime.backend;
  late final storage = runtime.mode == VendorDataMode.mock
      ? runtime.mockSupportPreferences
      : runtime.preferences;
  late final recoveryKey =
      'vendor.promotions.${base64Url.encode(utf8.encode('${runtime.mode.name}|${runtime.mode == VendorDataMode.mock ? runtime.revision : app.client.config.origin}|${app.scope.vendorId}|${app.auth.session?.staffId}'))}';
  List<JsonObject> rows = [];
  JsonObject? record, pending;
  int? used;
  bool loading = false,
      busy = false,
      stale = true,
      blocked = false,
      disposed = false;
  String? error, usageError;
  final denied = <String>{};
  bool get writable =>
      !loading && !busy && !stale && !blocked && pending == null;
  void emit() {
    if (!disposed) notifyListeners();
  }

  void validateRecord(JsonObject value, [String? expectedId]) {
    if (value['id'] is! String ||
        value['business_id'] != app.scope.vendorId ||
        value['version'] is! int ||
        value['version'] < 1 ||
        expectedId != null && value['id'] != expectedId ||
        !promotionTypes.containsKey(value['promotion_type']) ||
        !promotionTransitions.containsKey(value['status'])) {
      throw const FormatException('Incomplete or out-of-scope promotion');
    }
  }

  Future<void> restore() async {
    try {
      final raw = await storage.read(recoveryKey);
      if (raw == null || raw == '{}') {
        pending = null;
        blocked = false;
        return;
      }
      final value = jsonObject(jsonDecode(raw));
      if (value['key'] is! String ||
          value['body'] is! Map ||
          value['params'] is! Map ||
          value['method'] is! String ||
          value['path'] is! String) {
        throw const FormatException('Invalid saved update');
      }
      final op = app.promotionsApi.operation(value['method'], value['path']);
      if (!['POST', 'PATCH'].contains(op.method) ||
          !op.requiresHeader('Idempotency-Key') ||
          op.requiresHeader('If-Match') &&
              (value['version'] is! int || value['version'] < 1)) {
        throw const FormatException('Invalid saved operation');
      }
      pending = value;
      blocked = false;
    } catch (_) {
      blocked = true;
      error =
          'Your previous update could not be restored. Reopen Promotions before making changes.';
    }
  }

  Future<void> load() async {
    if (loading || busy || disposed) return;
    loading = true;
    stale = true;
    error = null;
    emit();
    try {
      await restore();
      if (id == null) {
        final response = await app.promotionsApi.getPromotions();
        final list = response.object['promotions'];
        if (list != null && list is! List) {
          throw const FormatException('Invalid promotion list');
        }
        final next = (list as List? ?? []).map(jsonObject).toList();
        for (final row in next) {
          validateRecord(row);
        }
        rows = next;
      } else {
        final response = await app.promotionsApi.getPromotionsPromotionId(
          promotionId: id!,
        );
        validateRecord(response.object, id);
        record = response.object;
        usageError = null;
        used = null;
        try {
          final usage = await app.promotionsApi.getPromotionsPromotionIdUsage(
            promotionId: id!,
          );
          if (usage.object['used'] is! int || usage.object['used'] < 0) {
            throw const FormatException('Invalid usage response');
          }
          used = usage.object['used'];
        } catch (e) {
          usageError = e is ApiException
              ? e.message
              : 'Usage could not be loaded.';
        }
      }
      stale = false;
    } catch (e) {
      error = e is ApiException
          ? e.message
          : 'Promotions could not be loaded. Try again.';
    } finally {
      loading = false;
      emit();
    }
  }

  bool allowed(String method, String path) =>
      writable && !denied.contains('$method $path');

  Future<JsonObject?> save({
    String? method,
    String? path,
    JsonObject? body,
    String? promotionId,
    int? version,
  }) async {
    if (pending != null && method != null) return null;
    if (busy ||
        loading ||
        blocked ||
        disposed ||
        pending == null && !writable) {
      return null;
    }
    if (pending == null && denied.contains('$method $path')) return null;
    busy = true;
    error = null;
    emit();
    try {
      if (pending == null) {
        final value = <String, dynamic>{
          'method': method!,
          'path': path!,
          'body': jsonObject(jsonDecode(jsonEncode(body!))),
          'params': {'promotionId': ?promotionId},
          'version': version,
          'key': VendorCommand.create().idempotencyKey,
        };
        await storage.write(recoveryKey, jsonEncode(value));
        pending = value;
      }
      final attempt = pending!;
      final op = app.promotionsApi.operation(
        attempt['method'],
        attempt['path'],
      );
      final response = await app.promotionsApi.request(
        attempt['method'],
        attempt['path'],
        pathParameters: Map<String, String>.from(attempt['params']),
        body: VendorPayload(jsonObject(attempt['body'])),
        command: VendorCommand(
          idempotencyKey: attempt['key'],
          ifMatch: op.requiresHeader('If-Match')
              ? '${attempt['version']}'
              : null,
        ),
      );
      final result = response.object;
      if ((attempt['path'] as String).endsWith('/coupons')) {
        if (result['coupon_id'] is! String ||
            (result['coupon_id'] as String).isEmpty) {
          throw const FormatException('Incomplete coupon confirmation');
        }
      } else {
        validateRecord(result, attempt['params']['promotionId']);
      }
      try {
        await storage.write(recoveryKey, '{}');
        pending = null;
      } catch (_) {
        blocked = true;
        error = 'Saved on the server. Reopen Promotions to finish recovery.';
      }
      return result;
    } catch (e) {
      error = e is ApiException
          ? e.message
          : pending == null
          ? 'Recovery details could not be saved. Nothing was submitted.'
          : 'The result could not be confirmed. Retry the original update.';
      if (e is ApiException &&
          [400, 401, 403, 404, 409, 412, 422, 429].contains(e.statusCode)) {
        if (e.statusCode == 403 && pending != null) {
          denied.add('${pending!['method']} ${pending!['path']}');
        }
        try {
          await storage.write(recoveryKey, '{}');
          pending = null;
        } catch (_) {
          blocked = true;
        }
        if ([409, 412].contains(e.statusCode)) stale = true;
      }
      return null;
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
