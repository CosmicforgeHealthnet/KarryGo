import '../../../core/network/friendly_messages.dart';
import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../app/vendor_app.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/network/vendor_response.dart';

enum SettingsKind { profile, brands, branches, capabilities, staff, roles }

class SettingsController extends ChangeNotifier {
  SettingsController(this.app, this.kind);
  final VendorApp app;
  final SettingsKind kind;
  List<JsonObject> rows = [];
  bool loading = false,
      busy = false,
      stale = true,
      restricted = false,
      hasMore = false,
      disposed = false;
  String? error, feedback;
  final Set<String> denied = {};
  final Map<String, VendorCommand> _commands = {};
  static const business = '/api/v1/vendor/businesses/{businessId}';
  Map<String, String> get scope => {'businessId': app.scope.vendorId!};
  bool get iam => kind == SettingsKind.staff || kind == SettingsKind.roles;
  bool get paginated => {
    SettingsKind.brands,
    SettingsKind.branches,
    SettingsKind.capabilities,
  }.contains(kind);
  String get title => switch (kind) {
    SettingsKind.profile => 'Business profile',
    SettingsKind.brands => 'Brands',
    SettingsKind.branches => 'Branches',
    SettingsKind.capabilities => 'Business types',
    SettingsKind.staff => 'Team',
    SettingsKind.roles => 'Roles & permissions',
  };
  String get collection => switch (kind) {
    SettingsKind.profile => business,
    SettingsKind.brands => '$business/brands',
    SettingsKind.branches => '$business/branches',
    SettingsKind.capabilities => '$business/capabilities',
    SettingsKind.staff => '/api/v1/vendor/staff',
    SettingsKind.roles => '/api/v1/vendor/roles',
  };
  String get idKey => switch (kind) {
    SettingsKind.brands => 'brandId',
    SettingsKind.branches => 'branchId',
    SettingsKind.capabilities => 'capabilityId',
    _ => 'id',
  };
  String get detail =>
      kind == SettingsKind.profile ? collection : '$collection/{$idKey}';
  Map<String, String> params([JsonObject? row]) => {
    if (!iam) ...scope,
    if (row != null && kind != SettingsKind.profile) idKey: row['id'] as String,
  };
  VendorFeatureApi api(String path) =>
      path.startsWith('/api/v1/vendor/onboarding/')
      ? app.onboardingApi
      : path.startsWith('/api/v1/vendor/businesses') ||
            path.endsWith('/capability-types')
      ? app.businessApi
      : app.accessApi;
  void _notify() {
    if (!disposed) notifyListeners();
  }

  bool allowed(String method, String path) =>
      !busy && !loading && !stale && !denied.contains('$method $path');
  Future<VendorResponse> read(
    String path, {
    Map<String, String> parameters = const {},
    Map<String, String> query = const {},
  }) =>
      api(path).request('GET', path, pathParameters: parameters, query: query);
  static List<JsonObject> objects(Object? value, [String? key]) {
    if (value is Map) value = value[key ?? 'items'];
    if (value is! List) {
      throw const FormatException('Incomplete settings response');
    }
    return value.map(jsonObject).toList();
  }

  Future<List<JsonObject>> all(
    String path,
    Map<String, String> parameters,
  ) async {
    final result = <JsonObject>[];
    while (true) {
      final response = await read(
        path,
        parameters: parameters,
        query: {'limit': '100', 'offset': '${result.length}'},
      );
      final next = objects(response.data);
      result.addAll(next);
      final data = response.object;
      if ((data['pagination'] as Map?)?['has_more'] != true) return result;
      if (next.isEmpty) throw const FormatException('Incomplete settings page');
    }
  }

  Future<void> load({bool more = false}) async {
    if (loading || busy || disposed) return;
    loading = true;
    stale = true;
    error = null;
    restricted = false;
    _notify();
    try {
      final response = await read(
        collection,
        parameters: params(),
        query: {
          if (paginated) 'limit': '50',
          if (paginated) 'offset': more ? '${rows.length}' : '0',
        },
      );
      final next = kind == SettingsKind.profile
          ? [response.object]
          : objects(
              response.data,
              kind == SettingsKind.staff
                  ? 'staff'
                  : kind == SettingsKind.roles
                  ? 'roles'
                  : null,
            );
      if (disposed) return;
      rows = more ? [...rows, ...next] : next;
      hasMore =
          response.data is Map &&
          ((response.object['pagination'] as Map?)?['has_more'] == true);
      stale = false;
    } catch (e) {
      restricted = e is ApiException && e.statusCode == 403;
      if (restricted) rows = [];
      error = message(e);
    } finally {
      loading = false;
      _notify();
    }
  }

  Future<JsonObject> readRecord(JsonObject row) async =>
      (await read(detail, parameters: params(row))).object;
  Future<bool> mutate(
    String method,
    String path,
    JsonObject? body, {
    Map<String, String> parameters = const {},
  }) async {
    if (!allowed(method, path)) return false;
    busy = true;
    error = null;
    feedback = null;
    _notify();
    final feature = api(path);
    final op = feature.operation(method, path);
    final fingerprint = jsonEncode([method, path, parameters, body]);
    final command =
        op
            .parametersIn('header')
            .any((h) => h['name'].toString().toLowerCase() == 'idempotency-key')
        ? _commands.putIfAbsent(fingerprint, VendorCommand.create)
        : null;
    var saved = false;
    try {
      await feature.request(
        method,
        path,
        pathParameters: parameters,
        body: body == null ? null : VendorPayload(body),
        command: command,
        retryAuthentication: false,
      );
      _commands.remove(fingerprint);
      saved = true;
    } catch (e) {
      error = message(e);
      if (e is ApiException &&
          e.statusCode == 403 &&
          !serverMessageOf(e).contains('step-up')) {
        denied.add('$method $path');
      }
      if (e is! ApiException ||
          e.statusCode == null ||
          e.statusCode == 409 ||
          (e.statusCode ?? 0) >= 500) {
        stale = true;
        feedback =
            'The result needs checking. Refresh before making another change.';
      } else {
        _commands.remove(fingerprint);
      }
    } finally {
      busy = false;
      _notify();
    }
    if (saved) {
      await load();
      if (error != null) {
        feedback = 'Saved. Refresh to load the latest details.';
      }
    }
    return saved;
  }

  static String message(Object e) => e is ApiException
      ? e.message
      : e is StateError
      ? e.message.toString()
      : e is FormatException
      ? 'Settings data was incomplete. Refresh and try again.'
      : e is VendorContractException
      ? 'This change could not be sent. Check the details and try again.'
      : 'Could not connect. Please try again.';
  @override
  void dispose() {
    disposed = true;
    super.dispose();
  }
}
