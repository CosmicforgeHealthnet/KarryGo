import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
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

  /// The identifier (phone number or email) shown to the user.
  final String phone;
  final ValueChanged<String> onVerify;
  final VoidCallback onBack;

  /// Called after a successful resend (e.g. to reset the UI timer externally).
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

  @override
  void initState() {
    super.initState();
    _startTimer();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) _focusNode.requestFocus();
    });
  }

  /// Unfocus first (clears Flutter's internal "already focused" state),
  /// then re-request after 1 frame — fix for the known Flutter bug where
  /// requestFocus() is a no-op when keyboard was dismissed via back button.
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
              // Back arrow
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
                'Enter the OTP sent to your phone or email.',
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
              GestureDetector(
                onTap: _requestFocus,
                child: ValueListenableBuilder<TextEditingValue>(
                  valueListenable: _otpController,
                  builder: (context, value, _) => _OtpSlots(value: value.text),
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
                    onTap: _canResend && !_isLoading ? _resend : null,
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
                      onPressed: canContinue && !_isLoading && !_hasVerified
                          ? _verify
                          : null,
                      style: FilledButton.styleFrom(
                        backgroundColor: const Color(0xFF4CAF50),
                        disabledBackgroundColor: const Color(
                          0xFF4CAF50,
                        ).withValues(alpha: 0.4),
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

  bool _isLoading = false;

  /// Latched to true after the first successful verify so that the OTP screen
  /// never re-submits while _handlePostVerify is running asynchronously.
  bool _hasVerified = false;

  void _resend() async {
    final messenger = ScaffoldMessenger.of(context);
    setState(() => _isLoading = true);

    try {
      // Use the controller's resendOtp which knows whether this is login or signup.
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
    // Guard: block if loading OR already successfully verified.
    // _hasVerified stays true while _handlePostVerify runs asynchronously so
    // the user cannot re-tap Continue and trigger a second /auth/verify call.
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
          // Do NOT reset _isLoading on success — keep the button disabled
          // while _handlePostVerify runs /provider/me asynchronously.
          // The OTP screen stays mounted until _go() replaces it.
          setState(() => _hasVerified = true);
          debugPrint('[AUTH_UI] verify success');
          widget.onVerify(otp);
        },
        failure: (error) {
          // Only re-enable the button on failure so the user can correct and retry.
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
