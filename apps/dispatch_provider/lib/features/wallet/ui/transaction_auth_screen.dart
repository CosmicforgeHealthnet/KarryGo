import '../../../shared/widgets/journey_icon.dart';
import '../../../shared/widgets/dispatch_screen_frame.dart';
import 'dart:math';

import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
import '../../../shared/widgets/numeric_keypad.dart';
import '../state/wallet_controller.dart';
import 'withdrawal_result_screen.dart';
import '../../../shared/widgets/async_content.dart';
import 'change_transaction_pin_screen.dart';

enum _AuthPhase { loading, enter, create, confirm }

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
  static const int _pinLength = 6;

  _AuthPhase _phase = _AuthPhase.loading;
  String _pin = '';
  String _firstPin = '';
  bool _submitting = false;
  String? _error;

  String? _idempotencyKey;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) _loadPinStatus();
    });
  }

  Future<void> _loadPinStatus() async {
    final result = await widget.walletController.loadPinStatus();
    if (!mounted) return;
    result.when(
      success: (isSet) {
        setState(() => _phase = isSet ? _AuthPhase.enter : _AuthPhase.create);
      },
      failure: (err) {
        setState(() {
          _phase = _AuthPhase.loading;
          _error = err.message;
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
      success: (withdrawal) {
        Navigator.of(context).pushReplacement(
          MaterialPageRoute(
            builder: (_) => WithdrawalResultScreen(withdrawal: withdrawal),
          ),
        );
      },
      failure: (err) {
        setState(() {
          _submitting = false;
          _pin = '';
          if (err.code == 'pin_not_set') {
            _phase = _AuthPhase.create;
            _error = 'Set a withdrawal PIN to continue.';
          } else {
            _error = err.message.isNotEmpty
                ? err.message
                : 'Withdrawal failed. Please try again.';
          }
          if (err.statusCode != null &&
              err.statusCode! >= 400 &&
              err.statusCode! < 500 &&
              err.statusCode != 408 &&
              err.statusCode != 429) {
            _idempotencyKey = null;
          }
        });
      },
    );
  }

  void _onForgotPin() {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => ChangeTransactionPinScreen(
          walletController: widget.walletController,
        ),
      ),
    );
  }

  // ── Copy ──────────────────────────────────────────────────────────

  String get _topBarTitle => switch (_phase) {
    _AuthPhase.create => 'Create transaction PIN',
    _AuthPhase.confirm => 'Confirm transaction PIN',
    _ => 'Enter transaction PIN',
  };

  String get _heading => switch (_phase) {
    _AuthPhase.create => 'Create your transaction PIN',
    _AuthPhase.confirm => 'Confirm your transaction PIN',
    _ => 'Enter your transaction PIN',
  };

  String get _subtitle => switch (_phase) {
    _AuthPhase.loading => '',
    _AuthPhase.enter =>
      'This PIN is for financial transactions only (withdrawals, bank accounts). '
          'Keep it private and do not share it with anyone.',
    _AuthPhase.create =>
      'Choose a 6-digit PIN. You will use it for every withdrawal.',
    _AuthPhase.confirm => 'Re-enter your new PIN to confirm.',
  };

  String get _buttonLabel => switch (_phase) {
    _AuthPhase.confirm => 'Save PIN & Withdraw',
    _AuthPhase.loading => 'Please wait...',
    _ => 'Continue',
  };

  bool get _showForgotLink => _phase == _AuthPhase.enter;

  // ── Build ─────────────────────────────────────────────────────────

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    final ready = _phase != _AuthPhase.loading;

    return PopScope(
      canPop: !_submitting,
      child: Scaffold(
        backgroundColor: colors.background,
        body: SafeArea(
          child: Column(
            children: [
              _buildTopBar(context),
              Expanded(
                child: SingleChildScrollView(
                  padding: const EdgeInsets.fromLTRB(24, 16, 24, 8),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      if (!ready && _error != null)
                        ActionNotice(
                          message: _error!,
                          isError: true,
                          onRetry: _loadPinStatus,
                        )
                      else if (!ready)
                        Padding(
                          padding: const EdgeInsets.symmetric(vertical: 60),
                          child: Center(
                            child: CircularProgressIndicator(
                              color: colors.brandTeal500,
                            ),
                          ),
                        )
                      else ...[
                        _buildLockIcon(),
                        const SizedBox(height: 22),
                        Text(
                          _heading,
                          textAlign: TextAlign.center,
                          style: TextStyle(
                            fontSize: 20,
                            fontWeight: FontWeight.w700,
                            color: colors.text,
                            letterSpacing: -0.4,
                            height: 1.2,
                          ),
                        ),
                        const SizedBox(height: 12),
                        Text(
                          _subtitle,
                          textAlign: TextAlign.center,
                          style: TextStyle(
                            fontSize: 14,
                            fontWeight: FontWeight.w400,
                            color: colors.textMuted,
                            height: 1.5,
                          ),
                        ),
                        const SizedBox(height: 28),
                        _buildPinSlots(),
                        if (_error != null) ...[
                          const SizedBox(height: 18),
                          _buildError(),
                        ],
                        if (_showForgotLink) ...[
                          const SizedBox(height: 18),
                          Center(
                            child: GestureDetector(
                              onTap: _onForgotPin,
                              behavior: HitTestBehavior.opaque,
                              child: Padding(
                                padding: const EdgeInsets.symmetric(
                                  vertical: 6,
                                ),
                                child: Text(
                                  'Change PIN',
                                  style: TextStyle(
                                    fontSize: 14,
                                    fontWeight: FontWeight.w700,
                                    color: colors.brandTeal500,
                                  ),
                                ),
                              ),
                            ),
                          ),
                        ],
                      ],
                    ],
                  ),
                ),
              ),
              if (ready) ...[
                Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 24),
                  child: NumericKeypad(
                    onKeyTap: _onKeyTap,
                    enabled: !_submitting,
                  ),
                ),
                const SizedBox(height: 12),
                _buildBottomButton(enabled: _isValid && !_submitting),
              ] else
                const SizedBox(height: 100),
            ],
          ),
        ),
      ),
    );
  }

  // ── Top bar ───────────────────────────────────────────────────────

  Widget _buildTopBar(BuildContext context) {
    final colors = context.palette;
    return Padding(
      padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
      child: Stack(
        alignment: Alignment.center,
        children: [
          Align(
            alignment: Alignment.centerLeft,
            child: DispatchBackButton(
              onPressed: _submitting
                  ? () {}
                  : () => Navigator.of(context).pop(),
            ),
          ),
          Text(
            _topBarTitle,
            style: TextStyle(
              fontSize: 16,
              fontWeight: FontWeight.w700,
              color: colors.text,
              letterSpacing: -0.2,
            ),
          ),
          const SizedBox(width: 40),
        ],
      ),
    );
  }

  // ── Lock icon circle ──────────────────────────────────────────────

  Widget _buildLockIcon() {
    final colors = context.palette;
    return Center(
      child: Container(
        width: 88,
        height: 88,
        decoration: BoxDecoration(color: colors.teal10, shape: BoxShape.circle),
        child: Center(
          child: JourneyIcon(
            JourneySymbol.key,
            size: 38,
            color: colors.brandTeal500,
          ),
        ),
      ),
    );
  }

  // ── PIN slots ─────────────────────────────────────────────────────

  Widget _buildPinSlots() {
    final colors = context.palette;
    return Row(
      mainAxisAlignment: MainAxisAlignment.center,
      children: List.generate(_pinLength, (i) {
        final filled = i < _pin.length;
        return Container(
          width: ((MediaQuery.sizeOf(context).width - 64) / _pinLength - 8)
              .clamp(24.0, 42.0),
          height: 48,
          margin: const EdgeInsets.symmetric(horizontal: 4),
          alignment: Alignment.center,
          decoration: BoxDecoration(
            color: colors.surface,
            borderRadius: BorderRadius.circular(10),
            border: Border.all(
              color: filled ? colors.brandTeal500 : colors.divider,
              width: filled ? 1.6 : 1.2,
            ),
          ),
          child: filled
              ? Container(
                  width: 12,
                  height: 12,
                  decoration: BoxDecoration(
                    color: colors.text,
                    shape: BoxShape.circle,
                  ),
                )
              : null,
        );
      }),
    );
  }

  // ── Error ─────────────────────────────────────────────────────────

  Widget _buildError() {
    final colors = context.palette;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
      decoration: BoxDecoration(
        color: colors.dangerSoft,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: colors.danger, width: 1),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Icon(Icons.error_outline_rounded, size: 18, color: colors.danger),
          const SizedBox(width: 10),
          Expanded(
            child: Text(
              _error!,
              style: TextStyle(
                fontSize: 13,
                fontWeight: FontWeight.w500,
                color: colors.danger,
                height: 1.4,
              ),
            ),
          ),
        ],
      ),
    );
  }

  // ── Bottom button ─────────────────────────────────────────────────

  Widget _buildBottomButton({required bool enabled}) {
    return SafeArea(
      top: false,
      child: Padding(
        padding: const EdgeInsets.fromLTRB(24, 0, 24, 16),
        child: SizedBox(
          width: double.infinity,
          height: 56,
          child: FilledButton(
            onPressed: enabled ? _onContinue : null,
            style: FilledButton.styleFrom(
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(14),
              ),
            ),
            child: _submitting
                ? const SizedBox(
                    width: 22,
                    height: 22,
                    child: CircularProgressIndicator(
                      color: Color(0xFF081F44),
                      strokeWidth: 2.2,
                    ),
                  )
                : Text(
                    _buttonLabel,
                    style: const TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
          ),
        ),
      ),
    );
  }
}
