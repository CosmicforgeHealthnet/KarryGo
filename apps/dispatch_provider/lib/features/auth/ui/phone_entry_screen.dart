import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';

import '../../../core/theme/theme_extensions.dart';
import '../state/dispatch_auth_controller.dart';

class PhoneEntryScreen extends StatefulWidget {
  const PhoneEntryScreen({
    super.key,
    required this.onContinue,
    required this.controller,
    this.onLoginTap,
  });

  final void Function(String phone, String email) onContinue;
  final DispatchAuthController controller;
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
    final c = context.palette;
    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        child: Column(
          children: [
            Expanded(
              child: LayoutBuilder(
                builder: (context, constraints) {
                  return SingleChildScrollView(
                    keyboardDismissBehavior:
                        ScrollViewKeyboardDismissBehavior.onDrag,
                    padding: const EdgeInsets.symmetric(horizontal: 24),
                    child: ConstrainedBox(
                      constraints: BoxConstraints(
                        minHeight: constraints.maxHeight,
                      ),
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
            Padding(
              padding: const EdgeInsets.only(
                left: 24,
                right: 24,
                top: 16,
                bottom: 16,
              ),
              child: Text.rich(
                TextSpan(
                  text: 'By continuing, you agree to our\n',
                  style: TextStyle(
                    color: c.textMuted,
                    fontSize: 13,
                    height: 1.5,
                  ),
                  children: [
                    TextSpan(
                      text: 'Terms of Service',
                      style: TextStyle(
                        color: c.text,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                    const TextSpan(text: ' and '),
                    TextSpan(
                      text: 'Privacy Policy',
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
          ],
        ),
      ),
    );
  }

  Widget _buildContent() {
    final c = context.palette;

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
        const SizedBox(height: 32),
        Text(
          "Let's get you started",
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
          'Sign in or create a provider account with your phone number.',
          style: TextStyle(
            fontSize: 16,
            fontWeight: FontWeight.w400,
            color: c.textMuted,
            height: 1.4,
          ),
        ),
        const SizedBox(height: 20),
        const _FieldLabel('Phone number'),
        const SizedBox(height: 10),
        ValueListenableBuilder<TextEditingValue>(
          valueListenable: _phoneController,
          builder: (context, value, _) {
            return _FieldShell(
              trailing: value.text.isEmpty
                  ? null
                  : _ClearButton(controller: _phoneController),
              child: Row(
                children: [
                  const _NigeriaFlag(),
                  const SizedBox(width: 10),
                  Text(
                    '+234',
                    style: TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w600,
                      color: c.text,
                    ),
                  ),
                  const SizedBox(width: 12),
                  Container(
                    width: 1,
                    height: 24,
                    color: c.border,
                  ),
                  const SizedBox(width: 12),
                  Expanded(
                    child: TextField(
                      controller: _phoneController,
                      keyboardType: TextInputType.phone,
                      textInputAction: TextInputAction.next,
                      style: TextStyle(
                        fontSize: 16,
                        fontWeight: FontWeight.w500,
                        color: c.text,
                      ),
                      inputFormatters: [_PhoneFormatter()],
                      decoration: InputDecoration(
                        hintText: '812 345 6789',
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
                  ),
                ],
              ),
            );
          },
        ),
        const SizedBox(height: 12),
        const _FieldLabel('Email address'),
        const SizedBox(height: 10),
        ValueListenableBuilder<TextEditingValue>(
          valueListenable: _emailController,
          builder: (context, value, _) {
            return _FieldShell(
              leadingBuilder: (focused) => Icon(
                Icons.mail_outline_rounded,
                size: 22,
                color: focused ? c.brandTeal500 : c.textMuted,
              ),
              trailing: value.text.isEmpty
                  ? null
                  : _ClearButton(controller: _emailController),
              child: TextField(
                controller: _emailController,
                keyboardType: TextInputType.emailAddress,
                textInputAction: TextInputAction.done,
                autocorrect: false,
                onSubmitted: (_) => _signup(),
                inputFormatters: [
                  FilteringTextInputFormatter.deny(RegExp(r'\s')),
                ],
                style: TextStyle(
                  fontSize: 16,
                  fontWeight: FontWeight.w500,
                  color: c.text,
                ),
                decoration: InputDecoration(
                  hintText: 'you@example.com',
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
        const SizedBox(height: 10),
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
                "We'll send your verification code to your phone.",
                style: TextStyle(
                  fontSize: 13,
                  fontWeight: FontWeight.w500,
                  color: c.textMuted,
                ),
              ),
            ),
          ],
        ),
        const SizedBox(height: 18),
        _ContinueButton(
          phoneListenable: _phoneController,
          emailListenable: _emailController,
          isLoading: _isLoading,
          onTap: _signup,
        ),
        const SizedBox(height: 4),
        GestureDetector(
          behavior: HitTestBehavior.opaque,
          onTap: widget.onLoginTap,
          child: Padding(
            padding: const EdgeInsets.symmetric(vertical: 10),
            child: Text.rich(
              TextSpan(
                text: 'Already have an account? ',
                style: TextStyle(
                  color: c.textMuted,
                  fontSize: 14,
                  fontWeight: FontWeight.w500,
                ),
                children: [
                  TextSpan(
                    text: 'Log In',
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
        const Spacer(),
      ],
    );
  }

  void _signup() async {
    if (_isLoading) return;

    final rawPhone = _phoneController.text.replaceAll(' ', '').trim();
    final email = _emailController.text.trim();

    if (rawPhone.isEmpty) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Please enter your phone number.'),
          backgroundColor: Colors.red,
        ),
      );
      return;
    }
    if (email.isEmpty) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Please enter your email address.'),
          backgroundColor: Colors.red,
        ),
      );
      return;
    }

    final emailRegex = RegExp(r'^[\w.+\-]+@[\w\-]+\.[\w.\-]+$');
    if (!emailRegex.hasMatch(email)) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Please enter a valid email address.'),
          backgroundColor: Colors.red,
        ),
      );
      return;
    }

    FocusScope.of(context).unfocus();

    String subscriber = rawPhone;
    if (subscriber.startsWith('+234')) {
      subscriber = subscriber.substring(4);
    } else if (subscriber.startsWith('234')) {
      subscriber = subscriber.substring(3);
    }
    while (subscriber.startsWith('0')) {
      subscriber = subscriber.substring(1);
    }

    if (subscriber.length != 10 || !RegExp(r'^\d{10}$').hasMatch(subscriber)) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Please enter a valid Nigerian phone number.'),
          backgroundColor: Colors.red,
        ),
      );
      return;
    }

    final phone = '+234$subscriber';
    debugPrint('[SIGNUP] Normalized phone: $phone');

    final messenger = ScaffoldMessenger.of(context);
    setState(() => _isLoading = true);

    final result = await widget.controller.signupStart(
      phoneNumber: phone,
      email: email,
    );

    if (!mounted) return;
    setState(() => _isLoading = false);

    result.when(
      success: (_) => widget.onContinue(phone, email),
      failure: (error) {
        messenger.showSnackBar(
          SnackBar(
            content: Text(_signupErrorMessage(error)),
            backgroundColor: Colors.red.shade800,
            duration: const Duration(seconds: 5),
          ),
        );
      },
    );
  }

  static String _signupErrorMessage(ApiException error) {
    switch (error.code) {
      case ApiErrorCode.conflict:
        return 'An account already exists with this phone number or email. Please log in instead.';
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

class _FieldLabel extends StatelessWidget {
  const _FieldLabel(this.text);
  final String text;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Text(
      text,
      style: TextStyle(
        fontSize: 14,
        fontWeight: FontWeight.w600,
        color: c.text,
      ),
    );
  }
}

class _ClearButton extends StatelessWidget {
  const _ClearButton({required this.controller});
  final TextEditingController controller;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return IconButton(
      onPressed: controller.clear,
      icon: const Icon(Icons.close_rounded),
      iconSize: 18,
      color: c.textDisabled,
      padding: EdgeInsets.zero,
      constraints: const BoxConstraints.tightFor(width: 44, height: 44),
    );
  }
}

class _FieldShell extends StatefulWidget {
  const _FieldShell({
    required this.child,
    this.leadingBuilder,
    this.trailing,
  });

  final Widget child;
  final Widget Function(bool focused)? leadingBuilder;
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
            if (widget.leadingBuilder != null) ...[
              widget.leadingBuilder!(_focused),
              const SizedBox(width: 12),
            ],
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

class _PhoneFormatter extends TextInputFormatter {
  @override
  TextEditingValue formatEditUpdate(
    TextEditingValue oldValue,
    TextEditingValue newValue,
  ) {
    final allDigits = newValue.text.replaceAll(RegExp(r'\D'), '');
    final digits =
        allDigits.length > 13 ? allDigits.substring(0, 13) : allDigits;
    final first = digits.startsWith('0') ? 4 : 3;

    final buffer = StringBuffer();
    for (var i = 0; i < digits.length; i++) {
      if (i == first || i == first + 3) buffer.write(' ');
      buffer.write(digits[i]);
    }
    final text = buffer.toString();

    final end = newValue.selection.end < 0
        ? newValue.text.length
        : (newValue.selection.end > newValue.text.length
              ? newValue.text.length
              : newValue.selection.end);
    var digitsBeforeCursor =
        newValue.text.substring(0, end).replaceAll(RegExp(r'\D'), '').length;
    if (digitsBeforeCursor > digits.length) digitsBeforeCursor = digits.length;

    var offset = 0;
    var seen = 0;
    while (offset < text.length && seen < digitsBeforeCursor) {
      if (text[offset] != ' ') seen++;
      offset++;
    }
    return TextEditingValue(
      text: text,
      selection: TextSelection.collapsed(offset: offset),
    );
  }
}

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
    final c = context.palette;
    final buttonGlow = c.brandTeal500.withValues(alpha: 0.20);

    return ListenableBuilder(
      listenable: Listenable.merge([phoneListenable, emailListenable]),
      builder: (context, _) {
        final canContinue = phoneListenable.text.trim().isNotEmpty &&
            emailListenable.text.trim().isNotEmpty;
        final enabled = canContinue && !isLoading;
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
            onPressed: enabled ? onTap : null,
            style: FilledButton.styleFrom(
              backgroundColor: c.brandTeal500,
              disabledBackgroundColor:
                  c.brandTeal500.withValues(alpha: 0.45),
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(12),
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
    );
  }
}

// ── Nigeria flag (national colors — theme-independent) ────────────────

class _NigeriaFlag extends StatelessWidget {
  const _NigeriaFlag();

  @override
  Widget build(BuildContext context) {
    return ClipRRect(
      borderRadius: BorderRadius.circular(3),
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
      Container(width: 7, height: 16, color: color);
}