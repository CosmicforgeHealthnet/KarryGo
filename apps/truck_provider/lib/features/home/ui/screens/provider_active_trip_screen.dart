import 'package:flutter/material.dart';
import 'package:image_picker/image_picker.dart';

import '../../../auth/models/provider_auth_models.dart';
import '../../state/provider_home_controller.dart';
import '../widgets/provider_app_colors.dart';
import '../widgets/provider_home_map.dart';
import '../widgets/provider_request_card.dart';

/// Full-screen active trip overlay (Figma 2036–2043).
/// Renders map + status-driven bottom sheet.
/// When "End Trip" is tapped the completion sub-screen is shown inline.
class ProviderActiveTripScreen extends StatefulWidget {
  const ProviderActiveTripScreen({
    super.key,
    required this.controller,
    required this.booking,
  });

  final ProviderHomeController controller;
  final ProviderBooking booking;

  @override
  State<ProviderActiveTripScreen> createState() => _ProviderActiveTripScreenState();
}

class _ProviderActiveTripScreenState extends State<ProviderActiveTripScreen> {
  bool _showCompletion = false;

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: widget.controller,
      builder: (context, _) {
        final state = widget.controller.state;
        final booking = state.activeBooking ?? widget.booking;

        if (_showCompletion || booking.status == 'delivered') {
          return _CompletionScreen(
            booking: booking,
            controller: widget.controller,
          );
        }

        return Stack(
          children: [
            // ─── Full-screen map ───────────────────────────────────
            const Positioned.fill(child: ProviderHomeMap()),

            // ─── Bottom sheet: trip info ───────────────────────────
            Positioned(
              left: 0,
              right: 0,
              bottom: 0,
              child: _TripSheet(
                booking: booking,
                isLoading: state.isLoading,
                error: state.error,
                onConfirmPickup: widget.controller.confirmPickup,
                onEndTrip: () => setState(() => _showCompletion = true),
                onCancel: () => widget.controller.cancelActiveTrip(),
              ),
            ),
          ],
        );
      },
    );
  }
}

// ─── Trip bottom sheet (Figma 2036 / 2037 / 2038) ────────────────────────────

class _TripSheet extends StatelessWidget {
  const _TripSheet({
    required this.booking,
    required this.isLoading,
    this.error,
    required this.onConfirmPickup,
    required this.onEndTrip,
    required this.onCancel,
  });

  final ProviderBooking booking;
  final bool isLoading;
  final String? error;
  final VoidCallback onConfirmPickup;
  final VoidCallback onEndTrip;
  final VoidCallback onCancel;

