import 'dart:convert';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import '../../../core/contracts/vendor_contract.dart';

abstract interface class WithdrawalAttemptStore {
  Future<JsonObject?> read();
  Future<void> save(JsonObject value);
  Future<void> clear();
}

class MemoryWithdrawalAttemptStore implements WithdrawalAttemptStore {
  JsonObject? value;
  @override
  Future<JsonObject?> read() async =>
      value == null ? null : jsonObject(jsonDecode(jsonEncode(value)));
  @override
  Future<void> save(JsonObject value) async {
    this.value = jsonObject(jsonDecode(jsonEncode(value)));
  }

  @override
  Future<void> clear() async {
    value = null;
  }
}

class SecureWithdrawalAttemptStore implements WithdrawalAttemptStore {
  SecureWithdrawalAttemptStore(
    String origin,
    String vendorId, {
    FlutterSecureStorage? storage,
  }) : _storage = storage ?? const FlutterSecureStorage(),
       _key =
           'vendor.withdrawal.${base64Url.encode(utf8.encode('$origin|$vendorId'))}';
  final FlutterSecureStorage _storage;
  final String _key;
  @override
  Future<JsonObject?> read() async {
    final value = await _storage.read(key: _key);
    return value == null ? null : jsonObject(jsonDecode(value));
  }

  @override
  Future<void> save(JsonObject value) =>
      _storage.write(key: _key, value: jsonEncode(value));
  @override
  Future<void> clear() => _storage.delete(key: _key);
}
