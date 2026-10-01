import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';

import '../../../core/theme/theme_extensions.dart';
import '../state/dispatch_auth_controller.dart';

class LoginScreen extends StatefulWidget {
  const LoginScreen({
    super.key,
    required this.controller,
    required this.onContinue,
    required this.onCreateAccountTap,
  });

  final DispatchAuthController controller;
  final ValueChanged<String> onContinue;
  final VoidCallback onCreateAccountTap;

  @override
  State<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends State<LoginScreen> {
  final _identifierController = TextEditingController(
    text: kDebugMode ? '+2348077770001' : '',
  );
  bool _isLoading = false;

  @override
  void dispose() {
    _identifierController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Scaffold(
      backgroundColor: c.background,
      resizeToAvoidBottomInset: true,
      body: SafeArea(
        child: LayoutBuilder(
          builder: (context, constraints) {
            return SingleChildScrollView(
              keyboardDismissBehavior: ScrollViewKeyboardDismissBehavior.onDrag,
              padding: const EdgeInsets.symmetric(horizontal: 24),
              child: ConstrainedBox(
                constraints: BoxConstraints(minHeight: constraints.maxHeight),
                child: IntrinsicHeight(
                  child: TweenAnimationBuilder<double>(
                    tween: Tween(begin: 0, end: 1),
                    duration: const Duration(milliseconds: 550),
                    curve: Curves.easeOutCubic,
                    builder: (context, t, child) {
                      return Opacity(
                        opacity: t,
                        child: Transform.translate(
                          offset: Offset(0, (1 - t) * 18),
                          child: child,
                        ),
                      );
                    },
                    child: _buildContent(),
                  ),
                ),
              ),
            );
          },
        ),
      ),
    );
  }

  Widget _buildContent() {
    final c = context.palette;
    final focusRing = c.brandTeal500.withValues(alpha: 0.15);
    final buttonGlow = c.brandTeal500.withValues(alpha: 0.20);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const SizedBox(height: 32),
        Center(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Image.asset(
                'assets/figma/splash_logo.png',
                width: 88,
              ),
              const SizedBox(height: 4),
              Text(
                'Karryam',
                style: TextStyle(
                  fontSize: 24,
                  fontWeight: FontWeight.w700,
                  color: c.text,
                  letterSpacing: -0.6,
                  height: 1.0,
                ),
              ),
            ],
          ),
        ),
        const SizedBox(height: 44),
        Text(
          'Welcome back!',
          style: TextStyle(
            fontSize: 30,
            fontWeight: FontWeight.w700,
            color: c.text,
            letterSpacing: -0.6,
            height: 1.15,
          ),
        ),
        const SizedBox(height: 8),
        Text(
          'Log in to continue.',
          style: TextStyle(
            fontSize: 16,
            fontWeight: FontWeight.w400,
            color: c.textMuted,
            height: 1.4,
          ),
        ),
        const SizedBox(height: 32),
        Text(
          'Phone number or email',
          style: TextStyle(
            fontSize: 14,
            fontWeight: FontWeight.w600,
            color: c.text,
          ),
        ),
        const SizedBox(height: 10),
        ValueListenableBuilder<TextEditingValue>(
          valueListenable: _identifierController,
          builder: (context, value, _) {
            final isEmail = value.text.contains('@');
            return _FieldShell(
              leadingBuilder: (focused) => AnimatedSwitcher(
                duration: const Duration(milliseconds: 180),
                child: Icon(
                  isEmail ? Icons.mail_outline_rounded : Icons.phone_outlined,
                  key: ValueKey(isEmail),
                  size: 22,
                  color: focused ? c.brandTeal500 : c.textMuted,
                ),
              ),
              trailing: value.text.isEmpty
                  ? null
                  : IconButton(
                      onPressed: _identifierController.clear,
                      icon: const Icon(Icons.close_rounded),
                      iconSize: 18,
                      color: c.textDisabled,
                      padding: EdgeInsets.zero,
                      constraints: const BoxConstraints.tightFor(
                        width: 44,
                        height: 44,
                      ),
                    ),
              child: TextField(
                controller: _identifierController,
                keyboardType: TextInputType.emailAddress,
                textInputAction: TextInputAction.done,
                autocorrect: false,
                onSubmitted: (_) => _login(),
                inputFormatters: [
                  FilteringTextInputFormatter.deny(RegExp(r'\s')),
                ],
                style: TextStyle(
                  fontSize: 16,
                  fontWeight: FontWeight.w500,
                  color: c.text,
                ),
                decoration: InputDecoration(
                  hintText: '+2348012345678',
                  hintStyle: TextStyle(
                    color: c.textDisabled,
                    fontSize: 16,
                    fontWeight: FontWeight.w400,
                  ),
                  border: InputBorder.none,
                  isDense: true,
                  contentPadding: const EdgeInsets.symmetric(vertical: 16),
                ),
              ),
            );
          },
        ),
        const SizedBox(height: 14),
        Row(
          children: [
            Icon(
              Icons.verified_user_outlined,
              size: 16,
              color: c.brandTeal500,
            ),
            const SizedBox(width: 8),
            Expanded(
              child: Text(
                "We'll send you a verification code.",
                style: TextStyle(
                  fontSize: 13,
                  fontWeight: FontWeight.w500,
                  color: c.textMuted,
                ),
              ),
            ),
          ],
        ),
        const SizedBox(height: 28),
        ValueListenableBuilder<TextEditingValue>(
          valueListenable: _identifierController,
          builder: (context, value, _) {
            final canContinue = value.text.trim().isNotEmpty;
            final enabled = canContinue && !_isLoading;
            return AnimatedContainer(
              duration: const Duration(milliseconds: 220),
              height: 56,
              decoration: BoxDecoration(
                borderRadius: BorderRadius.circular(12),
                boxShadow: enabled
                    ? [
                        BoxShadow(
                          color: buttonGlow,
                          blurRadius: 20,
                          offset: const Offset(0, 8),
                        ),
                      ]
                    : const [],
              ),
              child: FilledButton(
                onPressed: enabled ? _login : null,
                style: FilledButton.styleFrom(
                  backgroundColor: c.brandTeal500,
                  disabledBackgroundColor:
                      c.brandTeal500.withValues(alpha: 0.45),
                  shape: RoundedRectangleBorder(
                    borderRadius: BorderRadius.circular(12),
                  ),
                ),
                child: _isLoading
                    ? const SizedBox(
                        width: 22,
                        height: 22,
                        child: CircularProgressIndicator(
                          color: Colors.white,
                          strokeWidth: 2,
                        ),
                      )
                    : const Row(
                        mainAxisAlignment: MainAxisAlignment.center,
                        children: [
                          Text(
                            'Continue',
                            style: TextStyle(
                              fontSize: 16,
                              fontWeight: FontWeight.w700,
                              color: Colors.white,
                            ),
                          ),
                          SizedBox(width: 8),
                          Icon(
                            Icons.arrow_forward_rounded,
                            size: 20,
                            color: Colors.white,
                          ),
                        ],
                      ),
              ),
            );
          },
        ),
        const Spacer(),
        const SizedBox(height: 24),
        Material(
          color: c.teal10,
          borderRadius: BorderRadius.circular(12),
          child: InkWell(
            borderRadius: BorderRadius.circular(12),
            onTap: widget.onCreateAccountTap,
            child: Container(
              height: 56,
              alignment: Alignment.center,
              decoration: BoxDecoration(
                borderRadius: BorderRadius.circular(12),
                border: Border.all(color: c.teal20),
              ),
              child: Text.rich(
                TextSpan(
                  text: "Don't have an account? ",
                  style: TextStyle(
                    color: c.textMuted,
                    fontSize: 14,
                    fontWeight: FontWeight.w500,
                  ),
                  children: [
                    TextSpan(
                      text: 'Create account',
                      style: TextStyle(
                        color: c.text,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                  ],
                ),
                textAlign: TextAlign.center,
              ),
            ),
          ),
        ),
        const SizedBox(height: 20),
      ],
    );
  }

  void _login() async {
    if (_isLoading) return;
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

  static String _loginErrorMessage(ApiException error, String identifier) {
    switch (error.code) {
      case ApiErrorCode.notFound:
        if (identifier.contains('@')) {
          return 'No account found with this email address. Please check it or sign up.';
        }
        return 'No account found with this phone number. Please check it or sign up.';

      case ApiErrorCode.network:
        return 'Cannot connect to Karryam server. Check backend URL/network and try again.';

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

// ═══════════════════════════════════════════════════════════════════════════
// Field shell — theme-aware
// ═══════════════════════════════════════════════════════════════════════════

class _FieldShell extends StatefulWidget {
  const _FieldShell({
    required this.child,
    required this.leadingBuilder,
    this.trailing,
  });

  final Widget child;
  final Widget Function(bool focused) leadingBuilder;
  final Widget? trailing;

  @override
  State<_FieldShell> createState() => _FieldShellState();
}

class _FieldShellState extends State<_FieldShell> {
  bool _focused = false;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final focusRing = c.brandTeal500.withValues(alpha: 0.15);

    return Focus(
      canRequestFocus: false,
      skipTraversal: true,
      onFocusChange: (focused) => setState(() => _focused = focused),
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 180),
        curve: Curves.easeOut,
        height: 58,
        decoration: BoxDecoration(
          color: _focused ? c.surface : c.inputFill,
          borderRadius: BorderRadius.circular(12),
          border: Border.all(
            color: _focused ? c.brandTeal500 : c.border,
            width: _focused ? 1.6 : 1,
          ),
          boxShadow: _focused
              ? [
                  BoxShadow(
                    color: focusRing,
                    blurRadius: 0,
                    spreadRadius: 4,
                  ),
                ]
              : const [],
        ),
        child: Row(
          children: [
            const SizedBox(width: 16),
            widget.leadingBuilder(_focused),
            const SizedBox(width: 12),
            Expanded(
              child: Theme(
                data: Theme.of(context).copyWith(
                  inputDecorationTheme: const InputDecorationTheme(
                    border: InputBorder.none,
                    enabledBorder: InputBorder.none,
                    focusedBorder: InputBorder.none,
                    disabledBorder: InputBorder.none,
                    errorBorder: InputBorder.none,
                    focusedErrorBorder: InputBorder.none,
                    filled: false,
                  ),
                ),
                child: widget.child,
              ),
            ),
            widget.trailing ?? const SizedBox(width: 16),
          ],
        ),
      ),
    );
  }
}