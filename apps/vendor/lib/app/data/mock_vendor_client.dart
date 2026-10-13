import 'mock_media_store.dart';
import 'mock_pastry_store.dart';
import 'mock_cloud_kitchen_store.dart';
import 'mock_crm_store.dart';
import 'mock_costs_store.dart';
import 'mock_production_operations_store.dart';
import 'mock_production_batches_store.dart';
import 'mock_recipes_store.dart';
import 'mock_production_materials_store.dart';
import 'mock_purchasing_store.dart';
import 'mock_taxonomy_store.dart';
import 'mock_scheduling_store.dart';
import 'mock_promotions_store.dart';
import 'mock_vendor_missions_store.dart';
import 'mock_readiness_store.dart';
import 'dart:convert';
import 'mock_onboarding_store.dart';
import 'mock_dashboard_data.dart';
import 'mock_catalogue_store.dart';
import 'mock_advanced_catalogue_store.dart';
import 'mock_finance_store.dart';
import 'mock_settings_store.dart';
import 'mock_inventory_store.dart';
import 'mock_support_store.dart';
import 'package:http/http.dart' as http;

enum MockAuthScenario {
  multipleVendors('Multiple vendors'),
  singleVendor('Single vendor'),
  expiredCode('Expired code'),
  accessUnavailable('No vendor access'),
  accessSuspended('Suspended access'),
  connectionFailure('Connection failure'),
  rateLimited('Rate limited');

  const MockAuthScenario(this.label);
  final String label;
}

