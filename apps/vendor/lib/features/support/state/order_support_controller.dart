import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../app/vendor_runtime.dart';
import '../../../app/vendor_app.dart';
import '../../../app/data/vendor_preferences.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_command.dart';
import '../models/support_models.dart';

String sanitizeSupportDescription(String value) =>
    value.trim().replaceAll(RegExp(r'\b[0-9]{4,8}\b'), '[REDACTED]');

class OrderSupportController extends ChangeNotifier {
  OrderSupportController(this.runtime, this.orderId)
    : app = runtime.backend,
      storage = runtime.mode == VendorDataMode.mock
          ? runtime.mockSupportPreferences
          : runtime.preferences,
      storageKey =
          'vendor.support.${base64Url.encode(utf8.encode('${runtime.mode.name}|${runtime.mode == VendorDataMode.mock ? runtime.revision : runtime.backend.client.config.origin}|${runtime.backend.scope.vendorId}|$orderId'))}';
  final VendorApp app;
  final VendorPreferences storage;
  final String storageKey;
  final VendorRuntime runtime;
  final String orderId;
  List<JsonObject> cases = [];
  JsonObject? pending;
  bool loading = false, busy = false, disposed = false, initialized = false;
  bool readDenied = false, writeDenied = false, journalBlocked = false;
  String? error, commandError;
  List<String> get categories =>
      (app.supportApi.operations
                  .firstWhere((op) => op.method == 'POST')
                  .requestSchema!['properties']
              as Map)['category']['enum']
          .cast<String>();
  bool get canCreate =>
      initialized &&
      !loading &&
      !busy &&
      !journalBlocked &&
      !writeDenied &&
      pending == null;
  void emit() {
    if (!disposed) notifyListeners();
  }

  Future<void> initialize() async {
    loading = true;
    emit();
    try {
      final saved = await storage.read(storageKey);
      if (saved != null && saved.isNotEmpty) {
        final value = jsonObject(jsonDecode(saved));
        if (value.isNotEmpty) {
          if (value['key'] is! String ||
              (value['key'] as String).isEmpty ||
              (value['key'] as String).length > 160 ||
              value['body'] is! Map ||
              value['order_id'] != orderId) {
            throw const FormatException('Invalid recovery record');
          }
          validateBody(jsonObject(value['body']));
          pending = value;
        }
      }
    } catch (_) {
      journalBlocked = true;
      commandError =
          'Could not restore the previous report. Reopen this screen before creating another.';
    }
    initialized = true;
    loading = false;
    await load();
  }

  Future<void> clearPending() async {
    await storage.write(storageKey, '{}');
    pending = null;
    journalBlocked = false;
  }

  void validateBody(JsonObject body) {
    final description = body['description'];
    final refs = body['line_refs'];
    if (!categories.contains(body['category']) ||
        description is! String ||
        description.trim().isEmpty ||
        utf8.encode(description).length > 1000 ||
        body.keys.any(
          (key) => !{'category', 'description', 'line_refs'}.contains(key),
        ) ||
        refs != null &&
            (refs is! List ||
                refs.length > 20 ||
                refs.any(
                  (ref) => ref is! String || ref.isEmpty || ref.length > 300,
                ))) {
      throw const FormatException(
        'Choose an issue and enter a description within 1,000 bytes, with at most 20 affected items.',
      );
    }
  }

  Future<void> load() async {
    if (loading || busy || disposed) return;
    loading = true;
    error = null;
    readDenied = false;
    emit();
    try {
      final response = await app.supportApi
          .getCommerceOrdersVendorOrderIdSupportCases(vendorOrderId: orderId);
      final values = response.object['cases'];
      if (values is! List) {
        throw const FormatException('Incomplete support response');
      }
      if (disposed) return;
      cases = values.map(jsonObject).toList();
      if (pending != null &&
          cases.any((c) => c['operation_ref'] == pending!['key'])) {
        try {
          await clearPending();
          commandError = null;
        } catch (_) {
          journalBlocked = true;
          commandError =
              'Your report is recorded. Refresh to finish saving its recovery state.';
        }
      }
    } catch (e) {
      error = message(e);
      readDenied = e is ApiException && e.statusCode == 403;
    } finally {
      loading = false;
      emit();
    }
  }

  Future<JsonObject?> create(JsonObject? body) async {
    if (busy ||
        loading ||
        disposed ||
        writeDenied ||
        journalBlocked ||
        !initialized) {
      return null;
    }
    if (pending == null && body == null) return null;
    busy = true;
    commandError = null;
    emit();
    JsonObject? result;
    try {
      if (pending == null) {
        final description = sanitizeSupportDescription(
          body!['description'] as String,
        );
        if (!categories.contains(body['category']) ||
            description.isEmpty ||
            utf8.encode(description).length > 1000) {
          throw const FormatException(
            'Choose an issue and enter a description within 1,000 bytes.',
          );
        }
        final request = {
          'category': body['category'],
          'description': description,
          if (body['line_refs'] != null) 'line_refs': body['line_refs'],
        };
        final attempt = {
          'order_id': orderId,
          'key': VendorCommand.create().idempotencyKey!,
          'body': request,
        };
        validateBody(request);
        await storage.write(storageKey, jsonEncode(attempt));
        pending = attempt;
      }
      final response = await app.supportApi
          .postCommerceOrdersVendorOrderIdSupportCases(
            vendorOrderId: orderId,
            command: VendorCommand(idempotencyKey: pending!['key'] as String),
            body: PostCommerceOrdersVendorOrderIdSupportCasesRequest.fromJson(
              jsonObject(pending!['body']),
            ),
          );
      final record = response.object;
      if (record['link_ref'] is! String || record['status'] is! String) {
        throw const FormatException('Incomplete report response');
      }
      result = record;
      cases = [
        record,
        ...cases.where((c) => c['link_ref'] != record['link_ref']),
      ];
      try {
        await clearPending();
      } catch (_) {
        journalBlocked = true;
        commandError =
            'Your report is recorded. Refresh to finish saving its recovery state.';
      }
    } catch (e) {
      commandError = e is FormatException ? e.message : message(e);
      if (pending == null && e is! FormatException) {
        commandError =
            'Could not save your report for a safe retry. Nothing was submitted.';
      } else if (e is ApiException &&
          [400, 401, 403, 404, 422, 429].contains(e.statusCode)) {
        writeDenied = e.statusCode == 403;
        try {
          await clearPending();
        } catch (_) {
          journalBlocked = true;
        }
      }
    } finally {
      busy = false;
      emit();
    }
    return result;
  }

  static String message(Object e) => e is ApiException
      ? e.message
      : 'Could not connect. Refresh or retry the original report.';
  @override
  void dispose() {
    disposed = true;
    super.dispose();
  }
}
