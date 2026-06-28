import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../home/ui/widgets/provider_app_colors.dart';
import '../state/provider_auth_controller.dart';

class ProviderOtpScreen extends StatefulWidget {
  const ProviderOtpScreen({super.key, required this.controller});
  final ProviderAuthController controller;

  @override
  State<ProviderOtpScreen> createState() => _ProviderOtpScreenState();
}

class _ProviderOtpScreenState extends State<ProviderOtpScreen> {
  final _controllers = List.generate(6, (_) => TextEditingController());
  final _focusNodes = List.generate(6, (_) => FocusNode());
  String get _code => _controllers.map((c) => c.text).join();
  bool get _complete => _code.length == 6;

  static const int _resendCooldownSeconds = 30;

  late int _secondsLeft;
  Timer? _timer;

  @override
  void initState() {
    super.initState();
    _secondsLeft = _resendCooldownSeconds;
    _startTimer();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) _focusNodes[0].requestFocus();
    });
  }

  void _startTimer() {
    _timer?.cancel();
    _timer = Timer.periodic(const Duration(seconds: 1), (t) {
      if (!mounted) {
        t.cancel();
        return;
      }
      setState(() {
        if (_secondsLeft > 0) {
          _secondsLeft--;
        } else {
          t.cancel();
        }
      });
    });
  }

  @override
  void dispose() {
    _timer?.cancel();
    for (final c in _controllers) {
      c.dispose();
    }
    for (final f in _focusNodes) {
      f.dispose();
    }
    super.dispose();
  }

  void _onDigitEntered(int index, String value) {
    final digit = value.replaceAll(RegExp(r'\D'), '');
    if (digit.isEmpty) {
      _controllers[index].clear();
      if (index > 0) {
        _focusNodes[index - 1].requestFocus();
        _controllers[index - 1].clear();
      }
      setState(() {});
      return;
    }
    _controllers[index].text = digit[digit.length - 1];
    _controllers[index].selection =
        TextSelection.fromPosition(const TextPosition(offset: 1));
    setState(() {});
    if (index < 5) {
      _focusNodes[index + 1].requestFocus();
    } else {
      _focusNodes[index].unfocus();
    }
  }

  void _submit() {
    if (!_complete) return;
    widget.controller.verifyOtp(_code);
  }

  void _resend() {
    final s = widget.controller.state;
    widget.controller.startAuth(phone: s.phone, email: s.email);
    setState(() => _secondsLeft = _resendCooldownSeconds);
    _startTimer();
  }

  String get _timerLabel {
    final m = (_secondsLeft ~/ 60).toString().padLeft(2, '0');
    final s = (_secondsLeft % 60).toString().padLeft(2, '0');
    return '$m:$s';
  }

  @override
  Widget build(BuildContext context) {
    final state = widget.controller.state;
    return Scaffold(
      backgroundColor: kProviderSurface,
      appBar: AppBar(
        backgroundColor: kProviderSurface,
        elevation: 0,
        leading: IconButton(
          icon: const Icon(Icons.arrow_back_ios_new_rounded, color: kProviderText, size: 20),
          onPressed: widget.controller.backToPhoneEntry,
        ),
        title: const Text(
          'OTP Confirmation!',
          style: TextStyle(color: kProviderText, fontWeight: FontWeight.w800, fontSize: 18),
        ),
        centerTitle: false,
      ),
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              const SizedBox(height: 8),

              RichText(
                text: TextSpan(
                  style: const TextStyle(color: kProviderMuted, fontSize: 14, height: 1.5),
                  children: [
                    TextSpan(
                      text: state.identifierType == 'email'
                          ? 'We sent you a 6-digit code to '
                          : 'We sent you a 6-digit code via your number ',
                    ),
                    TextSpan(
                      text: state.identifierType == 'email'
                          ? state.email
                          : '+234 ${state.phone}',
                      style: const TextStyle(color: kProviderGreen, fontWeight: FontWeight.w700),
                    ),
                  ],
                ),
              ),
              const SizedBox(height: 32),

              if (state.debugOtp != null) ...[
                Container(
                  width: double.infinity,
                  padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
                  decoration: BoxDecoration(
                    color: kProviderGreenTint,
                    borderRadius: BorderRadius.circular(12),
                    border: Border.all(color: kProviderGreen, width: 1.5),
                  ),
                  child: Column(
                    children: [
                      const Text(
                        'Your OTP code',
                        style: TextStyle(color: kProviderMuted, fontSize: 12, fontWeight: FontWeight.w500),
                      ),
                      const SizedBox(height: 4),
                      Text(
                        state.debugOtp!,
                        style: const TextStyle(
                          color: kProviderGreen,
                          fontSize: 36,
                          fontWeight: FontWeight.w900,
                          letterSpacing: 8,
                        ),
                      ),
                    ],
                  ),
                ),
                const SizedBox(height: 16),
              ],

              const Text(
                'Enter OTP',
                style: TextStyle(color: kProviderText, fontSize: 13, fontWeight: FontWeight.w700),
              ),
              const SizedBox(height: 12),

              Row(
                mainAxisAlignment: MainAxisAlignment.spaceBetween,
                children: List.generate(6, (i) => _OtpCircle(
                  controller: _controllers[i],
                  focusNode: _focusNodes[i],
                  onChanged: (v) => _onDigitEntered(i, v),
                  onBackspace: i > 0
                      ? () {
                          _controllers[i].clear();
                          _focusNodes[i - 1].requestFocus();
                          _controllers[i - 1].clear();
                          setState(() {});
                        }
                      : null,
                )),
              ),
              const SizedBox(height: 20),

              Row(
                mainAxisAlignment: MainAxisAlignment.spaceBetween,
                children: [
                  GestureDetector(
                    onTap: _secondsLeft == 0 ? _resend : null,
                    child: Text(
                      'Resend Code',
                      style: TextStyle(
                        color: _secondsLeft == 0 ? kProviderGreen : kProviderMuted,
                        fontWeight: FontWeight.w600,
                        fontSize: 13,
                        decoration: _secondsLeft == 0 ? TextDecoration.underline : null,
                      ),
                    ),
                  ),
                  Text(
                    _timerLabel,
                    style: const TextStyle(color: kProviderText, fontWeight: FontWeight.w700, fontSize: 13),
                  ),
                ],
              ),

              if (state.error != null) ...[
                const SizedBox(height: 12),
                Text(state.error!, style: const TextStyle(color: Colors.red, fontSize: 12), textAlign: TextAlign.center),
              ],

              const Spacer(),

              SizedBox(
                height: 52,
                child: FilledButton(
                  onPressed: _complete && !state.isLoading ? _submit : null,
                  style: FilledButton.styleFrom(
                    backgroundColor: kProviderGreen,
                    disabledBackgroundColor: kProviderGreen.withValues(alpha: 0.4),
                    shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(999)),
                  ),
                  child: state.isLoading
                      ? const SizedBox.square(
                          dimension: 20,
                          child: CircularProgressIndicator(strokeWidth: 2, color: Colors.white),
                        )
                      : const Text(
                          'Continue',
                          style: TextStyle(color: Colors.white, fontWeight: FontWeight.w700, fontSize: 16),
                        ),
                ),
              ),
              const SizedBox(height: 16),
            ],
          ),
        ),
      ),
    );
  }
}

