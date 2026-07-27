import 'dart:math';

import 'package:flutter/material.dart';
import '../../../shared/widgets/numeric_keypad.dart';
import '../state/wallet_controller.dart';
import 'payment_processing_screen.dart';

enum _AuthPhase { loading, enter, create, confirm }

/// Transaction authorization — the real withdrawal PIN gate.
///
/// payment-wallet-service requires a transaction PIN on every withdrawal:
/// - PIN already set → enter it → withdraw.
/// - No PIN yet → create + confirm → save via POST /provider/pin → withdraw.
/// Wrong-PIN / locked errors from the server surface inline.
class TransactionAuthScreen extends StatefulWidget {
  const TransactionAuthScreen({
    super.key,
    required this.walletController,
    required this.amount,
    required this.bankAccountId,
  });

  final WalletController walletController;
  final double amount;
  final String bankAccountId;

  @override
  State<TransactionAuthScreen> createState() => _TransactionAuthScreenState();
}

class _TransactionAuthScreenState extends State<TransactionAuthScreen> {
  static const int _pinLength = 4;

  _AuthPhase _phase = _AuthPhase.loading;
  String _pin = '';
  String _firstPin = '';
  bool _submitting = false;
  String? _error;

  // Stable idempotency key for this attempt — generated once on first submit.
  String? _idempotencyKey;

  @override
  void initState() {
    super.initState();
    _loadPinStatus();
  }

  Future<void> _loadPinStatus() async {
    final result = await widget.walletController.loadPinStatus();
    if (!mounted) return;
    result.when(
      success: (isSet) {
        setState(() => _phase = isSet ? _AuthPhase.enter : _AuthPhase.create);
      },
      failure: (err) {
        // Can't determine — try the enter phase; the server rejects with
        // pin_not_set if none exists, which routes to create below.
        setState(() {
          _phase = _AuthPhase.enter;
          _error = null;
        });
      },
    );
  }

  bool get _isValid => _pin.length == _pinLength;

  void _onKeyTap(String key) {
    if (_submitting) return;
    setState(() {
      if (key == 'backspace') {
        if (_pin.isNotEmpty) _pin = _pin.substring(0, _pin.length - 1);
      } else if (_pin.length < _pinLength) {
        _pin += key;
      }
    });
  }

  String _generateIdempotencyKey() {
    final rand = Random.secure();
    final bytes = List<int>.generate(16, (_) => rand.nextInt(256));
    bytes[6] = (bytes[6] & 0x0f) | 0x40;
    bytes[8] = (bytes[8] & 0x3f) | 0x80;
    final hex = bytes.map((b) => b.toRadixString(16).padLeft(2, '0')).join();
    return '${hex.substring(0, 8)}-${hex.substring(8, 12)}-'
        '${hex.substring(12, 16)}-${hex.substring(16, 20)}-'
        '${hex.substring(20, 32)}';
  }

  Future<void> _onContinue() async {
    if (!_isValid || _submitting) return;
    switch (_phase) {
      case _AuthPhase.create:
        setState(() {
          _firstPin = _pin;
          _pin = '';
          _error = null;
          _phase = _AuthPhase.confirm;
        });
      case _AuthPhase.confirm:
        if (_pin != _firstPin) {
          setState(() {
            _pin = '';
            _firstPin = '';
            _phase = _AuthPhase.create;
            _error = 'PINs did not match. Try again.';
          });
          return;
        }
        await _savePinAndWithdraw();
      case _AuthPhase.enter:
        await _withdraw(_pin);
      case _AuthPhase.loading:
        break;
    }
  }

  Future<void> _savePinAndWithdraw() async {
    setState(() {
      _submitting = true;
      _error = null;
    });
    final result = await widget.walletController.setupPin(_pin);
    if (!mounted) return;
    var saved = false;
    result.when(
      success: (_) => saved = true,
      failure: (err) {
        setState(() {
          _submitting = false;
          _pin = '';
          _firstPin = '';
          _phase = _AuthPhase.create;
          _error = err.message.isNotEmpty
              ? err.message
              : 'Could not save your PIN. Try again.';
        });
      },
    );
    if (saved) await _withdraw(_pin);
  }

