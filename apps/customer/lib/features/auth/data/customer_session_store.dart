import 'dart:math';

import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import '../models/customer_auth_models.dart';

abstract interface class CustomerSessionStore {
  Future<CustomerSession?> readSession();
  Future<void> saveSession(CustomerSession session);
  Future<void> clearSession();
  Future<String> readOrCreateDeviceId();
}

class SecureCustomerSessionStore implements CustomerSessionStore {
  SecureCustomerSessionStore({FlutterSecureStorage? storage})
    : _storage = storage ?? const FlutterSecureStorage();

  static const _sessionKey = 'customer.auth.session';
  static const _deviceIdKey = 'customer.auth.device_id';

  final FlutterSecureStorage _storage;

  @override
  Future<CustomerSession?> readSession() async {
    final value = await _storage.read(key: _sessionKey);
    if (value == null || value.isEmpty) {
      return null;
    }

    return CustomerSession.fromJsonString(value);
  }

  @override
  Future<void> saveSession(CustomerSession session) {
    return _storage.write(key: _sessionKey, value: session.toJsonString());
  }

  @override
  Future<void> clearSession() {
    return _storage.delete(key: _sessionKey);
  }

  @override
  Future<String> readOrCreateDeviceId() async {
    final existing = await _storage.read(key: _deviceIdKey);
    if (existing != null && existing.isNotEmpty) {
      return existing;
    }

    final deviceId = _newDeviceId();
    await _storage.write(key: _deviceIdKey, value: deviceId);
    return deviceId;
  }
}

class MemoryCustomerSessionStore implements CustomerSessionStore {
  CustomerSession? _session;
  String? _deviceId;

  @override
  Future<CustomerSession?> readSession() async => _session;

  @override
  Future<void> saveSession(CustomerSession session) async {
    _session = session;
  }

  @override
  Future<void> clearSession() async {
    _session = null;
  }

  @override
  Future<String> readOrCreateDeviceId() async {
    _deviceId ??= _newDeviceId();
    return _deviceId!;
  }
}

String _newDeviceId() {
  final random = Random.secure().nextInt(1 << 32).toRadixString(16);
  return 'customer-device-${DateTime.now().microsecondsSinceEpoch}-$random';
}
