import 'package:flutter/material.dart';

import '../../../core/widgets/karryam_ui.dart';
import '../../home/ui/widgets/provider_app_colors.dart';
import '../state/provider_withdrawal_controller.dart';
import 'provider_withdrawal_processing_screen.dart';
import 'widgets/wallet_keypad.dart';
import 'widgets/wallet_widgets.dart';

/// Step 3 — authorize the withdrawal with the provider's transaction PIN
/// (Figma 2285).
///
/// Real authorization gate: the PIN is stored (hashed) in
/// payment-wallet-service and verified server-side with lockout when the
/// withdrawal is submitted. First-time users create the PIN here (enter +
/// confirm) before authorizing.
class ProviderTransactionAuthScreen extends StatefulWidget {
  const ProviderTransactionAuthScreen({super.key, required this.controller});

  final ProviderWithdrawalController controller;

  @override
  State<ProviderTransactionAuthScreen> createState() => _ProviderTransactionAuthScreenState();
}

enum _PinPhase { enter, create, confirmCreate }

class _ProviderTransactionAuthScreenState extends State<ProviderTransactionAuthScreen> {
  String _pin = '';
  String _firstPin = '';
  String? _localError;
  late _PinPhase _phase;

  static const _pinLength = 4;

  @override
  void initState() {
    super.initState();
    // pinSet == false -> guide the provider through creating a PIN first.
    // null (status fetch failed) is treated as set; the server rejects with
    // pin_not_set if that guess is wrong.
    _phase = widget.controller.pinSet == false ? _PinPhase.create : _PinPhase.enter;
  }

  void _appendPin(String d) {
    if (_pin.length >= _pinLength) return;
    setState(() {
      _pin += d;
      _localError = null;
    });
  }

  void _deletePin() {
    if (_pin.isEmpty) return;
    setState(() => _pin = _pin.substring(0, _pin.length - 1));
  }

  bool get _canConfirm => _pin.length == _pinLength && !widget.controller.savingPin;

  String get _prompt {
    switch (_phase) {
      case _PinPhase.enter:
        return 'Please input your secure pin to authorize withdrawal';
      case _PinPhase.create:
        return 'Create a secure 4-digit pin to authorize your withdrawals';
      case _PinPhase.confirmCreate:
        return 'Re-enter your new pin to confirm';
    }
  }

  String get _buttonLabel {
    switch (_phase) {
      case _PinPhase.enter:
        return 'Withdraw Now';
      case _PinPhase.create:
        return 'Continue';
      case _PinPhase.confirmCreate:
        return 'Save Pin & Withdraw';
    }
  }

  Future<void> _confirm() async {
    final controller = widget.controller;
    switch (_phase) {
      case _PinPhase.create:
        setState(() {
          _firstPin = _pin;
          _pin = '';
          _phase = _PinPhase.confirmCreate;
        });
        return;

      case _PinPhase.confirmCreate:
        if (_pin != _firstPin) {
          setState(() {
            _pin = '';
            _firstPin = '';
            _phase = _PinPhase.create;
            _localError = 'Pins did not match. Try again.';
          });
          return;
        }
        final saved = await controller.setupPin(_pin);
        if (!mounted) return;
        if (!saved) {
          setState(() {
            _pin = '';
            _firstPin = '';
            _phase = _PinPhase.create;
            _localError = controller.pinError ?? 'Pin could not be saved.';
          });
          return;
        }
        _authorize(_pin);
        return;

      case _PinPhase.enter:
        _authorize(_pin);
        return;
    }
  }

  void _authorize(String pin) {
    widget.controller.setEnteredPin(pin);
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => ProviderWithdrawalProcessingScreen(controller: widget.controller),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: SafeArea(
        child: AnimatedBuilder(
          animation: widget.controller,
          builder: (context, _) => Column(
            children: [
              const WalletFlowAppBar(title: 'Transaction Authorization'),
              const SizedBox(height: 28),
              KIconTile(Icons.lock_outline_rounded, size: 56),
              const SizedBox(height: 16),
              Expanded(child: _buildPin()),
              Padding(
                padding: EdgeInsets.fromLTRB(20, 8, 20, 16 + MediaQuery.of(context).padding.bottom),
                child: WalletPrimaryButton(
                  label: widget.controller.savingPin ? 'Saving...' : _buttonLabel,
                  onPressed: _canConfirm ? _confirm : null,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildPin() {
    return Column(
      children: [
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 20),
          child: Text(
            _prompt,
            textAlign: TextAlign.center,
            style: TextStyle(color: kProviderText, fontSize: 15, fontWeight: FontWeight.w500, height: 1.4),
          ),
        ),
        const SizedBox(height: 22),
        Row(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            for (var i = 0; i < _pinLength; i++)
              Padding(
                padding: const EdgeInsets.symmetric(horizontal: 9),
                child: _PinDot(filled: i < _pin.length),
              ),
          ],
        ),
        if (_localError != null) ...[
          const SizedBox(height: 14),
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 20),
            child: Text(
              _localError!,
              textAlign: TextAlign.center,
              style: TextStyle(color: kProviderRejectText, fontSize: 13),
            ),
          ),
        ],
        const Spacer(),
        WalletKeypad(onDigit: _appendPin, onDelete: _deletePin, deleteTint: kTealTint),
        const SizedBox(height: 8),
      ],
    );
  }
}

class _PinDot extends StatelessWidget {
  const _PinDot({required this.filled});
  final bool filled;

  @override
  Widget build(BuildContext context) {
    return AnimatedContainer(
      duration: const Duration(milliseconds: 150),
      width: 52,
      height: 56,
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: filled ? kTeal : kProviderBorder, width: filled ? 1.8 : 1.2),
        color: kCard,
      ),
      child: filled ? Center(child: Icon(Icons.circle, size: 12, color: kProviderText)) : null,
    );
  }
}
