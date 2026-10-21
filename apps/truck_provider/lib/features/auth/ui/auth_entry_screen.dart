import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../../core/widgets/karryam_ui.dart';
import '../../home/ui/widgets/provider_app_colors.dart';
import '../state/provider_auth_controller.dart';

class ProviderAuthEntryScreen extends StatefulWidget {
  const ProviderAuthEntryScreen({super.key, required this.controller});
  final ProviderAuthController controller;

  @override
  State<ProviderAuthEntryScreen> createState() => _ProviderAuthEntryScreenState();
}

class _ProviderAuthEntryScreenState extends State<ProviderAuthEntryScreen> {
  final _phoneCtrl = TextEditingController();
  final _emailCtrl = TextEditingController();
  bool _useEmail = false;

  bool get _valid {
    if (_useEmail) {
      final e = _emailCtrl.text.trim();
      return e.contains('@') && e.length >= 5;
    }
    return _phoneCtrl.text.trim().length >= 7;
  }

  @override
  void dispose() {
    _phoneCtrl.dispose();
    _emailCtrl.dispose();
    super.dispose();
  }

  void _submit() {
    if (!_valid) return;
    if (_useEmail) {
      widget.controller.startAuth(email: _emailCtrl.text.trim().toLowerCase());
    } else {
      widget.controller.startAuth(phone: _phoneCtrl.text.trim());
    }
  }

  void _toggleMode() {
    setState(() => _useEmail = !_useEmail);
  }

  @override
  Widget build(BuildContext context) {
    final state = widget.controller.state;
    return Scaffold(
      body: SingleChildScrollView(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            const KBrandHero(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  KEyebrow('Karryam truck partner', color: Color(0xFF99E7DC)),
                  SizedBox(height: 8),
                  Text(
                    'Your road\nstarts here.',
                    style: TextStyle(fontSize: 30, height: 1.12, fontWeight: FontWeight.w700, letterSpacing: -0.8),
                  ),
                ],
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(24, 24, 24, 32),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  Text(
                    _useEmail
                        ? 'Sign in or join Karryam with your email address.'
                        : 'Sign in or join Karryam with your phone number.',
                    style: const TextStyle(color: kProviderMuted, fontSize: 14, height: 1.45),
                  ),
                  const SizedBox(height: 20),
                  KFieldLabel(_useEmail ? 'Email address' : 'Phone number'),
                  if (_useEmail)
                    TextField(
                      controller: _emailCtrl,
                      keyboardType: TextInputType.emailAddress,
                      autofillHints: const [AutofillHints.email],
                      onChanged: (_) => setState(() {}),
                      onSubmitted: (_) => _submit(),
                      decoration: const InputDecoration(
                        hintText: 'you@example.com',
                        prefixIcon: Icon(Icons.mail_outline_rounded, color: kProviderMuted),
                      ),
                    )
                  else
                    TextField(
                      controller: _phoneCtrl,
                      keyboardType: TextInputType.phone,
                      autofillHints: const [AutofillHints.telephoneNumber],
                      inputFormatters: [FilteringTextInputFormatter.digitsOnly],
                      onChanged: (_) => setState(() {}),
                      onSubmitted: (_) => _submit(),
                      style: const TextStyle(fontSize: 16, fontWeight: FontWeight.w500, letterSpacing: 0.4),
                      decoration: const InputDecoration(
                        hintText: '0801 234 5678',
                        prefixIcon: Padding(
                          padding: EdgeInsets.only(left: 16, right: 10),
                          child: Text('🇳🇬  +234', style: TextStyle(fontSize: 15, color: kProviderText)),
                        ),
                        prefixIconConstraints: BoxConstraints(minWidth: 0, minHeight: 0),
                      ),
                    ),
                  const SizedBox(height: 8),
                  const Text(
                    "We'll send you a verification code.",
                    style: TextStyle(color: kProviderMuted, fontSize: 12.5),
                  ),
                  if (state.error != null) ...[
                    const SizedBox(height: 14),
                    KNotice(state.error!, tone: KTone.danger),
                  ],
                  const SizedBox(height: 22),
                  KPrimaryButton(
                    label: 'Send verification code',
                    arrow: true,
                    loading: state.isLoading,
                    onPressed: _valid ? _submit : null,
                  ),
                  const SizedBox(height: 12),
                  KSecondaryButton(
                    label: _useEmail ? 'Use phone number instead' : 'Use email address instead',
                    icon: _useEmail ? Icons.phone_iphone_rounded : Icons.mail_outline_rounded,
                    onPressed: _toggleMode,
                  ),
                  const SizedBox(height: 24),
                  const Text(
                    'By continuing you agree to the Karryam partner terms and privacy policy.',
                    textAlign: TextAlign.center,
                    style: TextStyle(color: kProviderMuted, fontSize: 12, height: 1.4),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}