  @override
  Widget build(BuildContext context) {
    final status = booking.status;
    final bottomPadding = MediaQuery.of(context).padding.bottom;
    final distanceKm = booking.distanceKm;
    final estMinutes = distanceKm != null ? (distanceKm / 30 * 60).round() : null;

    final bool isArrived = status == 'arrived_at_pickup';
    final bool inProgress = status == 'picked_up' || status == 'en_route_delivery';

    final String heading = inProgress
        ? 'Trip has started...'
        : isArrived
            ? 'Arrived at Pickup'
            : 'Arriving at pick up in 1 minute';

    final String subtitle = inProgress
        ? 'You are on your way to your destination'
        : isArrived
            ? 'You have gotten to the pick-up point.'
            : 'You are on your way to the pick-up point!';

    return Container(
      decoration: const BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.vertical(top: Radius.circular(24)),
        boxShadow: [BoxShadow(color: Color(0x18000000), blurRadius: 24, offset: Offset(0, -6))],
      ),
      padding: EdgeInsets.fromLTRB(20, 24, 20, bottomPadding + 20),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // ─── Heading ─────────────────────────────────────────────
          Text(
            heading,
            style: const TextStyle(color: kProviderText, fontWeight: FontWeight.w800, fontSize: 18),
          ),
          const SizedBox(height: 2),
          Text(subtitle, style: const TextStyle(color: kProviderMuted, fontSize: 13)),
          const SizedBox(height: 14),

          // ─── Customer card ────────────────────────────────────────
          Row(
            children: [
              ProviderAvatar(name: booking.displayName, size: 44),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      booking.displayName,
                      style: const TextStyle(
                        color: kProviderText,
                        fontWeight: FontWeight.w800,
                        fontSize: 16,
                      ),
                    ),
                    const SizedBox(height: 4),
                    BookingMetricsRow(
                      distanceKm: distanceKm,
                      minutes: estMinutes,
                      fareNaira: booking.fareEstimateNaira,
                    ),
                  ],
                ),
              ),
              // Call button
              Container(
                width: 40,
                height: 40,
                decoration: const BoxDecoration(color: kProviderGreen, shape: BoxShape.circle),
                child: const Icon(Icons.phone_rounded, color: Colors.white, size: 18),
              ),
            ],
          ),
          const SizedBox(height: 14),

          // ─── Route ───────────────────────────────────────────────
          BookingRoute(
            pickupAddress: booking.pickupAddress,
            dropoffAddress: booking.dropoffAddress,
          ),

          if (inProgress) ...[
            const SizedBox(height: 16),
            _ProgressBar(),
          ],

          const SizedBox(height: 16),

          if (error != null) ...[
            Text(error!, style: const TextStyle(color: Colors.red, fontSize: 12)),
            const SizedBox(height: 8),
          ],

          // ─── Action buttons ───────────────────────────────────────
          if (inProgress)
            SizedBox(
              width: double.infinity,
              height: 52,
              child: FilledButton(
                onPressed: isLoading ? null : onEndTrip,
                style: FilledButton.styleFrom(
                  backgroundColor: const Color(0xFFE5484D),
                  shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(30)),
                ),
                child: isLoading
                    ? const SizedBox.square(
                        dimension: 20,
                        child: CircularProgressIndicator(strokeWidth: 2, color: Colors.white),
                      )
                    : const Text(
                        'End Trip',
                        style: TextStyle(color: Colors.white, fontWeight: FontWeight.w800, fontSize: 15),
                      ),
              ),
            )
          else
            Row(
              children: [
                Expanded(
                  child: RejectRequestButton(
                    label: 'Cancel Trip',
                    onPressed: isLoading ? null : onCancel,
                  ),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: _StartTripButton(
                    enabled: isArrived && !isLoading,
                    isLoading: isLoading,
                    onPressed: isArrived && !isLoading ? onConfirmPickup : null,
                  ),
                ),
              ],
            ),
        ],
      ),
    );
  }
}

// ─── Progress bar (Figma 2038) ────────────────────────────────────────────────

class _ProgressBar extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        Row(
          children: [
            Container(
              width: 12,
              height: 12,
              decoration: const BoxDecoration(color: kProviderGreen, shape: BoxShape.circle),
            ),
            Expanded(
              child: Container(height: 3, color: kProviderGreen),
            ),
            Container(
              width: 12,
              height: 12,
              decoration: BoxDecoration(
                color: Colors.white,
                shape: BoxShape.circle,
                border: Border.all(color: kProviderGreen, width: 2),
              ),
            ),
          ],
        ),
        const SizedBox(height: 6),
        const Row(
          mainAxisAlignment: MainAxisAlignment.spaceBetween,
          children: [
            Text('Pickup', style: TextStyle(color: kProviderMuted, fontSize: 11)),
            Text('Destination', style: TextStyle(color: kProviderMuted, fontSize: 11)),
          ],
        ),
      ],
    );
  }
}

// ─── Start Trip button (green when arrived, muted when en-route) ──────────────

class _StartTripButton extends StatelessWidget {
  const _StartTripButton({required this.enabled, required this.isLoading, this.onPressed});

  final bool enabled;
  final bool isLoading;
  final VoidCallback? onPressed;

  @override
  Widget build(BuildContext context) {
    return FilledButton(
      onPressed: onPressed,
      style: FilledButton.styleFrom(
        backgroundColor: enabled ? kProviderGreen : kProviderBorder,
        foregroundColor: enabled ? Colors.white : kProviderMuted,
        elevation: 0,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
        padding: const EdgeInsets.symmetric(vertical: 14),
      ),
      child: isLoading
          ? const SizedBox.square(
              dimension: 18,
              child: CircularProgressIndicator(strokeWidth: 2, color: Colors.white),
            )
          : Text(
              'Start Trip',
              style: TextStyle(
                fontWeight: FontWeight.w700,
                fontSize: 14,
                color: enabled ? Colors.white : kProviderMuted,
              ),
            ),
    );
  }
}

