import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_command.dart';

const purchasingBase = '/api/v1/vendor/purchasing';
const purchaseOrderActions = <String, List<String>>{
  'DRAFT': ['submit', 'cancel'],
  'SUBMITTED': ['approve', 'reject', 'cancel'],
  'APPROVED': ['order', 'cancel'],
  'ORDERED': ['cancel'],
  'RECEIVED': ['close'],
};
const receiptActions = <String, List<String>>{
  'DRAFT': ['inspection'],
  'INSPECTED': ['inspection', 'confirm'],
  'POSTED': ['reverse'],
};
const discrepancyActions = <String, List<String>>{
  'OPEN': ['approve', 'reject', 'resolve'],
  'APPROVAL_REQUIRED': ['approve', 'reject'],
  'APPROVED': ['resolve'],
  'REJECTED': ['resolve'],
};

class PurchasingWorkspaceController extends ChangeNotifier {
  PurchasingWorkspaceController(
    this.runtime,
    this.path, {
    this.parameters = const {},
    this.branchId,
    this.wrapper,
  });
  final VendorRuntime runtime;
  final String path;
  final Map<String, String> parameters;
  final String? branchId, wrapper;
  late final app = runtime.backend;
  late final storage = runtime.mode == VendorDataMode.mock
      ? runtime.mockSupportPreferences
      : runtime.preferences;
  late final recoveryKey =
      'vendor.purchasing.${base64Url.encode(utf8.encode('${runtime.mode.name}|${runtime.mode == VendorDataMode.mock ? runtime.revision : app.client.config.origin}|${app.scope.vendorId}|${app.auth.session?.staffId}'))}';
  Object? data;
  JsonObject? pending;
  List<JsonObject> rows = [];
  JsonObject? record;
  bool loading = false,
      busy = false,
      stale = true,
      blocked = false,
      disposed = false,
      hasMore = false;
  final denied = <String>{};
  String? error;
  bool get writable =>
      !loading && !busy && !stale && !blocked && pending == null;
  bool allowed(String method, String route) =>
      writable && !denied.contains('$method $route');
  void emit() {
    if (!disposed) notifyListeners();
  }

  List<JsonObject> collection(Object? value) {
    if (value == null) return [];
    if (value is! List) throw const FormatException('Invalid purchasing list');
    return value.map(jsonObject).toList();
  }

  void validate(JsonObject r, {bool checkBranch = true}) {
    if (r['business_id'] != null && r['business_id'] != app.scope.vendorId ||
        checkBranch &&
            branchId != null &&
            r['branch_id'] != null &&
            r['branch_id'] != branchId) {
      throw const FormatException('Out-of-scope purchasing record');
    }
    if (r['version'] != null && (r['version'] is! int || r['version'] < 1)) {
      throw const FormatException('Invalid purchasing version');
    }
  }

  Future<Object?> fetch(
    String route, {
    Map<String, String> params = const {},
    Map<String, String> query = const {},
  }) async => (await app.purchasingApi.request(
    'GET',
    route,
    pathParameters: {
      for (final entry in params.entries)
        if (route.contains("{${entry.key}}")) entry.key: entry.value,
    },
    query: query,
  )).data;
  Future<void> restore() async {
    try {
      final raw = await storage.read(recoveryKey);
      if (raw == null || raw == '{}') {
        pending = null;
        blocked = false;
        return;
      }
      final saved = jsonObject(jsonDecode(raw));
      if (saved['key'] is! String ||
          saved['path'] is! String ||
          saved['method'] is! String ||
          saved['params'] is! Map ||
          saved['body'] is! Map ||
          !['POST', 'PUT', 'PATCH'].contains(saved['method']) ||
          !(saved['path'] as String).startsWith(purchasingBase)) {
        throw const FormatException('Invalid saved purchasing change');
      }
      app.purchasingApi.operation(saved['method'], saved['path']);
      pending = saved;
      blocked = false;
    } catch (_) {
      blocked = true;
      error =
          'Your saved purchasing change could not be restored. Reopen Purchasing before editing.';
    }
  }

