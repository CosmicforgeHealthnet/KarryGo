import 'package:flutter/material.dart';

import '../../../core/format/money_format.dart';
import '../../../core/widgets/karryam_ui.dart';
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
          backgroundColor: kNavy,
          body: SafeArea(
            child: Padding(
              padding: const EdgeInsets.all(24),
              child: Column(
                children: [
                  const Spacer(),
                  const SizedBox.square(
                    dimension: 64,
                    child: CircularProgressIndicator(strokeWidth: 3, color: kTeal, backgroundColor: Color(0x22FFFFFF)),
                  ),
                  const SizedBox(height: 32),
                  const Text(
                    'Payment Processing...',
                    style: TextStyle(color: Colors.white, fontSize: 22, fontWeight: FontWeight.w700),
                  ),
                  const SizedBox(height: 8),
                  const Text(
                    'Sending your money to the bank. Please keep the app open.',
                    textAlign: TextAlign.center,
                    style: TextStyle(color: Color(0xB3FFFFFF), fontSize: 13.5, height: 1.45),
                  ),
                  const SizedBox(height: 32),
                  Container(
                    width: double.infinity,
                    padding: const EdgeInsets.all(18),
                    decoration: BoxDecoration(
                      color: const Color(0x14FFFFFF),
                      borderRadius: BorderRadius.circular(18),
                    ),
                    child: Column(
                      children: [
                        const Text('Amount to pay · Total', style: TextStyle(color: Color(0xB3FFFFFF), fontSize: 12.5)),
                        const SizedBox(height: 6),
                        Text(
                          '₦ ${formatNaira(naira)}',
                          style: const TextStyle(color: Colors.white, fontSize: 28, fontWeight: FontWeight.w700),
                        ),
                      ],
                    ),
                  ),
                  const Spacer(flex: 2),
                ],
              ),
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
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            children: [
              const Spacer(),
              Container(
                width: 80,
                height: 80,
                decoration: const BoxDecoration(color: kProviderRejectBg, shape: BoxShape.circle),
                child: const Icon(Icons.error_outline_rounded, color: kProviderRejectText, size: 40),
              ),
              const SizedBox(height: 20),
              KHeading(center: true, title: 'Withdrawal failed', subtitle: message),
              const Spacer(flex: 2),
              KPrimaryButton(label: 'Go back', onPressed: onBack),
            ],
          ),
        ),
      ),
    );
  }
}
