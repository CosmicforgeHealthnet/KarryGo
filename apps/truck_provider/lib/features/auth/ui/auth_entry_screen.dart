import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

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
      backgroundColor: kProviderSurface,
      body: SafeArea(
        child: SingleChildScrollView(
          padding: const EdgeInsets.symmetric(horizontal: 24),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              const SizedBox(height: 48),

              // ── Illustration ────────────────────────────────────────────
              _Illustration(),
              const SizedBox(height: 32),

              // ── Headline ─────────────────────────────────────────────────
              const Text(
                'Welcome to KarryGo!',
                textAlign: TextAlign.center,
                style: TextStyle(
                  color: kProviderText,
                  fontSize: 26,
                  fontWeight: FontWeight.w800,
                  height: 1.2,
                ),
              ),
              const SizedBox(height: 6),
              const Text(
                "Let's get you moving.",
                textAlign: TextAlign.center,
                style: TextStyle(
                  color: kProviderGreen,
                  fontSize: 16,
                  fontWeight: FontWeight.w600,
                ),
              ),
              const SizedBox(height: 8),
              Text(
                _useEmail
                    ? 'Enter your email to continue.'
                    : 'Enter your phone number to continue.',
                textAlign: TextAlign.center,
                style: const TextStyle(color: kProviderMuted, fontSize: 13),
              ),
              const SizedBox(height: 32),

              // ── Contact label ────────────────────────────────────────────
              Text(
                _useEmail ? 'Enter your Email Address' : 'Enter your Phone Number',
                style: const TextStyle(
                  color: kProviderText,
                  fontSize: 13,
                  fontWeight: FontWeight.w700,
                ),
              ),
              const SizedBox(height: 8),

              // ── Contact input (email or phone) ───────────────────────────
              if (_useEmail)
                Container(
                  decoration: BoxDecoration(
                    color: Colors.white,
                    borderRadius: BorderRadius.circular(12),
                    border: Border.all(color: kProviderBorder),
                  ),
                  child: TextField(
                    controller: _emailCtrl,
                    keyboardType: TextInputType.emailAddress,
                    onChanged: (_) => setState(() {}),
                    onSubmitted: (_) => _submit(),
                    decoration: const InputDecoration(
                      hintText: 'you@example.com',
                      hintStyle: TextStyle(color: kProviderMuted, fontSize: 14),
                      border: InputBorder.none,
                      contentPadding: EdgeInsets.symmetric(horizontal: 16, vertical: 14),
                    ),
                  ),
                )
              else
                Container(
                  decoration: BoxDecoration(
                    color: Colors.white,
                    borderRadius: BorderRadius.circular(12),
                    border: Border.all(color: kProviderBorder),
                  ),
                  child: Row(
                    children: [
                      const Padding(
                        padding: EdgeInsets.symmetric(horizontal: 12),
                        child: Text('🇳🇬  +234', style: TextStyle(fontSize: 15, color: kProviderText)),
                      ),
                      Container(width: 1, height: 24, color: kProviderBorder),
                      Expanded(
                        child: TextField(
                          controller: _phoneCtrl,
                          keyboardType: TextInputType.phone,
                          inputFormatters: [FilteringTextInputFormatter.digitsOnly],
                          onChanged: (_) => setState(() {}),
                          onSubmitted: (_) => _submit(),
                          decoration: const InputDecoration(
                            hintText: '801 234 5678',
                            hintStyle: TextStyle(color: kProviderMuted, fontSize: 14),
                            border: InputBorder.none,
                            contentPadding: EdgeInsets.symmetric(horizontal: 12, vertical: 14),
                          ),
                        ),
                      ),
                    ],
                  ),
                ),
              const SizedBox(height: 6),
              const Text(
                "We'll send you a verification code.",
                style: TextStyle(color: kProviderGreen, fontSize: 12),
              ),
              const SizedBox(height: 4),
              GestureDetector(
                onTap: _toggleMode,
                child: Text(
                  _useEmail ? 'Use phone number instead' : 'Use email address instead',
                  style: const TextStyle(
                    color: kProviderGreen,
                    fontSize: 12,
                    fontWeight: FontWeight.w600,
                    decoration: TextDecoration.underline,
                  ),
                ),
              ),

              if (state.error != null) ...[
                const SizedBox(height: 8),
                Text(state.error!, style: const TextStyle(color: Colors.red, fontSize: 12)),
              ],
              const SizedBox(height: 24),

              // ── Continue button ──────────────────────────────────────────
              _PillButton(
                label: 'Continue',
                isLoading: state.isLoading,
                enabled: _valid && !state.isLoading,
                onTap: _submit,
              ),
              const SizedBox(height: 20),

              // ── Or divider ───────────────────────────────────────────────
              const _OrDivider(),
              const SizedBox(height: 20),

              // ── Google button ────────────────────────────────────────────
              _SocialButton(
                label: 'Continue with Google',
                icon: _googleIcon(),
                backgroundColor: Colors.white,
                textColor: kProviderText,
                borderColor: kProviderBorder,
                onTap: () => _showComingSoon(context),
              ),
              const SizedBox(height: 12),

              // ── Apple button ─────────────────────────────────────────────
              _SocialButton(
                label: 'Continue with Apple',
                icon: const Icon(Icons.apple, color: Colors.white, size: 20),
                backgroundColor: kProviderText,
                textColor: Colors.white,
                onTap: () => _showComingSoon(context),
              ),
              const SizedBox(height: 24),

              // // ── Log In link ──────────────────────────────────────────────
              // GestureDetector(
              //   onTap: () => _showComingSoon(context),
              //   child: RichText(
              //     textAlign: TextAlign.center,
              //     text: const TextSpan(
              //       style: TextStyle(color: kProviderMuted, fontSize: 13),
              //       children: [
              //         TextSpan(text: 'Already have an account? '),
              //         TextSpan(
              //           text: 'Log In',
              //           style: TextStyle(
              //             color: kProviderGreen,
              //             fontWeight: FontWeight.w700,
              //           ),
              //         ),
              //       ],
              //     ),
              //   ),
              // ),
              const SizedBox(height: 32),
            ],
          ),
        ),
      ),
    );
  }

  void _showComingSoon(BuildContext context) {
    ScaffoldMessenger.of(context).showSnackBar(
      const SnackBar(content: Text('Coming soon'), duration: Duration(seconds: 2)),
    );
  }

  Widget _googleIcon() {
    return Container(
      width: 20,
      height: 20,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        color: kProviderSurface,
      ),
      child: const Center(
        child: Text('G', style: TextStyle(fontSize: 13, fontWeight: FontWeight.w900, color: Color(0xFF4285F4))),
      ),
    );
  }
}

