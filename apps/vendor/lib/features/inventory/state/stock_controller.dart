import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../app/vendor_app.dart';
import '../../../app/data/vendor_preferences.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_command.dart';
import '../../catalogue/state/advanced_controller.dart';
import '../../catalogue/state/advanced_resource.dart';

enum StockSection {
  balances,
  receipts,
  transfers,
  quarantines,
  stocktakes,
  movements,
  alerts,
}

class StockController extends ChangeNotifier {
  StockController(this.app, this.branchId, {VendorPreferences? preferences})
    : preferences = preferences ?? VendorPreferences();
  final VendorApp app;
  final String branchId;
  final VendorPreferences preferences;
  late final recoveryKey =
      'vendor.stock-update.${base64Url.encode(utf8.encode('${app.client.config.origin}|${app.scope.vendorId}|${app.auth.session?.staffId}|$branchId'))}';
  JsonObject? pending;
  bool recoveryBlocked = false;
  bool get hasUnresolvedUpdate => pending != null || recoveryBlocked;
  Future<void>? _restoring;

  Future<void> restorePending() => _restoring ??= _restorePending();
  Future<void> _restorePending() async {
    try {
      final raw = await preferences.read(recoveryKey);
      if (raw == null || raw == '{}') {
        recoveryBlocked = false;
        error = null;
        emit();
        return;
      }
      final saved = jsonObject(jsonDecode(raw));
      if (saved['vendor'] != app.scope.vendorId ||
          saved['branch'] != branchId ||
          saved['method'] is! String ||
          saved['path'] is! String ||
          saved['parameters'] is! Map ||
          saved['fingerprint'] is! String ||
          saved['key'] is! String ||
          !RegExp(r'^[a-f0-9]{48}$').hasMatch(saved['key']) ||
          saved['body'] != null && saved['body'] is! Map ||
          !(saved['parameters'] as Map).values.every((v) => v is String) ||
          !['POST', 'PUT', 'PATCH', 'DELETE'].contains(saved['method']) ||
          !((saved['path'] as String).startsWith('$root/') ||
              (saved['path'] as String).startsWith('$branchPath/'))) {
        throw const FormatException('Invalid saved stock update');
      }
      final fingerprint =
          '${saved['method']} ${saved['path']} ${jsonEncode(saved['parameters'])} ${jsonEncode(saved['body'])}';
      if (saved['fingerprint'] != fingerprint ||
          (saved['path'] as String).startsWith('$branchPath/') &&
              saved['parameters']['branchId'] != branchId) {
        throw const FormatException('Saved stock update scope does not match');
      }
      app.shopCatalogueApi.operation(saved['method'], saved['path']);
      pending = saved;
      commands[saved['fingerprint']] = VendorCommand(
        idempotencyKey: saved['key'],
      );
      uncertainOutcome = true;
      recoveryBlocked = false;
      error = null;
    } catch (_) {
      recoveryBlocked = true;
      error =
          'The saved stock update could not be read. Retry recovery before making another update.';
    }
    emit();
  }

  Future<JsonObject?> retryPending() async {
    if (busy || disposed) return null;
    if (recoveryBlocked) {
      _restoring = null;
      await restorePending();
    }
    final saved = pending;
    if (saved == null || recoveryBlocked) return null;
    return mutate(
      saved['method'],
      saved['path'],
      parameters: Map<String, String>.from(saved['parameters']),
      body: saved['body'] == null ? null : jsonObject(saved['body']),
    );
  }

  Future<void> _clearPending(String fingerprint) async {
    await preferences.write(recoveryKey, '{}');
    pending = null;
    commands.remove(fingerprint);
    uncertainOutcome = false;
  }

