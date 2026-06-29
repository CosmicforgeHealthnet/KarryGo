import 'package:file_picker/file_picker.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import '../../verification/state/verification_controller.dart';

class FaceVerificationScreen extends StatefulWidget {
  const FaceVerificationScreen({
    super.key,
    required this.verificationController,
  });

  final VerificationController verificationController;

  @override
  State<FaceVerificationScreen> createState() => _FaceVerificationScreenState();
}

class _FaceVerificationScreenState extends State<FaceVerificationScreen> {
  _ScanState _state = _ScanState.idle;
  String? _errorMessage;
  bool _isSubmitting = false;
  bool _hasNavigatedAway = false;

  Future<void> _onVerifyFace() async {
    if (_isSubmitting) return;

    if (kDebugMode) debugPrint('[FACE] pick started');
    final result = await FilePicker.platform.pickFiles(
      type: FileType.custom,
      allowedExtensions: ['jpg', 'jpeg', 'png'],
      allowMultiple: false,
    );
    if (result == null || result.files.isEmpty) return;
    if (!mounted) return;
    final file = result.files.first;
    if (file.path == null) return;

    if (_isSubmitting) return;

    if (kDebugMode) debugPrint('[FACE] file selected: ${file.name}');

    setState(() {
      _isSubmitting = true;
      _state = _ScanState.scanning;
      _errorMessage = null;
    });

    if (kDebugMode) debugPrint('[FACE] submit started');
    final apiResult = await widget.verificationController.submitFace(
      selfieFilePath: file.path!,
    );

    if (!mounted) return;

    apiResult.when(
      success: (_) {
        if (kDebugMode) debugPrint('[FACE] submit success');
        setState(() {
          _isSubmitting = false;
          _state = _ScanState.success;
        });
      },
      failure: (error) {
        if (kDebugMode) debugPrint('[FACE] submit failed: ${error.message}');
        setState(() {
          _isSubmitting = false;
          _state = _ScanState.idle;
          _errorMessage = error.message;
        });
      },
    );
  }

  void _onOkay() {
    if (_hasNavigatedAway) return;
    _hasNavigatedAway = true;
    if (kDebugMode) debugPrint('[FACE] navigating once');
    Navigator.of(context).pop(true);
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        child: switch (_state) {
          _ScanState.idle => _IdleView(
            onVerify: _onVerifyFace,
            errorMessage: _errorMessage,
            isSubmitting: _isSubmitting,
          ),
          _ScanState.scanning => const _UploadingView(),
          _ScanState.success => _SuccessView(onOkay: _onOkay),
        },
      ),
    );
  }
}

enum _ScanState { idle, scanning, success }

// ── Idle ──────────────────────────────────────────────────────────────────────

class _IdleView extends StatelessWidget {
  const _IdleView({
    required this.onVerify,
    this.errorMessage,
    this.isSubmitting = false,
  });
  final VoidCallback onVerify;
  final String? errorMessage;
  final bool isSubmitting;

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(20, 12, 20, 0),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              GestureDetector(
                behavior: HitTestBehavior.opaque,
                onTap: () => Navigator.of(context).pop(),
                child: const Align(
                  alignment: Alignment.centerLeft,
                  child: Icon(
                    Icons.arrow_back_ios_new,
                    size: 20,
                    color: Color(0xFF1A1A1A),
                  ),
                ),
              ),
              const SizedBox(height: 16),
              const Text(
                'Face Verification',
                style: TextStyle(
                  fontSize: 20,
                  fontWeight: FontWeight.w800,
                  color: Color(0xFF1A1A1A),
                ),
              ),
              const SizedBox(height: 2),
              const Text(
                'Upload a clear selfie to verify your identity',
                style: TextStyle(fontSize: 13, color: Color(0xFF888888)),
              ),
            ],
          ),
        ),

        const SizedBox(height: 32),

        const Text(
          'Select a clear, front-facing selfie photo.',
          style: TextStyle(fontSize: 14, color: Color(0xFF1A1A1A)),
          textAlign: TextAlign.center,
        ),

        const SizedBox(height: 32),

        Expanded(
          child: Center(
            child: SizedBox(
              width: 260,
              height: 300,
              child: CustomPaint(
                painter: _CornerBracketPainter(),
                child: Center(
                  child: CustomPaint(
                    size: const Size(220, 260),
                    painter: _SilhouettePainter(),
                  ),
                ),
              ),
            ),
          ),
        ),

        if (errorMessage != null)
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 20),
            child: Text(
              errorMessage!,
              style: const TextStyle(color: Color(0xFFE53935), fontSize: 13),
              textAlign: TextAlign.center,
            ),
          ),

        const SizedBox(height: 8),

        Padding(
          padding: const EdgeInsets.fromLTRB(20, 0, 20, 16),
          child: SizedBox(
            height: 52,
            width: double.infinity,
            child: FilledButton(
              onPressed: isSubmitting ? null : onVerify,
              style: FilledButton.styleFrom(
                backgroundColor: const Color(0xFF4CAF50),
                disabledBackgroundColor: const Color(
                  0xFF4CAF50,
                ).withValues(alpha: 0.35),
                shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(999),
                ),
              ),
              child: isSubmitting
                  ? const SizedBox(
                      width: 20,
                      height: 20,
                      child: CircularProgressIndicator(
                        strokeWidth: 2,
                        color: Colors.white,
                      ),
                    )
                  : const Text(
                      'Select Selfie',
                      style: TextStyle(
                        fontSize: 16,
                        fontWeight: FontWeight.w700,
                        color: Colors.white,
                      ),
                    ),
            ),
          ),
        ),
      ],
    );
  }
}

// ── Uploading ─────────────────────────────────────────────────────────────────

