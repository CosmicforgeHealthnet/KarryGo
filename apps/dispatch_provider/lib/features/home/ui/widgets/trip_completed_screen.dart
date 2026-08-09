import 'dart:io';
import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:cosmicforge_logistics_ui_kit/cosmicforge_logistics_ui_kit.dart';
import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';

import '../home_screen.dart';
import '../../../../features/trips/state/trips_controller.dart';

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
  int _rating = 2;
  File? _proofFile;
  String? _proofFileName;
  final List<Offset> _signaturePoints = [];
  final GlobalKey _signaturePadKey = GlobalKey();
  bool _isSubmitting = false;
  String? _submitError;

  // Backend requires an actual image (JPEG/PNG) for the delivery photo.
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

  /// Rasterizes the drawn signature strokes to a PNG (white background),
  /// sized to the on-screen pad so coordinates match what was drawn.
  Future<Uint8List?> _renderSignaturePng() async {
    if (_signaturePoints.isEmpty) return null;
    final box =
        _signaturePadKey.currentContext?.findRenderObject() as RenderBox?;
    final size = box?.size ?? const Size(320, 140);
    final recorder = ui.PictureRecorder();
    final canvas = Canvas(recorder);
    canvas.drawRect(Offset.zero & size, Paint()..color = Colors.white);
    _SignaturePainter(points: _signaturePoints).paint(canvas, size);
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

      final code = await showModalBottomSheet<String>(
        context: context,
        isScrollControlled: true,
        shape: const RoundedRectangleBorder(
          borderRadius: BorderRadius.vertical(top: Radius.circular(20)),
        ),
        builder: (_) => const CosmicforgeLogisticsCodeEntrySheet(
          title: 'Verify the customer',
          subtitle: 'Scan the QR code on the customer\'s screen, or enter the code they read out.',
        ),
      );
      if (code == null || code.isEmpty || !mounted) return;

      setState(() {
        _isSubmitting = true;
        _submitError = null;
      });

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

      final completeResult = await controller.completeTrip(tripId);
      if (!mounted) return;

      completeResult.when(
        success: (_) {
          setState(() => _isSubmitting = false);
          widget.onConfirm();
        },
        failure: (err) => setState(() {
          _isSubmitting = false;
          _submitError = err.message;
        }),
      );
    } else {
      widget.onConfirm();
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        child: SingleChildScrollView(
          padding: const EdgeInsets.fromLTRB(16, 20, 16, 24),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const Text(
                'You have arrived',
                style: TextStyle(
                  fontSize: 18,
                  fontWeight: FontWeight.w800,
                  color: Color(0xFF1A1A1A),
                ),
              ),
              const SizedBox(height: 4),
              const Text(
                'You have reached your destination.',
                style: TextStyle(fontSize: 13, color: Color(0xFF888888)),
              ),
              const SizedBox(height: 12),
              Container(
                width: double.infinity,
                padding: const EdgeInsets.symmetric(
                  horizontal: 14,
                  vertical: 10,
                ),
                decoration: BoxDecoration(
                  color: const Color(0xFF4CAF50),
                  borderRadius: BorderRadius.circular(8),
                ),
                child: const Text(
                  'Ensure receiver confirms package immediately before you leave.',
                  style: TextStyle(
                    fontSize: 12,
                    color: Colors.white,
                    fontWeight: FontWeight.w600,
                  ),
                ),
              ),
              const SizedBox(height: 20),
              Center(
                child: Column(
                  children: [
                    Container(
                      width: 64,
                      height: 64,
                      decoration: const BoxDecoration(
                        color: Color(0xFFD0D0D0),
                        shape: BoxShape.circle,
                      ),
                      child: const Icon(
                        Icons.person,
                        size: 36,
                        color: Colors.white,
                      ),
                    ),
                    const SizedBox(height: 10),
                    Text(
                      widget.request.customerName,
                      style: const TextStyle(
                        fontSize: 22,
                        fontWeight: FontWeight.w800,
                        color: Color(0xFF1A1A1A),
                      ),
                    ),
                    const SizedBox(height: 6),
                    TripStats(request: widget.request),
                  ],
                ),
              ),
              const SizedBox(height: 16),
              const Divider(color: Color(0xFFF0F0F0)),
              const SizedBox(height: 12),
              _MetaRow(
                label: 'Booking ID:',
                value: widget.request.bookingId.isNotEmpty
                    ? widget.request.bookingId
                    : '—',
              ),
              const SizedBox(height: 8),
              _MetaRow(
                label: 'Date:',
                value: _formatDate(widget.request.createdAt),
              ),
              const SizedBox(height: 20),
              const Center(
                child: Text(
                  'Customer Review',
                  style: TextStyle(
                    fontSize: 14,
                    fontWeight: FontWeight.w700,
                    color: Color(0xFF1A1A1A),
                  ),
                ),
              ),
              const SizedBox(height: 10),
              Center(
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: List.generate(5, (i) {
                    return GestureDetector(
                      onTap: () => setState(() => _rating = i + 1),
                      child: Icon(
                        Icons.star,
                        size: 28,
                        color: i < _rating
                            ? const Color(0xFF4CAF50)
                            : const Color(0xFFCCCCCC),
                      ),
                    );
                  }),
                ),
              ),
              const SizedBox(height: 20),
              const Divider(color: Color(0xFFF0F0F0)),
              const SizedBox(height: 16),
              const Text(
                'Receiver Information',
                style: TextStyle(
                  fontSize: 16,
                  fontWeight: FontWeight.w800,
                  color: Color(0xFF1A1A1A),
                ),
              ),
              const SizedBox(height: 12),
              _InfoField(
                label: 'Full Name',
                value: widget.request.receiverName.isNotEmpty
                    ? widget.request.receiverName
                    : '—',
              ),
              const SizedBox(height: 10),
              _InfoField(
                label: 'Phone Number',
                value: widget.request.receiverPhone.isNotEmpty
                    ? widget.request.receiverPhone
                    : '—',
              ),
              if (widget.request.notes != null &&
                  widget.request.notes!.isNotEmpty) ...[
                const SizedBox(height: 20),
                const Text(
                  'Notes',
                  style: TextStyle(
                    fontSize: 16,
                    fontWeight: FontWeight.w800,
                    color: Color(0xFF1A1A1A),
                  ),
                ),
                const SizedBox(height: 8),
                Text(
                  widget.request.notes!,
                  style: const TextStyle(
                    fontSize: 13,
                    color: Color(0xFF555555),
                    height: 1.5,
                  ),
                ),
              ],

              // ── Proof of delivery ─────────────────────────────────────────
              if (widget.tripId != null) ...[
                const SizedBox(height: 20),
                const Divider(color: Color(0xFFF0F0F0)),
                const SizedBox(height: 16),
                const Text(
                  'Have you delivered the package?',
                  style: TextStyle(
                    fontSize: 16,
                    fontWeight: FontWeight.w800,
                    color: Color(0xFF1A1A1A),
                  ),
                ),
                const SizedBox(height: 6),
                const Text(
                  'Please upload a photo or PDF as proof of delivery.',
                  style: TextStyle(
                    fontSize: 12,
                    color: Color(0xFF888888),
                    height: 1.5,
                  ),
                ),
                const SizedBox(height: 16),
                const Text(
                  'Proof of completion',
                  style: TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w700,
                    color: Color(0xFF1A1A1A),
                  ),
                ),
                const SizedBox(height: 10),
                GestureDetector(
                  onTap: _isSubmitting ? null : _pickFile,
                  child: Container(
                    width: double.infinity,
                    padding: const EdgeInsets.all(16),
                    decoration: BoxDecoration(
                      color: _proofFile != null
                          ? const Color(0xFFE8F5E9)
                          : const Color(0xFFF5F5F5),
                      borderRadius: BorderRadius.circular(12),
                      border: Border.all(
                        color: _proofFile != null
                            ? const Color(0xFF4CAF50)
                            : const Color(0xFFE0E0E0),
                      ),
                    ),
                    child: Row(
                      children: [
                        Icon(
                          _proofFile != null
                              ? Icons.check_circle
                              : Icons.attach_file,
                          color: _proofFile != null
                              ? const Color(0xFF4CAF50)
                              : const Color(0xFF888888),
                          size: 28,
                        ),
                        const SizedBox(width: 12),
                        Expanded(
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              Text(
                                _proofFile != null
                                    ? _proofFileName ?? 'File selected'
                                    : 'Tap to upload a photo (JPEG/PNG)',
                                style: TextStyle(
                                  fontSize: 13,
                                  fontWeight: FontWeight.w600,
                                  color: _proofFile != null
                                      ? const Color(0xFF2E7D32)
                                      : const Color(0xFF555555),
                                ),
                              ),
                              if (_proofFile != null) ...[
                                const SizedBox(height: 2),
                                const Text(
                                  'Tap to change',
                                  style: TextStyle(
                                    fontSize: 11,
                                    color: Color(0xFF888888),
                                  ),
                                ),
                              ],
                            ],
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
                const SizedBox(height: 20),
                const Text(
                  'Receiver signature',
                  style: TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w700,
                    color: Color(0xFF1A1A1A),
                  ),
                ),
                const SizedBox(height: 6),
                const Text(
                  'For confirmation and security purposes, the receiver is '
                  'required to sign below.',
                  style: TextStyle(fontSize: 12, color: Color(0xFF888888)),
                ),
                const SizedBox(height: 10),
                Container(
                  height: 140,
                  decoration: BoxDecoration(
                    color: const Color(0xFFF5F5F5),
                    borderRadius: BorderRadius.circular(12),
                    border: Border.all(color: const Color(0xFFE0E0E0)),
                  ),
                  child: GestureDetector(
                    onPanStart: (d) =>
                        setState(() => _signaturePoints.add(d.localPosition)),
                    onPanUpdate: (d) =>
                        setState(() => _signaturePoints.add(d.localPosition)),
                    onPanEnd: (_) => setState(
                      () => _signaturePoints.add(const Offset(-1, -1)),
                    ),
                    child: CustomPaint(
                      key: _signaturePadKey,
                      painter: _SignaturePainter(points: _signaturePoints),
                      size: Size.infinite,
                    ),
                  ),
                ),
                if (_signaturePoints.isNotEmpty)
                  Align(
                    alignment: Alignment.centerRight,
                    child: TextButton(
                      onPressed: () =>
                          setState(() => _signaturePoints.clear()),
                      child: const Text(
                        'Clear signature',
                        style: TextStyle(
                          color: Color(0xFF888888),
                          fontSize: 12,
                        ),
                      ),
                    ),
                  ),
                if (_submitError != null) ...[
                  const SizedBox(height: 10),
                  Text(
                    _submitError!,
                    style: const TextStyle(
                      fontSize: 12,
                      color: Color(0xFFE53935),
                    ),
                  ),
                ],
              ],

              const SizedBox(height: 24),
              SizedBox(
                width: double.infinity,
                height: 52,
                child: FilledButton(
                  onPressed: _isSubmitting ? null : _confirm,
                  style: FilledButton.styleFrom(
                    backgroundColor: const Color(0xFF4CAF50),
                    disabledBackgroundColor: const Color(
                      0xFF4CAF50,
                    ).withValues(alpha: 0.5),
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(999),
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
                      : const Text(
                          'Confirm',
                          style: TextStyle(
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

  static String _formatDate(DateTime dt) {
    const months = [
      'Jan',
      'Feb',
      'Mar',
      'Apr',
      'May',
      'Jun',
      'Jul',
      'Aug',
      'Sep',
      'Oct',
      'Nov',
      'Dec',
    ];
    return '${dt.day.toString().padLeft(2, '0')} '
        '${months[dt.month - 1]} ${dt.year}';
  }
}

class _SignaturePainter extends CustomPainter {
  _SignaturePainter({required this.points});
  final List<Offset> points;

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = const Color(0xFF1A1A1A)
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

class _MetaRow extends StatelessWidget {
  const _MetaRow({required this.label, required this.value});
  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: [
        Text(
          label,
          style: const TextStyle(
            fontSize: 13,
            fontWeight: FontWeight.w700,
            color: Color(0xFF1A1A1A),
          ),
        ),
        Text(
          value,
          style: const TextStyle(fontSize: 13, color: Color(0xFF888888)),
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
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          label,
          style: const TextStyle(
            fontSize: 13,
            fontWeight: FontWeight.w700,
            color: Color(0xFF1A1A1A),
          ),
        ),
        const SizedBox(height: 2),
        Text(
          value,
          style: const TextStyle(fontSize: 13, color: Color(0xFF888888)),
        ),
      ],
    );
  }
}
