import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';

const recipesBase = '/api/v1/vendor/recipes';

class RecipesWorkspaceController extends ChangeNotifier {
  RecipesWorkspaceController(
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
      'vendor.recipes.${base64Url.encode(utf8.encode('${runtime.mode.name}|${runtime.mode == VendorDataMode.mock ? runtime.revision : app.client.config.origin}|${app.scope.vendorId}|${app.auth.session?.staffId}'))}';
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
  String searchQuery = '';
  bool get writable =>
      !loading && !busy && !stale && !blocked && pending == null;
  bool allowed(String method, String route) =>
      writable && !denied.contains('$method $route');
  void emit() {
    if (!disposed) notifyListeners();
  }

  List<JsonObject> collection(Object? value) {
    if (value == null) return [];
    if (value is! List) throw const FormatException('Invalid recipes list');
    return value.map(jsonObject).toList();
  }

  void validate(JsonObject r, {bool checkBranch = true}) {
    if (r['business_id'] != null && r['business_id'] != app.scope.vendorId ||
        checkBranch &&
            branchId != null &&
            r['branch_id'] != null &&
            r['branch_id'] != branchId) {
      throw const FormatException('Out-of-scope recipes record');
    }
    if (r['version'] is Map) {
      validate(jsonObject(r['version']), checkBranch: checkBranch);
      return;
    }
    if (r['version'] != null && (r['version'] is! int || r['version'] < 1)) {
      throw const FormatException('Invalid recipes version');
    }
  }

  VendorFeatureApi api(String route) =>
      route.startsWith('/api/v1/vendor/recipe-cost/')
      ? app.recipeCostApi
      : app.recipesApi;
  Future<Object?> fetch(
    String route, {
    Map<String, String> params = const {},
    Map<String, String> query = const {},
  }) async => (await api(route).request(
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
          !((saved['path'] as String).startsWith(recipesBase) ||
              (saved['path'] as String).startsWith(
                '/api/v1/vendor/recipe-cost/',
              ))) {
        throw const FormatException('Invalid saved recipes change');
      }
      api(saved['path']).operation(saved['method'], saved['path']);
      pending = saved;
      blocked = false;
    } catch (_) {
      blocked = true;
      error =
          'Your saved recipes change could not be restored. Reopen Recipes before editing.';
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
          if (path.startsWith('/api/v1/vendor/recipe-cost/') &&
              !path.endsWith('/history') &&
              !path.endsWith('/latest'))
            'branch_id': ?branchId,
          if (path.endsWith('/history')) ...{
            'limit': '200',
            'offset': append ? '${rows.length}' : '0',
          },
          if (path == recipesBase) ...{
            'limit': '200',
            'offset': append ? '${rows.length}' : '0',
            if (searchQuery.isNotEmpty) 'q': searchQuery,
            if (status != null && status.isNotEmpty) 'status': status,
          },
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
            next.length == 200 &&
            (path == recipesBase || path.endsWith('/history'));
      } else {
        record = jsonObject(value);
        validate(record!);
        rows = [];
        hasMore = false;
      }
      data = value;
      stale = false;
    } catch (e) {
      if (allowMissing &&
          path.contains('/branches/') &&
          e is ApiException &&
          e.statusCode == 404) {
        record = null;
        rows = [];
        data = null;
        stale = false;
      } else {
        error = e is ApiException
            ? e.message
            : 'Recipes could not be loaded. Refresh to try again.';
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
              'query': query,
              'key': VendorCommand.create().idempotencyKey,
            }),
          ),
        );
        await storage.write(recoveryKey, jsonEncode(frozen));
        pending = frozen;
      }
      final attempt = pending!;
      final result = (await api(attempt['path']).request(
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
        command: VendorCommand(idempotencyKey: attempt['key']),
        contractHeaders: {
          if (attempt['header_version'] != null)
            'If-Match': '${attempt['header_version']}',
        },
      )).object;
      if (result.isEmpty) {
        throw const FormatException('Missing recipes confirmation');
      }
      validate(result, checkBranch: false);
      await storage.write(recoveryKey, '{}');
      pending = null;
      return result;
    } catch (e) {
      error = e is ApiException
          ? e.message
          : pending == null
          ? 'Nothing was submitted. Recovery details could not be saved.'
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
