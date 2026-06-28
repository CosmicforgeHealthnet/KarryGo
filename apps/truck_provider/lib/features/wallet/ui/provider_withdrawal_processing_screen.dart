import 'package:flutter/material.dart';

import '../../../core/format/money_format.dart';
import '../../home/ui/widgets/provider_app_colors.dart';
import '../state/provider_withdrawal_controller.dart';
import 'provider_withdrawal_receipt_screen.dart';

/// Step 4 — runs the withdrawal request and shows the processing state
/// (Figma 2283). On success it replaces itself with the receipt; on failure it
/// surfaces the error with a way back.
class ProviderWithdrawalProcessingScreen extends StatefulWidget {
  const ProviderWithdrawalProcessingScreen({super.key, required this.controller});

  final ProviderWithdrawalController controller;

  @override
  State<ProviderWithdrawalProcessingScreen> createState() =>
      _ProviderWithdrawalProcessingScreenState();
}

class _ProviderWithdrawalProcessingScreenState extends State<ProviderWithdrawalProcessingScreen> {
  bool _navigated = false;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) => _submit());
  }

  Future<void> _submit() async {
    final ok = await widget.controller.submitWithdrawal();
    if (!mounted || _navigated) return;
    if (ok) {
      _navigated = true;
      Navigator.of(context).pushReplacement(
        MaterialPageRoute(builder: (_) => ProviderWithdrawalReceiptScreen(controller: widget.controller)),
      );
    }
    // On failure the AnimatedBuilder below renders the error state.
  }

  @override
  Widget build(BuildContext context) {
    final naira = widget.controller.amountKobo / 100;
    return AnimatedBuilder(
      animation: widget.controller,
      builder: (context, _) {
        final error = widget.controller.submitError;
        if (error != null && !widget.controller.submitting) {
          return _ErrorScaffold(
            message: error,
            onBack: () => Navigator.of(context).pop(),
          );
        }
        return Scaffold(
          backgroundColor: kProviderGreen,
          body: Center(
            child: Column(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                const Text('Amount to pay', style: TextStyle(color: Colors.white, fontSize: 14)),
                const SizedBox(height: 8),
                const Text(
                  'Total',
                  style: TextStyle(color: Colors.white, fontSize: 14, fontWeight: FontWeight.w700),
                ),
                const SizedBox(height: 6),
                Text(
                  '₦ ${formatNaira(naira)}',
                  style: const TextStyle(color: Colors.white, fontSize: 26, fontWeight: FontWeight.w800),
                ),
                const SizedBox(height: 40),
                const SizedBox.square(
                  dimension: 26,
                  child: CircularProgressIndicator(strokeWidth: 2.5, color: Colors.white),
                ),
              ],
            ),
          ),
          bottomNavigationBar: const Padding(
            padding: EdgeInsets.only(bottom: 40),
            child: Text(
              'Payment Processing...',
              textAlign: TextAlign.center,
              style: TextStyle(color: Colors.white, fontSize: 18, fontWeight: FontWeight.w700),
            ),
          ),
        );
      },
    );
  }
}

class _ErrorScaffold extends StatelessWidget {
  const _ErrorScaffold({required this.message, required this.onBack});
  final String message;
  final VoidCallback onBack;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              const Icon(Icons.error_outline_rounded, color: kProviderRejectText, size: 56),
              const SizedBox(height: 16),
              const Text(
                'Withdrawal failed',
                style: TextStyle(color: kProviderText, fontSize: 20, fontWeight: FontWeight.w800),
              ),
              const SizedBox(height: 10),
              Text(
                message,
                textAlign: TextAlign.center,
                style: const TextStyle(color: kProviderMuted, fontSize: 14, height: 1.4),
              ),
              const SizedBox(height: 28),
              SizedBox(
                width: double.infinity,
                height: 54,
                child: FilledButton(
                  onPressed: onBack,
                  style: FilledButton.styleFrom(
                    backgroundColor: kProviderGreen,
                    shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(999)),
                  ),
                  child: const Text(
                    'Go back',
                    style: TextStyle(color: Colors.white, fontSize: 16, fontWeight: FontWeight.w700),
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
