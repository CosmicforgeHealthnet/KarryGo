import 'package:flutter/foundation.dart';

/// Local selection only. The server resolves business membership and scopes.
class VendorScope extends ChangeNotifier {
  String? _vendorId, _branchId;
  int _revision = 0;
  String? get vendorId => _vendorId;
  String? get branchId => _branchId;
  int get revision => _revision;
  void select({required String vendorId, String? branchId}) {
    if (_vendorId == vendorId && _branchId == branchId) return;
    _vendorId = vendorId;
    _branchId = branchId;
    _revision++;
    notifyListeners();
  }

  void clear() {
    _vendorId = null;
    _branchId = null;
    _revision++;
    notifyListeners();
  }
}