// ─── Completion / Proof screen (Figma 2040–2043) ─────────────────────────────

class _CompletionScreen extends StatefulWidget {
  const _CompletionScreen({required this.booking, required this.controller});

  final ProviderBooking booking;
  final ProviderHomeController controller;

  @override
  State<_CompletionScreen> createState() => _CompletionScreenState();
}

class _CompletionScreenState extends State<_CompletionScreen> {
  XFile? _photo;
  bool _pickingPhoto = false;
  final List<Offset> _signaturePoints = [];

  Future<void> _pickPhoto() async {
    if (_pickingPhoto) return;
    setState(() => _pickingPhoto = true);
    try {
      final picker = ImagePicker();
      final file = await picker.pickImage(source: ImageSource.camera, imageQuality: 80);
      if (file != null && mounted) setState(() => _photo = file);
    } finally {
      if (mounted) setState(() => _pickingPhoto = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final booking = widget.booking;
    final distanceKm = booking.distanceKm;
    final estMinutes = distanceKm != null ? (distanceKm / 30 * 60).round() : null;
    final bottomPadding = MediaQuery.of(context).padding.bottom;

    return AnimatedBuilder(
      animation: widget.controller,
      builder: (context, _) {
        final state = widget.controller.state;

        return Scaffold(
          backgroundColor: Colors.white,
          body: Stack(
            children: [
              // Map in background (peeking behind the sheet)
              const Positioned.fill(child: ProviderHomeMap()),

              // White scrollable sheet
              DraggableScrollableSheet(
                initialChildSize: 0.85,
                minChildSize: 0.6,
                maxChildSize: 1.0,
                builder: (context, scrollController) {
                  return Container(
                    decoration: const BoxDecoration(
                      color: Colors.white,
                      borderRadius: BorderRadius.vertical(top: Radius.circular(24)),
                    ),
                    child: SingleChildScrollView(
                      controller: scrollController,
                      padding: EdgeInsets.fromLTRB(20, 20, 20, bottomPadding + 24),
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Center(
                            child: Container(
                              width: 36,
                              height: 4,
                              decoration: BoxDecoration(
                                color: kProviderBorder,
                                borderRadius: BorderRadius.circular(2),
                              ),
                            ),
                          ),
                          const SizedBox(height: 16),

                          // ─── "You have arrived" heading ─────────────
                          const Text(
                            'You have arrived',
                            style: TextStyle(
                              color: kProviderText,
                              fontWeight: FontWeight.w800,
                              fontSize: 22,
                            ),
                          ),
                          const SizedBox(height: 4),
                          const Text(
                            'You have reached your destination.',
                            style: TextStyle(color: kProviderMuted, fontSize: 13),
                          ),
                          const SizedBox(height: 14),

                          // ─── Green warning banner ─────────────────────
                          Container(
                            width: double.infinity,
                            padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
                            decoration: BoxDecoration(
                              color: kProviderGreen,
                              borderRadius: BorderRadius.circular(8),
                            ),
                            child: const Text(
                              'Ensure receiver confirm package immediately before you leave.',
                              style: TextStyle(color: Colors.white, fontSize: 12, fontWeight: FontWeight.w600),
                              textAlign: TextAlign.center,
                            ),
                          ),
                          const SizedBox(height: 20),

                          // ─── Customer summary ─────────────────────────
                          Center(
                            child: Column(
                              children: [
                                ProviderAvatar(name: booking.displayName, size: 64),
                                const SizedBox(height: 10),
                                Text(
                                  booking.displayName,
                                  style: const TextStyle(
                                    color: kProviderText,
                                    fontWeight: FontWeight.w800,
                                    fontSize: 20,
                                  ),
                                ),
                                const SizedBox(height: 6),
                                BookingMetricsRow(
                                  distanceKm: distanceKm,
                                  minutes: estMinutes,
                                  fareNaira: booking.fareEstimateNaira,
                                ),
                              ],
                            ),
                          ),
                          const SizedBox(height: 20),

                          // ─── Trip reference + date ────────────────────
                          _InfoRow(label: 'Trip Completed:', value: booking.shortId),
                          const SizedBox(height: 10),
                          _InfoRow(label: 'Date:', value: _formatDate(booking.createdAt)),
                          const SizedBox(height: 24),

                          // ─── Truck Haul Information ───────────────────
                          const Text(
                            'Truck Haul Information',
                            style: TextStyle(
                              color: kProviderText,
                              fontWeight: FontWeight.w800,
                              fontSize: 18,
                            ),
                          ),
                          const SizedBox(height: 14),
                          _HaulRow(question: 'What are you moving?', answer: booking.packageContent.isNotEmpty ? booking.packageContent : '—'),
                          _HaulRow(
                            question: 'Load weight category',
                            helper: 'Let us know how heavy the item is.',
                            answer: booking.weightCategory.isNotEmpty ? booking.weightCategory : '${booking.cargoWeightKg} kg',
                          ),
                          _HaulRow(
                            question: 'Truck Type',
                            answer: booking.preferredTruckType.isNotEmpty ? booking.preferredTruckType : 'Select truck type',
                          ),
                          _HaulRow(
                            question: 'Do you need loaders?',
                            helper: 'Let us know if you need extra hands to help you load the truck.',
                            answer: booking.requiresHelpers ? 'Yes ( ${booking.helperCount} )' : 'No',
                          ),
                          const SizedBox(height: 8),

                          // ─── Have you completed the service? ──────────
                          const Text(
                            'Have you Completed the service?',
                            style: TextStyle(
                              color: kProviderText,
                              fontWeight: FontWeight.w800,
                              fontSize: 18,
                            ),
                          ),
                          const SizedBox(height: 4),
                          const Text(
                            'For confirmation and security purposes, receiver is required to append their signature.',
                            style: TextStyle(color: kProviderMuted, fontSize: 12, height: 1.4),
                          ),
                          const SizedBox(height: 20),

                          // ─── Proof of completion ──────────────────────
                          const Text(
                            'Proof of completion',
                            style: TextStyle(color: kProviderText, fontWeight: FontWeight.w700, fontSize: 14),
                          ),
                          const SizedBox(height: 4),
                          const Text(
                            'Please take a picture of the package.',
                            style: TextStyle(color: kProviderMuted, fontSize: 12),
                          ),
                          const SizedBox(height: 12),

                          // Photo section
                          Row(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              if (_photo != null) ...[
                                ClipRRect(
                                  borderRadius: BorderRadius.circular(10),
                                  child: Image.network(
                                    _photo!.path,
                                    width: 72,
                                    height: 72,
                                    fit: BoxFit.cover,
                                    errorBuilder: (context2, e, st) => Container(
                                      width: 72,
                                      height: 72,
                                      color: kProviderGreenTint,
                                      child: const Icon(Icons.image_rounded, color: kProviderGreen),
                                    ),
                                  ),
                                ),
                                const SizedBox(width: 12),
                              ],
                              Column(
                                crossAxisAlignment: CrossAxisAlignment.start,
                                children: [
                                  GestureDetector(
                                    onTap: _pickingPhoto ? null : _pickPhoto,
                                    child: Container(
                                      padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 12),
                                      decoration: BoxDecoration(
                                        color: kProviderGreen,
                                        borderRadius: BorderRadius.circular(10),
                                      ),
                                      child: _pickingPhoto
                                          ? const SizedBox.square(
                                              dimension: 16,
                                              child: CircularProgressIndicator(strokeWidth: 2, color: Colors.white),
                                            )
                                          : Row(
                                              mainAxisSize: MainAxisSize.min,
                                              children: const [
                                                Icon(Icons.camera_alt_rounded, color: Colors.white, size: 16),
                                                SizedBox(width: 8),
                                                Text(
                                                  'Take Photo',
                                                  style: TextStyle(
                                                    color: Colors.white,
                                                    fontWeight: FontWeight.w700,
                                                    fontSize: 13,
                                                  ),
                                                ),
                                              ],
                                            ),
                                    ),
                                  ),
                                ],
                              ),
                            ],
                          ),
                          const SizedBox(height: 20),

                          // ─── Signature pad ─────────────────────────────
                          const Text(
                            'Please Sign Below',
                            style: TextStyle(
                              color: kProviderText,
                              fontWeight: FontWeight.w700,
                              fontSize: 14,
                            ),
                            textAlign: TextAlign.center,
                          ),
                          const SizedBox(height: 10),
                          Container(
                            height: 140,
                            decoration: BoxDecoration(
                              color: Colors.white,
                              borderRadius: BorderRadius.circular(12),
                              border: Border.all(color: kProviderBorder, width: 1.5),
                            ),
                            child: ClipRRect(
                              borderRadius: BorderRadius.circular(12),
                              child: GestureDetector(
                                onPanStart: (d) {
                                  setState(() => _signaturePoints.add(d.localPosition));
                                },
                                onPanUpdate: (d) {
                                  setState(() => _signaturePoints.add(d.localPosition));
                                },
                                onPanEnd: (_) {
                                  setState(() => _signaturePoints.add(const Offset(-1, -1)));
                                },
                                child: CustomPaint(
                                  painter: _SignaturePainter(points: _signaturePoints),
                                  size: const Size(double.infinity, 140),
                                ),
                              ),
                            ),
                          ),
                          if (_signaturePoints.isNotEmpty)
                            TextButton(
                              onPressed: () => setState(() => _signaturePoints.clear()),
                              child: const Text('Clear signature', style: TextStyle(color: kProviderMuted, fontSize: 12)),
                            ),
                          const SizedBox(height: 20),

                          if (state.error != null) ...[
                            Text(
                              state.error!,
                              style: const TextStyle(color: Colors.red, fontSize: 13),
                              textAlign: TextAlign.center,
                            ),
                            const SizedBox(height: 12),
                          ],

                          // ─── Confirm button ────────────────────────────
                          SizedBox(
                            width: double.infinity,
                            height: 54,
                            child: FilledButton(
                              onPressed: state.isLoading ? null : widget.controller.confirmDelivery,
                              style: FilledButton.styleFrom(
                                backgroundColor: kProviderGreen,
                                shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(14)),
                              ),
                              child: state.isLoading
                                  ? const SizedBox.square(
                                      dimension: 20,
                                      child: CircularProgressIndicator(strokeWidth: 2, color: Colors.white),
                                    )
                                  : const Text(
                                      'Confirm',
                                      style: TextStyle(
                                        color: Colors.white,
                                        fontWeight: FontWeight.w800,
                                        fontSize: 16,
                                      ),
                                    ),
                            ),
                          ),
                        ],
                      ),
                    ),
                  );
                },
              ),
            ],
          ),
        );
      },
    );
  }

  String _formatDate(DateTime dt) {
    const days = ['Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday'];
    const months = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
    return '${days[dt.weekday - 1]}, ${dt.day} ${months[dt.month - 1]} ${dt.year}';
  }
}

