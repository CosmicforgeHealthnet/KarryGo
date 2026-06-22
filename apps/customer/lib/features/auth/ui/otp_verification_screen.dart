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
  final _otpController = TextEditingController();
  final _focusNode = FocusNode();
  late int _remainingSeconds;
  Timer? _countdownTimer;

  @override
  void initState() {
    super.initState();
    _remainingSeconds = widget.state.otpExpiresIn;
    _startCountdown();
  }

  @override
  void dispose() {
    _otpController.dispose();
    _focusNode.dispose();
    _countdownTimer?.cancel();
    super.dispose();
  }

  void _startCountdown() {
    _countdownTimer?.cancel();
    _countdownTimer = Timer.periodic(const Duration(seconds: 1), (_) {
      setState(() {
        if (_remainingSeconds > 0) {
          _remainingSeconds--;
        } else {
          _countdownTimer?.cancel();
        }
      });
    });
  }

  @override
  Widget build(BuildContext context) {
    final fieldError = _fieldError(widget.state.error, 'otp');
    final channel = widget.state.identifierType == CustomerAuthIdentifierType.email
        ? 'email'
        : 'number';

    return FigmaPhoneScaffold(
      key: const ValueKey(CustomerAppRoutes.otpVerification),
      bottom: ValueListenableBuilder<TextEditingValue>(
        valueListenable: _otpController,
        builder: (context, value, _) {
          final canContinue = value.text.length == 6;
          return FigmaPrimaryButton(
            label: 'Continue',
            isLoading: widget.state.isLoading,
            onPressed: canContinue && !widget.state.isLoading ? _verify : null,
          );
        },
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
          const SizedBox(height: 32),
          const Text(
            'Enter OTP',
            style: TextStyle(
              color: CustomerFigmaColors.text,
              fontSize: 15,
              fontWeight: FontWeight.w900,
            ),
          ),
          const SizedBox(height: 14),
          ValueListenableBuilder<TextEditingValue>(
            valueListenable: _otpController,
            builder: (context, value, _) {
              return GestureDetector(
                onTap: _focusNode.requestFocus,
                child: _OtpSlots(
                  value: value.text,
                  hasError: fieldError != null,
                ),
              );
            },
          ),
          SizedBox(
            height: 1,
            child: Opacity(
              opacity: 0.01,
              child: TextField(
                autofocus: true,
                focusNode: _focusNode,
                controller: _otpController,
                keyboardType: TextInputType.number,
                maxLength: 6,
                inputFormatters: [FilteringTextInputFormatter.digitsOnly],
                decoration: const InputDecoration(counterText: ''),
                onSubmitted: (_) => _verify(),
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
                    : () => widget.controller.resendOtp(),
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
          if (kDebugMode && widget.state.debugOtp != null) ...[
            const SizedBox(height: 8),
            Text(
              'Local test code: ${widget.state.debugOtp}',
              style: const TextStyle(
                color: CustomerFigmaColors.primary,
                fontSize: 12,
                fontWeight: FontWeight.w700,
              ),
            ),
          ],
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

  void _verify() {
    FocusScope.of(context).unfocus();
    widget.controller.verifyOtp(_otpController.text);
  }
}

class _OtpSlots extends StatelessWidget {
  const _OtpSlots({required this.value, required this.hasError});

  final String value;
  final bool hasError;

  @override
  Widget build(BuildContext context) {
    final chars = value.split('').take(6).toList();

    return Row(
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: List.generate(6, (index) {
        final hasValue = index < chars.length;
        final Color borderColor;
        if (hasError) {
          borderColor = Colors.red.shade400;
        } else if (hasValue) {
          borderColor = CustomerFigmaColors.primary;
        } else {
          borderColor = CustomerFigmaColors.border;
        }

        return Container(
          width: 54,
          height: 54,
          alignment: Alignment.center,
          decoration: BoxDecoration(
            color: Colors.white,
            shape: BoxShape.circle,
            border: Border.all(color: borderColor, width: 1.5),
            boxShadow: [
              BoxShadow(
                color: Colors.black.withValues(alpha: 0.05),
                blurRadius: 12,
                offset: const Offset(0, 4),
              ),
            ],
          ),
          child: Text(
            hasValue ? chars[index] : '',
            style: const TextStyle(
              color: CustomerFigmaColors.text,
              fontSize: 17,
              fontWeight: FontWeight.w800,
            ),
          ),
        );
      }),
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
