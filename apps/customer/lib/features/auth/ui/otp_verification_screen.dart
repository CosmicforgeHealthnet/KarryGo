import 'dart:async';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:cosmicforge_logistics_ui_kit/cosmicforge_logistics_ui_kit.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../../app/app_routes.dart';
import '../../../shared/widgets/figma_customer_widgets.dart';
import '../state/customer_auth_controller.dart';

class OtpVerificationScreen extends StatefulWidget {
  const OtpVerificationScreen({
    super.key,
    required this.controller,
    required this.state,
  });

  final CustomerAuthController controller;
  final CustomerAuthState state;

  @override
  State<OtpVerificationScreen> createState() => _OtpVerificationScreenState();
}

class _OtpVerificationScreenState extends State<OtpVerificationScreen> {
  final _controllers = List.generate(6, (_) => TextEditingController());
  final _focusNodes = List.generate(6, (_) => FocusNode());
  String get _code => _controllers.map((c) => c.text).join();
  bool get _complete => _code.length == 6;

  static const int _resendCooldownSeconds = 30;

  late int _remainingSeconds;
  Timer? _countdownTimer;

  @override
  void initState() {
    super.initState();
    _remainingSeconds = _resendCooldownSeconds;
    _startCountdown();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) _focusNodes[0].requestFocus();
    });
  }

  @override
  void dispose() {
    _countdownTimer?.cancel();
    for (final c in _controllers) {
      c.dispose();
    }
    for (final f in _focusNodes) {
      f.dispose();
    }
    super.dispose();
  }

  void _startCountdown() {
    _countdownTimer?.cancel();
    _countdownTimer = Timer.periodic(const Duration(seconds: 1), (t) {
      if (!mounted) {
        t.cancel();
        return;
      }
      setState(() {
        if (_remainingSeconds > 0) {
          _remainingSeconds--;
        } else {
          t.cancel();
        }
      });
    });
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
      _verify();
    }
  }

  void _verify() {
    if (!_complete) return;
    FocusScope.of(context).unfocus();
    widget.controller.verifyOtp(_code);
  }

  void _resend() {
    widget.controller.resendOtp();
    setState(() => _remainingSeconds = _resendCooldownSeconds);
    _startCountdown();
  }

  @override
  Widget build(BuildContext context) {
    final fieldError = _fieldError(widget.state.error, 'otp');
    final channel =
        widget.state.identifierType == CustomerAuthIdentifierType.email
            ? 'email'
            : 'number';

    return FigmaPhoneScaffold(
      key: const ValueKey(CustomerAppRoutes.otpVerification),
      bottom: FigmaPrimaryButton(
        label: 'Continue',
        isLoading: widget.state.isLoading,
        onPressed: _complete && !widget.state.isLoading ? _verify : null,
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          FigmaBackButton(onPressed: widget.controller.backToPhoneEntry),
          const SizedBox(height: 8),
          const Text(
            'OTP Confirmation!',
            style: TextStyle(
              color: CustomerFigmaColors.text,
              fontSize: 18,
              fontWeight: FontWeight.w900,
            ),
          ),
          const SizedBox(height: 10),
          Text.rich(
            TextSpan(
              text: 'We sent you a 6-digit code via your $channel\n',
              style: const TextStyle(
                color: CustomerFigmaColors.muted,
                fontSize: 12,
                height: 1.45,
              ),
              children: [
                TextSpan(
                  text: widget.state.activeIdentifier,
                  style: const TextStyle(
                    color: CustomerFigmaColors.primary,
                    fontWeight: FontWeight.w800,
                  ),
                ),
              ],
            ),
          ),
          const SizedBox(height: 28),
          if (kDebugMode && widget.state.debugOtp != null) ...[
            Container(
              width: double.infinity,
              padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
              decoration: BoxDecoration(
                color: CustomerFigmaColors.primaryTint,
                borderRadius: BorderRadius.circular(12),
                border: Border.all(
                  color: CustomerFigmaColors.primary,
                  width: 1.5,
                ),
              ),
              child: Column(
                children: [
                  const Text(
                    'Your OTP code',
                    style: TextStyle(
                      color: CustomerFigmaColors.muted,
                      fontSize: 12,
                      fontWeight: FontWeight.w500,
                    ),
                  ),
                  const SizedBox(height: 4),
                  Text(
                    widget.state.debugOtp!,
                    style: const TextStyle(
                      color: CustomerFigmaColors.primary,
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
          const Text(
            'Enter OTP',
            style: TextStyle(
              color: CustomerFigmaColors.text,
              fontSize: 15,
              fontWeight: FontWeight.w900,
            ),
          ),
          const SizedBox(height: 14),
          Row(
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: List.generate(
              6,
              (i) => _OtpCircle(
                controller: _controllers[i],
                focusNode: _focusNodes[i],
                hasError: fieldError != null,
                onChanged: (v) => _onDigitEntered(i, v),
                onBackspace: i > 0
                    ? () {
                        _controllers[i].clear();
                        _focusNodes[i - 1].requestFocus();
                        _controllers[i - 1].clear();
                        setState(() {});
                      }
                    : null,
              ),
            ),
          ),
          if (fieldError != null) ...[
            const SizedBox(height: 10),
            CosmicforgeLogisticsFieldError(message: fieldError),
          ],
          const SizedBox(height: 22),
          Row(
            children: [
              TextButton(
                onPressed: widget.state.isLoading || _remainingSeconds > 0
                    ? null
                    : _resend,
                style: TextButton.styleFrom(
                  padding: EdgeInsets.zero,
                  foregroundColor: CustomerFigmaColors.primary,
                  textStyle: const TextStyle(
                    fontSize: 12,
                    fontWeight: FontWeight.w800,
                  ),
                ),
                child: const Text('Resend Code'),
              ),
              const Spacer(),
              Text(
                _formatTimer(_remainingSeconds),
                style: const TextStyle(
                  color: CustomerFigmaColors.primary,
                  fontSize: 12,
                  fontWeight: FontWeight.w700,
                ),
              ),
            ],
          ),
          if (widget.state.error != null && fieldError == null) ...[
            const SizedBox(height: 18),
            CosmicforgeLogisticsErrorBanner(
              title: widget.state.error!.title,
              message: widget.state.error!.message,
              onClose: widget.controller.dismissError,
            ),
          ],
          const Spacer(),
        ],
      ),
    );
  }
}

// ─── Single OTP circle ───────────────────────────────────────────────────────

class _OtpCircle extends StatefulWidget {
  const _OtpCircle({
    required this.controller,
    required this.focusNode,
    required this.onChanged,
    required this.hasError,
    this.onBackspace,
  });

  final TextEditingController controller;
  final FocusNode focusNode;
  final ValueChanged<String> onChanged;
  final bool hasError;
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
    final Color borderColor;
    if (widget.hasError) {
      borderColor = Colors.red.shade400;
    } else if (_focused || filled) {
      borderColor = CustomerFigmaColors.primary;
    } else {
      borderColor = CustomerFigmaColors.border;
    }

    return Container(
      width: 48,
      height: 56,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        color: filled ? CustomerFigmaColors.primaryTint : Colors.white,
        border: Border.all(
          color: borderColor,
          width: _focused || filled || widget.hasError ? 2 : 1.5,
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
          style: const TextStyle(
            fontSize: 20,
            fontWeight: FontWeight.w800,
            color: CustomerFigmaColors.primary,
          ),
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

String _formatTimer(int seconds) {
  if (seconds <= 0) {
    return '00:00';
  }
  final minutes = (seconds ~/ 60).toString().padLeft(2, '0');
  final remainder = (seconds % 60).toString().padLeft(2, '0');
  return '$minutes:$remainder';
}

String? _fieldError(ApiException? error, String field) {
  if (error == null) {
    return null;
  }
  for (final fieldError in error.fields) {
    if (fieldError.field == field) {
      return fieldError.message;
    }
  }
  return null;
}
