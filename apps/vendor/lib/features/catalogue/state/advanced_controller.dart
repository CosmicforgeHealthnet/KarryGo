import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../app/vendor_app.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/network/vendor_response.dart';
import 'advanced_resource.dart';

class AdvancedController extends ChangeNotifier {
  AdvancedController(this.app, this.resource);
  final VendorApp app;
  final AdvancedResource resource;
  VendorFeatureApi get api =>
      resource.food ? app.foodCatalogueApi : app.shopCatalogueApi;
  List<JsonObject> rows = [];
  JsonObject metadata = {};
  final labels = <String, String>{};
  bool loading = false,
      busy = false,
      stale = true,
      restricted = false,
      hasMore = false,
      archived = false,
      disposed = false;
  String? error, feedback;
  final denied = <String>{};
  final commands = <String, VendorCommand>{};
  void emit() {
    if (!disposed) notifyListeners();
  }

  bool allowed(String method, String path) =>
      resource.writable &&
      resource.parent['status'] != 'ARCHIVED' &&
      !resource.readOnly &&
      !loading &&
      !busy &&
      !stale &&
      !denied.contains('$method $path');
  static List<JsonObject> objects(Object? data, [String key = 'items']) {
    if (data is Map) data = data[key];
    if (data is! List) {
      throw const FormatException('Incomplete catalogue response');
    }
    return data.map(jsonObject).toList();
  }

  Future<VendorResponse> get(
    String path, {
    Map<String, String> parameters = const {},
    Map<String, String> query = const {},
  }) => api.request('GET', path, pathParameters: parameters, query: query);
  JsonObject profile(JsonObject data) => switch (resource.kind) {
    AdvancedKind.prep ||
    AdvancedKind.logistics => jsonObject(data['profile'] ?? {'version': 0}),
    AdvancedKind.models => jsonObject(data['configuration']),
    _ => data,
  };
  Future<void> load({bool more = false}) async {
    if (loading || busy || disposed) return;
    loading = true;
    stale = true;
    restricted = false;
    error = null;
    emit();
    try {
      final op = api.operation('GET', resource.path);
      final queryNames = op.parametersIn('query').map((p) => p['name']).toSet();
      final response = await get(
        resource.path,
        parameters: resource.parameters,
        query: {
          if (queryNames.contains('include_archived'))
            'include_archived': '$archived',
          if (queryNames.contains('limit')) 'limit': '50',
          if (queryNames.contains('offset'))
            'offset': more ? '${rows.length}' : '0',
          if (resource.kind == AdvancedKind.shopHistory &&
              resource.parent['product_ref'] != null)
            'product_ref': resource.parent['product_ref'].toString(),
        },
      );
      if (disposed) return;
      metadata = response.data is Map ? response.object : {};
      final next = resource.singleton
          ? [profile(response.object)]
          : objects(response.data, switch (resource.kind) {
              AdvancedKind.schedules => 'schedules',
              AdvancedKind.listingUnits => 'sell_units',
              AdvancedKind.foodHistory => 'price_versions',
              _ => 'items',
            });
      rows = more ? [...rows, ...next] : next;
      hasMore =
          metadata['has_more'] == true ||
          (metadata['pagination'] as Map?)?['has_more'] == true;
      await loadLabels();
      stale = false;
    } catch (e) {
      restricted = e is ApiException && e.statusCode == 403;
      if (restricted) rows = [];
      error = message(e);
    } finally {
      loading = false;
      emit();
    }
  }

  Future<void> loadLabels() async {
    if (resource.food ||
        resource.kind == AdvancedKind.measures ||
        resource.kind == AdvancedKind.aliases ||
        resource.readOnly) {
      return;
    }
    final product =
        resource.parameters['productId'] ??
        metadata['listing']?['product_ref']?.toString();
    final reads = <Future<void>>[];
    Future<void> collect(
      VendorFeatureApi feature,
      String path,
      Map<String, String> params,
      List<String> keys,
    ) async {
      try {
        final records = await all(feature, path, parameters: params);
        for (final row in records) {
          final name = keys
              .map((k) => row[k])
              .where((v) => v != null && v.toString().isNotEmpty)
              .firstOrNull;
          if (name != null && row['id'] != null) {
            labels[row['id'].toString()] = name.toString();
          }
        }
      } catch (_) {}
    }

    reads.add(
      collect(app.shopCatalogueApi, '/api/v1/vendor/shop/measurements', {}, [
        'display_name',
      ]),
    );
    if (product != null) {
      reads.add(
        collect(
          app.shopCatalogueApi,
          '/api/v1/vendor/shop/products/{productId}/variants',
          {'productId': product},
          ['name'],
        ),
      );
      reads.add(
        collect(
          app.shopCatalogueApi,
          '/api/v1/vendor/shop/products/{productId}/sell-units',
          {'productId': product},
          ['display_name'],
        ),
      );
    }
    await Future.wait(reads);
  }

