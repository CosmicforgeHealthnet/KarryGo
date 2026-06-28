import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../home/ui/widgets/provider_app_colors.dart';
import '../state/provider_profile_controller.dart';
import 'widgets/provider_profile_widgets.dart';

/// Change-phone flow: New Phone Number entry (Figma 2113) → OTP confirmation
/// (Account option-1 style). Pops `true` once the phone is changed.
class ProviderChangePhoneScreen extends StatefulWidget {
  const ProviderChangePhoneScreen({super.key, required this.profileController});

  final ProviderProfileController profileController;

  @override
  State<ProviderChangePhoneScreen> createState() => _ProviderChangePhoneScreenState();
}

enum _Step { entry, otp }

class _ProviderChangePhoneScreenState extends State<ProviderChangePhoneScreen> {
  final _phoneCtrl = TextEditingController();
  _Step _step = _Step.entry;
  bool _busy = false;
  String? _error;
  String _otp = '';

  @override
  void initState() {
    super.initState();
    _phoneCtrl.addListener(() => setState(() {}));
  }

  @override
  void dispose() {
    _phoneCtrl.dispose();
    super.dispose();
  }

  bool get _canVerify => _phoneCtrl.text.trim().length >= 7 && !_busy;

  Future<void> _startChange() async {
    setState(() {
      _busy = true;
      _error = null;
    });
    final phone = '+234${_phoneCtrl.text.trim()}';
    final ok = await widget.profileController.startPhoneChange(phone);
    if (!mounted) return;
    setState(() {
      _busy = false;
      if (ok) {
        _step = _Step.otp;
      } else {
        _error = widget.profileController.error;
      }
    });
  }

  Future<void> _verifyOtp() async {
    setState(() {
      _busy = true;
      _error = null;
    });
    final ok = await widget.profileController.verifyPhoneChange(_otp);
    if (!mounted) return;
    setState(() => _busy = false);
    if (ok) {
      Navigator.of(context).pop(true);
    } else {
      setState(() => _error = widget.profileController.error);
    }
  }

  void _back() {
    if (_step == _Step.otp) {
      setState(() {
        _step = _Step.entry;
        _error = null;
        _otp = '';
      });
    } else {
      Navigator.of(context).pop(false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(20, 16, 20, 16),
          child: _step == _Step.entry ? _buildEntry() : _buildOtp(),
        ),
      ),
    );
  }

  Widget _buildEntry() {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        ProviderProfileHeader(
          title: 'New Phone Number',
          subtitle: 'Enter new phone number to proceed',
          onBack: _back,
        ),
        const SizedBox(height: 28),
        const Text(
          'Enter your Phone Number',
          style: TextStyle(color: kProviderText, fontSize: 14, fontWeight: FontWeight.w700),
        ),
        const SizedBox(height: 12),
        Row(
          children: [
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 16),
              decoration: BoxDecoration(
                color: Colors.white,
                borderRadius: BorderRadius.circular(12),
                border: Border.all(color: kProviderBorder),
              ),
              child: const Row(
                children: [
                  Text('🇳🇬', style: TextStyle(fontSize: 18)),
                  SizedBox(width: 6),
                  Text('+234', style: TextStyle(color: kProviderText, fontSize: 14, fontWeight: FontWeight.w600)),
                  Icon(Icons.keyboard_arrow_down_rounded, size: 18, color: kProviderMuted),
                ],
              ),
            ),
            const SizedBox(width: 10),
            Expanded(
              child: TextField(
                controller: _phoneCtrl,
                keyboardType: TextInputType.phone,
                inputFormatters: [FilteringTextInputFormatter.digitsOnly],
                style: const TextStyle(color: kProviderGreen, fontSize: 15, fontWeight: FontWeight.w600),
                decoration: providerWhiteField('8067735987'),
              ),
            ),
          ],
        ),
        if (_error != null) ...[
          const SizedBox(height: 16),
          ProviderErrorText(_error),
        ],
        const Spacer(),
        ProviderPrimaryButton(
          label: 'Verify',
          isLoading: _busy,
          onPressed: _canVerify ? _startChange : null,
        ),
      ],
    );
  }

  Widget _buildOtp() {
    final debugOtp = widget.profileController.debugOtp;
    final phone = widget.profileController.pendingPhone;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        ProviderProfileHeader(
          title: 'OTP Confirmation!',
          onBack: _back,
        ),
        const SizedBox(height: 8),
        RichText(
          text: TextSpan(
            style: const TextStyle(color: kProviderMuted, fontSize: 14, height: 1.5),
            children: [
              const TextSpan(text: 'We sent you a  6-digit code via your number\n'),
              TextSpan(
                text: phone,
                style: const TextStyle(color: kProviderGreen, fontWeight: FontWeight.w700),
              ),
            ],
          ),
        ),
        const SizedBox(height: 28),
        if (debugOtp != null) ...[
          Container(
            width: double.infinity,
            padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
            decoration: BoxDecoration(
              color: kProviderGreenTint,
              borderRadius: BorderRadius.circular(12),
              border: Border.all(color: kProviderGreen, width: 1.5),
            ),
            child: Column(
              children: [
                const Text('Your OTP code', style: TextStyle(color: kProviderMuted, fontSize: 12)),
                const SizedBox(height: 4),
                Text(
                  debugOtp,
                  style: const TextStyle(
                      color: kProviderGreen, fontSize: 32, fontWeight: FontWeight.w900, letterSpacing: 8),
                ),
              ],
            ),
          ),
          const SizedBox(height: 16),
        ],
        const Text('Enter OTP',
            style: TextStyle(color: kProviderText, fontSize: 14, fontWeight: FontWeight.w700)),
        const SizedBox(height: 14),
        ProviderOtpBoxes(
          onChanged: (code) => setState(() => _otp = code),
        ),
        if (_error != null) ...[
          const SizedBox(height: 16),
          ProviderErrorText(_error),
        ],
        const Spacer(),
        ProviderPrimaryButton(
          label: 'Confirm',
          isLoading: _busy,
          onPressed: _otp.length == 6 && !_busy ? _verifyOtp : null,
        ),
      ],
    );
  }
}