  Future<void> load({bool append = false, String? status}) async {
    if (loading || busy || disposed) return;
    loading = true;
    stale = true;
    error = null;
    emit();
    try {
      await restore();
      final response = await fetch(
        path,
        params: parameters,
        query: {
          'branch_id': ?branchId,
          if (status != null && status.isNotEmpty) 'status': status,
          if ([
                '$purchasingBase/suppliers',
                '$purchasingBase/supplier-categories',
                '$purchasingBase/purchase-orders',
              ].contains(path) ||
              path.endsWith('/products')) ...{
            'limit': '100',
            'offset': append ? '${rows.length}' : '0',
          },
          if (path.endsWith('/goods-receipts') ||
              path.endsWith('/supplier-discrepancies') ||
              path.endsWith('/purchase-cost-history'))
            'limit': '200',
        },
      );
      final value = wrapper == null ? response : jsonObject(response)[wrapper];
      if (value == null || value is List) {
        final next = collection(value);
        for (final r in next) {
          validate(r);
        }
        rows = append ? [...rows, ...next] : next;
        record = null;
        hasMore =
            next.length == 100 &&
            ([
                  '$purchasingBase/suppliers',
                  '$purchasingBase/supplier-categories',
                  '$purchasingBase/purchase-orders',
                ].contains(path) ||
                path.endsWith('/products'));
      } else {
        record = jsonObject(value);
        validate(record!);
        rows = [];
        hasMore = false;
      }
      data = value;
      stale = false;
    } catch (e) {
      final missing =
          e is ApiException &&
          e.statusCode == 404 &&
          (path.endsWith('/terms') &&
                  e.message.toLowerCase().contains('supplier terms') ||
              path.endsWith('/approval-policy') &&
                  e.message.toLowerCase().contains('approval policy'));
      if (missing) {
        record = null;
        rows = [];
        data = null;
        stale = false;
        error = null;
      } else {
        error = e is ApiException
            ? e.message
            : 'Purchasing could not be loaded. Refresh to try again.';
      }
    } finally {
      loading = false;
      emit();
    }
  }

  Future<JsonObject?> save({
    String? method,
    String? route,
    JsonObject? body,
    Map<String, String> params = const {},
    int? headerVersion,
  }) async {
    if (busy ||
        loading ||
        blocked ||
        disposed ||
        pending != null && method != null ||
        pending == null && !allowed(method!, route!)) {
      return null;
    }
    busy = true;
    error = null;
    emit();
    try {
      if (pending != null &&
          (pending!['path'] as String).contains('/alerts/')) {
        final saved = pending!;
        final result = jsonObject(
          await fetch(
            '$purchasingBase/alerts',
            query: {
              'limit': '200',
              if (saved['alert_branch'] != null)
                'branch_id': saved['alert_branch'],
            },
          ),
        );
        final alert = collection(result['alerts'])
            .where(
              (a) =>
                  a['id'] == saved['params']['alertId'] &&
                  a['status'] == saved['alert_target'],
            )
            .firstOrNull;
        if (alert == null) {
          error =
              'The alert action is still unconfirmed. Check its latest status; the app will not resubmit it.';
          return null;
        }
        validate(alert, checkBranch: false);
        await storage.write(recoveryKey, '{}');
        pending = null;
        return {'alert': alert};
      }
      if (pending == null) {
        final frozen = jsonObject(
          jsonDecode(
            jsonEncode({
              'method': method,
              'path': route,
              'body': body!,
              'params': {
                for (final entry in params.entries)
                  if (route!.contains('{${entry.key}}')) entry.key: entry.value,
              },
              'header_version': headerVersion,
              'key': VendorCommand.create().idempotencyKey,
              if (route!.contains('/alerts/')) ...{
                'alert_target': route.endsWith('/resolve')
                    ? 'RESOLVED'
                    : 'ACKNOWLEDGED',
                'alert_branch': branchId,
              },
            }),
          ),
        );
        await storage.write(recoveryKey, jsonEncode(frozen));
        pending = frozen;
      }
      final attempt = pending!;
      final result = (await app.purchasingApi.request(
        attempt['method'],
        attempt['path'],
        pathParameters: Map<String, String>.from(attempt['params']),
        body: VendorPayload(jsonObject(attempt['body'])),
        command: (attempt['path'] as String).contains('/alerts/')
            ? null
            : VendorCommand(idempotencyKey: attempt['key']),
        contractHeaders: {
          if (attempt['header_version'] != null)
            'If-Match': '${attempt['header_version']}',
        },
      )).object;
      if (result.isEmpty) {
        throw const FormatException('Missing purchasing confirmation');
      }
      if ((attempt['path'] as String).contains('/alerts/')) {
        final alert = jsonObject(result['alert']);
        if (alert['id'] != attempt['params']['alertId'] ||
            alert['status'] != attempt['alert_target']) {
          throw const FormatException('Missing alert confirmation');
        }
        validate(alert, checkBranch: false);
      } else if (result['receipt'] is Map) {
        validate(jsonObject(result['receipt']), checkBranch: false);
      } else {
        validate(result, checkBranch: false);
      }
      await storage.write(recoveryKey, '{}');
      pending = null;
      return result;
    } catch (e) {
      error = e is ApiException
          ? e.message
          : pending == null
          ? 'Nothing was submitted. Recovery details could not be saved.'
          : pending!['alert_target'] != null
          ? 'The alert result is unconfirmed. Check the saved action to read its latest status.'
          : 'The outcome is unconfirmed. Retry the original saved change.';
      if (e is ApiException &&
          [400, 401, 403, 404, 409, 412, 422].contains(e.statusCode)) {
        if (e.statusCode == 403 && pending != null) {
          denied.add('${pending!['method']} ${pending!['path']}');
        }
        try {
          await storage.write(recoveryKey, '{}');
          pending = null;
        } catch (_) {
          blocked = true;
        }
        stale = true;
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
