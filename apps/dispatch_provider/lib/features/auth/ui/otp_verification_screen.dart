import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

class OtpVerificationScreen extends StatefulWidget {
  const OtpVerificationScreen({
    super.key,
    required this.phone,
    required this.onVerify,
    required this.onBack,
    required this.onResend,
  });

  final String phone;
  final ValueChanged<String> onVerify;
  final VoidCallback onBack;
  final VoidCallback onResend;

  @override
  State<OtpVerificationScreen> createState() => _OtpVerificationScreenState();
}

class _OtpVerificationScreenState extends State<OtpVerificationScreen> {
  final _otpController = TextEditingController();
  final _focusNode = FocusNode();

  int _secondsLeft = 120;
  bool _canResend = false;

  @override
  void initState() {
    super.initState();
    _startTimer();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) _focusNode.requestFocus();
    });
  }

  /// Unfocus first (clears Flutter's internal "already focused" state),
  /// then re-request after 1 frame — this is the fix for the known Flutter
  /// bug where requestFocus() is a no-op when keyboard was dismissed
  /// via back button while the FocusNode still holds focus internally.
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
    return Scaffold(
      backgroundColor: const Color(0xFFF7F7F7),
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(24, 16, 24, 32),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              // ✅ Standard auth screen back arrow — no AppBar
              GestureDetector(
                onTap: widget.onBack,
                child: const Padding(
                  padding: EdgeInsets.only(top: 4, bottom: 4),
                  child: Align(
                    alignment: Alignment.centerLeft,
                    child: Icon(
                      Icons.arrow_back_ios_new,
                      size: 20,
                      color: Color(0xFF1A1A1A),
                    ),
                  ),
                ),
              ),
              const SizedBox(height: 24),
              const Text(
                'OTP Confirmation!',
                style: TextStyle(
                  fontSize: 22,
                  fontWeight: FontWeight.w800,
                  color: Color(0xFF1A1A1A),
                ),
              ),
              const SizedBox(height: 8),
              const Text(
                'We sent you a 6-digit code via your number',
                style: TextStyle(
                  color: Color(0xFF888888),
                  fontSize: 13,
                  height: 1.5,
                ),
              ),
              const SizedBox(height: 2),
              Text(
                widget.phone,
                style: const TextStyle(
                  color: Color(0xFF4CAF50),
                  fontSize: 13,
                  fontWeight: FontWeight.w700,
                  height: 1.5,
                ),
              ),
              const SizedBox(height: 36),
              const Text(
                'Enter OTP',
                style: TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w700,
                  color: Color(0xFF1A1A1A),
                ),
              ),
              const SizedBox(height: 16),
              // ✅ _requestFocus: unfocus → next frame → requestFocus
              GestureDetector(
                onTap: _requestFocus,
                child: ValueListenableBuilder<TextEditingValue>(
                  valueListenable: _otpController,
                  builder: (context, value, _) =>
                      _OtpSlots(value: value.text),
                ),
              ),
              // Hidden real text field
              SizedBox(
                height: 1,
                child: Opacity(
                  opacity: 0.01,
                  child: TextField(
                    focusNode: _focusNode,
                    controller: _otpController,
                    keyboardType: TextInputType.number,
                    maxLength: 6,
                    inputFormatters: [FilteringTextInputFormatter.digitsOnly],
                    decoration: const InputDecoration(counterText: ''),
                    onChanged: (v) => setState(() {}),
                  ),
                ),
              ),
              const SizedBox(height: 20),
              Row(
                children: [
                  GestureDetector(
                    onTap: _canResend
                        ? () {
                            widget.onResend();
                            setState(() {
                              _secondsLeft = 120;
                              _canResend = false;
                            });
                            _startTimer();
                          }
                        : null,
                    child: Text(
                      'Resend Code',
                      style: TextStyle(
                        fontSize: 13,
                        fontWeight: FontWeight.w700,
                        color: _canResend
                            ? const Color(0xFF4CAF50)
                            : const Color(0xFFAAAAAA),
                      ),
                    ),
                  ),
                  const Spacer(),
                  if (!_canResend)
                    Text(
                      _timerText,
                      style: const TextStyle(
                        color: Color(0xFF4CAF50),
                        fontSize: 13,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                ],
              ),
              const SizedBox(height: 48),
              ValueListenableBuilder<TextEditingValue>(
                valueListenable: _otpController,
                builder: (context, value, _) {
                  final canContinue = value.text.length == 6;
                  return SizedBox(
                    height: 52,
                    child: FilledButton(
                      onPressed: canContinue
                          ? () => widget.onVerify(_otpController.text)
                          : null,
                      style: FilledButton.styleFrom(
                        backgroundColor: const Color(0xFF4CAF50),
                        disabledBackgroundColor:
                            const Color(0xFF4CAF50).withValues(alpha: 0.4),
                        shape: RoundedRectangleBorder(
                          borderRadius: BorderRadius.circular(999),
                        ),
                      ),
                      child: const Text(
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
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _OtpSlots extends StatelessWidget {
  const _OtpSlots({required this.value});
  final String value;

  @override
  Widget build(BuildContext context) {
    final chars = value.split('').take(6).toList();
    return Row(
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: List.generate(6, (i) {
        final hasValue = i < chars.length;
        final isActive = i == chars.length;
        return Container(
          width: 50,
          height: 52,
          alignment: Alignment.center,
          decoration: BoxDecoration(
            color: Colors.white,
            borderRadius: BorderRadius.circular(10),
            border: Border.all(
              color: hasValue || isActive
                  ? const Color(0xFF4CAF50)
                  : const Color(0xFFE0E0E0),
              width: hasValue || isActive ? 1.5 : 1.0,
            ),
          ),
          child: Text(
            hasValue ? chars[i] : '',
            style: const TextStyle(
              fontSize: 20,
              fontWeight: FontWeight.w700,
              color: Color(0xFF1A1A1A),
            ),
          ),
        );
      }),
    );
  }
}