// ─── 6-box OTP input (Figma OTP circles) ────────────────────────────────────────

class ProviderOtpBoxes extends StatefulWidget {
  const ProviderOtpBoxes({super.key, required this.onChanged});
  final ValueChanged<String> onChanged;

  @override
  State<ProviderOtpBoxes> createState() => _ProviderOtpBoxesState();
}

class _ProviderOtpBoxesState extends State<ProviderOtpBoxes> {
  final _controllers = List.generate(6, (_) => TextEditingController());
  final _focusNodes = List.generate(6, (_) => FocusNode());

  String get _code => _controllers.map((c) => c.text).join();

  @override
  void dispose() {
    for (final c in _controllers) {
      c.dispose();
    }
    for (final f in _focusNodes) {
      f.dispose();
    }
    super.dispose();
  }

  void _onChanged(int i, String v) {
    final digit = v.replaceAll(RegExp(r'\D'), '');
    if (digit.isEmpty) {
      _controllers[i].clear();
    } else {
      _controllers[i].text = digit[digit.length - 1];
      _controllers[i].selection = const TextSelection.collapsed(offset: 1);
      if (i < 5) _focusNodes[i + 1].requestFocus();
    }
    setState(() {});
    widget.onChanged(_code);
  }

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: List.generate(6, (i) {
        final filled = _controllers[i].text.isNotEmpty;
        return SizedBox(
          width: 52,
          height: 60,
          child: KeyboardListener(
            focusNode: FocusNode(),
            onKeyEvent: (event) {
              if (event is KeyDownEvent &&
                  event.logicalKey == LogicalKeyboardKey.backspace &&
                  _controllers[i].text.isEmpty &&
                  i > 0) {
                _focusNodes[i - 1].requestFocus();
                _controllers[i - 1].clear();
                setState(() {});
                widget.onChanged(_code);
              }
            },
            child: TextField(
              controller: _controllers[i],
              focusNode: _focusNodes[i],
              keyboardType: TextInputType.number,
              textAlign: TextAlign.center,
              maxLength: 1,
              inputFormatters: [FilteringTextInputFormatter.digitsOnly],
              style: const TextStyle(fontSize: 20, fontWeight: FontWeight.w800, color: kProviderGreen),
              decoration: InputDecoration(
                counterText: '',
                filled: true,
                fillColor: filled ? kProviderGreenTint : Colors.white,
                contentPadding: EdgeInsets.zero,
                enabledBorder: OutlineInputBorder(
                  borderRadius: BorderRadius.circular(30),
                  borderSide: BorderSide(color: filled ? kProviderGreen : kProviderBorder, width: 1.5),
                ),
                focusedBorder: OutlineInputBorder(
                  borderRadius: BorderRadius.circular(30),
                  borderSide: const BorderSide(color: kProviderGreen, width: 2),
                ),
              ),
              onChanged: (v) => _onChanged(i, v),
            ),
          ),
        );
      }),
    );
  }
}
