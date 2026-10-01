import 'package:cosmicforge_logistics_ui_kit/cosmicforge_logistics_ui_kit.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../state/provider_auth_controller.dart';

/// Driver sign-in. Phone only — the service issues no email challenge for
/// drivers, so offering one would be a dead end.
class PhoneEntryScreen extends StatefulWidget {
  const PhoneEntryScreen({super.key, required this.controller});

  final ProviderAuthController controller;

  @override
  State<PhoneEntryScreen> createState() => _PhoneEntryScreenState();
}

class _PhoneEntryScreenState extends State<PhoneEntryScreen> {
  final _phoneCtrl = TextEditingController();

  @override
  void initState() {
    super.initState();
    _phoneCtrl.text = widget.controller.state.phone;
  }

  @override
  void dispose() {
    _phoneCtrl.dispose();
    super.dispose();
  }

  void _submit() {
    final raw = _phoneCtrl.text.trim();
    if (raw.isEmpty) return;
    // The service normalises to +234 itself, but sending a bare local number
    // makes the round trip depend on that; normalise the obvious case here.
    final phone = raw.startsWith('0')
        ? '+234${raw.substring(1)}'
        : (raw.startsWith('+') ? raw : '+234$raw');
    widget.controller.startAuth(phone);
  }

  @override
  Widget build(BuildContext context) {
    final state = widget.controller.state;

    return Scaffold(
      backgroundColor: CosmicforgeLogisticsColors.surface,
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              const SizedBox(height: 40),
              Container(
                width: 64,
                height: 64,
                decoration: BoxDecoration(
                  color: CosmicforgeLogisticsColors.primaryTint,
                  borderRadius: BorderRadius.circular(18),
                ),
                child: const Icon(
                  Icons.local_taxi_rounded,
                  size: 32,
                  color: CosmicforgeLogisticsColors.primary,
                ),
              ),
              const SizedBox(height: 24),
              const Text(
                'Drive with Karry Go',
                style: TextStyle(
                  fontSize: 24,
                  fontWeight: FontWeight.w800,
                  color: CosmicforgeLogisticsColors.text,
                ),
              ),
              const SizedBox(height: 6),
              const Text(
                'Sign in with the phone number on your driver account.',
                style: TextStyle(
                  fontSize: 14,
                  color: CosmicforgeLogisticsColors.textMuted,
                ),
              ),
              const SizedBox(height: 28),
              TextField(
                controller: _phoneCtrl,
                keyboardType: TextInputType.phone,
                inputFormatters: [
                  FilteringTextInputFormatter.allow(RegExp(r'[0-9+]')),
                ],
                onSubmitted: (_) => _submit(),
                decoration: InputDecoration(
                  hintText: '+234 809 999 0001',
                  filled: true,
                  fillColor: CosmicforgeLogisticsColors.card,
                  prefixIcon: const Icon(Icons.phone_rounded),
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
                const SizedBox(height: 14),
                _ErrorBanner(message: state.error!),
              ],
              const Spacer(),
              SizedBox(
                height: 52,
                child: FilledButton(
                  onPressed: state.isLoading ? null : _submit,
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
                          'Continue',
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

class _ErrorBanner extends StatelessWidget {
  const _ErrorBanner({required this.message});

  final String message;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: CosmicforgeLogisticsColors.dangerSoft,
        borderRadius: BorderRadius.circular(10),
      ),
      child: Row(
        children: [
          const Icon(
            Icons.error_outline_rounded,
            size: 18,
            color: CosmicforgeLogisticsColors.danger,
          ),
          const SizedBox(width: 8),
          Expanded(
            child: Text(
              message,
              style: const TextStyle(
                color: CosmicforgeLogisticsColors.danger,
                fontSize: 12.5,
              ),
            ),
          ),
        ],
      ),
    );
  }
}
