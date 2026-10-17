import '../../../shared/widgets/dispatch_screen_frame.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../../core/theme/theme_extensions.dart';
import 'phone_number_changed_screen.dart';

class OtpConfirmationScreen extends StatefulWidget {
  const OtpConfirmationScreen({
    super.key,
    required this.phoneDisplay,
    required this.newCountryFlag,
    required this.newCountryCode,
    required this.newPhoneNumber,
  });

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

  int _secondsLeft = 30;
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
    await Future.delayed(const Duration(milliseconds: 600));
    if (!mounted) return;
    setState(() {
      _isLoading = false;
      _secondsLeft = 30;
      _canResend = false;
    });
    _startTimer();
    ScaffoldMessenger.of(
      context,
    ).showSnackBar(const SnackBar(content: Text('Verification code resent.')));
  }

  Future<void> _confirm() async {
    final otp = _otpController.text;
    if (otp.length != 6) return;

    setState(() => _isLoading = true);
    await Future.delayed(const Duration(milliseconds: 800));
    if (!mounted) return;
    setState(() => _isLoading = false);

    final confirmed = await Navigator.of(context).push<bool>(
      MaterialPageRoute(
        builder: (_) => PhoneNumberChangedScreen(
          newPhoneDisplay:
              '${widget.newCountryFlag} ${widget.newCountryCode} ${widget.newPhoneNumber}',
        ),
      ),
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
    final c = context.palette;

    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        child: Column(
          children: [
            // ── Header ──
            Padding(
              padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
              child: Row(
                children: [
                  DispatchBackButton(
                    onPressed: () => Navigator.of(context).pop(),
                  ),
                ],
              ),
            ),

            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 8, 20, 24),
                children: [
                  Text(
                    'Enter verification code',
                    style: TextStyle(
                      fontSize: 28,
                      fontWeight: FontWeight.w700,
                      color: c.text,
                      letterSpacing: -0.4,
                    ),
                  ),
                  const SizedBox(height: 6),
                  RichText(
                    text: TextSpan(
                      style: TextStyle(
                        fontSize: 15,
                        fontWeight: FontWeight.w500,
                        color: c.textMuted,
                        height: 1.5,
                      ),
                      children: [
                        const TextSpan(text: "We've sent a 6-digit code to "),
                        TextSpan(
                          text: widget.phoneDisplay,
                          style: TextStyle(
                            color: c.text,
                            fontWeight: FontWeight.w700,
                          ),
                        ),
                      ],
                    ),
                  ),
                  const SizedBox(height: 32),

                  GestureDetector(
                    onTap: _requestFocus,
                    child: ValueListenableBuilder<TextEditingValue>(
                      valueListenable: _otpController,
                      builder: (context, value, _) =>
                          _OtpSlots(value: value.text),
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
                        inputFormatters: [
                          FilteringTextInputFormatter.digitsOnly,
                        ],
                        decoration: const InputDecoration(counterText: ''),
                        onChanged: (_) => setState(() {}),
                      ),
                    ),
                  ),
                  const SizedBox(height: 20),
                  Row(
                    children: [
                      Text(
                        "Didn't receive code? ",
                        style: TextStyle(
                          fontSize: 13,
                          fontWeight: FontWeight.w500,
                          color: c.textMuted,
                        ),
                      ),
                      GestureDetector(
                        onTap: _canResend && !_isLoading ? _resend : null,
                        child: Text(
                          _canResend ? 'Resend' : 'Resend in $_timerText',
                          style: TextStyle(
                            fontSize: 13,
                            fontWeight: FontWeight.w700,
                            color: _canResend ? c.brandTeal500 : c.textMuted,
                          ),
                        ),
                      ),
                    ],
                  ),
                ],
              ),
            ),

            // ── Verify code ──
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton(
                  onPressed: _otpController.text.length == 6 && !_isLoading
                      ? _confirm
                      : null,
                  style: FilledButton.styleFrom(
                    textStyle: const TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  child: _isLoading
                      ? SizedBox(
                          width: 20,
                          height: 20,
                          child: CircularProgressIndicator(
                            strokeWidth: 2,
                            color: c.brandWhite,
                          ),
                        )
                      : const Text('Verify code'),
                ),
              ),
            ),
          ],
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
    final c = context.palette;
    final chars = value.split('').take(6).toList();
    return Row(
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: List.generate(6, (i) {
        final hasValue = i < chars.length;
        final isActive = i == chars.length;
        return Container(
          width: 48,
          height: 56,
          alignment: Alignment.center,
          decoration: BoxDecoration(
            color: c.inputFill,
            borderRadius: BorderRadius.circular(12),
            border: Border.all(
              color: hasValue || isActive ? c.brandTeal500 : c.border,
              width: hasValue || isActive ? 1.6 : 1.0,
            ),
          ),
          child: Text(
            hasValue ? chars[i] : '',
            style: TextStyle(
              fontSize: 20,
              fontWeight: FontWeight.w700,
              color: c.text,
            ),
          ),
        );
      }),
    );
  }
}