// ─── Single OTP circle ────────────────────────────────────────────────────────

class _OtpCircle extends StatefulWidget {
  const _OtpCircle({
    required this.controller,
    required this.focusNode,
    required this.onChanged,
    this.onBackspace,
  });

  final TextEditingController controller;
  final FocusNode focusNode;
  final ValueChanged<String> onChanged;
  final VoidCallback? onBackspace;

  @override
  State<_OtpCircle> createState() => _OtpCircleState();
}

class _OtpCircleState extends State<_OtpCircle> {
  bool _focused = false;

  @override
  void initState() {
    super.initState();
    widget.focusNode.addListener(_onFocusChange);
  }

  void _onFocusChange() {
    if (mounted) setState(() => _focused = widget.focusNode.hasFocus);
  }

  @override
  void dispose() {
    widget.focusNode.removeListener(_onFocusChange);
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final filled = widget.controller.text.isNotEmpty;
    return Container(
      width: 48,
      height: 56,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        color: filled ? kProviderGreenTint : Colors.white,
        border: Border.all(
          color: _focused || filled ? kProviderGreen : kProviderBorder,
          width: _focused || filled ? 2 : 1.5,
        ),
      ),
      child: KeyboardListener(
        focusNode: FocusNode(),
        onKeyEvent: (event) {
          if (event is KeyDownEvent &&
              event.logicalKey == LogicalKeyboardKey.backspace &&
              widget.controller.text.isEmpty) {
            widget.onBackspace?.call();
          }
        },
        child: TextField(
          controller: widget.controller,
          focusNode: widget.focusNode,
          keyboardType: TextInputType.number,
          textAlign: TextAlign.center,
          maxLength: 1,
          inputFormatters: [FilteringTextInputFormatter.digitsOnly],
          style: const TextStyle(fontSize: 20, fontWeight: FontWeight.w800, color: kProviderGreen),
          decoration: const InputDecoration(
            border: InputBorder.none,
            counterText: '',
            contentPadding: EdgeInsets.zero,
          ),
          onChanged: widget.onChanged,
        ),
      ),
    );
  }
}
