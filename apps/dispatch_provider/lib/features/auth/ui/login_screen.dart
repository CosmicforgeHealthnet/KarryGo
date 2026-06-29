import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../state/dispatch_auth_controller.dart';

/// Login screen — the first screen shown to unauthenticated users.
/// Accepts either a phone number (E.164 or local format) or an email address.
class LoginScreen extends StatefulWidget {
  const LoginScreen({
    super.key,
    required this.controller,
    required this.onContinue,
    required this.onCreateAccountTap,
  });

  final DispatchAuthController controller;

  /// Called with the identifier (phone or email) after loginStart succeeds.
  final ValueChanged<String> onContinue;

  /// Called when the user taps "Create account".
  final VoidCallback onCreateAccountTap;

  @override
  State<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends State<LoginScreen> {
  final _identifierController = TextEditingController();
  bool _isLoading = false;

  @override
  void dispose() {
    _identifierController.dispose();
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
              const SizedBox(height: 149),
              // Car illustration — same asset as signup screen.
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
                'Welcome back!',
                style: TextStyle(
                  fontSize: 22,
                  fontWeight: FontWeight.w800,
                  color: Color(0xFF1A1A1A),
                ),
              ),
              const SizedBox(height: 4),
              const Text(
                'Log in to continue.',
                style: TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w700,
                  color: Color(0xFF4CAF50),
                ),
              ),
              const SizedBox(height: 8),
              const Text(
                'Enter your phone number or email address.',
                style: TextStyle(fontSize: 13, color: Color(0xFF888888)),
              ),
              const SizedBox(height: 28),
              const Text(
                'Phone number or email',
                style: TextStyle(
                  fontSize: 13,
                  fontWeight: FontWeight.w700,
                  color: Color(0xFF1A1A1A),
                ),
              ),
              const SizedBox(height: 8),
              TextField(
                controller: _identifierController,
                keyboardType: TextInputType.emailAddress,
                autocorrect: false,
                style: const TextStyle(fontSize: 14, color: Color(0xFF1A1A1A)),
                inputFormatters: [
                  // Allow phone chars and email chars; no spaces in identifier.
                  FilteringTextInputFormatter.deny(RegExp(r'\s')),
                ],
                decoration: InputDecoration(
                  hintText: 'e.g. +2348012345678 or you@email.com',
                  hintStyle: const TextStyle(
                    color: Color(0xFFBBBBBB),
                    fontSize: 13,
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
                "We'll send you a verification code.",
                style: TextStyle(
                  fontSize: 12,
                  color: Color(0xFF4CAF50),
                  fontWeight: FontWeight.w500,
                ),
              ),
              const SizedBox(height: 32),
              ValueListenableBuilder<TextEditingValue>(
                valueListenable: _identifierController,
                builder: (context, value, _) {
                  final canContinue = value.text.trim().isNotEmpty;
                  return SizedBox(
                    height: 52,
                    child: FilledButton(
                      onPressed: canContinue && !_isLoading ? _login : null,
                      style: FilledButton.styleFrom(
                        backgroundColor: const Color(0xFF4CAF50),
                        disabledBackgroundColor: const Color(
                          0xFF4CAF50,
                        ).withValues(alpha: 0.45),
                        shape: RoundedRectangleBorder(
                          borderRadius: BorderRadius.circular(999),
                        ),
                      ),
                      child: _isLoading
                          ? const SizedBox(
                              width: 20,
                              height: 20,
                              child: CircularProgressIndicator(
                                color: Colors.white,
                                strokeWidth: 2,
                              ),
                            )
                          : const Text(
                              'Log In',
                              style: TextStyle(
                                fontSize: 16,
                                fontWeight: FontWeight.w700,
                                color: Colors.white,
                              ),
                            ),
                    ),
                  );
                },
              ),
              const SizedBox(height: 28),
              GestureDetector(
                onTap: widget.onCreateAccountTap,
                child: Text.rich(
                  TextSpan(
                    text: "Don't have an account? ",
                    style: const TextStyle(
                      color: Color(0xFF888888),
                      fontSize: 13,
                    ),
                    children: const [
                      TextSpan(
                        text: 'Create account',
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

  void _login() async {
    if (_isLoading) return; // guard against double-tap
    final rawIdentifier = _identifierController.text.trim();
    if (rawIdentifier.isEmpty) return;

    FocusScope.of(context).unfocus();

    final messenger = ScaffoldMessenger.of(context);
    final identifier = _normalizeLoginIdentifier(rawIdentifier);
    if (identifier == null) {
      messenger.showSnackBar(
        SnackBar(
          content: const Text(
            'Please enter a valid Nigerian phone number or email address.',
          ),
          backgroundColor: Colors.red.shade800,
          duration: const Duration(seconds: 5),
        ),
      );
      return;
    }

    setState(() => _isLoading = true);

    // loginStart always returns ApiResult — it never throws.
    // The outer try/catch is a last-resort safety net for unexpected errors;
    // it should NOT fire for normal backend error responses (404, 409, etc.).
    final result = await widget.controller.loginStart(identifier: identifier);

    if (!mounted) return;
    setState(() => _isLoading = false);

    result.when(
      success: (_) {
        widget.onContinue(identifier);
      },
      failure: (error) {
        messenger.showSnackBar(
          SnackBar(
            content: Text(_loginErrorMessage(error, rawIdentifier)),
            backgroundColor: Colors.red.shade800,
            duration: const Duration(seconds: 5),
          ),
        );
      },
    );
  }

  static String? _normalizeLoginIdentifier(String rawIdentifier) {
    final value = rawIdentifier.trim();
    if (value.isEmpty) return null;

    if (value.contains('@')) {
      final email = value.toLowerCase();
      final emailRegex = RegExp(r'^[\w.+\-]+@[\w\-]+\.[\w.\-]+$');
      return emailRegex.hasMatch(email) ? email : null;
    }

    final compact = value.replaceAll(RegExp(r'[\s\-()]'), '');
    if (compact.startsWith('+') && !compact.startsWith('+234')) {
      return RegExp(r'^\+\d{8,15}$').hasMatch(compact) ? compact : null;
    }

    String subscriber = compact;
    if (subscriber.startsWith('+234')) {
      subscriber = subscriber.substring(4);
    } else if (subscriber.startsWith('234')) {
      subscriber = subscriber.substring(3);
    }
    while (subscriber.startsWith('0')) {
      subscriber = subscriber.substring(1);
    }

    if (subscriber.length != 10 || !RegExp(r'^\d{10}$').hasMatch(subscriber)) {
      return null;
    }
    return '+234$subscriber';
  }

  /// Returns a user-friendly error message based on the error code and the
  /// type of identifier the user entered (email vs phone).
  static String _loginErrorMessage(ApiException error, String identifier) {
    switch (error.code) {
      case ApiErrorCode.notFound:
        if (identifier.contains('@')) {
          return 'No account found with this email address. Please check it or sign up.';
        }
        return 'No account found with this phone number. Please check it or sign up.';

      case ApiErrorCode.network:
        return 'Cannot connect to Cosmicforge Logistics server. Check backend URL/network and try again.';

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
