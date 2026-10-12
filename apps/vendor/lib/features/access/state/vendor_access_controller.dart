import 'package:flutter/foundation.dart';

/// Catalogue and denied-action hints; current grants are fetched by My access.
class VendorAccessController extends ChangeNotifier {
  List<Map<String, dynamic>> catalogue = const [];
  final Set<String> deniedActions = {};
  void acceptCatalogue(List<Map<String, dynamic>> value) {
    catalogue = List.unmodifiable(value);
    notifyListeners();
  }

  void recordDenial(String operation) {
    deniedActions.add(operation);
    notifyListeners();
  }

  void clear() {
    catalogue = const [];
    deniedActions.clear();
    notifyListeners();
  }
}
