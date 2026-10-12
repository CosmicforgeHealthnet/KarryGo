import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../app/vendor_app.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_response.dart';

class DashboardController extends ChangeNotifier {
  DashboardController(this.app);
  final VendorApp app;
  JsonObject business = {}, wallet = {};
  List<JsonObject> branches = [],
      capabilities = [],
      orders = [],
      transactions = [],
      menus = [],
      products = [];
  final Map<String, String> errors = {};
  final Set<String> restricted = {};
  bool loading = false,
      catalogueLoading = false,
      catalogueLoaded = false,
      _disposed = false;
  int _generation = 0;
  String? get branchId => app.scope.branchId;
  JsonObject? get branch =>
      branches.where((b) => b['id'] == branchId).firstOrNull;
  bool get food => capabilities.any(
    (c) => [
      'RESTAURANT',
      'FAST_FOOD',
      'CLOUD_KITCHEN',
      'PASTRY_BAKERY',
      'CATERING',
    ].contains(c['capability_type']),
  );
  bool get shop => capabilities.any(
    (c) =>
        ['SHOP_GROCERY', 'MARKET', 'WHOLESALE'].contains(c['capability_type']),
  );
  bool get awaitingActivation => {
    'DRAFT',
    'PENDING_VERIFICATION',
    'PENDING_REVIEW',
  }.contains(business['status']);
  int count(Set<String> statuses) =>
      orders.where((o) => statuses.contains(o['status'])).length;
  static const incoming = {'PAID', 'SENT_TO_VENDOR'};
  static const working = {
    'ACCEPTED',
    'PREPARING',
    'PICKING',
    'PACKING',
    'SUBSTITUTION_REQUIRED',
    'AWAITING_CUSTOMER',
  };
  static const ready = {'READY', 'READY_FOR_PICKUP'};
  static List<JsonObject> rows(Object? value, String key) {
    if (value is Map) value = value[key];
    if (value is! List) throw const FormatException('Incomplete response');
    return value.map(jsonObject).toList();
  }

  void _notify() {
    if (!_disposed) notifyListeners();
  }

  Future<void> _read(
    String key,
    Future<VendorResponse> Function() request,
    void Function(Object?) assign,
    int generation,
  ) async {
    try {
      final response = await request();
      if (_disposed || generation != _generation) return;
      assign(response.data);
      errors.remove(key);
      restricted.remove(key);
    } catch (e) {
      if (_disposed || generation != _generation) return;
      if (e is ApiException && e.statusCode == 403) {
        restricted.add(key);
      }
      errors[key] = e is ApiException
          ? e.statusCode == 404
                ? 'This feature is not enabled on the connected server.'
                : e.message
          : e is FormatException
          ? 'The server returned incomplete information. Please try again.'
          : 'Could not reach the server. Check your connection and try again.';
    }
  }

  Future<VendorResponse> _pages(
    Future<VendorResponse> Function(Map<String, String>) fetch,
  ) async {
    final items = <JsonObject>[];
    var offset = 0;
    while (true) {
      final response = await fetch({'limit': '100', 'offset': '$offset'});
      final data = jsonObject(response.data);
      final page = rows(data, 'items');
      items.addAll(page);
      final pagination = data['pagination'];
      if (pagination is! Map || pagination['has_more'] != true) {
        return VendorResponse(
          statusCode: response.statusCode,
          headers: response.headers,
          data: {'items': items},
        );
      }
      if (page.isEmpty) throw const FormatException('Invalid pagination');
      offset += page.length;
    }
  }

  Future<void> load() async {
    final generation = ++_generation;
    loading = true;
    errors.clear();
    restricted.clear();
    business = {};
    branches = [];
    capabilities = [];
    menus = [];
    products = [];
    catalogueLoaded = false;
    catalogueLoading = false;
    wallet = {};
    orders = [];
    transactions = [];
    _notify();
    final id = app.scope.vendorId!;
    await Future.wait([
      _read(
        'business',
        () => app.businessApi.getBusinessesBusinessId(businessId: id),
        (data) => business = jsonObject(data),
        generation,
      ),
      _read(
        'branches',
        () => _pages(
          (query) => app.businessApi.getBusinessesBusinessIdBranches(
            businessId: id,
            query: query,
          ),
        ),
        (data) => branches = rows(
          data,
          'items',
        ).where((b) => b['status'] != 'CLOSED').toList(),
        generation,
      ),
      _read(
        'capabilities',
        () => _pages(
          (query) => app.businessApi.getBusinessesBusinessIdCapabilities(
            businessId: id,
            query: query,
          ),
        ),
        (data) => capabilities = rows(data, 'items'),
        generation,
      ),
      _read('wallet', () => app.financeApi.getFinanceOverview(), (data) {
        final value = jsonObject(data);
        if (value['available_kobo'] is! int ||
            value['pending_kobo'] is! int ||
            value['currency'] is! String) {
          throw const FormatException('Incomplete wallet');
        }
        wallet = value;
      }, generation),
      _read(
        'transactions',
        () => app.financeApi.getFinanceTransactions(query: {'limit': '10'}),
        (data) => transactions = rows(data, 'items'),
        generation,
      ),
    ]);
    if (_disposed || generation != _generation) return;
    if (!branches.any((b) => b['id'] == branchId)) {
      app.scope.select(vendorId: id, branchId: branches.firstOrNull?['id']);
    }
    await _read(
      'orders',
      () => app.ordersApi.getCommerceOrders(
        query: {'limit': '50', 'branch_id': ?branchId},
      ),
      (data) => orders = rows(data, 'vendor_orders'),
      generation,
    );
    if (_disposed || generation != _generation) return;
    loading = false;
    _notify();
  }

  Future<void> selectBranch(String id) async {
    if (!branches.any((b) => b['id'] == id) || id == branchId) return;
    app.scope.select(vendorId: app.scope.vendorId!, branchId: id);
    menus = [];
    products = [];
    catalogueLoaded = false;
    catalogueLoading = false;
    await load();
  }

  Future<void> loadCatalogue() async {
    if (catalogueLoading || loading || _disposed) return;
    final generation = _generation;
    catalogueLoading = true;
    _notify();
    await Future.wait([
      if (food)
        _read(
          'menus',
          () => app.foodCatalogueApi.getFoodMenus(
            query: {'limit': '50', 'branch_id': ?branchId},
          ),
          (data) => menus = rows(data, 'menus'),
          generation,
        ),
      if (shop)
        _read(
          'products',
          () => app.shopCatalogueApi.getShopProducts(query: {'limit': '50'}),
          (data) => products = rows(data, 'items'),
          generation,
        ),
    ]);
    if (_disposed || generation != _generation) return;
    catalogueLoading = false;
    catalogueLoaded = true;
    _notify();
  }

  @override
  void dispose() {
    _disposed = true;
    _generation++;
    super.dispose();
  }
}
