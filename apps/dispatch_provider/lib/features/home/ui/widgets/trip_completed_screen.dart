import 'dart:async';
import 'dart:io';
import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';

import '../home_screen.dart';
import '../../../../core/theme/theme_extensions.dart';
import '../../../../features/trips/state/trips_controller.dart';
import 'trip_stats.dart';
import 'customer_verification_sheet.dart';

class TripCompletedScreen extends StatefulWidget {
  const TripCompletedScreen({
    super.key,
    required this.request,
    required this.onConfirm,
    this.tripId,
    this.tripsController,
  });

  final RequestModel request;
  final VoidCallback onConfirm;
  final String? tripId;
  final TripsController? tripsController;

  @override
  State<TripCompletedScreen> createState() => _TripCompletedScreenState();
}

class _TripCompletedScreenState extends State<TripCompletedScreen> {
  int _rating = 0;
  File? _proofFile;
  String? _proofFileName;
  final List<Offset> _signaturePoints = [];
  final GlobalKey _signaturePadKey = GlobalKey();
  bool _isSubmitting = false;
  bool _isSigning = false;
  String? _submitError;

  bool get _needsHandover => widget.tripId != null;

  Future<void> _pickFile() async {
    final result = await FilePicker.platform.pickFiles(
      type: FileType.custom,
      allowedExtensions: ['jpg', 'jpeg', 'png'],
    );
    if (result == null || result.files.isEmpty) return;
    final file = result.files.first;
    if (file.path == null) return;
    setState(() {
      _proofFile = File(file.path!);
      _proofFileName = file.name;
      _submitError = null;
    });
  }

  Future<Uint8List?> _renderSignaturePng() async {
    if (_signaturePoints.isEmpty) return null;
    final box =
        _signaturePadKey.currentContext?.findRenderObject() as RenderBox?;
    final size = box?.size ?? const Size(320, 140);
    final recorder = ui.PictureRecorder();
    final canvas = Canvas(recorder);
    canvas.drawRect(Offset.zero & size, Paint()..color = Colors.white);
    _SignaturePainter(
      points: _signaturePoints,
      inkColor: const Color(0xFF081F44),
    ).paint(canvas, size);
    final image = await recorder
        .endRecording()
        .toImage(size.width.ceil(), size.height.ceil());
    final data = await image.toByteData(format: ui.ImageByteFormat.png);
    return data?.buffer.asUint8List();
  }

  Future<void> _confirm() async {
    final tripId = widget.tripId;
    final controller = widget.tripsController;

    if (tripId != null && controller != null) {
      if (_proofFile == null) {
        setState(
          () => _submitError =
              'Please attach proof of delivery before confirming.',
        );
        return;
      }
      final signaturePng = await _renderSignaturePng();
      if (!mounted) return;
      if (signaturePng == null) {
        setState(
          () => _submitError =
              'Please have the receiver sign before confirming.',
        );
        return;
      }
      if (widget.request.receiverName.isEmpty ||
          widget.request.receiverPhone.isEmpty) {
        setState(
          () => _submitError =
              'Receiver details are missing for this trip. Contact support.',
        );
        return;
      }

      final code = await showCustomerVerificationSheet(context);
      if (code == null || code.isEmpty || !mounted) return;

      setState(() {
        _isSubmitting = true;
        _submitError = null;
      });

      // ── Step 1: submit proof (photo + signature + code) ────────────────
      final proofResult = await controller.submitProof(
        id: tripId,
        photoPath: _proofFile!.path,
        signaturePng: signaturePng,
        receiverName: widget.request.receiverName,
        receiverPhone: widget.request.receiverPhone,
        code: code,
      );
      if (!mounted) return;
      bool proofOk = false;
      proofResult.when(
        success: (_) => proofOk = true,
        failure: (err) => setState(() => _submitError = err.message),
      );
      if (!proofOk) {
        setState(() => _isSubmitting = false);
        return;
      }

      // ── Step 2: confirm recipient (secure-handover authority) ──────────
      // On legacy trips this is a no-op success. On secure-handover trips
      // it's the gate that completeTrip checks. 404 means the deployment
      // doesn't have the route — treat as not required.
      final confirmResult = await controller.confirmRecipient(
        id: tripId,
        verificationMethod: 'code',
        recipientContact: widget.request.receiverPhone,
        expectedSourceVersion: 0,
        idempotencyKey:
            'confirm-$tripId-${DateTime.now().microsecondsSinceEpoch}',
      );
      if (!mounted) return;
      bool confirmOk = true;
      confirmResult.when(
        success: (_) {},
        failure: (err) {
          if (err.statusCode == 404) {
            debugPrint(
              '[PROOF] recipient-confirmation route not present; continuing.',
            );
            return;
          }
          confirmOk = false;
          setState(() => _submitError = err.message.isNotEmpty
              ? err.message
              : 'Recipient confirmation failed.');
        },
      );
      if (!confirmOk) {
        setState(() => _isSubmitting = false);
        return;
      }

      // ── Step 3: complete trip ──────────────────────────────────────────
      final completeResult = await controller.completeTrip(tripId);
      if (!mounted) return;
      bool completeOk = false;
      completeResult.when(
        success: (_) => completeOk = true,
        failure: (err) => setState(() {
          _isSubmitting = false;
          _submitError = err.message;
        }),
      );
      if (!completeOk) return;

      // ── Step 4: rate the customer (fire-and-forget) ────────────────────
      // The rating was captured on this screen. Send it now, before we pop.
      // Failure here must not block the rider from finishing.
      if (_rating > 0) {
        unawaited(
          controller.rateCustomer(id: tripId, score: _rating),
        );
      }

      setState(() => _isSubmitting = false);
      widget.onConfirm();
    } else {
      widget.onConfirm();
    }
  }

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    final isDark = context.isDark;

