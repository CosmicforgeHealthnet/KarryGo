import '../../../core/theme/karryam_theme.dart';
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
    if (widget.controller.state.isLoading) return;
    if (raw.isEmpty) {
      widget.controller.startAuth(raw);
      return;
    }
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
      backgroundColor: KarryamColors.of(context).surface,
      body: SafeArea(
        child: SingleChildScrollView(
          padding: EdgeInsets.all(24),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              SizedBox(height: 40),
              Container(
                height: 150,
                alignment: Alignment.center,
                decoration: BoxDecoration(
                  color: KarryamColors.navy,
                  borderRadius: BorderRadius.circular(22),
                ),
                child: Image.asset(
                  'assets/branding/karryam-logo.png',
                  width: 205,
                ),
              ),
              SizedBox(height: 24),
              Text(
                'Good to have you here.',
                style: TextStyle(fontSize: 32, fontWeight: FontWeight.w700),
              ),
              SizedBox(height: 16),
              Text(
                'DRIVE WITH KARRYAM',
                style: TextStyle(
                  fontSize: 11,
                  fontWeight: FontWeight.w800,
                  color: KarryamColors.of(context).text,
                ),
              ),
              SizedBox(height: 6),
              Text(
                'Sign in with the phone number on your driver account.',
                style: TextStyle(
                  fontSize: 14,
                  color: KarryamColors.of(context).textMuted,
                ),
              ),
              SizedBox(height: 28),
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
                  fillColor: KarryamColors.of(context).card,
                  prefixIcon: Icon(Icons.phone_rounded),
                  border: OutlineInputBorder(
                    borderRadius: BorderRadius.circular(12),
                    borderSide: BorderSide(
                      color: KarryamColors.of(context).border,
                    ),
                  ),
                  enabledBorder: OutlineInputBorder(
                    borderRadius: BorderRadius.circular(12),
                    borderSide: BorderSide(
                      color: KarryamColors.of(context).border,
                    ),
                  ),
                ),
              ),
              if (state.error != null) ...[
                SizedBox(height: 14),
                _ErrorBanner(message: state.error!),
              ],
              SizedBox(height: 28),
              SizedBox(
                height: 52,
                child: FilledButton(
                  onPressed: state.isLoading ? null : _submit,
                  style: FilledButton.styleFrom(
                    backgroundColor: KarryamColors.of(context).primary,
                    disabledBackgroundColor: KarryamColors.of(
                      context,
                    ).primarySoft,
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(12),
                    ),
                  ),
                  child: state.isLoading
                      ? SizedBox(
                          width: 20,
                          height: 20,
                          child: CircularProgressIndicator(
                            strokeWidth: 2,
                            color: KarryamColors.of(context).onPrimary,
                          ),
                        )
                      : Text(
                          'Continue',
                          style: TextStyle(
                            color: KarryamColors.of(context).onPrimary,
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
      padding: EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: KarryamColors.of(context).dangerSoft,
        borderRadius: BorderRadius.circular(10),
      ),
      child: Row(
        children: [
          Icon(
            Icons.error_outline_rounded,
            size: 18,
            color: KarryamColors.of(context).danger,
          ),
          SizedBox(width: 8),
          Expanded(
            child: Text(
              message,
              style: TextStyle(
                color: KarryamColors.of(context).danger,
                fontSize: 12.5,
              ),
            ),
          ),
        ],
      ),
    );
  }
}
