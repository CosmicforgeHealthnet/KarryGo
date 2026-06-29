import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'phone_number_changed_dialog.dart';

class OtpConfirmationScreen extends StatefulWidget {
  const OtpConfirmationScreen({
    super.key,
    required this.phoneDisplay,
    required this.newCountryFlag,
    required this.newCountryCode,
    required this.newPhoneNumber,
  });

  /// The new phone number shown to the user (e.g. "🇳🇬 +234 8067735987").
  final String phoneDisplay;

  final String newCountryFlag;
  final String newCountryCode;
  final String newPhoneNumber;

  @override
  State<OtpConfirmationScreen> createState() => _OtpConfirmationScreenState();
}

class _OtpConfirmationScreenState extends State<OtpConfirmationScreen> {
  final _otpController = TextEditingController();
  final _focusNode = FocusNode();

  int _secondsLeft = 251; // matches "04:11" shown in the design
  bool _canResend = false;
  bool _isLoading = false;

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

  Future<void> _resend() async {
    setState(() => _isLoading = true);
    // TODO: call verification_api / dispatch_auth_controller resend.
    await Future.delayed(const Duration(milliseconds: 600));
    if (!mounted) return;
    setState(() {
      _isLoading = false;
      _secondsLeft = 251;
      _canResend = false;
    });
    _startTimer();
    ScaffoldMessenger.of(context).showSnackBar(
      const SnackBar(content: Text('Verification code resent successfully.')),
    );
  }

  Future<void> _confirm() async {
    final otp = _otpController.text;
    if (otp.length != 6) return;

    setState(() => _isLoading = true);
    // TODO: call verification_api / dispatch_auth_controller verify.
    await Future.delayed(const Duration(milliseconds: 800));
    if (!mounted) return;
    setState(() => _isLoading = false);

    final confirmed = await showDialog<bool>(
      context: context,
      barrierDismissible: false,
      builder: (_) => const PhoneNumberChangedDialog(),
    );

    if (!mounted) return;
    if (confirmed == true) {
      Navigator.of(context).pop({
        'flag': widget.newCountryFlag,
        'code': widget.newCountryCode,
        'number': widget.newPhoneNumber,
      });
    }
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
              GestureDetector(
                onTap: () => Navigator.of(context).pop(),
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
                'We sent you a  6-digit code via your number',
                style: TextStyle(
                  color: Color(0xFF888888),
                  fontSize: 13,
                  height: 1.5,
                ),
              ),
              const SizedBox(height: 2),
              Text(
                widget.phoneDisplay,
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
              const Spacer(),
              ValueListenableBuilder<TextEditingValue>(
                valueListenable: _otpController,
                builder: (context, value, _) {
                  final canContinue = value.text.length == 6;
                  return SizedBox(
                    height: 52,
                    child: FilledButton(
                      onPressed: canContinue && !_isLoading ? _confirm : null,
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
                              'Confirm',
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
