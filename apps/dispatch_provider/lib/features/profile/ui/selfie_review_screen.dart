import 'dart:io';

import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
import '../../verification/state/verification_controller.dart';
import 'selfie_image_quality.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Figma 07 — Review your selfie
// Header (back · title · Help) · photo card · info chip ·
// Retake (outlined) / Use this photo (filled teal).
// ═══════════════════════════════════════════════════════════════════════════

class SelfieReviewScreen extends StatefulWidget {
  const SelfieReviewScreen({
    super.key,
    required this.verificationController,
    required this.selfieFilePath,
  });

  final VerificationController verificationController;
  final String selfieFilePath;

  @override
  State<SelfieReviewScreen> createState() => _SelfieReviewScreenState();
}

class _SelfieReviewScreenState extends State<SelfieReviewScreen> {
  bool _isSubmitting = false;

  Future<void> _useThisPhoto() async {
    if (_isSubmitting) return;
    setState(() => _isSubmitting = true);

    final quality = await SelfieImageQuality.check(widget.selfieFilePath);
    if (!mounted) return;
    if (!quality.isUsable) {
      setState(() => _isSubmitting = false);
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(quality.problem!)),
      );
      return;
    }

    final result = await widget.verificationController.submitFace(
      selfieFilePath: widget.selfieFilePath,
    );

    if (!mounted) return;
    setState(() => _isSubmitting = false);

    String? error;
    result.when(
      success: (_) {},
      failure: (e) =>
          error = e.fields.isNotEmpty ? e.fields.first.message : e.message,
    );

    if (error != null) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('Selfie upload failed: $error')),
      );
      return;
    }

    widget.verificationController.refreshStatus();
    // Bubble true up — capture screen forwards it to intro, hub sees success.
    Navigator.of(context).pop(true);
  }

  void _showCaptureHelp() {
    showDialog<void>(
      context: context,
      builder: (dialogContext) => AlertDialog(
        title: const Text('Selfie capture tips'),
        content: const Text(
          'Use the front camera in a well-lit room. Keep your full face '
          'visible, remove anything covering it, and hold the phone steady. '
          'Make sure the preview actually shows your face before submitting. '
          'If the preview is blank, tap Retake and reopen the camera.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(dialogContext).pop(),
            child: const Text('Got it'),
          ),
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;

    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        child: Column(
          children: [
            // ── Header: back · title · Help ──
            Padding(
              padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
              child: Row(
                children: [
                  GestureDetector(
                    onTap: () => Navigator.of(context).pop(false),
                    behavior: HitTestBehavior.opaque,
                    child: Padding(
                      padding: const EdgeInsets.all(8),
                      child: Icon(
                        Icons.arrow_back_rounded,
                        size: 22,
                        color: c.text,
                      ),
                    ),
                  ),
                  const SizedBox(width: 4),
                  Expanded(
                    child: Text(
                      'Review your selfie',
                      style: TextStyle(
                        fontSize: 16,
                        fontWeight: FontWeight.w700,
                        color: c.text,
                        letterSpacing: -0.2,
                      ),
                    ),
                  ),
                  GestureDetector(
                    onTap: _showCaptureHelp,
                    behavior: HitTestBehavior.opaque,
                    child: Padding(
                      padding: const EdgeInsets.all(8),
                      child: Text(
                        'Help',
                        style: TextStyle(
                          fontSize: 15,
                          fontWeight: FontWeight.w600,
                          color: c.brandTeal500,
                        ),
                      ),
                    ),
                  ),
                ],
              ),
            ),

            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 16, 20, 24),
                children: [
                  // ── Photo card ──
                  Center(
                    child: ClipRRect(
                      borderRadius: BorderRadius.circular(20),
                      child: Image.file(
                        File(widget.selfieFilePath),
                        width: 280,
                        height: 320,
                        fit: BoxFit.cover,
                        errorBuilder: (context, error, stackTrace) => Container(
                          width: 280,
                          height: 320,
                          color: c.surface,
                          alignment: Alignment.center,
                          child: const Text(
                            'Unable to display this selfie. Please retake it.',
                            textAlign: TextAlign.center,
                          ),
                        ),
                      ),
                    ),
                  ),

                  const SizedBox(height: 20),

                  // ── Info chip ──
                  Container(
                    padding: const EdgeInsets.symmetric(
                      horizontal: 14,
                      vertical: 12,
                    ),
                    decoration: BoxDecoration(
                      color: c.teal10,
                      borderRadius: BorderRadius.circular(12),
                    ),
                    child: Row(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Icon(
                          Icons.lock_outline_rounded,
                          size: 18,
                          color: c.brandTeal600,
                        ),
                        const SizedBox(width: 10),
                        Expanded(
                          child: Text(
                            'This photo will be securely compared with your ID.',
                            style: TextStyle(
                              fontSize: 13,
                              fontWeight: FontWeight.w500,
                              color: c.brandTeal600,
                              height: 1.4,
                            ),
                          ),
                        ),
                      ],
                    ),
                  ),

                  const SizedBox(height: 28),

                  // ── Retake / Use this photo ──
                  Row(
                    children: [
                      Expanded(
                        child: SizedBox(
                          height: 48,
                          child: OutlinedButton(
                            onPressed: _isSubmitting
                                ? null
                                : () => Navigator.of(context).pop(false),
                            style: OutlinedButton.styleFrom(
                              foregroundColor: c.text,
                              side: BorderSide(
                                color: c.border,
                                width: 1.2,
                              ),
                              shape: RoundedRectangleBorder(
                                borderRadius: BorderRadius.circular(999),
                              ),
                              textStyle: const TextStyle(
                                fontSize: 15,
                                fontWeight: FontWeight.w700,
                              ),
                            ),
                            child: const Text('Retake'),
                          ),
                        ),
                      ),
                      const SizedBox(width: 12),
                      Expanded(
                        child: SizedBox(
                          height: 48,
                          child: FilledButton(
                            onPressed:
                                _isSubmitting ? null : _useThisPhoto,
                            style: FilledButton.styleFrom(
                              textStyle: const TextStyle(
                                fontSize: 15,
                                fontWeight: FontWeight.w700,
                              ),
                            ),
                            child: _isSubmitting
                                ? SizedBox(
                                    width: 18,
                                    height: 18,
                                    child: CircularProgressIndicator(
                                      strokeWidth: 2,
                                      color: c.btnPrimaryFg,
                                    ),
                                  )
                                : const Text('Use this photo'),
                          ),
                        ),
                      ),
                    ],
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}