// ─── Illustration ─────────────────────────────────────────────────────────────

class _Illustration extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    return Container(
      height: 180,
      decoration: BoxDecoration(
        color: kProviderGreenTint,
        borderRadius: BorderRadius.circular(20),
      ),
      child: Stack(
        alignment: Alignment.center,
        children: [
          Positioned(
            bottom: 20,
            child: Icon(Icons.local_shipping_rounded, size: 80, color: kProviderGreen.withValues(alpha: 0.2)),
          ),
          Column(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              const Icon(Icons.local_shipping_rounded, size: 56, color: kProviderGreen),
              const SizedBox(height: 8),
              Text(
                'Karry Go Provider',
                style: TextStyle(
                  color: kProviderGreen,
                  fontSize: 16,
                  fontWeight: FontWeight.w800,
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }
}

// ─── Shared widgets ───────────────────────────────────────────────────────────

class _PillButton extends StatelessWidget {
  const _PillButton({
    required this.label,
    required this.onTap,
    this.isLoading = false,
    this.enabled = true,
  });

  final String label;
  final VoidCallback onTap;
  final bool isLoading;
  final bool enabled;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: 52,
      child: FilledButton(
        onPressed: enabled ? onTap : null,
        style: FilledButton.styleFrom(
          backgroundColor: kProviderGreen,
          disabledBackgroundColor: kProviderGreen.withValues(alpha: 0.4),
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(999)),
        ),
        child: isLoading
            ? const SizedBox.square(
                dimension: 20,
                child: CircularProgressIndicator(strokeWidth: 2, color: Colors.white),
              )
            : Text(
                label,
                style: const TextStyle(
                  color: Colors.white,
                  fontWeight: FontWeight.w700,
                  fontSize: 16,
                ),
              ),
      ),
    );
  }
}

class _OrDivider extends StatelessWidget {
  const _OrDivider();

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        const Expanded(child: Divider(color: kProviderBorder, thickness: 1)),
        const Padding(
          padding: EdgeInsets.symmetric(horizontal: 12),
          child: Text('Or', style: TextStyle(color: kProviderMuted, fontSize: 13)),
        ),
        const Expanded(child: Divider(color: kProviderBorder, thickness: 1)),
      ],
    );
  }
}

class _SocialButton extends StatelessWidget {
  const _SocialButton({
    required this.label,
    required this.icon,
    required this.backgroundColor,
    required this.textColor,
    required this.onTap,
    this.borderColor,
  });

  final String label;
  final Widget icon;
  final Color backgroundColor;
  final Color textColor;
  final Color? borderColor;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        height: 52,
        decoration: BoxDecoration(
          color: backgroundColor,
          borderRadius: BorderRadius.circular(999),
          border: borderColor != null ? Border.all(color: borderColor!) : null,
        ),
        child: Row(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            icon,
            const SizedBox(width: 10),
            Text(
              label,
              style: TextStyle(
                color: textColor,
                fontWeight: FontWeight.w600,
                fontSize: 15,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
