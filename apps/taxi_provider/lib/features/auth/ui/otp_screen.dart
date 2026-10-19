import '../../../core/theme/karryam_theme.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../state/provider_auth_controller.dart';

/// OTP confirmation. In debug builds against a service running with
/// TAXI_OTP_DEBUG=true the code is shown on screen, matching the customer and
/// truck apps — it is never present in a release build or a real deployment.
class OtpScreen extends StatefulWidget {
  const OtpScreen({super.key, required this.controller});

  final ProviderAuthController controller;

  @override
  State<OtpScreen> createState() => _OtpScreenState();
}

class _OtpScreenState extends State<OtpScreen> {
  final _otpCtrl = TextEditingController();

  @override
  void dispose() {
    _otpCtrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final state = widget.controller.state;
    final debugOtp = state.challenge?.debugOtp;

    return Scaffold(
      backgroundColor: KarryamColors.of(context).surface,
      appBar: AppBar(
        backgroundColor: KarryamColors.of(context).surface,
        elevation: 0,
        leading: IconButton(
          onPressed: widget.controller.backToPhoneEntry,
          icon: Icon(Icons.arrow_back_rounded),
          color: KarryamColors.of(context).text,
        ),
      ),
      body: SafeArea(
        child: SingleChildScrollView(
          padding: EdgeInsets.fromLTRB(24, 0, 24, 24),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Text(
                'Enter your code',
                style: TextStyle(
                  fontSize: 22,
                  fontWeight: FontWeight.w800,
                  color: KarryamColors.of(context).text,
                ),
              ),
              SizedBox(height: 6),
              Text(
                'We sent a 6-digit code to ${state.destination}.',
                style: TextStyle(
                  fontSize: 14,
                  color: KarryamColors.of(context).textMuted,
                ),
              ),

              if (kDebugMode && debugOtp != null) ...[
                SizedBox(height: 18),
                Container(
                  padding: EdgeInsets.symmetric(vertical: 14),
                  decoration: BoxDecoration(
                    color: KarryamColors.of(context).primaryTint,
                    borderRadius: BorderRadius.circular(12),
                  ),
                  child: Column(
                    children: [
                      Text(
                        'Your OTP code',
                        style: TextStyle(
                          fontSize: 12,
                          color: KarryamColors.of(context).textMuted,
                        ),
                      ),
                      SizedBox(height: 4),
                      Text(
                        debugOtp,
                        style: TextStyle(
                          fontSize: 26,
                          fontWeight: FontWeight.w900,
                          letterSpacing: 6,
                          color: KarryamColors.of(context).primary,
                        ),
                      ),
                    ],
                  ),
                ),
              ],

              SizedBox(height: 20),
              TextField(
                controller: _otpCtrl,
                keyboardType: TextInputType.number,
                textAlign: TextAlign.center,
                maxLength: 6,
                inputFormatters: [FilteringTextInputFormatter.digitsOnly],
                style: TextStyle(
                  fontSize: 24,
                  fontWeight: FontWeight.w800,
                  letterSpacing: 10,
                ),
                onSubmitted: widget.controller.verifyOtp,
                decoration: InputDecoration(
                  counterText: '',
                  hintText: '••••••',
                  filled: true,
                  fillColor: KarryamColors.of(context).card,
                  border: OutlineInputBorder(
                    borderRadius: BorderRadius.circular(12),
                    borderSide: BorderSide(
                      color: KarryamColors.of(context).border,
                    ),
                  ),
                  enabledBorder: OutlineInputBorder(
                    borderRadius: BorderRadius.circular(12),
                    borderSide: BorderSide(
                      color: KarryamColors.of(context).border,
                    ),
                  ),
                ),
              ),

              if (state.error != null) ...[
                SizedBox(height: 12),
                Text(
                  state.error!,
                  style: TextStyle(
                    color: KarryamColors.of(context).danger,
                    fontSize: 12.5,
                  ),
                ),
              ],

              SizedBox(height: 28),
              SizedBox(
                height: 52,
                child: FilledButton(
                  onPressed: state.isLoading
                      ? null
                      : () => widget.controller.verifyOtp(_otpCtrl.text),
                  style: FilledButton.styleFrom(
                    backgroundColor: KarryamColors.of(context).primary,
                    disabledBackgroundColor: KarryamColors.of(
                      context,
                    ).primarySoft,
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(12),
                    ),
                  ),
                  child: state.isLoading
                      ? SizedBox(
                          width: 20,
                          height: 20,
                          child: CircularProgressIndicator(
                            strokeWidth: 2,
                            color: KarryamColors.of(context).onPrimary,
                          ),
                        )
                      : Text(
                          'Verify',
                          style: TextStyle(
                            color: KarryamColors.of(context).onPrimary,
                            fontSize: 16,
                            fontWeight: FontWeight.w700,
                          ),
                        ),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