    final padBg = isDark ? const Color(0xFF1F232B) : Colors.white;
    final padInk = isDark ? const Color(0xFFE8ECF2) : const Color(0xFF081F44);
    final padPlaceholder = isDark
        ? const Color(0xFF6B7280)
        : const Color(0xFFA7B0B8);

    return Scaffold(
      backgroundColor: colors.background,
      body: SafeArea(
        child: SingleChildScrollView(
          physics: _isSigning
              ? const NeverScrollableScrollPhysics()
              : const AlwaysScrollableScrollPhysics(),
          padding: const EdgeInsets.fromLTRB(20, 24, 20, 32),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              // ── Hero ───────────────────────────────────────────────
              Center(
                child: Column(
                  children: [
                    Container(
                      width: 64,
                      height: 64,
                      alignment: Alignment.center,
                      decoration: BoxDecoration(
                        color: colors.teal10,
                        shape: BoxShape.circle,
                      ),
                      child: Icon(
                        Icons.done_all_rounded,
                        size: 34,
                        color: colors.brandTeal500,
                      ),
                    ),
                    const SizedBox(height: 16),
                    Text(
                      'Complete your delivery',
                      textAlign: TextAlign.center,
                      style: TextStyle(
                        fontSize: 22,
                        fontWeight: FontWeight.w800,
                        color: colors.text,
                        letterSpacing: -0.6,
                        height: 1.15,
                      ),
                    ),
                    const SizedBox(height: 6),
                    Text(
                      "Collect proof and the receiver's signature to finish up.",
                      textAlign: TextAlign.center,
                      style: TextStyle(
                        fontSize: 13.5,
                        fontWeight: FontWeight.w400,
                        color: colors.textMuted,
                        height: 1.45,
                      ),
                    ),
                  ],
                ),
              ),

              const SizedBox(height: 24),

              // ── Info banner ────────────────────────────────────────
              if (_needsHandover)
                Container(
                  width: double.infinity,
                  padding: const EdgeInsets.symmetric(
                    horizontal: 14,
                    vertical: 12,
                  ),
                  decoration: BoxDecoration(
                    color: colors.warningSoft,
                    borderRadius: BorderRadius.circular(14),
                    border: Border.all(
                      color: colors.warning.withValues(alpha: 0.35),
                    ),
                  ),
                  child: Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Icon(
                        Icons.info_outline_rounded,
                        size: 18,
                        color: colors.warning,
                      ),
                      const SizedBox(width: 10),
                      Expanded(
                        child: Text(
                          'Ensure the receiver confirms the package '
                          'immediately before you leave.',
                          style: TextStyle(
                            fontSize: 12.5,
                            height: 1.4,
                            fontWeight: FontWeight.w600,
                            color: colors.warning,
                          ),
                        ),
                      ),
                    ],
                  ),
                ),

              if (_needsHandover) const SizedBox(height: 20),

              // ── Delivery summary card ─────────────────────────────
              Container(
                padding: const EdgeInsets.all(16),
                decoration: BoxDecoration(
                  color: colors.surface,
                  borderRadius: BorderRadius.circular(16),
                  border: Border.all(color: colors.divider),
                ),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        Container(
                          width: 44,
                          height: 44,
                          alignment: Alignment.center,
                          decoration: BoxDecoration(
                            color: colors.navy10,
                            shape: BoxShape.circle,
                          ),
                          child: Icon(
                            Icons.person_rounded,
                            size: 24,
                            color: colors.textMuted,
                          ),
                        ),
                        const SizedBox(width: 12),
                        Expanded(
                          child: Text(
                            widget.request.customerName,
                            style: TextStyle(
                              fontSize: 16,
                              fontWeight: FontWeight.w700,
                              color: colors.text,
                              letterSpacing: -0.2,
                            ),
                          ),
                        ),
                      ],
                    ),
                    const SizedBox(height: 12),
                    TripStats(request: widget.request),
                    const SizedBox(height: 12),
                    Container(height: 1, color: colors.divider),
                    const SizedBox(height: 12),
                    _MetaRow(
                      label: 'Booking ID',
                      value: widget.request.bookingId.isNotEmpty
                          ? widget.request.bookingId
                          : '—',
                    ),
                    const SizedBox(height: 6),
                    _MetaRow(
                      label: 'Date',
                      value: _formatDate(widget.request.createdAt),
                    ),
                  ],
                ),
              ),

              // ── Receiver info ─────────────────────────────────────
              if (widget.request.receiverName.isNotEmpty ||
                  widget.request.receiverPhone.isNotEmpty) ...[
                const SizedBox(height: 20),
                _SectionLabel(label: 'Receiver', colors: colors),
                const SizedBox(height: 10),
                Container(
                  padding: const EdgeInsets.all(16),
                  decoration: BoxDecoration(
                    color: colors.surface,
                    borderRadius: BorderRadius.circular(16),
                    border: Border.all(color: colors.divider),
                  ),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      _InfoField(
                        label: 'Full name',
                        value: widget.request.receiverName.isNotEmpty
                            ? widget.request.receiverName
                            : '—',
                      ),
                      const SizedBox(height: 12),
                      _InfoField(
                        label: 'Phone number',
                        value: widget.request.receiverPhone.isNotEmpty
                            ? widget.request.receiverPhone
                            : '—',
                      ),
                    ],
                  ),
                ),
              ],

              // ── Notes ─────────────────────────────────────────────
              if (widget.request.notes != null &&
                  widget.request.notes!.isNotEmpty) ...[
                const SizedBox(height: 20),
                _SectionLabel(label: 'Notes', colors: colors),
                const SizedBox(height: 10),
                Container(
                  width: double.infinity,
                  padding: const EdgeInsets.all(16),
                  decoration: BoxDecoration(
                    color: colors.surface,
                    borderRadius: BorderRadius.circular(16),
                    border: Border.all(color: colors.divider),
                  ),
                  child: Text(
                    widget.request.notes!,
                    style: TextStyle(
                      fontSize: 13.5,
                      color: colors.textMuted,
                      height: 1.5,
                    ),
                  ),
                ),
              ],

              // ── Proof of delivery ─────────────────────────────────
              if (_needsHandover) ...[
                const SizedBox(height: 24),
                _SectionLabel(
                  label: 'Proof of delivery',
                  colors: colors,
                ),
                const SizedBox(height: 6),
                Text(
                  'Upload a photo of the delivered package.',
                  style: TextStyle(
                    fontSize: 12.5,
                    height: 1.4,
                    color: colors.textMuted,
                  ),
                ),
                const SizedBox(height: 12),
                GestureDetector(
                  onTap: _isSubmitting ? null : _pickFile,
                  behavior: HitTestBehavior.opaque,
                  child: Container(
                    width: double.infinity,
                    padding: const EdgeInsets.all(16),
                    decoration: BoxDecoration(
                      color: _proofFile != null
                          ? colors.teal10
                          : colors.navy10,
                      borderRadius: BorderRadius.circular(16),
                      border: Border.all(
                        color: _proofFile != null
                            ? colors.brandTeal500.withValues(alpha: 0.5)
                            : colors.divider,
                        width: _proofFile != null ? 1.4 : 1,
                      ),
                    ),
                    child: Row(
                      children: [
                        Container(
                          width: 40,
                          height: 40,
                          alignment: Alignment.center,
                          decoration: BoxDecoration(
                            color: _proofFile != null
                                ? colors.brandTeal500
                                : colors.surface,
                            shape: BoxShape.circle,
                          ),
                          child: Icon(
                            _proofFile != null
                                ? Icons.check_rounded
                                : Icons.add_a_photo_outlined,
                            size: 20,
                            color: _proofFile != null
                                ? Colors.white
                                : colors.textMuted,
                          ),
                        ),
                        const SizedBox(width: 12),
                        Expanded(
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              Text(
                                _proofFile != null
                                    ? (_proofFileName ?? 'File selected')
                                    : 'Attach delivery photo',
                                style: TextStyle(
                                  fontSize: 14,
                                  fontWeight: FontWeight.w700,
                                  color: colors.text,
                                  letterSpacing: -0.1,
                                ),
                              ),
                              const SizedBox(height: 2),
                              Text(
                                _proofFile != null
                                    ? 'Tap to change'
                                    : 'JPEG or PNG · up to 10 MB',
                                style: TextStyle(
                                  fontSize: 12,
                                  color: colors.textMuted,
                                ),
                              ),
                            ],
                          ),
                        ),
                      ],
                    ),
                  ),
                ),

                // ── Signature ───────────────────────────────────────
                const SizedBox(height: 24),
                _SectionLabel(
                  label: 'Receiver signature',
                  colors: colors,
                ),
                const SizedBox(height: 6),
                Text(
                  'Have the receiver sign below to confirm handover.',
                  style: TextStyle(
                    fontSize: 12.5,
                    height: 1.4,
                    color: colors.textMuted,
                  ),
                ),
                const SizedBox(height: 12),
                Container(
                  height: 150,
                  decoration: BoxDecoration(
                    color: padBg,
                    borderRadius: BorderRadius.circular(16),
                    border: Border.all(
                      color: isDark
                          ? const Color(0xFF2A2D34)
                          : colors.divider,
                    ),
                  ),
                  child: ClipRRect(
                    borderRadius: BorderRadius.circular(16),
                    child: Listener(
                      onPointerDown: (e) {
                        setState(() {
                          _isSigning = true;
                          _signaturePoints.add(e.localPosition);
                        });
                      },
                      onPointerMove: (e) {
                        setState(
                          () => _signaturePoints.add(e.localPosition),
                        );
                      },
                      onPointerUp: (_) {
                        setState(() {
                          _signaturePoints.add(const Offset(-1, -1));
                          _isSigning = false;
                        });
                      },
                      onPointerCancel: (_) {
                        setState(() {
                          _signaturePoints.add(const Offset(-1, -1));
                          _isSigning = false;
                        });
                      },
                      child: Stack(
                        children: [
                          if (_signaturePoints.isEmpty)
                            Positioned.fill(
                              child: Center(
                                child: Text(
                                  'Sign here',
                                  style: TextStyle(
                                    fontSize: 13,
                                    fontWeight: FontWeight.w500,
                                    color: padPlaceholder,
                                  ),
                                ),
                              ),
                            ),
                          CustomPaint(
                            key: _signaturePadKey,
                            painter: _SignaturePainter(
                              points: _signaturePoints,
                              inkColor: padInk,
                            ),
                            size: Size.infinite,
                          ),
                        ],
                      ),
                    ),
                  ),
                ),
                if (_signaturePoints.isNotEmpty)
                  Align(
                    alignment: Alignment.centerRight,
                    child: TextButton(
                      onPressed: () => setState(
                        () => _signaturePoints.clear(),
                      ),
                      style: TextButton.styleFrom(
                        foregroundColor: colors.textMuted,
                        textStyle: const TextStyle(
                          fontSize: 12.5,
                          fontWeight: FontWeight.w600,
                        ),
                      ),
                      child: const Text('Clear signature'),
                    ),
                  ),

                // ── Rate this delivery ──────────────────────────────
                const SizedBox(height: 20),
                _SectionLabel(
                  label: 'Rate this delivery',
                  colors: colors,
                ),
                const SizedBox(height: 10),
                Row(
                  children: List.generate(5, (i) {
                    final filled = i < _rating;
                    return GestureDetector(
                      onTap: () => setState(() => _rating = i + 1),
                      behavior: HitTestBehavior.opaque,
                      child: Padding(
                        padding: const EdgeInsets.only(right: 8),
                        child: Icon(
                          filled
                              ? Icons.star_rounded
                              : Icons.star_outline_rounded,
                          size: 32,
                          color:
                              filled ? colors.warning : colors.navy40,
                        ),
                      ),
                    );
                  }),
                ),
              ],

              // ── Error ─────────────────────────────────────────────
              if (_submitError != null) ...[
                const SizedBox(height: 16),
                Container(
                  width: double.infinity,
                  padding: const EdgeInsets.symmetric(
                    horizontal: 14,
                    vertical: 12,
                  ),
                  decoration: BoxDecoration(
                    color: colors.dangerSoft,
                    borderRadius: BorderRadius.circular(12),
                  ),
                  child: Row(
                    children: [
                      Icon(
                        Icons.error_outline_rounded,
                        size: 18,
                        color: colors.danger,
                      ),
                      const SizedBox(width: 10),
                      Expanded(
                        child: Text(
                          _submitError!,
                          style: TextStyle(
                            fontSize: 12.5,
                            height: 1.4,
                            fontWeight: FontWeight.w600,
                            color: colors.danger,
                          ),
                        ),
                      ),
                    ],
                  ),
                ),
              ],

              // ── Confirm ───────────────────────────────────────────
              const SizedBox(height: 24),
              SizedBox(
                width: double.infinity,
                height: 54,
                child: FilledButton(
                  onPressed: _isSubmitting ? null : _confirm,
                  style: FilledButton.styleFrom(
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(16),
                    ),
                  ),
                  child: _isSubmitting
                      ? const SizedBox(
                          width: 22,
                          height: 22,
                          child: CircularProgressIndicator(
                            strokeWidth: 2.5,
                            color: Colors.white,
                          ),
                        )
                      : Text(
                          _needsHandover ? 'Confirm delivery' : 'Done',
                          style: const TextStyle(
                            fontSize: 15.5,
                            fontWeight: FontWeight.w700,
                            letterSpacing: -0.1,
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

  static String _formatDate(DateTime dt) {
    const months = [
      'Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun',
      'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec',
    ];
    return '${dt.day.toString().padLeft(2, '0')} '
        '${months[dt.month - 1]} ${dt.year}';
  }
}

// ── Small reused widgets ────────────────────────────────────────────────

class _SectionLabel extends StatelessWidget {
  const _SectionLabel({required this.label, required this.colors});
  final String label;
  final dynamic colors;

  @override
  Widget build(BuildContext context) {
    return Text(
      label,
      style: TextStyle(
        fontSize: 14.5,
        fontWeight: FontWeight.w800,
        color: colors.text,
        letterSpacing: -0.2,
      ),
    );
  }
}

class _MetaRow extends StatelessWidget {
  const _MetaRow({required this.label, required this.value});
  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Row(
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: [
        Text(
          label,
          style: TextStyle(
            fontSize: 13,
            fontWeight: FontWeight.w600,
            color: colors.textMuted,
          ),
        ),
        Text(
          value,
          style: TextStyle(
            fontSize: 13,
            fontWeight: FontWeight.w700,
            color: colors.text,
          ),
        ),
      ],
    );
  }
}

class _InfoField extends StatelessWidget {
  const _InfoField({required this.label, required this.value});
  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          label,
          style: TextStyle(
            fontSize: 11.5,
            fontWeight: FontWeight.w600,
            color: colors.textMuted,
            letterSpacing: 0.2,
          ),
        ),
        const SizedBox(height: 3),
        Text(
          value,
          style: TextStyle(
            fontSize: 14,
            fontWeight: FontWeight.w700,
            color: colors.text,
            letterSpacing: -0.1,
          ),
        ),
      ],
    );
  }
}

class _SignaturePainter extends CustomPainter {
  _SignaturePainter({required this.points, required this.inkColor});
  final List<Offset> points;
  final Color inkColor;

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = inkColor
      ..strokeWidth = 2.4
      ..strokeCap = StrokeCap.round
      ..style = PaintingStyle.stroke;
    for (var i = 0; i < points.length - 1; i++) {
      final a = points[i];
      final b = points[i + 1];
      if (a == const Offset(-1, -1) || b == const Offset(-1, -1)) continue;
      canvas.drawLine(a, b, paint);
    }
  }

  @override
  bool shouldRepaint(_SignaturePainter oldDelegate) => true;
}