import 'package:flutter/material.dart';
import 'email_verification_screen.dart';

enum _EmailStatus { unverified, verified }

enum _TwoStepStatus { off, on }

class TwoStepVerificationScreen extends StatefulWidget {
  const TwoStepVerificationScreen({super.key});

  @override
  State<TwoStepVerificationScreen> createState() =>
      _TwoStepVerificationScreenState();
}

class _TwoStepVerificationScreenState
    extends State<TwoStepVerificationScreen> {
  _EmailStatus _emailStatus = _EmailStatus.unverified;
  _TwoStepStatus _twoStepStatus = _TwoStepStatus.off;

  // TODO: replace with real email from profile controller
  final String _email = 'uiuxwithdema@gmail.com';

  bool get _canToggle => _emailStatus == _EmailStatus.verified;

  void _onVerifyTap() async {
    await Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => EmailVerificationScreen(email: _email),
      ),
    );
    if (!mounted) return;
    setState(() => _emailStatus = _EmailStatus.verified);
  }

  void _onToggle() {
    if (!_canToggle) return;
    setState(() {
      _twoStepStatus = _twoStepStatus == _TwoStepStatus.off
          ? _TwoStepStatus.on
          : _TwoStepStatus.off;
    });
    // TODO: wire to API
  }

  @override
  Widget build(BuildContext context) {
    final isOn = _twoStepStatus == _TwoStepStatus.on;

    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        child: Column(
          children: [
            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 12, 20, 24),
                children: [
                  GestureDetector(
                    behavior: HitTestBehavior.opaque,
                    onTap: () => Navigator.of(context).pop(),
                    child: const Align(
                      alignment: Alignment.centerLeft,
                      child: Icon(Icons.arrow_back_ios_new,
                          size: 20, color: Color(0xFF1A1A1A)),
                    ),
                  ),
                  const SizedBox(height: 16),
                  const Text(
                    'Two-step Verification',
                    style: TextStyle(
                      fontSize: 20,
                      fontWeight: FontWeight.w800,
                      color: Color(0xFF1A1A1A),
                    ),
                  ),
                  const SizedBox(height: 2),
                  const Text(
                    'This adds extra security through email verification.',
                    style: TextStyle(fontSize: 13, color: Color(0xFF888888)),
                  ),
                  const SizedBox(height: 24),
                  const Text(
                    'Two-step Verification',
                    style: TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                      color: Color(0xFF1A1A1A),
                    ),
                  ),
                  const SizedBox(height: 4),
                  const Text(
                    'To turn on two-step verification, you must verify your email.',
                    style: TextStyle(fontSize: 13, color: Color(0xFF888888)),
                  ),
                  const SizedBox(height: 16),
                  Container(
                    padding: const EdgeInsets.symmetric(
                        horizontal: 16, vertical: 16),
                    decoration: BoxDecoration(
                      border: Border.all(color: const Color(0xFFEEEEEE)),
                      borderRadius: BorderRadius.circular(12),
                    ),
                    child: Row(
                      children: [
                        Expanded(
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              const Text(
                                'Email Address',
                                style: TextStyle(
                                  fontSize: 13,
                                  fontWeight: FontWeight.w700,
                                  color: Color(0xFF1A1A1A),
                                ),
                              ),
                              const SizedBox(height: 2),
                              Text(
                                _email,
                                style: const TextStyle(
                                  fontSize: 13,
                                  color: Color(0xFF888888),
                                ),
                              ),
                            ],
                          ),
                        ),
                        if (_emailStatus == _EmailStatus.unverified)
                          GestureDetector(
                            onTap: _onVerifyTap,
                            child: const Text(
                              'Verify',
                              style: TextStyle(
                                fontSize: 13,
                                fontWeight: FontWeight.w700,
                                color: Color(0xFF05124D),
                              ),
                            ),
                          )
                        else
                          const Text(
                            'Verified',
                            style: TextStyle(
                              fontSize: 13,
                              fontWeight: FontWeight.w600,
                              color: Color(0xFF05124D),
                            ),
                          ),
                      ],
                    ),
                  ),
                ],
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 0, 20, 16),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton(
                  onPressed: _canToggle ? _onToggle : null,
                  style: FilledButton.styleFrom(
                    backgroundColor: isOn
                        ? const Color(0xFFE53935)
                        : const Color(0xFF05124D),
                    disabledBackgroundColor:
                        const Color(0xFF05124D).withValues(alpha: 0.35),
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(999),
                    ),
                  ),
                  child: Text(
                    isOn ? 'Turn Off' : 'Turn On',
                    style: TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                      color: isOn ? Colors.white : Colors.white,
                    ),
                  ),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}