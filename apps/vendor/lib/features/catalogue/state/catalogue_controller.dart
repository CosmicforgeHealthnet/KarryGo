import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../app/vendor_app.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import 'catalogue_resource.dart';

class CatalogueController extends ChangeNotifier {
  CatalogueController(this.app, this.resource);
  final VendorApp app;
  final CatalogueResource resource;
  VendorFeatureApi get api =>
      resource.food ? app.foodCatalogueApi : app.shopCatalogueApi;
  List<JsonObject> rows = [];
  final Map<String, String> labels = {};
  bool loading = false,
      busy = false,
      stale = true,
      hasMore = false,
      archived = false,
      changed = false,
      disposed = false,
      restricted = false;
  String? error, feedback;
  final Set<String> denied = {};
  VendorCommand? _command;
  String? _fingerprint;
  void _notify() {
    if (!disposed) notifyListeners();
  }

  bool allowed(String method, String path) =>
      resource.writable &&
      !busy &&
      !loading &&
      !stale &&
      !denied.contains('$method $path');
  Future<void> load({bool more = false}) async {
    if (loading || busy || disposed) return;
    loading = true;
    restricted = false;
    error = null;
    stale = true;
    _notify();
    try {
      final response = await api.request(
        'GET',
        resource.collection,
        pathParameters: resource.parameters,
        query: {
          if (resource.kind == CatalogueKind.menus && resource.branchId != null)
            'branch_id': resource.branchId!,
          if (resource.paginated) 'limit': '50',
          if (resource.paginated) 'offset': more ? '${rows.length}' : '0',
          if (api
              .operation('GET', resource.collection)
              .parametersIn('query')
              .any((p) => p['name'] == 'include_archived'))
            'include_archived': '$archived',
        },
      );
      if (disposed) return;
      final data = response.data;
      final list = data is List ? data : (data as Map)[resource.rowsKey];
      if (list is! List) {
        throw const FormatException('Incomplete catalogue response');
      }
      final next = list.map(jsonObject).toList();
      rows = more ? [...rows, ...next] : next;
      if (resource.kind == CatalogueKind.listings) {
        final productNames = <String, String>{},
            variantNames = <String, String>{};
        await Future.wait([
          for (final id in next.map((r) => r['product_ref'].toString()).toSet())
            (() async {
              try {
                final response = await app.shopCatalogueApi
                    .getShopProductsProductId(productId: id);
                productNames[id] = response.object['name'].toString();
              } catch (_) {}
            })(),
          for (final id in next.map((r) => r['variant_ref'].toString()).toSet())
            (() async {
              try {
                final response = await app.shopCatalogueApi
                    .getShopVariantsVariantId(variantId: id);
                variantNames[id] = response.object['name'].toString();
              } catch (_) {}
            })(),
        ]);
        if (disposed) return;
        for (final row in next) {
          final product = productNames[row['product_ref']],
              variant = variantNames[row['variant_ref']];
          labels[row['id'].toString()] = product == null || variant == null
              ? 'Product details unavailable'
              : '$product ? $variant';
        }
      }
      hasMore =
          data is Map &&
          (data['has_more'] == true ||
              (data['pagination'] is Map &&
                  data['pagination']['has_more'] == true));
      stale = false;
    } catch (e) {
      restricted = e is ApiException && e.statusCode == 403;
      error = message(e);
    } finally {
      loading = false;
      _notify();
    }
  }

  Future<JsonObject> read(JsonObject row) async {
    final path = resource.readDetail;
    if (path == null) {
      final response = await api.request(
        'GET',
        resource.collection,
        pathParameters: resource.parameters,
        query: {
          'include_archived': 'true',
          if (resource.paginated) 'limit': '100',
        },
      );
      final data = response.data;
      final list = data is List ? data : (data as Map)[resource.rowsKey];
      return (list as List)
          .map(jsonObject)
          .firstWhere((v) => v['id'] == row['id']);
    }
    final parameters = resource.kind == CatalogueKind.shopVariants
        ? {'variantId': row['id'].toString()}
        : resource.detailParameters(row['id'].toString());
    final response = await api.request('GET', path, pathParameters: parameters);
    return resource.kind == CatalogueKind.listings
        ? jsonObject(response.object['listing'])
        : response.object;
  }

  Future<String?> publicationBlocker(JsonObject menu) async {
    if (resource.kind != CatalogueKind.menus) return null;
    final vendorId = app.scope.vendorId!;
    final business = (await app.businessApi.getBusinessesBusinessId(
      businessId: vendorId,
    )).object;
    if (business['status'] != 'ACTIVE') {
      return {
            'PENDING_VERIFICATION',
            'PENDING_REVIEW',
            'DRAFT',
          }.contains(business['status'])
          ? 'Your business must be approved and activated before you can publish this menu. You can keep preparing it as a draft.'
          : 'Your business is ${business['status'].toString().toLowerCase()}. It must be active before you can publish this menu.';
    }
    final branchId = menu['branch_id']?.toString();
    if (branchId == null || branchId.isEmpty) {
      return 'Choose a branch before publishing this menu.';
    }
    final branch =
        (await app.businessApi.getBusinessesBusinessIdBranchesBranchId(
          businessId: vendorId,
          branchId: branchId,
        )).object;
    if (branch['status'] != 'ACTIVE') {
      return 'This branch must be activated before you can publish its menu.';
    }
    final response = await app.businessApi
        .getBusinessesBusinessIdBranchesBranchIdCapabilities(
          businessId: vendorId,
          branchId: branchId,
        );
    final data = response.data;
    final capabilities = data is List ? data : (data as Map)['items'];
    if (capabilities is! List) {
      throw const FormatException('Incomplete branch capability response');
    }
    if (!capabilities.any(
      (value) =>
          value is Map &&
          value['capability_type'] == menu['capability_type'] &&
          value['status'] == 'ACTIVE',
    )) {
      return 'The food capability for this branch must be activated before you can publish this menu.';
    }
    return null;
  }

  Future<bool> write(
    String method,
    String path,
    Map<String, String> parameters,
    JsonObject? body,
  ) async {
    if (!allowed(method, path)) return false;
    final fingerprint = jsonEncode([method, path, parameters, body]);
    if (fingerprint != _fingerprint) {
      _fingerprint = fingerprint;
      _command = VendorCommand.create();
    }
    busy = true;
    error = null;
    feedback = null;
    _notify();
    var saved = false;
    try {
      await api.request(
        method,
        path,
        pathParameters: parameters,
        body: body == null ? null : VendorPayload(body),
        command: _command,
      );
      saved = true;
      changed = true;
      _command = null;
      _fingerprint = null;
    } catch (e) {
      error = message(e);
      if (e is ApiException && e.statusCode == 403) denied.add('$method $path');
      if (e is! ApiException ||
          e.statusCode == null ||
          e.statusCode == 409 ||
          (e.statusCode ?? 0) >= 500) {
        stale = true;
      }
    } finally {
      busy = false;
    }
    if (saved) {
      await load();
      feedback = error == null
          ? 'Your catalogue is up to date.'
          : 'Saved. Refresh to see the latest catalogue.';
    }
    _notify();
    return saved;
  }

  static String message(Object e) => e is ApiException
      ? e.message
      : 'Could not load this catalogue. Please try again.';
  @override
  void dispose() {
    disposed = true;
    super.dispose();
  }
}
