import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:image_picker/image_picker.dart';
import 'package:path/path.dart' as p;

import '../../verification/state/verification_controller.dart';
import 'selfie_review_screen.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Figma 06 — Selfie capture (dark full-screen)
// Pure black bg · Cancel top-left · white circular face frame ·
// white shutter. Shutter opens native camera; on return → review screen 07.
// ═══════════════════════════════════════════════════════════════════════════

class SelfieCaptureScreen extends StatefulWidget {
  const SelfieCaptureScreen({
    super.key,
    required this.verificationController,
  });

  final VerificationController verificationController;

  @override
  State<SelfieCaptureScreen> createState() => _SelfieCaptureScreenState();
}

class _SelfieCaptureScreenState extends State<SelfieCaptureScreen> {
  final _picker = ImagePicker();
  bool _busy = false;

  Future<void> _capture() async {
    if (_busy) return;
    setState(() => _busy = true);

    try {
      final x = await _picker.pickImage(
        source: ImageSource.camera,
        imageQuality: 85,
        maxWidth: 1920,
        maxHeight: 1920,
        preferredCameraDevice: CameraDevice.front,
      );
      if (!mounted) return;
      setState(() => _busy = false);
      if (x == null) return;

      final ext = p.extension(x.path).replaceFirst('.', '').toLowerCase();
      if (!const {'jpg', 'jpeg', 'png'}.contains(ext)) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('Unsupported file type: .$ext')),
        );
        return;
      }

      final r = await Navigator.of(context).push<bool>(
        MaterialPageRoute(
          builder: (_) => SelfieReviewScreen(
            verificationController: widget.verificationController,
            selfieFilePath: x.path,
          ),
        ),
      );
      if (!mounted) return;
      if (r == true) Navigator.of(context).pop(true);
      // r == false → user tapped Retake; stay on this screen.
    } catch (_) {
      if (!mounted) return;
      setState(() => _busy = false);
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text(
            'Camera unavailable. Please grant camera permission in Settings.',
          ),
        ),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    return AnnotatedRegion<SystemUiOverlayStyle>(
      value: SystemUiOverlayStyle.light,
      child: Scaffold(
        backgroundColor: Colors.black,
        body: SafeArea(
          child: Column(
            children: [
              // ── Cancel ──
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 8, 20, 0),
                child: Row(
                  children: [
                    GestureDetector(
                      onTap: () => Navigator.of(context).pop(false),
                      behavior: HitTestBehavior.opaque,
                      child: const Padding(
                        padding: EdgeInsets.symmetric(
                          horizontal: 4,
                          vertical: 8,
                        ),
                        child: Text(
                          'Cancel',
                          style: TextStyle(
                            fontSize: 15,
                            fontWeight: FontWeight.w600,
                            color: Colors.white,
                          ),
                        ),
                      ),
                    ),
                  ],
                ),
              ),

              const Spacer(),

              // ── Face frame ──
              Container(
                width: 280,
                height: 280,
                alignment: Alignment.center,
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  border: Border.all(
                    color: Colors.white,
                    width: 2,
                  ),
                ),
                child: Icon(
                  Icons.person_rounded,
                  size: 140,
                  color: Colors.white.withValues(alpha: 0.20),
                ),
              ),

              const SizedBox(height: 28),

              // ── Caption ──
              const Text(
                'Position your face in the circle',
                textAlign: TextAlign.center,
                style: TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w500,
                  color: Colors.white,
                ),
              ),

              const Spacer(),

              // ── Shutter ──
              Padding(
                padding: const EdgeInsets.only(bottom: 40),
                child: GestureDetector(
                  onTap: _capture,
                  behavior: HitTestBehavior.opaque,
                  child: Container(
                    width: 76,
                    height: 76,
                    alignment: Alignment.center,
                    decoration: const BoxDecoration(
                      shape: BoxShape.circle,
                      color: Colors.white,
                    ),
                    child: _busy
                        ? const SizedBox(
                            width: 26,
                            height: 26,
                            child: CircularProgressIndicator(
                              strokeWidth: 2.4,
                              color: Colors.black,
                            ),
                          )
                        : null,
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