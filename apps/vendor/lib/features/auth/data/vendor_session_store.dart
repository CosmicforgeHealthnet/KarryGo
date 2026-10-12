import 'dart:convert';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../models/vendor_session.dart';

abstract interface class VendorSessionStore {
  Future<VendorSession?> read();
  Future<void> save(VendorSession session);
  Future<void> clear();
}

class SecureVendorSessionStore implements VendorSessionStore {
  SecureVendorSessionStore({FlutterSecureStorage? storage, this.namespace})
    : _storage = storage ?? const FlutterSecureStorage();
  final FlutterSecureStorage _storage;
  final String? namespace;
  String get _key => namespace == null
      ? 'vendor.auth.session'
      : 'vendor.auth.session.${base64Url.encode(utf8.encode(namespace!))}';
  @override
  Future<VendorSession?> read() async {
    final value = await _storage.read(key: _key);
    if (value == null || value.isEmpty) return null;
    try {
      return VendorSession.fromJson(jsonObject(jsonDecode(value)));
    } on FormatException {
      await clear();
      return null;
    } on TypeError {
      await clear();
      return null;
    }
  }

  @override
  Future<void> save(VendorSession session) =>
      _storage.write(key: _key, value: session.encode());
  @override
  Future<void> clear() => _storage.delete(key: _key);
}

class MemoryVendorSessionStore implements VendorSessionStore {
  VendorSession? _session;
  @override
  Future<VendorSession?> read() async => _session;
  @override
  Future<void> save(VendorSession session) async {
    _session = session;
  }

  @override
  Future<void> clear() async {
    _session = null;
  }
}
