import 'package:cosmicforge_logistics_ui_kit/cosmicforge_logistics_ui_kit.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../state/provider_auth_controller.dart';

/// OTP confirmation. In debug builds against a service running with
/// TAXI_OTP_DEBUG=true the code is shown on screen, matching the customer and
/// truck apps — it is never present in a release build or a real deployment.
class OtpScreen extends StatefulWidget {
  const OtpScreen({super.key, required this.controller});

  final ProviderAuthController controller;

  @override
  State<OtpScreen> createState() => _OtpScreenState();
}

class _OtpScreenState extends State<OtpScreen> {
  final _otpCtrl = TextEditingController();

  @override
  void dispose() {
    _otpCtrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final state = widget.controller.state;
    final debugOtp = state.challenge?.debugOtp;

    return Scaffold(
      backgroundColor: CosmicforgeLogisticsColors.surface,
      appBar: AppBar(
        backgroundColor: CosmicforgeLogisticsColors.surface,
        elevation: 0,
        leading: IconButton(
          onPressed: widget.controller.backToPhoneEntry,
          icon: const Icon(Icons.arrow_back_rounded),
          color: CosmicforgeLogisticsColors.text,
        ),
      ),
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(24, 0, 24, 24),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              const Text(
                'Enter your code',
                style: TextStyle(
                  fontSize: 22,
                  fontWeight: FontWeight.w800,
                  color: CosmicforgeLogisticsColors.text,
                ),
              ),
              const SizedBox(height: 6),
              Text(
                'We sent a 6-digit code to ${state.phone}.',
                style: const TextStyle(
                  fontSize: 14,
                  color: CosmicforgeLogisticsColors.textMuted,
                ),
              ),

              if (kDebugMode && debugOtp != null) ...[
                const SizedBox(height: 18),
                Container(
                  padding: const EdgeInsets.symmetric(vertical: 14),
                  decoration: BoxDecoration(
                    color: CosmicforgeLogisticsColors.primaryTint,
                    borderRadius: BorderRadius.circular(12),
                  ),
                  child: Column(
                    children: [
                      const Text(
                        'Your OTP code',
                        style: TextStyle(
                          fontSize: 12,
                          color: CosmicforgeLogisticsColors.textMuted,
                        ),
                      ),
                      const SizedBox(height: 4),
                      Text(
                        debugOtp,
                        style: const TextStyle(
                          fontSize: 26,
                          fontWeight: FontWeight.w900,
                          letterSpacing: 6,
                          color: CosmicforgeLogisticsColors.primary,
                        ),
                      ),
                    ],
                  ),
                ),
              ],

              const SizedBox(height: 20),
              TextField(
                controller: _otpCtrl,
                keyboardType: TextInputType.number,
                textAlign: TextAlign.center,
                maxLength: 6,
                inputFormatters: [FilteringTextInputFormatter.digitsOnly],
                style: const TextStyle(
                  fontSize: 24,
                  fontWeight: FontWeight.w800,
                  letterSpacing: 10,
                ),
                onSubmitted: widget.controller.verifyOtp,
                decoration: InputDecoration(
                  counterText: '',
                  hintText: '••••••',
                  filled: true,
                  fillColor: CosmicforgeLogisticsColors.card,
                  border: OutlineInputBorder(
                    borderRadius: BorderRadius.circular(12),
                    borderSide: const BorderSide(
                      color: CosmicforgeLogisticsColors.border,
                    ),
                  ),
                  enabledBorder: OutlineInputBorder(
                    borderRadius: BorderRadius.circular(12),
                    borderSide: const BorderSide(
                      color: CosmicforgeLogisticsColors.border,
                    ),
                  ),
                ),
              ),

              if (state.error != null) ...[
                const SizedBox(height: 12),
                Text(
                  state.error!,
                  style: const TextStyle(
                    color: CosmicforgeLogisticsColors.danger,
                    fontSize: 12.5,
                  ),
                ),
              ],

              const Spacer(),
              SizedBox(
                height: 52,
                child: FilledButton(
                  onPressed: state.isLoading
                      ? null
                      : () => widget.controller.verifyOtp(_otpCtrl.text),
                  style: FilledButton.styleFrom(
                    backgroundColor: CosmicforgeLogisticsColors.primary,
                    disabledBackgroundColor:
                        CosmicforgeLogisticsColors.primarySoft,
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(26),
                    ),
                  ),
                  child: state.isLoading
                      ? const SizedBox(
                          width: 20,
                          height: 20,
                          child: CircularProgressIndicator(
                            strokeWidth: 2,
                            color: Colors.white,
                          ),
                        )
                      : const Text(
                          'Verify',
                          style: TextStyle(
                            color: Colors.white,
                            fontSize: 16,
                            fontWeight: FontWeight.w700,
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