  static const branchPath = '/api/v1/vendor/shop/branches/{branchId}/inventory';
  static const root = '/api/v1/vendor/shop/inventory';
  late final reader = AdvancedController(
    app,
    const AdvancedResource(AdvancedKind.measures),
  );
  final records = <StockSection, List<JsonObject>>{};
  final errors = <StockSection, String>{};
  final denied = <String>{};
  final labels = <String, String>{};
  final commands = <String, VendorCommand>{};
  List<JsonObject> listings = [], measures = [];
  List<JsonObject> branches = [];
  bool loading = false,
      busy = false,
      disposed = false,
      uncertainOutcome = false;
  StockSection section = StockSection.balances;
  String? error, errorCode, setupError;
  List<JsonObject> get rows => records[section] ?? [];
  void emit() {
    if (!disposed) notifyListeners();
  }

  String name(Object? id) => labels[id] ?? id?.toString() ?? 'Stock item';
  String message(Object e) => e is ApiException
      ? e.message
      : 'Inventory could not be loaded. Try again.';
  static void validateQuarantine(JsonObject row) {
    if (row['id'] is! String ||
        row['branch_ref'] is! String ||
        row['stock_basis_ref'] is! String ||
        row['variant_ref'] is! String ||
        row['quantity'] is! int ||
        (row['quantity'] as int) <= 0 ||
        !{'ACTIVE', 'RELEASED', 'DISPOSED'}.contains(row['status']) ||
        row['reason_code'] is! String) {
      throw const FormatException('Incomplete quarantine');
    }
  }

  Future<JsonObject> quarantine(String id) async {
    final rows = await all(
      '$branchPath/quarantines',
      parameters: {'branchId': branchId},
    );
    for (final row in rows) {
      validateQuarantine(row);
      if (row['id'] == id && row['branch_ref'] == branchId) return row;
    }
    throw const FormatException(
      'Quarantine no longer available in this branch',
    );
  }

  static void validateReorderRule(JsonObject row) {
    for (final field in [
      'id',
      'branch_ref',
      'stock_basis_ref',
      'variant_ref',
      'status',
    ]) {
      if (row[field] is! String) {
        throw const FormatException('Incomplete reorder rule');
      }
    }
    for (final field in [
      'minimum_quantity',
      'target_quantity',
      'recommended_reorder_quantity',
      'version',
    ]) {
      if (row[field] is! int) {
        throw const FormatException('Incomplete reorder quantities');
      }
    }
    if (row['minimum_quantity'] < 0 ||
        row['target_quantity'] <= row['minimum_quantity'] ||
        row['target_quantity'] > 1000000000 ||
        row['recommended_reorder_quantity'] < 0 ||
        row['expiring_soon_hours'] != null &&
            (row['expiring_soon_hours'] is! int ||
                row['expiring_soon_hours'] < 1 ||
                row['expiring_soon_hours'] > 8760)) {
      throw const FormatException('Invalid reorder rule');
    }
  }

  Future<JsonObject?> reorderRule(String basis) async {
    final rows = await all(
      '$branchPath/reorder-rules',
      parameters: {'branchId': branchId},
    );
    for (final row in rows) {
      validateReorderRule(row);
      if (row['branch_ref'] != branchId) {
        throw const FormatException('Reorder branch scope does not match');
      }
    }
    return rows.where((row) => row['stock_basis_ref'] == basis).firstOrNull;
  }

  Future<List<JsonObject>> all(
    String path, {
    Map<String, String> parameters = const {},
  }) => reader.all(app.shopCatalogueApi, path, parameters: parameters);
  Future<void> load({StockSection? selected, bool append = false}) async {
    if (loading || busy || disposed) return;
    section = selected ?? section;
    final current = section;
    loading = true;
    error = null;
    emit();
    try {
      final response = await app.shopCatalogueApi.request(
        'GET',
        '$branchPath/${current.name}',
        pathParameters: {'branchId': branchId},
        query: current == StockSection.balances
            ? {}
            : {'limit': '100', 'offset': append ? '${rows.length}' : '0'},
      );
      final next = collection(response.data);
      if (current == StockSection.quarantines) {
        for (final row in next) {
          validateQuarantine(row);
          if (row["branch_ref"] != branchId) {
            throw const FormatException(
              "Quarantine branch scope does not match",
            );
          }
        }
      }
      if (current == StockSection.transfers && branches.isEmpty) {
        try {
          await loadBranches();
        } catch (_) {}
      }
      if (current == StockSection.balances) {
        for (final row in next) {
          if (row['stock_basis_ref'] is! String ||
              row['variant_ref'] is! String ||
              row['physical_on_hand'] is! int ||
              row['physical_available'] is! int) {
            throw const FormatException('Incomplete stock balance');
          }
        }
      }
      hasMore = response.data is Map && response.object['has_more'] == true;
      if (hasMore && next.isEmpty) {
        throw const FormatException('Inventory pagination made no progress');
      }
      records[current] = append ? [...rows, ...next] : next;
      errors.remove(current);
    } catch (e) {
      errors[current] = message(e);
      records.remove(current);
    }
    loading = false;
    emit();
  }

