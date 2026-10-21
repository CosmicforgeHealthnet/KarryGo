import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../../core/widgets/karryam_ui.dart';
import '../../home/ui/widgets/provider_app_colors.dart';
import '../state/provider_auth_controller.dart';

class ProviderOtpScreen extends StatefulWidget {
  const ProviderOtpScreen({super.key, required this.controller});
  final ProviderAuthController controller;

  @override
  State<ProviderOtpScreen> createState() => _ProviderOtpScreenState();
}

class _ProviderOtpScreenState extends State<ProviderOtpScreen> {
  // One real input (so paste, SMS autofill and fast typing all work) drawn
  // as six boxes.
  final _codeCtrl = TextEditingController();
  final _focus = FocusNode();
  String get _code => _codeCtrl.text;
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
      if (mounted) _focus.requestFocus();
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
    _codeCtrl.dispose();
    _focus.dispose();
    super.dispose();
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
    final destination = state.identifierType == 'email' ? state.email : _internationalPhone(state.phone);
    return Scaffold(
      appBar: KTopBar(title: 'Karryam partner', onBack: widget.controller.backToPhoneEntry),
      body: SafeArea(
        top: false,
        child: Column(
          children: [
            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(24, 12, 24, 24),
                children: [
                  const Center(child: KIconTile(Icons.sms_outlined, size: 64)),
                  const SizedBox(height: 20),
                  KHeading(
                    title: 'Check your phone.',
                    subtitle: state.identifierType == 'email'
                        ? 'Enter the 6-digit code we sent to $destination.'
                        : 'Enter the 6-digit code we sent to $destination.',
                  ),
                  if (state.debugOtp != null) ...[
                    const SizedBox(height: 16),
                    KNotice('Test code: ${state.debugOtp}', tone: KTone.success, icon: Icons.bug_report_outlined),
                  ],
                  const SizedBox(height: 24),
                  const KFieldLabel('Verification code'),
                  _CodeBoxes(
                    controller: _codeCtrl,
                    focusNode: _focus,
                    onChanged: (_) {
                      setState(() {});
                      if (_complete) _submit();
                    },
                  ),
                  const SizedBox(height: 16),
                  Row(
                    children: [
                      const Icon(Icons.schedule_rounded, size: 16, color: kProviderMuted),
                      const SizedBox(width: 6),
                      Text(
                        _secondsLeft == 0 ? "Didn't get it?" : 'Resend available in $_timerLabel',
                        style: const TextStyle(color: kProviderMuted, fontSize: 13),
                      ),
                      const Spacer(),
                      TextButton(
                        onPressed: _secondsLeft == 0 && !state.isLoading ? _resend : null,
                        child: const Text('Resend Code'),
                      ),
                    ],
                  ),
                  if (state.error != null) ...[
                    const SizedBox(height: 8),
                    KNotice(state.error!, tone: KTone.danger),
                  ],
                ],
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(24, 8, 24, 16),
              child: KPrimaryButton(
                label: 'Verify number',
                arrow: true,
                loading: state.isLoading,
                onPressed: _complete ? _submit : null,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// Six boxes over one invisible text field.
class _CodeBoxes extends StatelessWidget {
  const _CodeBoxes({required this.controller, required this.focusNode, required this.onChanged});

  final TextEditingController controller;
  final FocusNode focusNode;
  final ValueChanged<String> onChanged;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: focusNode.requestFocus,
      child: Stack(
        children: [
          // The real input: transparent, but focusable and autofill-capable.
          Opacity(
            opacity: 0,
            child: TextField(
              controller: controller,
              focusNode: focusNode,
              keyboardType: TextInputType.number,
              autofillHints: const [AutofillHints.oneTimeCode],
              maxLength: 6,
              inputFormatters: [FilteringTextInputFormatter.digitsOnly],
              showCursor: false,
              enableInteractiveSelection: false,
              decoration: const InputDecoration(counterText: '', border: InputBorder.none, filled: false),
              onChanged: onChanged,
            ),
          ),
          AnimatedBuilder(
            animation: Listenable.merge([controller, focusNode]),
            builder: (context, _) {
              final code = controller.text;
              return Row(
                children: [
                  for (var i = 0; i < 6; i++) ...[
                    if (i > 0) const SizedBox(width: 8),
                    Expanded(
                      child: AnimatedContainer(
                        duration: const Duration(milliseconds: 150),
                        height: 58,
                        alignment: Alignment.center,
                        decoration: BoxDecoration(
                          color: kCard,
                          borderRadius: BorderRadius.circular(14),
                          border: Border.all(
                            color: focusNode.hasFocus && i == code.length.clamp(0, 5)
                                ? kTeal
                                : (i < code.length ? kNavy : kProviderBorder),
                            width: focusNode.hasFocus && i == code.length.clamp(0, 5) ? 2 : 1.2,
                          ),
                        ),
                        child: Text(
                          i < code.length ? code[i] : '',
                          style: const TextStyle(fontSize: 22, fontWeight: FontWeight.w700, color: kProviderText),
                        ),
                      ),
                    ),
                  ],
                ],
              );
            },
          ),
        ],
      ),
    );
  }
}

/// "08031234567", "8031234567" or "+2348031234567" -> "+234 803 123 4567".
String _internationalPhone(String raw) {
  var digits = raw.replaceAll(RegExp(r'[^0-9]'), '');
  if (digits.startsWith('234')) digits = digits.substring(3);
  if (digits.startsWith('0')) digits = digits.substring(1);
  if (digits.length != 10) return '+234 $digits';
  return '+234 ${digits.substring(0, 3)} ${digits.substring(3, 6)} ${digits.substring(6)}';
}
