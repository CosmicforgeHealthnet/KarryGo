import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../app/vendor_app.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';

class OrderDetailController extends ChangeNotifier {
  OrderDetailController(this.app, this.id);
  final VendorApp app;
  final String id;
  JsonObject? order;
  List<JsonObject> picking = [];
  bool loading = false, busy = false, changed = false, stale = false;
  bool restricted = false, pickingRestricted = false, disposed = false;
  String? error, pickingError, feedback;
  final Set<String> denied = {};
  VendorCommand? _command;
  String? _fingerprint;

  void _notify() {
    if (!disposed) notifyListeners();
  }

  Future<void> load() async {
    if (loading || busy) return;
    loading = true;
    stale = true;
    error = null;
    restricted = false;
    _notify();
    try {
      final response = await app.ordersApi.getCommerceOrdersVendorOrderId(
        vendorOrderId: id,
      );
      if (disposed) return;
      final value = response.object;
      if (value['vendor_order_id'] != id ||
          value['business_id'] != app.scope.vendorId ||
          value['branch_id'] is! String ||
          (value['branch_id'] as String).isEmpty ||
          value['status'] is! String ||
          (value['status'] as String).isEmpty ||
          value['version'] is! int ||
          value['version'] < 1) {
        throw const FormatException('Invalid order details');
      }
      order = value;
      stale = false;
      await _loadPicking();
    } catch (e) {
      if (disposed) return;
      error = _message(e);
      restricted = e is ApiException && e.statusCode == 403;
      if (e is FormatException ||
          e is ApiException && [403, 404].contains(e.statusCode)) {
        order = null;
        picking = [];
      }
    } finally {
      loading = false;
      _notify();
    }
  }

  Future<void> _loadPicking() async {
    picking = [];
    pickingError = null;
    pickingRestricted = false;
    if (order?['domain'] != 'SHOP') return;
    try {
      final response = await app.ordersApi
          .getCommerceOrdersVendorOrderIdPickingQueue(vendorOrderId: id);
      if (disposed) return;
      if (response.data is! List) {
        throw const FormatException('Incomplete picking response');
      }
      picking = (response.data as List).map(jsonObject).toList();
    } catch (e) {
      pickingError = _message(e);
      pickingRestricted = e is ApiException && e.statusCode == 403;
    }
  }

  bool can(String path) =>
      !disposed &&
      order != null &&
      !restricted &&
      !loading &&
      !busy &&
      !stale &&
      !denied.contains(path);
  Future<bool> submit(
    String suffix,
    JsonObject body, {
    required int version,
  }) async {
    final path = '/api/v1/vendor/commerce/orders/{vendorOrderId}/$suffix';
    return _submit(app.ordersApi, path, {'vendorOrderId': id}, body, version);
  }

  Future<bool> line(
    String lineId,
    String action,
    JsonObject body,
    int version,
  ) => _submit(
    app.ordersApi,
    '/api/v1/vendor/commerce/orders/{vendorOrderId}/lines/{lineId}/$action',
    {'vendorOrderId': id, 'lineId': lineId},
    body,
    version,
  );
  Future<bool> _submit(
    VendorFeatureApi api,
    String path,
    Map<String, String> parameters,
    JsonObject body,
    int? version,
  ) async {
    if (!can(path)) return false;
    final fingerprint = '$path|$parameters|$body|$version';
    if (_fingerprint != fingerprint) {
      _command = VendorCommand.create(version: version);
      _fingerprint = fingerprint;
    }
    busy = true;
    error = null;
    feedback = null;
    _notify();
    var success = false;
    try {
      await api.request(
        'POST',
        path,
        pathParameters: parameters,
        body: VendorPayload(body),
        command: _command,
      );
      changed = true;
      success = true;
      stale = true;
      _command = null;
      _fingerprint = null;
      feedback = 'Saved. Order information has been refreshed.';
    } catch (e) {
      if (e is ApiException && e.statusCode == 403) {
        denied.add(path);
        restricted = true;
      }
      if (e is! ApiException ||
          e.statusCode == null ||
          e.statusCode == 409 ||
          (e.statusCode ?? 0) >= 500) {
        stale = true;
      }
      error = _message(e, writing: true);
    } finally {
      busy = false;
    }
    if (success) {
      final savedFeedback = feedback;
      await load();
      feedback = error == null
          ? savedFeedback
          : 'Saved. Refresh to see the latest order information.';
    }
    _notify();
    return success;
  }

  static String _message(Object e, {bool writing = false}) {
    if (e is ApiException) {
      return e.hasFieldErrors
          ? e.fields.map((field) => field.message).toSet().join('\n')
          : e.message;
    }
    if (e is FormatException) {
      return 'The server returned incomplete order information. Refresh before continuing.';
    }
    return writing
        ? 'The order could not be updated. Please try again.'
        : 'The order could not be loaded. Please try again.';
  }

  @override
  void dispose() {
    disposed = true;
    super.dispose();
  }
}
