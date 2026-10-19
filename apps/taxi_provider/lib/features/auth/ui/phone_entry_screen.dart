import '../../../core/theme/karryam_theme.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../state/provider_auth_controller.dart';

/// Driver sign-in with a phone number or an email address. Email codes
/// arrive by email; phone codes need an SMS provider the platform does not
/// have yet, so email is the dependable option for now.
class PhoneEntryScreen extends StatefulWidget {
  const PhoneEntryScreen({super.key, required this.controller});

  final ProviderAuthController controller;

  @override
  State<PhoneEntryScreen> createState() => _PhoneEntryScreenState();
}

class _PhoneEntryScreenState extends State<PhoneEntryScreen> {
  final _phoneCtrl = TextEditingController();
  final _emailCtrl = TextEditingController();

  @override
  void initState() {
    super.initState();
    _phoneCtrl.text = widget.controller.state.phone;
    _emailCtrl.text = widget.controller.state.email;
  }

  @override
  void dispose() {
    _phoneCtrl.dispose();
    _emailCtrl.dispose();
    super.dispose();
  }

  void _submit() {
    if (widget.controller.state.isLoading) return;
    if (widget.controller.state.useEmail) {
      widget.controller.startEmailAuth(_emailCtrl.text);
      return;
    }
    final raw = _phoneCtrl.text.trim();
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
                state.useEmail
                    ? 'Sign in with the email address on your driver account.'
                    : 'Sign in with the phone number on your driver account.',
                style: TextStyle(
                  fontSize: 14,
                  color: KarryamColors.of(context).textMuted,
                ),
              ),
              SizedBox(height: 28),
              TextField(
                key: ValueKey(state.useEmail),
                controller: state.useEmail ? _emailCtrl : _phoneCtrl,
                keyboardType: state.useEmail
                    ? TextInputType.emailAddress
                    : TextInputType.phone,
                autocorrect: false,
                inputFormatters: state.useEmail
                    ? const []
                    : [FilteringTextInputFormatter.allow(RegExp(r'[0-9+]'))],
                onSubmitted: (_) => _submit(),
                decoration: InputDecoration(
                  hintText:
                      state.useEmail ? 'you@example.com' : '+234 809 999 0001',
                  filled: true,
                  fillColor: KarryamColors.of(context).card,
                  prefixIcon: Icon(
                    state.useEmail ? Icons.mail_rounded : Icons.phone_rounded,
                  ),
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
              Align(
                alignment: Alignment.centerRight,
                child: TextButton(
                  onPressed: state.isLoading
                      ? null
                      : () => widget.controller.setUseEmail(!state.useEmail),
                  child: Text(
                    state.useEmail
                        ? 'Use phone number instead'
                        : 'Use email address instead',
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
