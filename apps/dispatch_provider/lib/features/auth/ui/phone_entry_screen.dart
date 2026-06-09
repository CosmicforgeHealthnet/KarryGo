import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_svg/flutter_svg.dart';
import 'package:karrygo_api_core/karrygo_api_core.dart';
import '../state/dispatch_auth_controller.dart';

/// Signup screen — collects phone number + email address.
/// Called [PhoneEntryScreen] to preserve existing import paths.
class PhoneEntryScreen extends StatefulWidget {
  const PhoneEntryScreen({
    super.key,
    required this.onContinue,
    required this.controller,
    this.onLoginTap,
  });

  /// Called with (phone, email) after signupStart succeeds.
  final void Function(String phone, String email) onContinue;
  final DispatchAuthController controller;

  /// Called when the user taps "Already have an account? Log In".
  final VoidCallback? onLoginTap;

  @override
  State<PhoneEntryScreen> createState() => _PhoneEntryScreenState();
}

class _PhoneEntryScreenState extends State<PhoneEntryScreen> {
  final _phoneController = TextEditingController();
  final _emailController = TextEditingController();
  bool _isLoading = false;

  @override
  void dispose() {
    _phoneController.dispose();
    _emailController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: const Color(0xFFF7F7F7),
      body: SafeArea(
        child: SingleChildScrollView(
          padding: const EdgeInsets.symmetric(horizontal: 24),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              const SizedBox(height: 120),
              // Car illustration
              Padding(
                padding: const EdgeInsets.only(left: 29.0),
                child: Image.asset(
                  'assets/figma/auth_car_header.png',
                  width: 323,
                  height: 88,
                  fit: BoxFit.contain,
                  alignment: Alignment.centerLeft,
                ),
              ),
              const SizedBox(height: 32),
              const Text(
                'Welcome to KarryGo!',
                style: TextStyle(
                  fontSize: 22,
                  fontWeight: FontWeight.w800,
                  color: Color(0xFF1A1A1A),
                ),
              ),
              const SizedBox(height: 4),
              const Text(
                "Let's get you moving.",
                style: TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w700,
                  color: Color(0xFF4CAF50),
                ),
              ),
              const SizedBox(height: 8),
              const Text(
                'Create your account to get started.',
                style: TextStyle(
                  fontSize: 13,
                  color: Color(0xFF888888),
                ),
              ),
              const SizedBox(height: 28),

              // ── Phone number ──────────────────────────────────────────────
              const Text(
                'Phone Number',
                style: TextStyle(
                  fontSize: 13,
                  fontWeight: FontWeight.w700,
                  color: Color(0xFF1A1A1A),
                ),
              ),
              const SizedBox(height: 8),
              Row(
                children: [
                  Container(
                    height: 52,
                    padding: const EdgeInsets.symmetric(horizontal: 12),
                    decoration: BoxDecoration(
                      color: Colors.white,
                      borderRadius: BorderRadius.circular(10),
                      border: Border.all(color: const Color(0xFF4CAF50)),
                    ),
                    child: const Row(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        _NigeriaFlag(),
                        SizedBox(width: 6),
                        Text(
                          '+234',
                          style: TextStyle(
                            fontSize: 13,
                            fontWeight: FontWeight.w600,
                            color: Color(0xFF1A1A1A),
                          ),
                        ),
                        SizedBox(width: 2),
                        Icon(
                          Icons.keyboard_arrow_down_rounded,
                          size: 18,
                          color: Color(0xFF1A1A1A),
                        ),
                      ],
                    ),
                  ),
                  const SizedBox(width: 10),
                  Expanded(
                    child: TextField(
                      controller: _phoneController,
                      keyboardType: TextInputType.phone,
                      style: const TextStyle(
                        fontSize: 14,
                        color: Color(0xFF1A1A1A),
                      ),
                      inputFormatters: [
                        FilteringTextInputFormatter.allow(RegExp(r'[0-9 ]')),
                      ],
                      decoration: InputDecoration(
                        hintText: '8067735987',
                        hintStyle: const TextStyle(
                          color: Color(0xFFBBBBBB),
                          fontSize: 14,
                        ),
                        filled: true,
                        fillColor: Colors.white,
                        contentPadding: const EdgeInsets.symmetric(
                          horizontal: 14,
                          vertical: 16,
                        ),
                        border: OutlineInputBorder(
                          borderRadius: BorderRadius.circular(10),
                          borderSide: const BorderSide(color: Color(0xFF4CAF50)),
                        ),
                        enabledBorder: OutlineInputBorder(
                          borderRadius: BorderRadius.circular(10),
                          borderSide: const BorderSide(color: Color(0xFF4CAF50)),
                        ),
                        focusedBorder: OutlineInputBorder(
                          borderRadius: BorderRadius.circular(10),
                          borderSide: const BorderSide(
                            color: Color(0xFF4CAF50),
                            width: 1.5,
                          ),
                        ),
                      ),
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 16),

              // ── Email address ─────────────────────────────────────────────
              const Text(
                'Email Address',
                style: TextStyle(
                  fontSize: 13,
                  fontWeight: FontWeight.w700,
                  color: Color(0xFF1A1A1A),
                ),
              ),
              const SizedBox(height: 8),
              TextField(
                controller: _emailController,
                keyboardType: TextInputType.emailAddress,
                autocorrect: false,
                inputFormatters: [
                  // Disallow spaces — email addresses never contain spaces.
                  FilteringTextInputFormatter.deny(RegExp(r'\s')),
                ],
                style: const TextStyle(
                  fontSize: 14,
                  color: Color(0xFF1A1A1A),
                ),
                decoration: InputDecoration(
                  hintText: 'you@example.com',
                  hintStyle: const TextStyle(
                    color: Color(0xFFBBBBBB),
                    fontSize: 14,
                  ),
                  filled: true,
                  fillColor: Colors.white,
                  contentPadding: const EdgeInsets.symmetric(
                    horizontal: 14,
                    vertical: 16,
                  ),
                  border: OutlineInputBorder(
                    borderRadius: BorderRadius.circular(10),
                    borderSide: const BorderSide(color: Color(0xFF4CAF50)),
                  ),
                  enabledBorder: OutlineInputBorder(
                    borderRadius: BorderRadius.circular(10),
                    borderSide: const BorderSide(color: Color(0xFF4CAF50)),
                  ),
                  focusedBorder: OutlineInputBorder(
                    borderRadius: BorderRadius.circular(10),
                    borderSide: const BorderSide(
                      color: Color(0xFF4CAF50),
                      width: 1.5,
                    ),
                  ),
                ),
              ),
              const SizedBox(height: 8),
              const Text(
                "We'll send your verification code to your phone.",
                style: TextStyle(
                  fontSize: 12,
                  color: Color(0xFF4CAF50),
                  fontWeight: FontWeight.w500,
                ),
              ),
              const SizedBox(height: 32),

              // ── Continue button ───────────────────────────────────────────
              _ContinueButton(
                phoneListenable: _phoneController,
                emailListenable: _emailController,
                isLoading: _isLoading,
                onTap: _signup,
              ),
              const SizedBox(height: 28),
              const _DividerLabel(label: 'Or'),
              const SizedBox(height: 20),
              // Google button
              SizedBox(
                height: 52,
                child: OutlinedButton(
                  onPressed: () {},
                  style: OutlinedButton.styleFrom(
                    backgroundColor: Colors.white,
                    side: const BorderSide(color: Color(0xFFDDDDDD)),
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(999),
                    ),
                  ),
                  child: Row(
                    mainAxisAlignment: MainAxisAlignment.center,
                    children: [
                      SvgPicture.string(
                        '''<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 48 48">
                          <path fill="#EA4335" d="M24 9.5c3.54 0 6.71 1.22 9.21 3.6l6.85-6.85C35.9 2.38 30.47 0 24 0 14.62 0 6.51 5.38 2.56 13.22l7.98 6.19C12.43 13.72 17.74 9.5 24 9.5z"/>
                          <path fill="#4285F4" d="M46.98 24.55c0-1.57-.15-3.09-.38-4.55H24v9.02h12.94c-.58 2.96-2.26 5.48-4.78 7.18l7.73 6c4.51-4.18 7.09-10.36 7.09-17.65z"/>
                          <path fill="#FBBC05" d="M10.53 28.59c-.48-1.45-.76-2.99-.76-4.59s.27-3.14.76-4.59l-7.98-6.19C.92 16.46 0 20.12 0 24c0 3.88.92 7.54 2.56 10.78l7.97-6.19z"/>
                          <path fill="#34A853" d="M24 48c6.48 0 11.93-2.13 15.89-5.81l-7.73-6c-2.18 1.48-4.97 2.31-8.16 2.31-6.26 0-11.57-4.22-13.47-9.91l-7.98 6.19C6.51 42.62 14.62 48 24 48z"/>
                          <path fill="none" d="M0 0h48v48H0z"/>
                        </svg>''',
                        width: 22,
                        height: 22,
                      ),
                      const SizedBox(width: 12),
                      const Text(
                        'Continue with Google',
                        style: TextStyle(
                          fontSize: 15,
                          fontWeight: FontWeight.w600,
                          color: Color(0xFF1A1A1A),
                        ),
                      ),
                    ],
                  ),
                ),
              ),
              const SizedBox(height: 12),
              // Apple button
              SizedBox(
                height: 52,
                child: FilledButton(
                  onPressed: () {},
                  style: FilledButton.styleFrom(
                    backgroundColor: Colors.black,
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(999),
                    ),
                  ),
                  child: const Row(
                    mainAxisAlignment: MainAxisAlignment.center,
                    children: [
                      Icon(Icons.apple, color: Colors.white, size: 22),
                      SizedBox(width: 10),
                      Text(
                        'Continue with Apple',
                        style: TextStyle(
                          fontSize: 15,
                          fontWeight: FontWeight.w600,
                          color: Colors.white,
                        ),
                      ),
                    ],
                  ),
                ),
              ),
              const SizedBox(height: 28),
              GestureDetector(
                onTap: widget.onLoginTap,
                child: Text.rich(
                  TextSpan(
                    text: 'Already have an account? ',
                    style: const TextStyle(
                      color: Color(0xFF888888),
                      fontSize: 13,
                    ),
                    children: const [
                      TextSpan(
                        text: 'Log In',
                        style: TextStyle(
                          color: Color(0xFF4CAF50),
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                    ],
                  ),
                  textAlign: TextAlign.center,
                ),
              ),
              const SizedBox(height: 32),
            ],
          ),
        ),
      ),
    );
  }

  void _signup() async {
    if (_isLoading) return; // guard against double-tap

    final rawPhone = _phoneController.text.replaceAll(' ', '').trim();
    final email = _emailController.text.trim();

    if (rawPhone.isEmpty) {
      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(
        content: Text('Please enter your phone number.'),
        backgroundColor: Colors.red,
      ));
      return;
    }
    if (email.isEmpty) {
      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(
        content: Text('Please enter your email address.'),
        backgroundColor: Colors.red,
      ));
      return;
    }

    // Basic email format validation.
    final emailRegex = RegExp(r'^[\w.+\-]+@[\w\-]+\.[\w.\-]+$');
    if (!emailRegex.hasMatch(email)) {
      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(
        content: Text('Please enter a valid email address.'),
        backgroundColor: Colors.red,
      ));
      return;
    }

    FocusScope.of(context).unfocus();

    // Normalize phone: strip country code prefix and re-add +234.
    String subscriber = rawPhone;
    if (subscriber.startsWith('+234')) {
      subscriber = subscriber.substring(4);
    } else if (subscriber.startsWith('234')) {
      subscriber = subscriber.substring(3);
    }
    while (subscriber.startsWith('0')) {
      subscriber = subscriber.substring(1);
    }

    // Nigerian subscriber number is 10 digits (e.g. 8012345678).
    if (subscriber.length != 10 || !RegExp(r'^\d{10}$').hasMatch(subscriber)) {
      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(
        content: Text('Please enter a valid Nigerian phone number.'),
        backgroundColor: Colors.red,
      ));
      return;
    }

    final phone = '+234$subscriber';
    debugPrint('[SIGNUP] Normalized phone: $phone');

    final messenger = ScaffoldMessenger.of(context);
    setState(() => _isLoading = true);

    // signupStart always returns ApiResult — it never throws.
    final result = await widget.controller.signupStart(
      phoneNumber: phone,
      email: email,
    );

    if (!mounted) return;
    setState(() => _isLoading = false);

    result.when(
      success: (_) => widget.onContinue(phone, email),
      failure: (error) {
        messenger.showSnackBar(SnackBar(
          content: Text(_signupErrorMessage(error)),
          backgroundColor: Colors.red.shade800,
          duration: const Duration(seconds: 5),
        ));
      },
    );
  }

  /// Returns a user-friendly message for signup errors.
  static String _signupErrorMessage(ApiException error) {
    switch (error.code) {
      case ApiErrorCode.conflict:
        return 'An account already exists with this phone number or email. Please log in instead.';
      case ApiErrorCode.network:
        final cause = error.cause?.toString() ?? '';
        if (cause.toLowerCase().contains('timeout')) {
          return 'The request timed out. Please try again.';
        }
        return 'Could not connect to the server. Please check your connection and try again.';
      case ApiErrorCode.rateLimited:
        return 'Too many attempts. Please wait a moment and try again.';
      case ApiErrorCode.validationFailed:
        return error.message.isNotEmpty
            ? error.message
            : 'Please check your details and try again.';
      default:
        return error.message.isNotEmpty
            ? error.message
            : 'Something went wrong. Please try again.';
    }
  }
}

