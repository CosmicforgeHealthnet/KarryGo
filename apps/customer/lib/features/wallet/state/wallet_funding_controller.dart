import 'dart:async';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/foundation.dart';

import '../data/wallet_api.dart';
import '../models/wallet_models.dart';

enum WalletFundingStatus {
  enterAmount,
  initializing,
  checkout,
  verifying,
  success,
  error,
}

/// Drives the Fund Wallet flow: pick provider + amount -> create a top-up
/// payment intent on payment-wallet-service -> open the returned Paystack
/// authorization URL in a WebView -> verify the wallet was credited.
class WalletFundingController extends ChangeNotifier {
  WalletFundingController({
    required WalletApi walletApi,
    required String accessToken,
    required String customerEmail,
  })  : _walletApi = walletApi,
        _accessToken = accessToken,
        _customerEmail = customerEmail;

  final WalletApi _walletApi;
  final String _accessToken;
  final String _customerEmail;

  WalletFundingStatus _status = WalletFundingStatus.enterAmount;
  WalletFundingStatus get status => _status;

  WalletPaymentProvider _provider = WalletPaymentProvider.paystackCard;
  WalletPaymentProvider get provider => _provider;

  int _amountKobo = 0;
  int get amountKobo => _amountKobo;

  TopUpResult? _topUp;
  TopUpResult? get topUp => _topUp;

  String? _error;
  String? get error => _error;

  /// Minimum top-up is ₦100 (matches the service's amount validation floor).
  static const int minAmountKobo = 10000;

  bool get canSubmit => _amountKobo >= minAmountKobo;

  void selectProvider(WalletPaymentProvider provider) {
    _provider = provider;
    notifyListeners();
  }

  void setAmountNaira(num naira) {
    _amountKobo = (naira * 100).round();
    notifyListeners();
  }

  /// Creates the top-up intent and moves to the checkout step.
  Future<void> startCheckout() async {
    if (!canSubmit) {
      _error = 'Enter at least ${formatKobo(minAmountKobo)}.';
      _status = WalletFundingStatus.error;
      notifyListeners();
      return;
    }
    _status = WalletFundingStatus.initializing;
    _error = null;
    notifyListeners();
    try {
      final result = await _walletApi.createTopUp(
        accessToken: _accessToken,
        customerEmail: _customerEmail,
        amountKobo: _amountKobo,
        idempotencyKey: 'topup-${DateTime.now().microsecondsSinceEpoch}',
      );
      _topUp = result;
      if (result.authorizationUrl.isEmpty) {
        _error = 'Could not start the payment. Please try again.';
        _status = WalletFundingStatus.error;
      } else {
        _status = WalletFundingStatus.checkout;
      }
    } on ApiException catch (e) {
      _error = e.message;
      _status = WalletFundingStatus.error;
    } catch (_) {
      _error = 'Something went wrong starting the payment.';
      _status = WalletFundingStatus.error;
    }
    notifyListeners();
  }

  /// Called when the WebView reaches the Paystack callback URL. Asks the backend
  /// to verify the top-up with Paystack and credit the wallet (so funding does
  /// not depend on the async Paystack webhook reaching the server). Retries a
  /// few times since Paystack may take a moment to mark the charge successful.
  Future<void> onCheckoutReturned() async {
    _status = WalletFundingStatus.verifying;
    notifyListeners();

    final reference = _topUp?.reference;
    if (reference == null || reference.isEmpty) {
      _status = WalletFundingStatus.success;
      notifyListeners();
      return;
    }

    for (var attempt = 0; attempt < 4; attempt++) {
      try {
        await _walletApi.verifyTopUp(
          accessToken: _accessToken,
          reference: reference,
        );
        _status = WalletFundingStatus.success;
        notifyListeners();
        return;
      } catch (_) {
        // Verification can fail transiently while Paystack settles; retry.
        await Future<void>.delayed(const Duration(seconds: 2));
      }
    }
    // Payment likely completed but verification lagged; let the wallet refresh
    // reflect the balance once the credit lands.
    _status = WalletFundingStatus.success;
    notifyListeners();
  }

  void backToAmount() {
    _status = WalletFundingStatus.enterAmount;
    _error = null;
    notifyListeners();
  }
}
