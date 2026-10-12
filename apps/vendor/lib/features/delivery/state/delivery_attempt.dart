import 'dart:convert';
import 'package:crypto/crypto.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../app/vendor_runtime.dart';
import '../../../app/vendor_app.dart';
import '../../../app/data/vendor_preferences.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_command.dart';

class DeliveryAttempt {
  DeliveryAttempt(VendorRuntime runtime, String flow)
    : app = runtime.backend,
      storage = runtime.mode == VendorDataMode.mock
          ? runtime.mockSupportPreferences
          : runtime.preferences,
      key =
          'vendor.delivery.${base64Url.encode(utf8.encode('${runtime.mode.name}|${runtime.mode == VendorDataMode.mock ? runtime.revision : runtime.backend.client.config.origin}|${runtime.backend.scope.vendorId}|${runtime.backend.auth.session?.staffId}|$flow'))}';
  final VendorApp app;
  final VendorPreferences storage;
  final String key;
  JsonObject? pending;
  bool blocked = false;
  String? error;
  String digest(String credential) =>
      sha256.convert(utf8.encode(credential)).toString();
  Future<void> restore() async {
    try {
      final saved = await storage.read(key);
      if (saved == null || saved == '{}') return;
      final value = jsonObject(jsonDecode(saved));
      if (value['key'] is! String ||
          value['body'] is! Map ||
          value['params'] is! Map ||
          value['version'] is! int ||
          value['method'] is! String ||
          value['path'] is! String ||
          value['body']['credential'] != null) {
        throw const FormatException('Invalid recovery state');
      }
      final op = app.deliveryApi.operation(value['method'], value['path']);
      if (!['PUT', 'POST'].contains(op.method) ||
          !op.parameters.any((p) => p['name'] == 'Idempotency-Key')) {
        throw const FormatException('Invalid recovery operation');
      }
      pending = value;
    } catch (_) {
      blocked = true;
      error =
          'Could not restore the previous delivery update. Reopen this screen before continuing.';
    }
  }

  Future<void> clear() async {
    await storage.write(key, '{}');
    pending = null;
  }

  Future<JsonObject?> send({
    String? method,
    String? path,
    Map<String, String>? params,
    JsonObject? body,
    int? version,
    String? credential,
    void Function(JsonObject)? validateResponse,
    Future<void> Function(JsonObject, String)? beforeClear,
  }) async {
    if (blocked) return null;
    error = null;
    try {
      if (pending == null) {
        if (method == null ||
            path == null ||
            params == null ||
            body == null ||
            version == null) {
          return null;
        }
        final value = <String, dynamic>{
          'key': VendorCommand.create().idempotencyKey,
          'method': method,
          'path': path,
          'params': params,
          'body': {...body}..remove('credential'),
          'version': version,
          if (credential != null) 'credential_digest': digest(credential),
        };
        await storage.write(key, jsonEncode(value));
        pending = value;
      }
      final attempt = pending!;
      if (attempt['credential_digest'] != null &&
          (credential == null ||
              digest(credential) != attempt['credential_digest'])) {
        error = 'Re-enter the same credential to retry this verification.';
        return null;
      }
      final response = await app.deliveryApi.request(
        attempt['method'] as String,
        attempt['path'] as String,
        pathParameters: Map<String, String>.from(attempt['params'] as Map),
        body: VendorPayload({
          ...jsonObject(attempt['body']),
          if (attempt['credential_digest'] != null) 'credential': credential,
        }),
        command: VendorCommand(
          idempotencyKey: attempt['key'] as String,
          ifMatch:
              app.deliveryApi
                  .operation(attempt['method'], attempt['path'])
                  .parametersIn('header')
                  .any((p) => p['name'].toString().toLowerCase() == 'if-match')
              ? attempt['version'].toString()
              : null,
        ),
      );
      final value = response.object;
      validateResponse?.call(value);
      await beforeClear?.call(value, attempt['key'] as String);
      try {
        await clear();
      } catch (_) {
        blocked = true;
        error = 'Saved on the server. Reopen this screen to finish recovery.';
      }
      return value;
    } catch (e) {
      error = e is ApiException
          ? e.message
          : pending == null
          ? 'Could not save recovery details. Nothing was submitted.'
          : 'The result could not be confirmed. Retry the same update.';
      final secure = pending?['credential_digest'] != null;
      if (e is ApiException &&
          [
            400,
            401,
            403,
            404,
            412,
            422,
            429,
            if (!secure) 409,
          ].contains(e.statusCode)) {
        try {
          await clear();
        } catch (_) {
          blocked = true;
        }
      }
      return null;
    }
  }
}
