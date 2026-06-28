import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/foundation.dart';

import '../../auth/models/provider_auth_models.dart';
import '../../home/data/provider_api.dart';
import '../models/earnings_models.dart';

/// Drives the provider Earnings/Wallet screen: loads the trip-earnings
/// projection from the hauling service and tracks the balance show/hide toggle.
class ProviderEarningsController extends ChangeNotifier {
  ProviderEarningsController({
    required ProviderApi api,
    required String? Function() accessToken,
  })  : _api = api,
        _accessToken = accessToken;

  final ProviderApi _api;
  final String? Function() _accessToken;

  ProviderEarnings _earnings = ProviderEarnings.empty;
  ProviderEarnings get earnings => _earnings;

  bool _isLoading = false;
  bool get isLoading => _isLoading;

  /// Distinguishes "still loading the first time" (show skeleton/spinner) from a
  /// background refresh (keep showing the last data).
  bool _hasLoaded = false;
  bool get hasLoaded => _hasLoaded;

  String? _error;
  String? get error => _error;

  bool _balanceHidden = false;
  bool get balanceHidden => _balanceHidden;

  String? get _token => _accessToken();

  void toggleBalanceVisibility() {
    _balanceHidden = !_balanceHidden;
    notifyListeners();
  }

  /// Loads earnings. Safe to call repeatedly (e.g. when the tab is shown or on
  /// pull-to-refresh).
  Future<void> load() async {
    final token = _token;
    if (token == null) return;
    if (_isLoading) return;

    _isLoading = true;
    _error = null;
    notifyListeners();

    try {
      final earnings = await _api.getEarnings(accessToken: token);
      _earnings = earnings;
      _hasLoaded = true;
      _error = null;
    } catch (e) {
      _error = e is ApiException ? e.message : e.toString();
    } finally {
      _isLoading = false;
      notifyListeners();
    }
  }

  /// Fetches the booking behind a trip transaction for the Transaction Detail
  /// screen. Returns null on failure (the screen degrades gracefully).
  Future<ProviderBooking?> fetchTripDetail(String bookingId) async {
    final token = _token;
    if (token == null || bookingId.isEmpty) return null;
    try {
      return await _api.getBooking(accessToken: token, bookingId: bookingId);
    } catch (_) {
      return null;
    }
  }
}
