import 'package:flutter/material.dart';

import '../../home/ui/widgets/provider_app_colors.dart';
import '../state/provider_withdrawal_controller.dart';
import 'provider_withdrawal_processing_screen.dart';
import 'widgets/wallet_keypad.dart';
import 'widgets/wallet_widgets.dart';

/// Step 3 — authorize the withdrawal with a secure PIN (Figma 2285) or account
/// password (Figma 2284).
///
/// Visual authorization gate: the provider account is OTP-based and has no
/// transaction PIN/password store yet, so this confirms intent and is not
/// validated server-side. Consistent with the app's other visual-only gates
/// (e.g. face verification). The actual withdrawal runs on the next screen.
class ProviderTransactionAuthScreen extends StatefulWidget {
  const ProviderTransactionAuthScreen({super.key, required this.controller});

  final ProviderWithdrawalController controller;

  @override
  State<ProviderTransactionAuthScreen> createState() => _ProviderTransactionAuthScreenState();
}

class _ProviderTransactionAuthScreenState extends State<ProviderTransactionAuthScreen> {
  bool _usePin = true;
  String _pin = '';
  final _passwordController = TextEditingController();
  bool _obscure = true;

  static const _pinLength = 4;

  @override
  void dispose() {
    _passwordController.dispose();
    super.dispose();
  }

  void _appendPin(String d) {
    if (_pin.length >= _pinLength) return;
    setState(() => _pin += d);
  }

  void _deletePin() {
    if (_pin.isEmpty) return;
    setState(() => _pin = _pin.substring(0, _pin.length - 1));
  }

  bool get _canConfirm =>
      _usePin ? _pin.length == _pinLength : _passwordController.text.trim().isNotEmpty;

  void _confirm() {
    FocusScope.of(context).unfocus();
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => ProviderWithdrawalProcessingScreen(controller: widget.controller),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        child: Column(
          children: [
            const WalletFlowAppBar(title: 'Transaction Authorization'),
            const SizedBox(height: 20),
            Expanded(child: _usePin ? _buildPin() : _buildPassword()),
            Padding(
              padding: EdgeInsets.fromLTRB(20, 8, 20, 16 + MediaQuery.of(context).padding.bottom),
              child: WalletPrimaryButton(
                label: _usePin ? 'Withdraw Now' : 'Confirm',
                onPressed: _canConfirm ? _confirm : null,
              ),
            ),
          ],
        ),
      ),
    );
  }

  // ─── PIN variant (Figma 2285) ───────────────────────────────────────────────
  Widget _buildPin() {
    return Column(
      children: [
        const Padding(
          padding: EdgeInsets.symmetric(horizontal: 20),
          child: Text(
            'Please input your secure pin to authorize withdrawal',
            textAlign: TextAlign.center,
            style: TextStyle(color: kProviderText, fontSize: 14),
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
        const SizedBox(height: 18),
        GestureDetector(
          onTap: () => setState(() => _usePin = false),
          behavior: HitTestBehavior.opaque,
          child: const Text(
            'Use Password',
            style: TextStyle(color: kProviderGreen, fontSize: 14, fontWeight: FontWeight.w700),
          ),
        ),
        const Spacer(),
        WalletKeypad(onDigit: _appendPin, onDelete: _deletePin, deleteTint: kProviderGreenTint),
        const SizedBox(height: 8),
      ],
    );
  }

  // ─── Password variant (Figma 2284) ──────────────────────────────────────────
  Widget _buildPassword() {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 20),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text(
            'Please input your account password to authorize withdrawal',
            style: TextStyle(color: kProviderText, fontSize: 14),
          ),
          const SizedBox(height: 24),
          const Text(
            'Confirm Password',
            style: TextStyle(color: kProviderText, fontSize: 14, fontWeight: FontWeight.w700),
          ),
          const SizedBox(height: 8),
          TextField(
            controller: _passwordController,
            obscureText: _obscure,
            onChanged: (_) => setState(() {}),
            decoration: InputDecoration(
              hintText: 'Confirm Password',
              hintStyle: const TextStyle(color: kProviderMuted),
              filled: true,
              fillColor: const Color(0xFFF1F2F4),
              contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 16),
              border: OutlineInputBorder(borderRadius: BorderRadius.circular(12), borderSide: BorderSide.none),
              suffixIcon: IconButton(
                onPressed: () => setState(() => _obscure = !_obscure),
                icon: Icon(
                  _obscure ? Icons.visibility_off_outlined : Icons.visibility_outlined,
                  color: kProviderMuted,
                ),
              ),
            ),
          ),
          const SizedBox(height: 14),
          Center(
            child: GestureDetector(
              onTap: () => setState(() => _usePin = true),
              behavior: HitTestBehavior.opaque,
              child: const Text(
                'Use Pin',
                style: TextStyle(color: kProviderGreen, fontSize: 14, fontWeight: FontWeight.w700),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _PinDot extends StatelessWidget {
  const _PinDot({required this.filled});
  final bool filled;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 52,
      height: 52,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        border: Border.all(color: kProviderGreen, width: 1.6),
        color: filled ? kProviderGreenTint : Colors.white,
      ),
      child: filled
          ? const Center(
              child: Icon(Icons.circle, size: 12, color: kProviderGreen),
            )
          : null,
    );
  }
}
