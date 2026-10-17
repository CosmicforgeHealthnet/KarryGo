import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';

const cloudKitchenBase = '/api/v1/vendor/cloud-kitchen';

class CloudKitchenWorkspaceController extends ChangeNotifier {
  CloudKitchenWorkspaceController(
    this.runtime,
    this.path, {
    this.parameters = const {},
    this.branchId,
    this.wrapper,
    this.allowMissing = false,
  });
  final VendorRuntime runtime;
  final String path;
  final Map<String, String> parameters;
  final String? branchId, wrapper;
  final bool allowMissing;
  late final app = runtime.backend;
  late final storage = runtime.mode == VendorDataMode.mock
      ? runtime.mockSupportPreferences
      : runtime.preferences;
  late final recoveryKey =
      'vendor.cloud-kitchen-workspace.${base64Url.encode(utf8.encode('${runtime.mode.name}|${runtime.mode == VendorDataMode.mock ? runtime.revision : app.client.config.origin}|${app.scope.vendorId}|${app.auth.session?.staffId}'))}';
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
  Map<String, String> readQuery = const {};
  bool get writable =>
      !loading && !busy && !stale && !blocked && pending == null;
  bool allowed(String method, String route) =>
      writable && !denied.contains('$method $route');
  void emit() {
    if (!disposed) notifyListeners();
  }

  List<JsonObject> collection(Object? value) {
    if (value == null) return [];
    if (value is! List) {
      throw const FormatException('Invalid cloud kitchen list');
    }
    return value.map(jsonObject).toList();
  }

  void validate(JsonObject r, {bool checkBranch = true}) {
    if (r['business_id'] != null && r['business_id'] != app.scope.vendorId ||
        checkBranch &&
            branchId != null &&
            r['branch_id'] != null &&
            r['branch_id'] != branchId) {
      throw const FormatException('Out-of-scope cloud kitchen record');
    }
    if (r['version'] != null && (r['version'] is! int || r['version'] < 0)) {
      throw const FormatException('Invalid cloud kitchen version');
    }
  }

  VendorFeatureApi api(String route) => app.cloudKitchenApi;
  String? nextCursor;
  Future<Object?> fetch(
    String route, {
    Map<String, String> params = const {},
    Map<String, String> query = const {},
  }) async {
    final result = await api(route).request(
      'GET',
      route,
      pathParameters: {
        for (final entry in params.entries)
          if (route.contains("{${entry.key}}")) entry.key: entry.value,
      },
      query: query,
    );

    return result.data;
  }

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
          !['POST', 'PUT', 'PATCH', 'DELETE'].contains(saved['method']) ||
          !(saved['path'] as String).startsWith('$cloudKitchenBase/')) {
        throw const FormatException('Invalid saved cloud kitchen change');
      }
      api(saved['path']).operation(saved['method'], saved['path']);
      pending = saved;
      blocked = false;
    } catch (_) {
      blocked = true;
      error =
          'Your saved cloud kitchen change could not be restored. Reopen cloud kitchen before editing.';
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
        query: {...readQuery},
      );
      final envelope = response is Map ? jsonObject(response) : null;
      final listKey =
          wrapper ??
          (envelope == null
              ? null
              : [
                  'items',
                  'campaigns',
                  'snapshots',
                  'targets',
                  'runs',
                  'deliveries',
                  'history',
                  'corporate_customers',
                  'contacts',
                  'memberships',
                  'locations',
                  'cost_centers',
                  'interactions',
                  'tasks',
                  'entries',
                ].where((key) => envelope[key] is List).firstOrNull);
      final value = listKey == null ? response : envelope?[listKey];
      if (value == null || value is List) {
        final next = collection(value);
        for (final r in next) {
          validate(r);
        }
        rows = append ? [...rows, ...next] : next;
        record = null;
        hasMore =
            envelope?['has_more'] == true ||
            (envelope?['pagination'] is Map &&
                (envelope!['pagination'] as Map)['has_more'] == true);
      } else {
        record = jsonObject(value);
        validate(record!);
        for (final key in ['profile', 'corporate_customer']) {
          if (record![key] is Map) validate(jsonObject(record![key]));
        }
        rows = [];
        hasMore = false;
      }
      data = value;
      stale = false;
    } catch (e) {
      error = e is ApiException
          ? e.message
          : 'cloud kitchen could not be loaded. Refresh to try again.';
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
    Map<String, String> query = const {},
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
      if (pending == null) {
        final schema = api(route!).operation(method!, route).requestSchema;
        final properties = schema?['properties'] as Map?;
        final normalized = <String, dynamic>{...body!};
        normalized.removeWhere(
          (key, value) =>
              value == null &&
              properties?[key] is Map &&
              (properties![key] as Map)['nullable'] != true,
        );
        if (schema != null) validateRequest(normalized, schema);
        final frozen = jsonObject(
          jsonDecode(
            jsonEncode({
              'method': method,
              'path': route,
              'body': normalized,
              'params': {
                for (final entry in params.entries)
                  if (route.contains('{${entry.key}}')) entry.key: entry.value,
              },
              'header_version': headerVersion,
              'query': query,
              'key': VendorCommand.create().idempotencyKey,
            }),
          ),
        );
        await storage.write(recoveryKey, jsonEncode(frozen));
        pending = frozen;
      }
      final attempt = pending!;
      final response = await api(attempt['path']).request(
        attempt['method'],
        attempt['path'],
        pathParameters: Map<String, String>.from(attempt['params']),
        query: Map<String, String>.from(attempt['query'] ?? {}),
        body:
            api(
                  attempt['path'],
                ).operation(attempt['method'], attempt['path']).requestSchema ==
                null
            ? null
            : VendorPayload(jsonObject(attempt['body'])),
        command:
            api(attempt['path'])
                .operation(attempt['method'], attempt['path'])
                .parametersIn('header')
                .any(
                  (p) =>
                      (p['name'] as String).toLowerCase() == 'idempotency-key',
                )
            ? VendorCommand(idempotencyKey: attempt['key'])
            : null,
        contractHeaders: {
          if (attempt['header_version'] != null)
            'If-Match': '${attempt['header_version']}',
        },
      );
      final acknowledged =
          (attempt['path'] as String).endsWith('/exceptions') &&
          response.data == null &&
          response.envelope?['success'] == true;
      final result = acknowledged ? <String, dynamic>{} : response.object;
      if (result.isEmpty && !acknowledged) {
        throw const FormatException('Missing cloud kitchen confirmation');
      }
      validate(result, checkBranch: false);
      await storage.write(recoveryKey, '{}');
      pending = null;
      return result;
    } catch (e) {
      error = e is ApiException
          ? e.message
          : e is VendorContractException
          ? 'Check the cloud kitchen details before submitting. Nothing was sent.'
          : pending == null
          ? 'Nothing was submitted. Recovery details could not be saved.'
          : 'The outcome is unconfirmed. Retry the original saved change.';
      if (e is VendorContractException) {
        try {
          await storage.write(recoveryKey, '{}');
          pending = null;
        } catch (_) {}
      }
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
