import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_command.dart';
import 'scheduling_workspace_controller.dart';

class ScheduledCreationController extends ChangeNotifier {
  ScheduledCreationController(this.runtime);
  final VendorRuntime runtime;
  late final app = runtime.backend;
  late final storage = runtime.mode == VendorDataMode.mock
      ? runtime.mockSupportPreferences
      : runtime.preferences;
  late final key =
      'vendor.scheduled-creation.${base64Url.encode(utf8.encode('${runtime.mode.name}|${runtime.mode == VendorDataMode.mock ? runtime.revision : app.client.config.origin}|${app.scope.vendorId}|${app.auth.session?.staffId}'))}';
  JsonObject? pending;
  bool busy = false, blocked = false, disposed = false;
  String? error;
  void emit() {
    if (!disposed) notifyListeners();
  }

  Future<void> restore() async {
    try {
      final raw = await storage.read(key);
      if (raw == null || raw == '{}') return;
      final value = jsonObject(jsonDecode(raw));
      if (value['vendor'] != app.scope.vendorId ||
          value['body'] is! Map ||
          value['body']['IdempotencyKey'] is! String ||
          value['body']['BranchID'] is! String ||
          value['body']['QuoteID'] is! String ||
          value['body']['CustomerID'] is! String) {
        throw const FormatException('Invalid saved schedule');
      }
      pending = value;
    } catch (_) {
      blocked = true;
      error =
          'The saved schedule could not be read. Resolve recovery before creating another.';
    }
    emit();
  }

  Future<JsonObject?> create({JsonObject? source, JsonObject? changes}) async {
    if (busy || blocked || pending != null && source != null) return null;
    busy = true;
    error = null;
    emit();
    try {
      if (pending == null) {
        if (source == null ||
            changes == null ||
            source['BusinessID'] != app.scope.vendorId ||
            app.scope.branchId != null &&
                source['BranchID'] != app.scope.branchId ||
            source['QuoteID'] is! String ||
            source['CustomerID'] is! String ||
            DateTime.tryParse(
                  '${source['ExpiresAt']}',
                )?.isAfter(DateTime.now()) !=
                true) {
          throw const VendorContractException(
            'Choose a current approved quote within your branch scope.',
          );
        }
        const fields = ['QuoteID', 'CustomerID', 'BranchID'];
        const editable = [
          'DesiredDeliveryAt',
          'DeliveryWindowStart',
          'DeliveryWindowEnd',
          'ProductionReadyBy',
          'ExpectedReadyAt',
          'TentativeVehicleClass',
          'Capacity',
        ];
        if (changes.keys.any((k) => !editable.contains(k))) {
          throw const VendorContractException('Unsupported schedule change.');
        }
        final body = <String, dynamic>{
          for (final name in fields)
            if (source.containsKey(name)) name: source[name],
          ...changes,
          'IdempotencyKey': VendorCommand.create().idempotencyKey,
        };
        final schema = app.scheduledCommerceApi
            .operation('POST', '$schedulingBase/scheduled-orders')
            .requestSchema;
        if (schema != null) validateRequest(body, schema);
        final saved = jsonObject(
          jsonDecode(jsonEncode({'vendor': app.scope.vendorId, 'body': body})),
        );
        await storage.write(key, jsonEncode(saved));
        pending = saved;
      }
      final result = (await app.scheduledCommerceApi.request(
        'POST',
        '$schedulingBase/scheduled-orders',
        body: VendorPayload(jsonObject(pending!['body'])),
      )).object;
      if (result['ID'] is! String ||
          result['BusinessID'] != app.scope.vendorId ||
          result['BranchID'] != pending!['body']['BranchID'] ||
          result['QuoteID'] != pending!['body']['QuoteID'] ||
          result['CustomerID'] != pending!['body']['CustomerID'] ||
          !['RESERVED', 'NOT_REQUIRED'].contains(result['CapacityStatus'])) {
        throw const FormatException('Missing schedule confirmation');
      }
      await storage.write(key, '{}');
      pending = null;
      return result;
    } catch (e) {
      error = e is ApiException
          ? e.message
          : pending != null
          ? 'The outcome is unconfirmed. Retry the saved schedule to avoid duplicates.'
          : 'Nothing was submitted. Check the schedule details and try again.';
      if (e is ApiException &&
          [400, 401, 403, 404, 409, 412, 422].contains(e.statusCode)) {
        try {
          await storage.write(key, '{}');
          pending = null;
        } catch (_) {
          blocked = true;
        }
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