class _UploadingView extends StatelessWidget {
  const _UploadingView();

  @override
  Widget build(BuildContext context) {
    return const Column(
      mainAxisAlignment: MainAxisAlignment.center,
      children: [
        CircularProgressIndicator(color: Color(0xFF4CAF50)),
        SizedBox(height: 24),
        Text(
          'Uploading selfie…',
          style: TextStyle(fontSize: 16, color: Color(0xFF1A1A1A)),
        ),
        SizedBox(height: 8),
        Text(
          'Please wait while we verify your face.',
          style: TextStyle(fontSize: 13, color: Color(0xFF888888)),
        ),
      ],
    );
  }
}

// ── Success ───────────────────────────────────────────────────────────────────

class _SuccessView extends StatelessWidget {
  const _SuccessView({required this.onOkay});
  final VoidCallback onOkay;

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        const Spacer(),
        Stack(
          alignment: Alignment.bottomRight,
          children: [
            Container(
              width: 110,
              height: 110,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                border: Border.all(color: const Color(0xFF4CAF50), width: 3),
                color: const Color(0xFFE8D5CC),
              ),
              child: ClipOval(child: Container(color: const Color(0xFFE8D5CC))),
            ),
            Container(
              width: 32,
              height: 32,
              decoration: const BoxDecoration(
                shape: BoxShape.circle,
                color: Color(0xFF4CAF50),
              ),
              child: const Icon(Icons.check, size: 18, color: Colors.white),
            ),
          ],
        ),
        const SizedBox(height: 28),
        const Text(
          'Identity Verification request Submitted!',
          style: TextStyle(
            fontSize: 18,
            fontWeight: FontWeight.w800,
            color: Color(0xFF1A1A1A),
          ),
          textAlign: TextAlign.center,
        ),
        const SizedBox(height: 10),
        const Padding(
          padding: EdgeInsets.symmetric(horizontal: 32),
          child: Text(
            'Your request has been submitted successfully, your documents are now being processed.',
            style: TextStyle(
              fontSize: 13,
              color: Color(0xFF888888),
              height: 1.5,
            ),
            textAlign: TextAlign.center,
          ),
        ),
        const SizedBox(height: 12),
        const Padding(
          padding: EdgeInsets.symmetric(horizontal: 32),
          child: Text(
            'You will receive a follow up mail on verification status.',
            style: TextStyle(
              fontSize: 13,
              color: Color(0xFF4CAF50),
              height: 1.5,
            ),
            textAlign: TextAlign.center,
          ),
        ),
        const Spacer(),
        Padding(
          padding: const EdgeInsets.fromLTRB(20, 0, 20, 16),
          child: SizedBox(
            height: 52,
            width: double.infinity,
            child: FilledButton(
              onPressed: onOkay,
              style: FilledButton.styleFrom(
                backgroundColor: const Color(0xFF4CAF50),
                shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(999),
                ),
              ),
              child: const Text(
                'Okay',
                style: TextStyle(
                  fontSize: 16,
                  fontWeight: FontWeight.w700,
                  color: Colors.white,
                ),
              ),
            ),
          ),
        ),
      ],
    );
  }
}

// ── Painters ──────────────────────────────────────────────────────────────────

class _CornerBracketPainter extends CustomPainter {
  const _CornerBracketPainter();
  static const double strokeWidth = 2.5;

  static const _bracketLength = 24.0;
  static const _radius = 10.0;
  static const _color = Color(0xFF4CAF50);
  static const _padding = 12.0;

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = _color
      ..strokeWidth = strokeWidth
      ..style = PaintingStyle.stroke
      ..strokeCap = StrokeCap.round;

    final l = _padding;
    final r = size.width - _padding;
    final t = _padding;
    final b = size.height - _padding;

    canvas.drawPath(
      Path()
        ..moveTo(l, t + _bracketLength)
        ..lineTo(l, t + _radius)
        ..arcToPoint(
          Offset(l + _radius, t),
          radius: const Radius.circular(_radius),
        )
        ..lineTo(l + _bracketLength, t),
      paint,
    );
    canvas.drawPath(
      Path()
        ..moveTo(r - _bracketLength, t)
        ..lineTo(r - _radius, t)
        ..arcToPoint(
          Offset(r, t + _radius),
          radius: const Radius.circular(_radius),
        )
        ..lineTo(r, t + _bracketLength),
      paint,
    );
    canvas.drawPath(
      Path()
        ..moveTo(l, b - _bracketLength)
        ..lineTo(l, b - _radius)
        ..arcToPoint(
          Offset(l + _radius, b),
          radius: const Radius.circular(_radius),
          clockwise: false,
        )
        ..lineTo(l + _bracketLength, b),
      paint,
    );
    canvas.drawPath(
      Path()
        ..moveTo(r - _bracketLength, b)
        ..lineTo(r - _radius, b)
        ..arcToPoint(
          Offset(r, b - _radius),
          radius: const Radius.circular(_radius),
          clockwise: false,
        )
        ..lineTo(r, b - _bracketLength),
      paint,
    );
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}

class _SilhouettePainter extends CustomPainter {
  static const _color = Color(0xFF4CAF50);

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = _color
      ..strokeWidth = 3.0
      ..style = PaintingStyle.stroke
      ..strokeCap = StrokeCap.round;

    final cx = size.width / 2;
    final headRadius = size.width * 0.22;
    final headCY = size.height * 0.33;
    canvas.drawCircle(Offset(cx, headCY), headRadius, paint);

    final shoulderRect = Rect.fromCenter(
      center: Offset(cx, size.height * 0.78),
      width: size.width * 0.78,
      height: size.height * 0.55,
    );
    canvas.drawArc(shoulderRect, 3.14, 3.14, false, paint);
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}
