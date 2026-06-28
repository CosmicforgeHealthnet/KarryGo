import 'dart:async';

import 'package:flutter/material.dart';

import '../../home/ui/widgets/provider_app_colors.dart';
import 'widgets/provider_profile_widgets.dart';

/// Face Verification (Figma 2142/2143) plus the submitted-success state
/// (Account option-3). The scan is a visual KYC gate — documents are already
/// submitted to the backend on the previous screen.
class ProviderFaceVerificationScreen extends StatefulWidget {
  const ProviderFaceVerificationScreen({super.key});

  @override
  State<ProviderFaceVerificationScreen> createState() => _ProviderFaceVerificationScreenState();
}

enum _FaceStep { idle, scanning, done }

class _ProviderFaceVerificationScreenState extends State<ProviderFaceVerificationScreen> {
  _FaceStep _step = _FaceStep.idle;
  int _percent = 0;
  Timer? _timer;

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }

  void _startScan() {
    setState(() {
      _step = _FaceStep.scanning;
      _percent = 0;
    });
    _timer = Timer.periodic(const Duration(milliseconds: 120), (t) {
      if (!mounted) {
        t.cancel();
        return;
      }
      setState(() => _percent += 5);
      if (_percent >= 100) {
        t.cancel();
        setState(() => _step = _FaceStep.done);
      }
    });
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(20, 16, 20, 24),
          child: _step == _FaceStep.done ? _buildDone() : _buildScan(),
        ),
      ),
    );
  }

  Widget _buildScan() {
    final scanning = _step == _FaceStep.scanning;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        ProviderProfileHeader(title: 'Face Verification', subtitle: 'Scan your face to verify your identity'),
        const SizedBox(height: 24),
        Text(
          scanning ? 'Please keep your face centered on the screen and facing forward.' : 'We will automatically detect the face!',
          textAlign: TextAlign.center,
          style: const TextStyle(color: kProviderText, fontSize: 14, height: 1.5),
        ),
        const SizedBox(height: 40),
        Expanded(
          child: Center(
            child: AspectRatio(
              aspectRatio: 0.82,
              child: CustomPaint(
                painter: _FaceFramePainter(),
                child: scanning
                    ? Container(
                        margin: const EdgeInsets.all(24),
                        decoration: BoxDecoration(
                          color: kProviderGreenTint,
                          borderRadius: BorderRadius.circular(24),
                        ),
                        child: const Icon(Icons.face_rounded, size: 96, color: kProviderGreen),
                      )
                    : const SizedBox.expand(),
              ),
            ),
          ),
        ),
        const SizedBox(height: 24),
        if (scanning)
          Column(
            children: [
              Text('$_percent%',
                  style: const TextStyle(color: kProviderText, fontSize: 18, fontWeight: FontWeight.w800)),
              const SizedBox(height: 4),
              const Text('Verifying your face...', style: TextStyle(color: kProviderMuted, fontSize: 13)),
              const SizedBox(height: 8),
            ],
          )
        else
          ProviderPrimaryButton(label: 'Verify Face', onPressed: _startScan),
      ],
    );
  }

  Widget _buildDone() {
    return Column(
      children: [
        const Spacer(),
        Container(
          width: 150,
          height: 150,
          decoration: BoxDecoration(
            color: kProviderGreenTint,
            shape: BoxShape.circle,
            border: Border.all(color: kProviderGreen, width: 3),
          ),
          child: const Icon(Icons.check_rounded, color: kProviderGreen, size: 72),
        ),
        const SizedBox(height: 28),
        const Text(
          'Identity Verification request Submitted!',
          textAlign: TextAlign.center,
          style: TextStyle(color: kProviderText, fontSize: 20, fontWeight: FontWeight.w800),
        ),
        const SizedBox(height: 10),
        const Text(
          'Your request has been submitted successfully, your documents are now being processed.',
          textAlign: TextAlign.center,
          style: TextStyle(color: kProviderMuted, fontSize: 13, height: 1.5),
        ),
        const SizedBox(height: 8),
        const Text(
          'You will receive a follow up mail on verification status.',
          textAlign: TextAlign.center,
          style: TextStyle(color: kProviderGreen, fontSize: 13, height: 1.5),
        ),
        const Spacer(),
        ProviderPrimaryButton(label: 'Okay', onPressed: () => Navigator.of(context).pop()),
      ],
    );
  }
}

/// Corner brackets + circle + arc framing guide (Figma 2142).
class _FaceFramePainter extends CustomPainter {
  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = kProviderGreen
      ..style = PaintingStyle.stroke
      ..strokeWidth = 3
      ..strokeCap = StrokeCap.round;

    const corner = 40.0;
    // Top-left
    canvas.drawLine(const Offset(0, corner), const Offset(0, 0), paint);
    canvas.drawLine(const Offset(0, 0), const Offset(corner, 0), paint);
    // Top-right
    canvas.drawLine(Offset(size.width - corner, 0), Offset(size.width, 0), paint);
    canvas.drawLine(Offset(size.width, 0), Offset(size.width, corner), paint);
    // Bottom-left
    canvas.drawLine(Offset(0, size.height - corner), Offset(0, size.height), paint);
    canvas.drawLine(Offset(0, size.height), Offset(corner, size.height), paint);
    // Bottom-right
    canvas.drawLine(Offset(size.width - corner, size.height), Offset(size.width, size.height), paint);
    canvas.drawLine(Offset(size.width, size.height), Offset(size.width, size.height - corner), paint);

    // Head circle + shoulders arc
    final cx = size.width / 2;
    canvas.drawCircle(Offset(cx, size.height * 0.34), size.width * 0.22, paint);
    final shoulders = Rect.fromCircle(center: Offset(cx, size.height * 0.92), radius: size.width * 0.30);
    canvas.drawArc(shoulders, 3.4, 2.5, false, paint);
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}