class MockVendorClient extends http.BaseClient {
  MockVendorClient({
    this.scenario = MockAuthScenario.multipleVendors,
    this.onboardingScenario = MockOnboardingScenario.draft,
    this.dashboardScenario = MockDashboardScenario.populated,
    this.latency = const Duration(milliseconds: 450),
  });
  final MockAuthScenario scenario;
  final Duration latency;
  final MockOnboardingScenario onboardingScenario;
  final MockDashboardScenario dashboardScenario;
  final Map<String, MockOnboardingStore> _businesses = {};
  final Map<String, MockDashboardData> _dashboards = {};
  final Map<String, MockCatalogueStore> _catalogues = {};
  final Map<String, MockTaxonomyStore> _taxonomies = {};
  final Map<String, MockPurchasingStore> _purchasing = {};
  final Map<String, MockProductionMaterialsStore> _productionMaterials = {};
  final Map<String, MockRecipesStore> _recipes = {};
  final Map<String, MockCostsStore> _costs = {};
  final Map<String, MockCrmStore> _crm = {};
  final _pastries = <String, MockPastryStore>{};
  final _cloudKitchens = <String, MockCloudKitchenStore>{};
  final _productionOperations = <String, MockProductionOperationsStore>{};
  final Map<String, MockProductionBatchesStore> _productionBatches = {};
  final Map<String, MockAdvancedCatalogueStore> _advancedCatalogues = {};
  final Map<String, MockFinanceStore> _finance = {};
  final Map<String, MockSettingsStore> _settings = {};
  final Map<String, MockInventoryStore> _inventory = {};
  final Map<String, MockSupportStore> _support = {};
  final Map<String, MockSchedulingStore> _scheduling = {};
  final Map<String, MockPromotionsStore> _promotions = {};
  final Map<String, MockReadinessStore> _readiness = {};
  final Map<String, MockVendorMissionsStore> _vendorMissions = {};
  final _media = <String, MockMediaStore>{};
  static const code = '123456';
  static const vendorIds = [
    '8ab421d4-7e6c-4bce-9d3e-05f18e21a064',
    'c178b37a-36f2-4a41-8d20-6d437c24bf72',
  ];
  String? _challenge;
  String? _identifier;
  DateTime? _expiresAt;
  bool _consumed = false;
  bool _limited = false;
  int _sequence = 0;

  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    final bytes = await request.finalize().toBytes();
    if (request.url.path.startsWith('/api/v1/media-files/vendor-uploads/')) {
      for (final media in _media.values) {
        final result = media.redeem(request, bytes);
        if (result != null) return result;
      }
      for (final store in _businesses.values) {
        final result = store.redeem(request, bytes);
        if (result != null) return result;
      }
      return _error(404, 'NOT_FOUND', 'Upload grant not found.');
    }
    final body = bytes.isEmpty
        ? <String, dynamic>{}
        : jsonDecode(utf8.decode(bytes)) as Map<String, dynamic>;
    await Future<void>.delayed(latency);
    if (scenario == MockAuthScenario.connectionFailure) {
      throw http.ClientException('Mock connection failure', request.url);
    }
    if (scenario == MockAuthScenario.rateLimited && !_limited) {
      _limited = true;
      return _error(
        429,
        'RATE_LIMITED',
        'Too many requests. Try again shortly.',
        headers: {'retry-after': '3'},
      );
    }
    if (!request.url.path.startsWith('/api/v1/vendor/auth/') ||
        request.url.path.contains('/step-up/')) {
      final bearer =
          request.headers['Authorization'] ??
          request.headers['authorization'] ??
          '';
      final parts = bearer.split(':');
      if (!bearer.startsWith('Bearer mock-access:') ||
          parts.length < 3 ||
          !vendorIds.contains(parts[1])) {
        return _error(401, 'UNAUTHORIZED', 'Sign in required.');
      }
      final store = _businesses.putIfAbsent(
        parts[1],
        () => MockOnboardingStore(parts[1], onboardingScenario),
      );
      final dashboard = _dashboards.putIfAbsent(
        parts[1],
        () => MockDashboardData(store, dashboardScenario),
      );
      if ({'ACTIVE', 'PAUSED'}.contains(store.business['status'])) {
        final promotionResult = _promotions
            .putIfAbsent(parts[1], () => MockPromotionsStore(dashboard))
            .handle(request, body);
        if (promotionResult != null) {
          if (promotionResult['_message'] != null) {
            return _error(
              promotionResult['_status'] as int,
              promotionResult['_code'].toString(),
              promotionResult['_message'].toString(),
            );
          }
          return _ok(
            promotionResult['_data'],
            status: promotionResult['_status'] as int,
          );
        }
        final scheduling = _scheduling.putIfAbsent(
          parts[1],
          () => MockSchedulingStore(
            dashboard,
            _productionOperations.putIfAbsent(
              parts[1],
              () => MockProductionOperationsStore(
                [for (final b in store.branches) '${b['id']}'],
                timezones: {
                  for (final b in store.branches)
                    '${b['id']}': '${b['timezone']}',
                },
                empty: dashboardScenario == MockDashboardScenario.empty,
              ),
            ),
          ),
        );
        final schedulingResult = scheduling.handle(request, body);
        if (schedulingResult != null) {
          if (schedulingResult['_message'] != null) {
            return _error(
              schedulingResult['_status'] as int,
              schedulingResult['_code'].toString(),
              schedulingResult['_message'].toString(),
            );
          }
          return _ok(
            schedulingResult['_data'],
            status: schedulingResult['_status'] as int,
          );
        }

        final deliveries = _vendorMissions.putIfAbsent(
          parts[1],
          () => MockVendorMissionsStore(
            dashboard,
            _settings.putIfAbsent(
              parts[1],
              () => MockSettingsStore(store, dashboardScenario),
            ),
          ),
        );
        final deliveryResult = deliveries.handle(request, body);
        if (deliveryResult != null) {
          if (deliveryResult['_message'] != null) {
            return _error(
              deliveryResult['_status'] as int,
              deliveryResult['_code'].toString(),
              deliveryResult['_message'].toString(),
            );
          }
          return _ok(
            deliveryResult['_data'],
            status: deliveryResult['_status'] as int,
          );
        }
        final readiness = _readiness.putIfAbsent(
          parts[1],
          () => MockReadinessStore(dashboard),
        );
        final readinessResult = readiness.handle(request, body);
        if (readinessResult != null) {
          if (readinessResult['_message'] != null) {
            return _error(
              readinessResult['_status'] as int,
              readinessResult['_code'].toString(),
              readinessResult['_message'].toString(),
            );
          }
          return _ok(
            readinessResult['_data'],
            status: readinessResult['_status'] as int,
          );
        }
        final support = _support.putIfAbsent(
          parts[1],
          () => MockSupportStore(dashboard),
        );
        final result = support.handle(request, body);
        if (result != null) {
          if (result['_message'] != null) {
            return _error(
              result['_status'] as int,
              'SUPPORT_REQUEST_REFUSED',
              result['_message'].toString(),
            );
          }
          return _ok(result['_data'], status: result['_status'] as int);
        }
      }
      final settings = _settings.putIfAbsent(
        parts[1],
        () => MockSettingsStore(store, dashboardScenario),
      );
      if (request.url.path.startsWith('/api/v1/vendor/media/')) {
        final catalogue = _catalogues.putIfAbsent(
          parts[1],
          () => MockCatalogueStore(
            store,
            empty: dashboardScenario == MockDashboardScenario.empty,
          ),
        );
        final pastry = _pastries.putIfAbsent(
          parts[1],
          () => MockPastryStore(
            store,
            _productionOperations.putIfAbsent(
              parts[1],
              () => MockProductionOperationsStore([
                for (final b in store.branches) '${b['id']}',
              ], empty: dashboardScenario == MockDashboardScenario.empty),
            ),
            empty: dashboardScenario == MockDashboardScenario.empty,
          ),
        );
        bool eligible(String kind, String id, bool write) {
          final Map<String, dynamic>? row = switch (kind) {
            'FOOD_ITEM' => catalogue.read('/api/v1/vendor/food/items/$id', {}),
            'SHOP_PRODUCT' => catalogue.read(
              '/api/v1/vendor/shop/products/$id',
              {},
            ),
            'BRAND' => store.brands.where((r) => r['id'] == id).firstOrNull,
            'PASTRY_REQUEST' =>
              pastry.requests.where((r) => r['id'] == id).firstOrNull,
            _ => null,
          };
          return row != null &&
              row['_status'] == null &&
              (!write ||
                  ![
                    'ARCHIVED',
                    'CANCELLED',
                    'FULFILLED',
                    'DECLINED',
                  ].contains(row['status']));
        }

        final result = await _media
            .putIfAbsent(parts[1], () => MockMediaStore(parts[1]))
            .handle(request, body, eligible);
        if (result != null) return result;
      }
      final settingsResponse = settings.handle(request, body);
      if (settingsResponse != null) {
        if (settingsResponse['_message'] != null) {
          return _error(
            settingsResponse['_status'] as int,
            'SETTINGS_REQUEST_REFUSED',
            settingsResponse['_message'].toString(),
          );
        }
        return _ok(
          settingsResponse['_data'],
          status: settingsResponse['_status'] as int,
        );
      }
      if ({'ACTIVE', 'PAUSED'}.contains(store.business['status']) &&
          request.url.path.startsWith('/api/v1/vendor/crm/')) {
        final result = _crm
            .putIfAbsent(
              parts[1],
              () => MockCrmStore(
                store,
                settings,
                empty: dashboardScenario == MockDashboardScenario.empty,
              ),
            )
            .handle(request, body);
        if (result?['_bytes'] is List<int>) {
          return http.StreamedResponse(
            Stream.value(result!['_bytes'] as List<int>),
            200,
            headers: {
              'content-type': result['_mime'] as String,
              'cache-control': 'no-store',
            },
          );
        }
        if (result != null) {
          if (result['_message'] != null) {
            return _error(
              result['_status'] as int,
              result['_code'].toString(),
              result['_message'].toString(),
            );
          }
          return _ok(result['_data'], status: result['_status'] as int);
        }
      }
      if ({'ACTIVE', 'PAUSED'}.contains(store.business['status']) &&
          request.url.path.startsWith('/api/v1/vendor/finance/')) {
        final finance = _finance.putIfAbsent(
          parts[1],
          () => MockFinanceStore(store, dashboardScenario),
        );
        final result = request.method == 'GET'
            ? finance.read(request.url.path, request.url.queryParameters)
            : request.method == 'POST' &&
                  request.url.path.endsWith('/withdrawals')
            ? finance.request(body, request.headers)
            : null;
        if (result != null) {
          if (result['_status'] is int) {
            return _error(
              result['_status'] as int,
              'FINANCE_REQUEST_REFUSED',
              result['_message'].toString(),
            );
          }
          return _ok(
            result.containsKey('_list') ? result['_list'] : result,
            status: request.method == 'POST' ? 202 : 200,
          );
        }
      }
      if ({'ACTIVE', 'PAUSED'}.contains(store.business['status'])) {
        final catalogue = _catalogues.putIfAbsent(
          parts[1],
          () => MockCatalogueStore(
            store,
            empty: dashboardScenario == MockDashboardScenario.empty,
          ),
        );
        if (request.url.path.startsWith('/api/v1/vendor/pastry/')) {
          final pastry = _pastries.putIfAbsent(
            parts[1],
            () => MockPastryStore(
              store,
              _productionOperations.putIfAbsent(
                parts[1],
                () => MockProductionOperationsStore(
                  [for (final b in store.branches) '${b['id']}'],
                  timezones: {
                    for (final b in store.branches)
                      '${b['id']}': '${b['timezone']}',
                  },
                  empty: dashboardScenario == MockDashboardScenario.empty,
                ),
              ),
              empty: dashboardScenario == MockDashboardScenario.empty,
            ),
          );
          pastry.crm = _crm.putIfAbsent(
            parts[1],
            () => MockCrmStore(
              store,
              settings,
              empty: dashboardScenario == MockDashboardScenario.empty,
            ),
          );
          final result = pastry.handle(request, body);
          if (result != null) {
            if (result['_message'] != null) {
              return _error(
                result['_status'] as int,
                result['_code'].toString(),
                result['_message'].toString(),
              );
            }
            return _ok(result['_data'], status: result['_status'] as int);
          }
        }
        if (request.url.path.startsWith('/api/v1/vendor/cloud-kitchen/')) {
          final production = _productionOperations.putIfAbsent(
            parts[1],
            () => MockProductionOperationsStore(
              [for (final b in store.branches) '${b['id']}'],
              timezones: {
                for (final b in store.branches)
                  '${b['id']}': '${b['timezone']}',
              },
              empty: dashboardScenario == MockDashboardScenario.empty,
            ),
          );
          final kitchen = _cloudKitchens.putIfAbsent(
            parts[1],
            () => MockCloudKitchenStore(
              store,
              catalogue,
              production,
              empty: dashboardScenario == MockDashboardScenario.empty,
              crm: _crm.putIfAbsent(
                parts[1],
                () => MockCrmStore(
                  store,
                  settings,
                  empty: dashboardScenario == MockDashboardScenario.empty,
                ),
              ),
            ),
          );
          final result = kitchen.handle(request, body);
          if (result != null) {
            if (result['_message'] != null) {
              return _error(
                result['_status'] as int,
                result['_code'].toString(),
                result['_message'].toString(),
              );
            }
            return _ok(result['_data'], status: result['_status'] as int);
          }
        }
        if (request.url.path.contains('/taxonomy/')) {
          final taxonomy = _taxonomies.putIfAbsent(
            parts[1],
            () => MockTaxonomyStore(catalogue, store.vendorId),
          );
          final result = taxonomy.handle(request, body);
          if (result != null) {
            if (result['_message'] != null) {
              return _error(
                result['_status'] as int,
                result['_code'].toString(),
                result['_message'].toString(),
              );
            }
            return _ok(result['_data'], status: result['_status'] as int);
          }
        }
        if (request.url.path.startsWith('/api/v1/vendor/production/')) {
          final production = _productionOperations.putIfAbsent(
            parts[1],
            () => MockProductionOperationsStore(
              [for (final branch in store.branches) '${branch['id']}'],
              timezones: {
                for (final branch in store.branches)
                  '${branch['id']}': '${branch['timezone']}',
              },
              empty: dashboardScenario == MockDashboardScenario.empty,
            ),
          );
          final result = production.handle(request, body);
          if (result != null) {
            if (result['_message'] != null) {
              return _error(
                result['_status'] as int,
                result['_code'].toString(),
                result['_message'].toString(),
              );
            }
            return _ok(result['_data'], status: result['_status'] as int);
          }
        }
        if (RegExp(
          r'^/api/v1/vendor/(production-batches|production-stock-policies|cost|production-cost)(/|$)',
        ).hasMatch(request.url.path)) {
          final materials = _productionMaterials.putIfAbsent(
            parts[1],
            () => MockProductionMaterialsStore(
              store.vendorId,
              branchIds: [
                for (final branch in store.branches) '${branch['id']}',
              ],
              empty: dashboardScenario == MockDashboardScenario.empty,
            ),
          );
          final recipes = _recipes.putIfAbsent(
            parts[1],
            () => MockRecipesStore(
              store.vendorId,
              catalogue,
              materials,
              empty: dashboardScenario == MockDashboardScenario.empty,
            ),
          );
          final stock = _purchasing.putIfAbsent(
            parts[1],
            () => MockPurchasingStore(
              store,
              empty: dashboardScenario == MockDashboardScenario.empty,
            ),
          );
          final production = _productionBatches.putIfAbsent(
            parts[1],
            () => MockProductionBatchesStore(
              recipes,
              stock,
              jobExists: (id, branch) =>
                  _productionOperations[parts[1]]?.hasJob(id, branch) == true,
            ),
          );
          final result =
              RegExp(
                r'^/api/v1/vendor/(cost|production-cost)(/|$)',
              ).hasMatch(request.url.path)
              ? _costs
                    .putIfAbsent(parts[1], () => MockCostsStore(production))
                    .handle(request, body)
              : production.handle(request, body);
          if (result != null) {
            if (result['_message'] != null) {
              return _error(
                result['_status'] as int,
                result['_code'].toString(),
                result['_message'].toString(),
              );
            }
            return _ok(result['_data'], status: result['_status'] as int);
          }
        }
        if (RegExp(
          r'^/api/v1/vendor/(recipes|recipe-cost)(/|$)',
        ).hasMatch(request.url.path)) {
          final materials = _productionMaterials.putIfAbsent(
            parts[1],
            () => MockProductionMaterialsStore(
              store.vendorId,
              branchIds: [
                for (final branch in store.branches) '${branch['id']}',
              ],
              empty: dashboardScenario == MockDashboardScenario.empty,
            ),
          );
          final recipes = _recipes.putIfAbsent(
            parts[1],
            () => MockRecipesStore(
              store.vendorId,
              catalogue,
              materials,
              empty: dashboardScenario == MockDashboardScenario.empty,
            ),
          );
          final result = recipes.handle(request, body);
          if (result != null) {
            if (result['_message'] != null) {
              return _error(
                result['_status'] as int,
                result['_code'].toString(),
                result['_message'].toString(),
              );
            }
            return _ok(result['_data'], status: result['_status'] as int);
          }
        }
        if (RegExp(
          r'^/api/v1/vendor/(ingredients|packaging-materials)(/|$)',
        ).hasMatch(request.url.path)) {
          final materials = _productionMaterials.putIfAbsent(
            parts[1],
            () => MockProductionMaterialsStore(
              store.vendorId,
              branchIds: [
                for (final branch in store.branches) '${branch["id"]}',
              ],
              empty: dashboardScenario == MockDashboardScenario.empty,
            ),
          );
          final result = materials.handle(request, body);
          if (result != null) {
            if (result['_message'] != null) {
              return _error(
                result['_status'] as int,
                result['_code'].toString(),
                result['_message'].toString(),
              );
            }
            return _ok(result['_data'], status: result['_status'] as int);
          }
        }
        if (request.url.path.contains('/purchasing/') ||
            RegExp(
              r'^/api/v1/vendor/inventory/branches/[^/]+/balances$',
            ).hasMatch(request.url.path)) {
          final purchasing = _purchasing.putIfAbsent(
            parts[1],
            () => MockPurchasingStore(
              store,
              empty: dashboardScenario == MockDashboardScenario.empty,
            ),
          );
          final result = purchasing.handle(request, body);
          if (result != null) {
            if (result['_message'] != null) {
              return _error(
                result['_status'] as int,
                result['_code'].toString(),
                result['_message'].toString(),
              );
            }
            return _ok(result['_data'], status: result['_status'] as int);
          }
        }
        final advanced = _advancedCatalogues.putIfAbsent(
          parts[1],
          () => MockAdvancedCatalogueStore(catalogue, store),
        );
        final inventory = _inventory.putIfAbsent(
          parts[1],
          () => MockInventoryStore(catalogue, advanced),
        );
        final inventoryResult = inventory.handle(request, body);
        if (inventoryResult != null) {
          if (inventoryResult['_message'] != null) {
            return _error(
              inventoryResult['_status'] as int,
              inventoryResult['_code'].toString(),
              inventoryResult['_message'].toString(),
            );
          }
          return _ok(
            inventoryResult['_data'],
            status: inventoryResult['_status'] as int,
          );
        }
        final advancedResult = advanced.handle(request, body);
        if (advancedResult != null) {
          if (advancedResult['_message'] != null) {
            return _error(
              advancedResult['_status'] as int,
              advancedResult['_code']?.toString() ??
                  'CATALOGUE_CONFIGURATION_REFUSED',
              advancedResult['_message'].toString(),
            );
          }
          return _ok(
            advancedResult['_data'],
            status: advancedResult['_status'] as int,
          );
        }
        final path = request.url.path;
        final result = request.method == 'GET'
            ? catalogue.read(path, request.url.queryParameters)
            : catalogue.handles(path)
            ? catalogue.write(request.method, path, body, request.headers)
            : null;
        if (result != null) {
          if (result['_status'] is int) {
            return _error(
              result['_status'] as int,
              'CATALOGUE_COMMAND_REFUSED',
              result['_message'].toString(),
            );
          }
          return _ok(result.containsKey('_list') ? result['_list'] : result);
        }
      }
      if (request.method == 'POST' &&
          store.business['status'] == 'ACTIVE' &&
          request.url.path.startsWith('/api/v1/vendor/commerce/orders/')) {
        final result = dashboard.command(
          request.url.path,
          body,
          request.headers,
          canReplace: _catalogues[parts[1]]?.acceptsReplacement,
        );
        if (result['_status'] is int) {
          return _error(
            result['_status'] as int,
            'ORDER_COMMAND_REFUSED',
            result['_message'].toString(),
          );
        }
        return _ok(result);
      }
      if (request.method == 'GET' &&
          {'ACTIVE', 'PAUSED'}.contains(store.business['status'])) {
        final path = request.url.path;
        if (path.startsWith('/api/v1/vendor/finance/') &&
            dashboardScenario == MockDashboardScenario.restricted) {
          return _error(
            403,
            'FORBIDDEN',
            'You do not have permission to view finance.',
          );
        }
        if (path == '/api/v1/vendor/commerce/orders' &&
            dashboardScenario == MockDashboardScenario.unavailable) {
          return _error(
            503,
            'UNAVAILABLE',
            'Orders are temporarily unavailable.',
          );
        }
        final data = dashboard.response(path, request.url.queryParameters);
        if (data != null) {
          return _ok(data.containsKey('_list') ? data['_list'] : data);
        }
      }
      return store.handle(request, body);
    }
    if (request.method != 'POST') {
      return _error(
        405,
        'METHOD_NOT_ALLOWED',
        'This mock supports auth POST requests only.',
      );
    }
    switch (request.url.path) {
      case '/api/v1/vendor/auth/start':
        if ((body['phone'] == null) == (body['email'] == null)) {
          return _error(422, 'VALIDATION_ERROR', 'Provide phone or email.');
        }
        _identifier = (body['phone'] ?? body['email']) as String;
        return _issueChallenge();
      case '/api/v1/vendor/auth/resend':
        if (!_matches(body) ||
            _challenge != body['challenge_id'] ||
            _consumed) {
          return _error(
            404,
            'NOT_FOUND',
            'No active verification code to resend. Start again.',
          );
        }
        return _issueChallenge();
      case '/api/v1/vendor/auth/register/verify':
      case '/api/v1/vendor/auth/verify':
        if (!_matches(body) ||
            _challenge != body['challenge_id'] ||
            _consumed ||
            _expiresAt == null ||
            DateTime.now().isAfter(_expiresAt!)) {
          return _error(
            401,
            'UNAUTHORIZED',
            'Invalid or expired verification code.',
          );
        }
        if (body['otp'] != code) {
          return _error(
            401,
            'UNAUTHORIZED',
            'Invalid or expired verification code.',
          );
        }
        if (request.url.path == '/api/v1/vendor/auth/register/verify') {
          final name = body['legal_name']?.toString().trim() ?? '';
          if (name.length < 2 ||
              name.length > 200 ||
              !RegExp(
                r'^[A-Z]{3}$',
              ).hasMatch(body['currency']?.toString() ?? '') ||
              (body['timezone']?.toString().isEmpty ?? true)) {
            return _error(
              422,
              'VALIDATION_ERROR',
              'Check your business details.',
            );
          }
          final store = MockOnboardingStore(
            vendorIds.first,
            MockOnboardingScenario.draft,
          );
          store.business['legal_name'] = name;
          store.business['trading_name'] = null;
          store.business['currency'] = body['currency'];
          store.business['timezone'] = body['timezone'];
          _businesses[vendorIds.first] = store;
          _consumed = true;
          return _ok(_tokens(vendorIds.first));
        }
        if (scenario == MockAuthScenario.accessUnavailable) {
          return _error(
            403,
            'FORBIDDEN',
            'You do not have active staff access to any vendor business.',
          );
        }
        if (scenario == MockAuthScenario.accessSuspended) {
          return _error(
            403,
            'FORBIDDEN',
            'Your access has been suspended. Contact your business owner.',
          );
        }
        final vendorId = body['vendor_id'] as String?;
        if (vendorId == null && scenario != MockAuthScenario.singleVendor) {
          return _ok({
            'requires_vendor_choice': true,
            'available_vendor_ids': vendorIds,
          });
        }
        if (vendorId != null && !vendorIds.contains(vendorId)) {
          return _error(
            401,
            'UNAUTHORIZED',
            'You do not have staff access to this vendor business.',
          );
        }
        _consumed = true;
        return _ok(_tokens(vendorId ?? vendorIds.first));
      case '/api/v1/vendor/auth/refresh':
        if (!(body['refresh_token'] as String? ?? '').startsWith(
          'mock-refresh:',
        )) {
          return _error(401, 'UNAUTHORIZED', 'Sign in required.');
        }
        final vendorId = (body['refresh_token'] as String).split(':')[1];
        _businesses[vendorId]?.invalidateSecurityChallenges();
        return _ok(_tokens(vendorId));
      case '/api/v1/vendor/auth/logout':
        return _ok(null, status: 204);
      default:
        return _error(
          501,
          'NOT_IMPLEMENTED',
          'This feature is outside the mock auth scope.',
        );
    }
  }

  bool _matches(Map<String, dynamic> body) =>
      _identifier != null && (body['phone'] ?? body['email']) == _identifier;
  http.StreamedResponse _issueChallenge() {
    _challenge = 'mock-challenge-${++_sequence}';
    _consumed = false;
    final seconds = scenario == MockAuthScenario.expiredCode ? 1 : 300;
    _expiresAt = DateTime.now().add(Duration(seconds: seconds));
    return _ok({'challenge_id': _challenge, 'expires_in': seconds});
  }

  Map<String, Object> _tokens(String vendorId) => {
    'access_token': 'mock-access:$vendorId:${++_sequence}',
    'refresh_token': 'mock-refresh:$vendorId:$_sequence',
    'expires_in': 900,
    'staff_id': 'mock-staff-1',
    'vendor_id': vendorId,
  };
  http.StreamedResponse _ok(Object? data, {int status = 200}) => _response(
    status,
    status == 204 ? '' : jsonEncode({'success': true, 'data': data}),
  );
  http.StreamedResponse _error(
    int status,
    String code,
    String message, {
    Map<String, String> headers = const {},
  }) => _response(
    status,
    jsonEncode({
      'success': false,
      'error': {'code': code, 'message': message},
    }),
    headers: headers,
  );
  http.StreamedResponse _response(
    int status,
    String body, {
    Map<String, String> headers = const {},
  }) => http.StreamedResponse(
    Stream.value(utf8.encode(body)),
    status,
    headers: {'content-type': 'application/json', ...headers},
  );
}
