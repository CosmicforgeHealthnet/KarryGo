import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../app/vendor_app.dart';
import '../../../core/contracts/vendor_contract.dart';
import 'advanced_controller.dart';
import 'advanced_resource.dart';

enum CatalogueTool { foodPrice, shopPrice, lookup }

class CatalogueToolsController extends ChangeNotifier {
  CatalogueToolsController(this.app, this.tool, {this.record = const {}});
  final VendorApp app;
  final CatalogueTool tool;
  final JsonObject record;
  late final reader = AdvancedController(
    app,
    const AdvancedResource(AdvancedKind.measures),
  );
  JsonObject subject = {}, result = {};
  List<JsonObject> variants = [], groups = [], units = [];
  final selectedModifiers = <String>{};
  String? variantId, sellUnitId, identifierType;
  String channel = 'retail';
  bool preparing = false,
      loading = false,
      ready = false,
      queried = false,
      notFound = false,
      restricted = false,
      disposed = false;
  String? error;
  void emit() {
    if (!disposed) notifyListeners();
  }

  void invalidate() {
    result = {};
    error = null;
    queried = false;
    notFound = false;
    restricted = false;
    emit();
  }

  void selectVariant(String? value) {
    if (loading) return;
    variantId = value;
    invalidate();
  }

  void selectUnit(String? value) {
    if (loading) return;
    sellUnitId = value;
    invalidate();
  }

  void selectChannel(String value) {
    if (loading) return;
    channel = value;
    invalidate();
  }

  void toggleModifier(String id, bool selected) {
    if (loading) return;
    selected ? selectedModifiers.add(id) : selectedModifiers.remove(id);
    invalidate();
  }

  bool get selectionValid =>
      selectedModifiers.length <= 50 &&
      groups.every((group) {
        final count = (group['modifiers'] as List)
            .where((m) => selectedModifiers.contains(m['id']))
            .length;
        return count >= group['min_select'] && count <= group['max_select'];
      });
  Future<void> prepare() async {
    if (preparing || loading || disposed) return;
    preparing = true;
    ready = false;
    invalidate();
    variants = [];
    groups = [];
    units = [];
    selectedModifiers.clear();
    variantId = null;
    sellUnitId = null;
    try {
      if (tool == CatalogueTool.foodPrice) {
        final api = app.foodCatalogueApi,
            params = {'itemId': record['id'].toString()};
        subject = (await api.request(
          'GET',
          '/api/v1/vendor/food/items/{itemId}',
          pathParameters: params,
        )).object;
        variants = (await reader.all(
          api,
          '/api/v1/vendor/food/items/{itemId}/variants',
          key: 'variants',
          parameters: params,
        )).where((r) => r['status'] != 'ARCHIVED').toList();
        final attached = (await api.request(
          'GET',
          '/api/v1/vendor/food/items/{itemId}/modifier-groups',
          pathParameters: params,
        )).object;
        final loaded = await Future.wait(
          AdvancedController.objects(attached, 'modifier_groups').map((
            link,
          ) async {
            final group = jsonObject(link['group']);
            if (group['status'] == 'ARCHIVED') return <String, dynamic>{};
            final modifiers = await reader.all(
              api,
              '/api/v1/vendor/food/modifier-groups/{groupId}/modifiers',
              key: 'modifiers',
              parameters: {'groupId': group['id'].toString()},
            );
            if (group['min_select'] is! int || group['max_select'] is! int) {
              throw const FormatException('Missing selection rules');
            }
            return {
              ...group,
              'modifiers': modifiers
                  .where((m) => m['status'] != 'ARCHIVED')
                  .toList(),
            };
          }),
        );
        groups = loaded.where((g) => g.isNotEmpty).toList();
      } else if (tool == CatalogueTool.shopPrice) {
        final api = app.shopCatalogueApi;
        final detail = (await api.request(
          'GET',
          '/api/v1/vendor/shop/listings/{listingId}',
          pathParameters: {'listingId': record['id'].toString()},
        )).object;
        subject = jsonObject(detail['listing']);
        final enabled = AdvancedController.objects(detail, 'sell_units')
            .where((u) => u['enabled'] == true)
            .map((u) => u['sell_unit_ref'])
            .toSet();
        units =
            (await reader.all(
                  api,
                  '/api/v1/vendor/shop/products/{productId}/sell-units',
                  parameters: {'productId': subject['product_ref'].toString()},
                ))
                .where(
                  (u) => enabled.contains(u['id']) && u['status'] == 'ACTIVE',
                )
                .toList();
      }
      ready = true;
    } catch (e) {
      error = AdvancedController.message(e);
      restricted = e is ApiException && e.statusCode == 403;
    } finally {
      preparing = false;
      emit();
    }
  }