class _InfoRow extends StatelessWidget {
  const _InfoRow({required this.label, required this.value});
  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(label, style: const TextStyle(color: kProviderText, fontWeight: FontWeight.w700, fontSize: 13)),
        const SizedBox(width: 12),
        Expanded(
          child: Text(
            value,
            style: const TextStyle(color: kProviderMuted, fontSize: 13),
            textAlign: TextAlign.end,
          ),
        ),
      ],
    );
  }
}

class _HaulRow extends StatelessWidget {
  const _HaulRow({required this.question, required this.answer, this.helper});
  final String question;
  final String answer;
  final String? helper;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(question, style: const TextStyle(color: kProviderText, fontWeight: FontWeight.w700, fontSize: 14)),
          if (helper != null) ...[
            const SizedBox(height: 2),
            Text(helper!, style: const TextStyle(color: kProviderMuted, fontSize: 12, height: 1.3)),
          ],
          const SizedBox(height: 3),
          Text(answer, style: const TextStyle(color: kProviderText, fontWeight: FontWeight.w700, fontSize: 14)),
        ],
      ),
    );
  }
}

// ─── Signature painter ────────────────────────────────────────────────────────

class _SignaturePainter extends CustomPainter {
  _SignaturePainter({required this.points});
  final List<Offset> points;

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = const Color(0xFF1A1A1A)
      ..strokeWidth = 2.5
      ..strokeCap = StrokeCap.round
      ..style = PaintingStyle.stroke;

    for (var i = 0; i < points.length - 1; i++) {
      if (points[i] != const Offset(-1, -1) && points[i + 1] != const Offset(-1, -1)) {
        canvas.drawLine(points[i], points[i + 1], paint);
      }
    }
  }

  @override
  bool shouldRepaint(_SignaturePainter oldDelegate) => true;
}