  Future<void> _withdraw(String pin) async {
    // Generate a stable idempotency key per attempt.
    _idempotencyKey ??= _generateIdempotencyKey();
    final idempotencyKey = _idempotencyKey!;
    final amountKobo = (widget.amount * 100).round();

    setState(() {
      _submitting = true;
      _error = null;
    });

    final result = await widget.walletController.requestWithdrawal(
      bankAccountId: widget.bankAccountId,
      amountKobo: amountKobo,
      idempotencyKey: idempotencyKey,
      pin: pin,
    );

    if (!mounted) return;

    result.when(
      success: (_) {
        Navigator.of(context).pushReplacement(
          MaterialPageRoute(
            builder: (_) => PaymentProcessingScreen(amount: widget.amount),
          ),
        );
      },
      failure: (err) {
        setState(() {
          _submitting = false;
          _pin = '';
          // Server says no PIN exists — route to first-time creation.
          if (err.code == 'pin_not_set') {
            _phase = _AuthPhase.create;
            _error = 'Set a withdrawal PIN to continue.';
          } else {
            _error = err.message.isNotEmpty
                ? err.message
                : 'Withdrawal failed. Please try again.';
          }
          // Reset idempotency key on non-idempotent errors (anything except
          // network/timeout where retrying with the same key is correct).
          if (err.code != 'network') _idempotencyKey = null;
        });
      },
    );
  }

  String get _subtitle => switch (_phase) {
    _AuthPhase.loading => 'Checking your PIN status...',
    _AuthPhase.enter => 'Please input your secure pin to authorize withdrawal',
    _AuthPhase.create => 'Create a 4-digit withdrawal PIN to secure your payouts',
    _AuthPhase.confirm => 'Re-enter your new PIN to confirm',
  };

  String get _buttonLabel => switch (_phase) {
    _AuthPhase.enter => 'Withdraw Now',
    _AuthPhase.create => 'Continue',
    _AuthPhase.confirm => 'Save PIN & Withdraw',
    _AuthPhase.loading => 'Please wait...',
  };

  @override
  Widget build(BuildContext context) {
    final ready = _phase != _AuthPhase.loading;
    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(20, 12, 20, 20),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  GestureDetector(
                    onTap: () => Navigator.of(context).pop(),
                    behavior: HitTestBehavior.opaque,
                    child: const Padding(
                      padding: EdgeInsets.all(12),
                      child: Icon(
                        Icons.arrow_back_ios_new,
                        size: 18,
                        color: Color(0xFF1A1A1A),
                      ),
                    ),
                  ),
                  const SizedBox(width: 4),
                  Expanded(
                    child: Text(
                      _phase == _AuthPhase.create ||
                              _phase == _AuthPhase.confirm
                          ? 'Create Transaction PIN'
                          : 'Transaction Authorization',
                      style: const TextStyle(
                        fontSize: 17,
                        fontWeight: FontWeight.w700,
                        color: Color(0xFF1A1A1A),
                      ),
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 24),
              Text(
                _subtitle,
                style: const TextStyle(fontSize: 13, color: Color(0xFF888888)),
              ),
              const SizedBox(height: 24),
              if (ready)
                Row(
                  mainAxisAlignment: MainAxisAlignment.center,
                  children: List.generate(_pinLength, (i) {
                    final filled = i < _pin.length;
                    return Container(
                      width: 56,
                      height: 56,
                      margin: const EdgeInsets.symmetric(horizontal: 8),
                      alignment: Alignment.center,
                      decoration: BoxDecoration(
                        shape: BoxShape.circle,
                        border: Border.all(
                          color: const Color(0xFF4CAF50),
                          width: 1.5,
                        ),
                      ),
                      child: Text(
                        filled ? '•' : '',
                        style: const TextStyle(
                          fontSize: 26,
                          fontWeight: FontWeight.w700,
                          color: Color(0xFF1A1A1A),
                        ),
                      ),
                    );
                  }),
                )
              else
                const Center(
                  child: Padding(
                    padding: EdgeInsets.symmetric(vertical: 20),
                    child: CircularProgressIndicator(color: Color(0xFF4CAF50)),
                  ),
                ),
              if (_error != null) ...[
                const SizedBox(height: 12),
                Center(
                  child: Text(
                    _error!,
                    style: const TextStyle(
                      fontSize: 13,
                      color: Color(0xFFE53935),
                    ),
                    textAlign: TextAlign.center,
                  ),
                ),
              ],
              const Spacer(),
              if (ready) NumericKeypad(onKeyTap: _onKeyTap),
              const SizedBox(height: 16),
              GestureDetector(
                onTap: (ready && _isValid && !_submitting) ? _onContinue : null,
                child: Container(
                  width: double.infinity,
                  height: 52,
                  alignment: Alignment.center,
                  decoration: BoxDecoration(
                    color: (ready && _isValid && !_submitting)
                        ? const Color(0xFF4CAF50)
                        : const Color(0xFFA8D5B5),
                    borderRadius: BorderRadius.circular(999),
                  ),
                  child: _submitting
                      ? const SizedBox(
                          width: 22,
                          height: 22,
                          child: CircularProgressIndicator(
                            color: Colors.white,
                            strokeWidth: 2,
                          ),
                        )
                      : Text(
                          _buttonLabel,
                          style: const TextStyle(
                            fontSize: 15,
                            fontWeight: FontWeight.w700,
                            color: Colors.white,
                          ),
                        ),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
