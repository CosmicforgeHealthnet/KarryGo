import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../../core/theme/theme_extensions.dart';
import '../state/dispatch_auth_controller.dart';

class OtpVerificationScreen extends StatefulWidget {
  const OtpVerificationScreen({
    super.key,
    required this.phone,
    required this.onVerify,
    required this.onBack,
    required this.onResend,
    required this.controller,
  });

  final String phone;
  final ValueChanged<String> onVerify;
  final VoidCallback onBack;
  final VoidCallback onResend;
  final DispatchAuthController controller;

  @override
  State<OtpVerificationScreen> createState() => _OtpVerificationScreenState();
}

class _OtpVerificationScreenState extends State<OtpVerificationScreen> {
  final _otpController = TextEditingController();
  final _focusNode = FocusNode();

  int _secondsLeft = 120;
  bool _canResend = false;
  bool _isLoading = false;
  bool _hasVerified = false;

  @override
  void initState() {
    super.initState();
    _startTimer();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) _focusNode.requestFocus();
    });
  }

  void _requestFocus() {
    _focusNode.unfocus();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) _focusNode.requestFocus();
    });
  }

  void _startTimer() {
    Future.doWhile(() async {
      await Future.delayed(const Duration(seconds: 1));
      if (!mounted) return false;
      setState(() {
        if (_secondsLeft > 0) {
          _secondsLeft--;
        } else {
          _canResend = true;
        }
      });
      return _secondsLeft > 0;
    });
  }

  String get _timerText {
    final m = (_secondsLeft ~/ 60).toString().padLeft(2, '0');
    final s = (_secondsLeft % 60).toString().padLeft(2, '0');
    return '$m:$s';
  }

  @override
  void dispose() {
    _otpController.dispose();
    _focusNode.dispose();
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
              child: SingleChildScrollView(
                padding: const EdgeInsets.symmetric(horizontal: 24),
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
            Padding(
              padding: const EdgeInsets.fromLTRB(24, 8, 24, 0),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  _buildVerifyButton(),
                  const SizedBox(height: 20),
                ],
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
        const SizedBox(height: 8),
        Row(
          children: [
            GestureDetector(
              onTap: widget.onBack,
              behavior: HitTestBehavior.opaque,
              child: Padding(
                padding: const EdgeInsets.symmetric(vertical: 8, horizontal: 4),
                child: Icon(
                  Icons.arrow_back,
                  size: 24,
                  color: c.text,
                ),
              ),
            ),
            const Spacer(),
            GestureDetector(
              onTap: () {},
              behavior: HitTestBehavior.opaque,
              child: Container(
                padding: const EdgeInsets.symmetric(
                  horizontal: 12,
                  vertical: 6,
                ),
                decoration: BoxDecoration(
                  color: c.teal10,
                  borderRadius: BorderRadius.circular(999),
                ),
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Container(
                      width: 18,
                      height: 18,
                      decoration: BoxDecoration(
                        shape: BoxShape.circle,
                        border: Border.all(
                          color: c.brandTeal500,
                          width: 1.5,
                        ),
                      ),
                      child: Center(
                        child: Text(
                          '?',
                          style: TextStyle(
                            fontSize: 11,
                            fontWeight: FontWeight.w700,
                            color: c.brandTeal500,
                            height: 1.0,
                          ),
                        ),
                      ),
                    ),
                    const SizedBox(width: 6),
                    Text(
                      'Help',
                      style: TextStyle(
                        fontSize: 15,
                        fontWeight: FontWeight.w600,
                        color: c.brandTeal500,
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ],
        ),
        const SizedBox(height: 16),
        Text(
          'Verify your number',
          style: TextStyle(
            fontSize: 30,
            fontWeight: FontWeight.w700,
            color: c.text,
            letterSpacing: -0.6,
            height: 1.15,
          ),
        ),
        const SizedBox(height: 8),
        Text.rich(
          TextSpan(
            text: "We've sent a 6-digit code to\n",
            style: TextStyle(
              fontSize: 16,
              fontWeight: FontWeight.w400,
              color: c.textMuted,
              height: 1.5,
            ),
            children: [
              TextSpan(
                text: widget.phone,
                style: TextStyle(
                  fontWeight: FontWeight.w700,
                  color: c.text,
                ),
              ),
            ],
          ),
        ),
        const SizedBox(height: 28),

        if (kDebugMode && widget.controller.debugOtp != null) ...[
          Container(
            width: double.infinity,
            padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
            decoration: BoxDecoration(
              color: c.teal10,
              borderRadius: BorderRadius.circular(12),
              border: Border.all(
                color: c.brandTeal500,
                width: 1.5,
              ),
            ),
            child: Column(
              children: [
                Text(
                  'DEBUG — Your OTP code',
                  style: TextStyle(
                    color: c.textMuted,
                    fontSize: 12,
                    fontWeight: FontWeight.w600,
                    letterSpacing: 0.5,
                  ),
                ),
                const SizedBox(height: 4),
                Text(
                  widget.controller.debugOtp!,
                  style: TextStyle(
                    color: c.text,
                    fontSize: 36,
                    fontWeight: FontWeight.w900,
                    letterSpacing: 8,
                  ),
                ),
              ],
            ),
          ),
          const SizedBox(height: 24),
        ],

        GestureDetector(
          onTap: _requestFocus,
          behavior: HitTestBehavior.opaque,
          child: ListenableBuilder(
            listenable: Listenable.merge([_otpController, _focusNode]),
            builder: (context, _) => _OtpSlots(
              value: _otpController.text,
              focused: _focusNode.hasFocus,
            ),
          ),
        ),

        SizedBox(
          height: 1,
          child: Opacity(
            opacity: 0.01,
            child: TextField(
              focusNode: _focusNode,
              controller: _otpController,
              keyboardType: TextInputType.number,
              maxLength: 6,
              autofillHints: const [AutofillHints.oneTimeCode],
              inputFormatters: [FilteringTextInputFormatter.digitsOnly],
              decoration: const InputDecoration(counterText: ''),
              onChanged: (_) => setState(() {}),
            ),
          ),
        ),

        const SizedBox(height: 16),
        Center(
          child: GestureDetector(
            onTap: _canResend && !_isLoading ? _resend : null,
            behavior: HitTestBehavior.opaque,
            child: Padding(
              padding: const EdgeInsets.symmetric(vertical: 8),
              child: Text.rich(
                TextSpan(
                  text: "Didn't receive the code? ",
                  style: TextStyle(
                    fontSize: 15,
                    color: c.textMuted,
                    height: 1.5,
                  ),
                  children: [
                    TextSpan(
                      text: _canResend ? 'Resend' : 'Resend in $_timerText',
                      style: TextStyle(
                        color: _canResend ? c.brandTeal500 : c.text,
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
        const SizedBox(height: 16),
      ],
    );
  }

  Widget _buildVerifyButton() {
    final c = context.palette;
    final buttonGlow = c.brandTeal500.withValues(alpha: 0.20);

    return ValueListenableBuilder<TextEditingValue>(
      valueListenable: _otpController,
      builder: (context, value, _) {
        final canContinue = value.text.length == 6;
        final enabled = canContinue && !_isLoading && !_hasVerified;
        return AnimatedContainer(
          duration: const Duration(milliseconds: 220),
          height: 58,
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
            onPressed: enabled ? _verify : null,
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
                : const Text(
                    'Verify and continue',
                    style: TextStyle(
                      fontSize: 17,
                      fontWeight: FontWeight.w700,
                      color: Colors.white,
                    ),
                  ),
          ),
        );
      },
    );
  }

  void _resend() async {
    final messenger = ScaffoldMessenger.of(context);
    setState(() => _isLoading = true);

    try {
      final result = await widget.controller.resendOtp();
      if (!mounted) return;
      setState(() => _isLoading = false);

      result.when(
        success: (_) {
          widget.onResend();
          setState(() {
            _secondsLeft = 120;
            _canResend = false;
          });
          _startTimer();
          messenger.showSnackBar(
            const SnackBar(
              content: Text('Verification code resent successfully.'),
            ),
          );
        },
        failure: (error) {
          messenger.showSnackBar(
            SnackBar(
              content: Text(error.message),
              backgroundColor: Colors.red.shade800,
            ),
          );
        },
      );
    } catch (e) {
      if (!mounted) return;
      setState(() => _isLoading = false);
      messenger.showSnackBar(
        SnackBar(
          content: Text('Failed to resend code: $e'),
          backgroundColor: Colors.red.shade800,
        ),
      );
    }
  }

  void _verify() async {
    if (_isLoading || _hasVerified) {
      debugPrint('[AUTH_UI] verify ignored — already submitting or verified');
      return;
    }
    final otp = _otpController.text;
    if (otp.length != 6) return;

    debugPrint('[AUTH_UI] verify started');

    final messenger = ScaffoldMessenger.of(context);
    setState(() => _isLoading = true);

    try {
      final result = await widget.controller.verifyOtp(otp);

      if (!mounted) return;

      result.when(
        success: (data) {
          setState(() => _hasVerified = true);
          debugPrint('[AUTH_UI] verify success');
          widget.onVerify(otp);
        },
        failure: (error) {
          setState(() => _isLoading = false);
          debugPrint('[AUTH_UI] verify failed: ${error.message}');
          messenger.showSnackBar(
            SnackBar(
              content: Text(error.message),
              backgroundColor: Colors.red.shade800,
            ),
          );
        },
      );
    } catch (e) {
      if (!mounted) return;
      setState(() => _isLoading = false);
      debugPrint('[AUTH_UI] verify exception: $e');
      messenger.showSnackBar(
        SnackBar(
          content: Text('An unexpected error occurred: $e'),
          backgroundColor: Colors.red.shade800,
        ),
      );
    }
  }
}

class _OtpSlots extends StatelessWidget {
  const _OtpSlots({required this.value, required this.focused});

  final String value;
  final bool focused;

  @override
  Widget build(BuildContext context) {
    final chars = value.split('').take(6).toList();
    return Row(
      children: [
        for (int i = 0; i < 6; i++) ...[
          Expanded(
            child: _OtpSlot(
              char: i < chars.length ? chars[i] : null,
              isActive: focused && i == chars.length,
            ),
          ),
          if (i < 5) const SizedBox(width: 10),
        ],
      ],
    );
  }
}

class _OtpSlot extends StatelessWidget {
  const _OtpSlot({required this.char, required this.isActive});

  final String? char;
  final bool isActive;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final focusRing = c.brandTeal500.withValues(alpha: 0.15);

    final hasValue = char != null;
    final Widget content;
    if (hasValue) {
      content = Text(
        char!,
        key: ValueKey(char),
        style: TextStyle(
          fontSize: 24,
          fontWeight: FontWeight.w700,
          color: c.text,
        ),
      );
    } else if (isActive) {
      content = const _BlinkingCaret(key: ValueKey('caret'));
    } else {
      content = const SizedBox.shrink(key: ValueKey('empty'));
    }

    return AnimatedContainer(
      duration: const Duration(milliseconds: 160),
      curve: Curves.easeOut,
      height: 58,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: c.surface,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(
          color: hasValue || isActive ? c.brandTeal500 : c.border,
          width: hasValue || isActive ? 1.5 : 1,
        ),
        boxShadow: isActive
            ? [
                BoxShadow(
                  color: focusRing,
                  blurRadius: 0,
                  spreadRadius: 4,
                ),
              ]
            : const [],
      ),
      child: AnimatedSwitcher(
        duration: const Duration(milliseconds: 160),
        transitionBuilder: (child, animation) => ScaleTransition(
          scale: animation,
          child: FadeTransition(opacity: animation, child: child),
        ),
        child: content,
      ),
    );
  }
}

class _BlinkingCaret extends StatefulWidget {
  const _BlinkingCaret({super.key});

  @override
  State<_BlinkingCaret> createState() => _BlinkingCaretState();
}

class _BlinkingCaretState extends State<_BlinkingCaret>
    with SingleTickerProviderStateMixin {
  late final AnimationController _controller = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 600),
  )..repeat(reverse: true);

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return FadeTransition(
      opacity: _controller,
      child: Container(
        width: 2,
        height: 24,
        decoration: BoxDecoration(
          color: c.brandTeal500,
          borderRadius: BorderRadius.circular(1),
        ),
      ),
    );
  }
}