  String label(Object? reference, String fallback) => reference == null
      ? fallback
      : labels[reference.toString()] ?? reference.toString();

  Future<JsonObject> fresh(JsonObject row) async {
    if (resource.singleton) {
      return profile(
        (await get(resource.path, parameters: resource.parameters)).object,
      );
    }
    if ({
      AdvancedKind.schedules,
      AdvancedKind.measures,
    }.contains(resource.kind)) {
      final data = (await get(
        resource.detail,
        parameters: resource.params(row),
      )).object;
      return resource.kind == AdvancedKind.measures
          ? jsonObject(data['measurement'])
          : data;
    }
    final names = api
        .operation('GET', resource.path)
        .parametersIn('query')
        .map((p) => p['name'])
        .toSet();
    final data = (await get(
      resource.path,
      parameters: resource.parameters,
      query: {
        if (names.contains('include_archived')) 'include_archived': 'true',
      },
    )).data;
    final values = objects(
      data,
      resource.kind == AdvancedKind.listingUnits ? 'sell_units' : 'items',
    );
    return values.firstWhere(
      (v) => v['id'] == row['id'],
      orElse: () => throw StateError(
        'This record is no longer available. Refresh the catalogue.',
      ),
    );
  }

  Future<List<JsonObject>> all(
    VendorFeatureApi feature,
    String path, {
    Map<String, String> parameters = const {},
    String key = 'items',
  }) async {
    final names = feature
        .operation('GET', path)
        .parametersIn('query')
        .map((p) => p['name'])
        .toSet();
    final result = <JsonObject>[];
    while (true) {
      final response = await feature.request(
        'GET',
        path,
        pathParameters: parameters,
        query: {
          if (names.contains('limit')) 'limit': '100',
          if (names.contains('offset')) 'offset': '${result.length}',
        },
      );
      final next = objects(response.data, key);
      result.addAll(next);
      if (response.data is! Map ||
          !(response.object['has_more'] == true ||
              (response.object['pagination'] as Map?)?['has_more'] == true)) {
        return result;
      }
      if (next.isEmpty) {
        throw const FormatException('Incomplete catalogue page');
      }
    }
  }

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
    emit();
    final fingerprint = jsonEncode([method, path, parameters, body]);
    final op = api.operation(method, path);
    final command =
        op
            .parametersIn('header')
            .any((p) => p['name'].toString().toLowerCase() == 'idempotency-key')
        ? commands.putIfAbsent(fingerprint, VendorCommand.create)
        : null;
    bool saved = false;
    try {
      await api.request(
        method,
        path,
        pathParameters: parameters,
        body: body == null ? null : VendorPayload(body),
        command: command,
        retryAuthentication: false,
      );
      commands.remove(fingerprint);
      saved = true;
    } catch (e) {
      error = message(e);
      if (e is ApiException && e.statusCode == 403) denied.add('$method $path');
      if (e is! ApiException ||
          e.statusCode == null ||
          e.statusCode == 409 ||
          (e.statusCode ?? 0) >= 500) {
        stale = true;
        feedback = 'Refresh to check the result before making another change.';
      } else {
        commands.remove(fingerprint);
      }
    } finally {
      busy = false;
      emit();
    }
    if (saved) {
      await load();
      if (error != null) {
        feedback = 'Saved. Refresh to load the latest catalogue.';
      }
    }
    return saved;
  }

  static String message(Object e) => e is ApiException
      ? e.message
      : e is StateError
      ? e.message.toString()
      : e is FormatException
      ? 'The catalogue response was incomplete. Refresh and try again.'
      : 'Could not connect. Please try again.';
  @override
  void dispose() {
    disposed = true;
    super.dispose();
  }
}