  bool hasMore = false;
  Future<void> more() => load(append: true);
  static List<JsonObject> collection(Object? data) {
    if (data == null) return [];
    if (data is Map) {
      if (!data.containsKey('items')) {
        throw const FormatException('Incomplete inventory response');
      }
      data = data['items'];
    }
    if (data == null) return [];
    if (data is! List) {
      throw const FormatException('Incomplete inventory response');
    }
    return data.map(jsonObject).toList();
  }

  Future<void> prepare() async {
    await restorePending();
    setupError = null;
    loading = true;
    emit();
    try {
      listings = await all(
        '/api/v1/vendor/shop/branches/{branchId}/listings',
        parameters: {'branchId': branchId},
      );
      final products = await all('/api/v1/vendor/shop/products');
      for (final product in products) {
        labels[product['id'].toString()] = product['name'].toString();
        final variants = await all(
          '/api/v1/vendor/shop/products/{productId}/variants',
          parameters: {'productId': product['id'].toString()},
        );
        for (final variant in variants) {
          labels[variant['id'].toString()] =
              '${product['name']} / ${variant['name']}';
        }
      }
      measures = await all('/api/v1/vendor/shop/measurements');
      for (final measure in measures) {
        labels[measure['id'].toString()] = measure['display_name'].toString();
      }
    } catch (e) {
      setupError = message(e);
    }
    loading = false;
    emit();
    await load();
  }

  Future<void> loadBranches() async {
    branches = await reader.all(
      app.businessApi,
      '/api/v1/vendor/businesses/{businessId}/branches',
      parameters: {'businessId': app.scope.vendorId!},
    );
    for (final branch in branches) {
      labels[branch['id'].toString()] = branch['display_name'].toString();
    }
    emit();
  }

  Future<List<JsonObject>> branchBalances(String branch) async => collection(
    (await app.shopCatalogueApi.request(
      'GET',
      '$branchPath/balances',
      pathParameters: {'branchId': branch},
    )).data,
  );

  Future<JsonObject> transfer(String id) async {
    final row = (await app.shopCatalogueApi.request(
      'GET',
      '$root/transfers/{transferId}',
      pathParameters: {'transferId': id},
    )).object;
    if (row['id'] != id ||
        row['source_branch_ref'] is! String ||
        row['destination_branch_ref'] is! String ||
        row['status'] is! String ||
        row['lines'] is! List) {
      throw const FormatException('Incomplete transfer');
    }
    for (final raw in row['lines']) {
      final line = jsonObject(raw);
      if (line['id'] is! String ||
          line['variant_ref'] is! String ||
          [
            'sent_quantity',
            'received_quantity',
            'damaged_quantity',
            'missing_quantity',
          ].any((k) => line[k] is! int || line[k] < 0)) {
        throw const FormatException('Incomplete transfer line');
      }
    }
    return row;
  }

