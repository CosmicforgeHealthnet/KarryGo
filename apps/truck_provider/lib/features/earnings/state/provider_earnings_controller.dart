import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/foundation.dart';

import '../../auth/models/provider_auth_models.dart';
import '../../home/data/provider_api.dart';
import '../../wallet/data/provider_wallet_api.dart';
import '../../wallet/models/wallet_models.dart';
import '../models/earnings_models.dart';

/// Drives the provider Earnings/Wallet screen: loads the trip-earnings
/// projection from the hauling service, the real ledger balance from
/// payment-wallet-service, and tracks the balance show/hide toggle.
class ProviderEarningsController extends ChangeNotifier {
  ProviderEarningsController({
    required ProviderApi api,
    required String? Function() accessToken,
    ProviderWalletApi? walletApi,
  })  : _api = api,
        _walletApi = walletApi,
        _accessToken = accessToken;

  final ProviderApi _api;
  final ProviderWalletApi? _walletApi;
  final String? Function() _accessToken;

  ProviderEarnings _earnings = ProviderEarnings.empty;
  ProviderEarnings get earnings => _earnings;

  /// Real withdrawable balance from payment-wallet-service. Null until it has
  /// been fetched successfully; the balance card falls back to the hauling
  /// trip-earnings estimate while null so the screen still renders offline.
  WalletBalance? _walletBalance;
  WalletBalance? get walletBalance => _walletBalance;

  /// True when the balance card is showing the wallet ledger figures rather
  /// than the booking-derived estimate.
  bool get hasWalletBalance => _walletBalance != null;

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
      // Trip stats/transactions (hauling) and the real ledger balance
      // (payment-wallet) load in parallel; the wallet fetch failing must not
      // take down the whole screen, so it is awaited separately below.
      final walletFuture = _loadWalletBalance(token);
      final earnings = await _api.getEarnings(accessToken: token);
      _earnings = earnings;
      _hasLoaded = true;
      _error = null;
      await walletFuture;
    } catch (e) {
      _error = e is ApiException ? e.message : e.toString();
    } finally {
      _isLoading = false;
      notifyListeners();
    }
  }

  /// Fetches the payment-wallet balance; keeps the last good value (or null)
  /// on failure so the card can fall back to the hauling estimate.
  Future<void> _loadWalletBalance(String token) async {
    final walletApi = _walletApi;
    if (walletApi == null) return;
    try {
      _walletBalance = await walletApi.getBalance(accessToken: token);
    } catch (_) {
      // Non-fatal: the balance card shows the booking-derived estimate.
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