  Future<void> preview({int quantity = 1}) async {
    if (loading ||
        preparing ||
        !ready ||
        disposed ||
        tool == CatalogueTool.lookup) {
      return;
    }
    if (tool == CatalogueTool.foodPrice && !selectionValid) {
      result = {};
      error = 'Complete the required choices before checking the price.';
      emit();
      return;
    }
    if (tool == CatalogueTool.shopPrice &&
        (quantity < 1 || quantity > 1000000000)) {
      result = {};
      error = 'Enter a quantity between 1 and 1,000,000,000.';
      emit();
      return;
    }
    loading = true;
    invalidate();
    try {
      final data = tool == CatalogueTool.foodPrice
          ? (await app.foodCatalogueApi.request(
              'POST',
              '/api/v1/vendor/food/items/{itemId}/pricing/resolve',
              pathParameters: {'itemId': record['id'].toString()},
              body: VendorPayload({
                if (variantId != null) 'variant_ref': variantId,
                'modifier_refs': selectedModifiers.toList(),
              }),
            )).object
          : (await app.shopCatalogueApi.request(
              'GET',
              '/api/v1/vendor/shop/listings/{listingId}/resolved-price',
              pathParameters: {'listingId': record['id'].toString()},
              query: {
                'quantity': '$quantity',
                'channel': channel,
                'sell_unit_id': ?sellUnitId,
              },
            )).object;
      if (disposed) return;
      if (data['currency'] is! String ||
          (tool == CatalogueTool.foodPrice
              ? data['effective_unit_price_minor'] is! int ||
                    data['base_price_minor'] is! int ||
                    data['modifier_total_minor'] is! int ||
                    data['modifiers'] is! List
              : data['quantity'] is! int ||
                    data['order_rules_satisfied'] is! bool ||
                    !data.containsKey('unit_price_minor') ||
                    !data.containsKey('total_price_minor') ||
                    data['price_source'] is! String)) {
        throw const FormatException('Incomplete price response');
      }
      if (tool == CatalogueTool.foodPrice) {
        for (final modifier in AdvancedController.objects(data, 'modifiers')) {
          if (modifier['name'] is! String ||
              modifier['price_delta_minor'] is! int) {
            throw const FormatException('Incomplete modifier price');
          }
        }
        if (data['base_source'] is! String) {
          throw const FormatException('Missing price source');
        }
      } else {
        for (final field in [
          'quantity',
          'minimum_order_quantity',
          'order_multiple',
        ]) {
          if (data[field] is! int) {
            throw const FormatException('Missing order rules');
          }
        }
        for (final field in [
          'unit_price_minor',
          'total_price_minor',
          'maximum_order_quantity',
        ]) {
          if (data[field] != null && data[field] is! int) {
            throw const FormatException('Invalid price or order rules');
          }
        }
        if (data['mode'] is! String || data['availability'] is! String) {
          throw const FormatException('Incomplete price context');
        }
      }
      result = data;
    } catch (e) {
      error = AdvancedController.message(e);
      restricted = e is ApiException && e.statusCode == 403;
    } finally {
      loading = false;
      queried = true;
      emit();
    }
  }

  Future<void> lookup(String value, {String? type}) async {
    if (loading ||
        disposed ||
        tool != CatalogueTool.lookup ||
        value.trim().isEmpty) {
      return;
    }
    loading = true;
    invalidate();
    identifierType = type;
    try {
      final data = (await app.shopCatalogueApi.request(
        'GET',
        '/api/v1/vendor/shop/identifier-lookup',
        query: {'value': value.trim(), 'type': ?type},
      )).object;
      for (final field in ['identifier', 'variant', 'product']) {
        if (jsonObject(data[field])['id'] is! String) {
          throw const FormatException('Incomplete lookup response');
        }
      }
      if (!disposed) result = data;
    } catch (e) {
      notFound =
          e is ApiException &&
          e.statusCode == 404 &&
          e.code == 'SHOP_IDENTIFIER_NOT_FOUND';
      if (!notFound) error = AdvancedController.message(e);
      restricted = e is ApiException && e.statusCode == 403;
    } finally {
      loading = false;
      queried = true;
      emit();
    }
  }

  @override
  void dispose() {
    disposed = true;
    reader.dispose();
    super.dispose();
  }
}