  Future<JsonObject> detail(String kind, String id) async =>
      (await app.shopCatalogueApi.request(
        'GET',
        '$root/$kind/{${kind == 'receipts' ? 'receiptId' : 'stocktakeId'}}',
        pathParameters: {kind == 'receipts' ? 'receiptId' : 'stocktakeId': id},
      )).object;
  Future<JsonObject> balance(String id) async {
    final row = (await app.shopCatalogueApi.request(
      'GET',
      '$branchPath/balances/{stockBasisId}',
      pathParameters: {'branchId': branchId, 'stockBasisId': id},
    )).object;
    for (final key in [
      'on_hand_quantity',
      'available_quantity',
      'reserved_quantity',
      'quarantined_quantity',
      'in_transit_quantity',
      'backorder_quantity',
      'revision',
    ]) {
      if (row[key] is! int) {
        throw const FormatException('Incomplete stock balance');
      }
    }
    if (row['stock_basis_ref'] != id || row['branch_ref'] != branchId) {
      throw const FormatException('Stock balance scope does not match');
    }
    return row;
  }

  Future<JsonObject?> mutate(
    String method,
    String path, {
    JsonObject? body,
    Map<String, String> parameters = const {},
  }) async {
    if (busy || disposed || denied.contains('$method $path')) return null;
    busy = true;
    error = null;
    errorCode = null;
    uncertainOutcome = false;
    emit();
    final fingerprint =
        '$method $path ${jsonEncode(parameters)} ${jsonEncode(body)}';
    final command = commands.putIfAbsent(fingerprint, VendorCommand.create);
    try {
      await restorePending();
      if (recoveryBlocked) {
        commands.remove(fingerprint);
        return null;
      }
      if (pending != null && pending!['fingerprint'] != fingerprint) {
        commands.remove(fingerprint);
        uncertainOutcome = true;
        error =
            'Confirm the saved stock update before making a different update.';
        return null;
      }
      final op = app.shopCatalogueApi.operation(method, path);
      if (pending == null) {
        if (op.requestSchema != null && body != null) {
          validateRequest(body, op.requestSchema!);
        }
        final saved = jsonObject(
          jsonDecode(
            jsonEncode({
              'vendor': app.scope.vendorId,
              'branch': branchId,
              'method': method,
              'path': path,
              'parameters': parameters,
              'body': body,
              'fingerprint': fingerprint,
              'key': command.idempotencyKey,
            }),
          ),
        );
        await preferences.write(recoveryKey, jsonEncode(saved));
        pending = saved;
      }
      final response = await app.shopCatalogueApi.request(
        method,
        path,
        pathParameters: parameters,
        body: body == null
            ? (op.requestRequired ? VendorPayload({}) : null)
            : VendorPayload(body),
        command: commands[fingerprint] ?? command,
        retryAuthentication: false,
      );
      if (path.startsWith('$root/quarantines')) {
        validateQuarantine(response.object);
        if (response.object['branch_ref'] != branchId) {
          throw const FormatException('Quarantine branch scope does not match');
        }
      }
      if (path == '$root/stock-bases/{stockBasisId}/reorder-rule') {
        validateReorderRule(response.object);
        if (response.object['branch_ref'] != branchId ||
            response.object['stock_basis_ref'] != parameters['stockBasisId']) {
          throw const FormatException('Reorder stock scope does not match');
        }
      }
      await _clearPending(fingerprint);
      return response.object;
    } catch (e) {
      error = e is ApiException
          ? e.message
          : pending == null
          ? 'Your stock update was not sent. Check the details and try again.'
          : 'The stock update could not be confirmed. Retry the saved update to avoid duplicating stock.';
      if (e is VendorContractException && pending == null) {
        commands.remove(fingerprint);
      }
      errorCode = e is ApiException ? e.code : null;
      if (e is ApiException && e.statusCode == 403) denied.add('$method $path');
      if (e is ApiException &&
          e.statusCode != null &&
          e.statusCode! >= 400 &&
          e.statusCode! < 500 &&
          e.statusCode != 408) {
        try {
          await _clearPending(fingerprint);
        } catch (_) {
          recoveryBlocked = true;
        }
      }
      uncertainOutcome = commands.containsKey(fingerprint);
      return null;
    } finally {
      busy = false;
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