// ── Helpers ────────────────────────────────────────────────────────────────────

class _ContinueButton extends StatelessWidget {
  const _ContinueButton({
    required this.phoneListenable,
    required this.emailListenable,
    required this.isLoading,
    required this.onTap,
  });

  final TextEditingController phoneListenable;
  final TextEditingController emailListenable;
  final bool isLoading;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: Listenable.merge([phoneListenable, emailListenable]),
      builder: (context, _) {
        final canContinue =
            phoneListenable.text.trim().isNotEmpty &&
            emailListenable.text.trim().isNotEmpty;
        return SizedBox(
          height: 52,
          child: FilledButton(
            onPressed: canContinue && !isLoading ? onTap : null,
            style: FilledButton.styleFrom(
              backgroundColor: const Color(0xFF4CAF50),
              disabledBackgroundColor:
                  const Color(0xFF4CAF50).withValues(alpha: 0.45),
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(999),
              ),
            ),
            child: isLoading
                ? const SizedBox(
                    width: 20,
                    height: 20,
                    child: CircularProgressIndicator(
                      color: Colors.white,
                      strokeWidth: 2,
                    ),
                  )
                : const Text(
                    'Continue',
                    style: TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                      color: Colors.white,
                    ),
                  ),
          ),
        );
      },
    );
  }
}

class _NigeriaFlag extends StatelessWidget {
  const _NigeriaFlag();

  @override
  Widget build(BuildContext context) {
    return ClipRRect(
      borderRadius: BorderRadius.circular(2),
      child: const Row(
        children: [
          _Stripe(color: Color(0xFF008751)),
          _Stripe(color: Colors.white),
          _Stripe(color: Color(0xFF008751)),
        ],
      ),
    );
  }
}

class _Stripe extends StatelessWidget {
  const _Stripe({required this.color});
  final Color color;

  @override
  Widget build(BuildContext context) =>
      Container(width: 8, height: 18, color: color);
}

class _DividerLabel extends StatelessWidget {
  const _DividerLabel({required this.label});
  final String label;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        const Expanded(child: Divider(color: Color(0xFFDDDDDD))),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 12),
          child: Text(
            label,
            style: const TextStyle(
              color: Color(0xFF888888),
              fontWeight: FontWeight.w500,
              fontSize: 13,
            ),
          ),
        ),
        const Expanded(child: Divider(color: Color(0xFFDDDDDD))),
      ],
    );
  }
}
