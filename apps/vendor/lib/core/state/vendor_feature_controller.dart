import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../network/vendor_response.dart';
import 'vendor_scope.dart';

enum VendorLoadPhase { idle, loading, ready, failed }

/// Retains exact backend data; never invents transitions or pagination fields.
class VendorFeatureController extends ChangeNotifier {
  VendorFeatureController(this.scope) {
    scope.addListener(reset);
  }
  final VendorScope scope;
  VendorLoadPhase phase = VendorLoadPhase.idle;
  VendorResponse? response;
  Object? error;
  bool commandPending = false;
  int _generation = 0;
  Future<VendorResponse?> load(Future<VendorResponse> Function() fetch) async {
    final generation = ++_generation;
    phase = VendorLoadPhase.loading;
    error = null;
    notifyListeners();
    try {
      final value = await fetch();
      if (generation != _generation) return null;
      response = value;
      phase = VendorLoadPhase.ready;
      notifyListeners();
      return value;
    } catch (e) {
      if (generation == _generation) {
        error = e;
        phase = VendorLoadPhase.failed;
        notifyListeners();
      }
      return null;
    }
  }

  Future<VendorResponse> command(Future<VendorResponse> Function() send) async {
    if (commandPending) throw StateError('A command is already pending');
    final generation = _generation;
    commandPending = true;
    error = null;
    notifyListeners();
    try {
      return await send();
    } catch (e) {
      if (generation == _generation) error = e;
      rethrow;
    } finally {
      if (generation == _generation) {
        commandPending = false;
        notifyListeners();
      }
    }
  }

  bool get conflict =>
      error is ApiException && (error as ApiException).statusCode == 409;
  bool get forbidden =>
      error is ApiException && (error as ApiException).statusCode == 403;
  void reset() {
    _generation++;
    response = null;
    error = null;
    commandPending = false;
    phase = VendorLoadPhase.idle;
    notifyListeners();
  }

  @override
  void dispose() {
    _generation++;
    scope.removeListener(reset);
    super.dispose();
  }
}
