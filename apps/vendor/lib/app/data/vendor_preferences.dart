import 'package:flutter_secure_storage/flutter_secure_storage.dart';

class VendorPreferences {
  VendorPreferences({this.inMemory = false, FlutterSecureStorage? storage})
    : _storage = storage ?? const FlutterSecureStorage();
  final bool inMemory;
  final FlutterSecureStorage _storage;
  final _memory = <String, String>{};
  Future<String?> read(String key) async =>
      inMemory ? _memory[key] : _storage.read(key: key);
  Future<void> write(String key, String value) async {
    if (inMemory) {
      _memory[key] = value;
    } else {
      await _storage.write(key: key, value: value);
    }
  }
}
