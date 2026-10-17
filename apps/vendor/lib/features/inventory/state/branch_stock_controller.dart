import 'dart:convert';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../app/vendor_app.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_command.dart';
import '../../settings/state/settings_controller.dart';

/// One stocked ingredient or packaging item at a branch.
class BranchStockItem {
  const BranchStockItem(this.id, this.name, this.onHand, [this.unit]);
  final String id, name;
  final int onHand;

  /// Plain unit label such as "kg", or null when it is not known.
  final String? unit;
  String amount(int value) => unit == null ? '$value' : '$value $unit';
}

String? stockUnitLabel(Object? code) {
  final c = '${code ?? ''}'.trim().toUpperCase();
  if (c.isEmpty) return null;
  return switch (c) {
    'G' || 'GRAM' || 'GRAMS' => 'g',
    'KG' || 'KILOGRAM' || 'KILOGRAMS' => 'kg',
    'ML' ||
    'MILLILITRE' ||
    'MILLILITRES' ||
    'MILLILITER' ||
    'MILLILITERS' => 'ml',
    'L' || 'LITRE' || 'LITRES' || 'LITER' || 'LITERS' => 'litres',
    'EA' || 'EACH' || 'COUNT' || 'UNIT' || 'UNITS' => 'units',
    _ => c.toLowerCase(),
  };
}

/// Reads and writes the general (non-shop) inventory routes used by food
/// branches for stock counts and branch-to-branch moves.
class BranchStockController {
  BranchStockController(this.app);
  final VendorApp app;
  static const root = '/api/v1/vendor/inventory';
  final _commands = <String, VendorCommand>{};

  Future<List<JsonObject>> branches() async {
    final reader = SettingsController(app, SettingsKind.branches);
    try {
      final rows = await reader.all('${SettingsController.business}/branches', {
        'businessId': app.scope.vendorId!,
      });
      return rows
          .where(
            (r) =>
                r['id'] is String &&
                r['status'] != 'ARCHIVED' &&
                (r['business_id'] == null ||
                    r['business_id'] == app.scope.vendorId),
          )
          .toList();
    } finally {
      reader.dispose();
    }
  }

  static String branchName(JsonObject branch) =>
      '${branch['display_name'] ?? branch['name'] ?? 'Branch'}';

  Map<String, String>? _units;

  /// Stock units by item name, read from the ingredient and packaging setup.
  /// Best effort: an ambiguous or unreadable name simply has no unit.
  Future<Map<String, String>> units() async {
    if (_units != null) return _units!;
    final byName = <String, List<JsonObject>>{};
    for (final path in [
      '/api/v1/vendor/ingredients',
      '/api/v1/vendor/packaging-materials',
    ]) {
      try {
        final response = await app.productionMasterApi.request(
          'GET',
          path,
          query: {'limit': '200'},
        );
        final data = response.data;
        final rows = data is List
            ? data
            : data is Map && data['items'] is List
            ? data['items'] as List
            : const [];
        for (final row in rows.map(jsonObject)) {
          if (row['status'] == 'ARCHIVED') continue;
          final name = '${row['name'] ?? ''}'.trim().toLowerCase();
          if (name.isNotEmpty) (byName[name] ??= []).add(row);
        }
      } catch (_) {}
    }
    final out = <String, String>{};
    byName.forEach((name, rows) {
      final tracked = rows
          .where((r) => r['inventory_control_mode'] == 'INVENTORY_CONTROLLED')
          .toList();
      final pick = rows.length == 1
          ? rows.first
          : tracked.length == 1
          ? tracked.first
          : null;
      final unit = stockUnitLabel(pick?['default_unit_code']);
      if (unit != null) out[name] = unit;
    });
    return _units = out;
  }

  Future<List<BranchStockItem>> items(String branchId) async {
    final unitByName = await units();
    final out = <BranchStockItem>[];
    var unnamed = 0;
    while (true) {
      final response = await app.inventoryApi.request(
        'GET',
        '$root/branches/{branchId}/balances',
        pathParameters: {'branchId': branchId},
        query: {'limit': '200', 'offset': '${out.length}'},
      );
      final rows = SettingsController.objects(response.data);
      for (final row in rows) {
        if (row['branch_id'] != null && row['branch_id'] != branchId) {
          throw const FormatException('Stock scope mismatch');
        }
        final id = row['inventory_item_id'];
        if (id is! String) continue;
        final name = '${row['name'] ?? ''}'.trim();
        out.add(
          BranchStockItem(
            id,
            name.isNotEmpty ? name : 'Unnamed item ${++unnamed}',
            (row['on_hand'] as num?)?.toInt() ?? 0,
            name.isEmpty ? null : unitByName[name.toLowerCase()],
          ),
        );
      }
      if (rows.length < 200) return out;
    }
  }

  /// Sends a change. A retry of the identical request reuses the same
  /// Idempotency-Key so a lost response can never apply twice.
  Future<JsonObject> send(
    String method,
    String path, {
    Map<String, String> parameters = const {},
    JsonObject? body,
  }) async {
    final fingerprint =
        '$method $path ${jsonEncode(parameters)} ${jsonEncode(body)}';
    final command = _commands.putIfAbsent(fingerprint, VendorCommand.create);
    try {
      final response = await app.inventoryApi.request(
        method,
        path,
        pathParameters: parameters,
        body: body == null ? null : VendorPayload(body),
        command: command,
      );
      _commands.remove(fingerprint);
      return response.object;
    } on ApiException catch (e) {
      final status = e.statusCode;
      if (status != null && status >= 400 && status < 500 && status != 408) {
        _commands.remove(fingerprint);
      }
      rethrow;
    }
  }

  /// Plain-English message for a failed stock request.
  static String message(Object e, {String? conflict}) {
    if (e is ApiException) {
      if (e.code == ApiErrorCode.network) return e.message;
      switch (e.statusCode) {
        case 401:
          return 'Your session has expired. Please sign in again.';
        case 403:
          return 'You don’t have permission to change stock at this branch. Ask your business owner for access.';
        case 404:
          return 'We couldn’t find this stock record. Start again.';
        case 409:
          return conflict ??
              'Stock changed while you were working. Refresh and try again.';
        case 400:
        case 422:
          return 'Check the items and quantities, then try again.';
      }
      if ((e.statusCode ?? 0) >= 500) {
        return 'The service is temporarily unavailable. Please try again.';
      }
    }
    return 'Something went wrong. Please try again.';
  }
}
