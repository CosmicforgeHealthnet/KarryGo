import 'dart:io';
import 'dart:math' as math;
import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:cosmicforge_logistics_ui_kit/cosmicforge_logistics_ui_kit.dart';
import 'package:flutter/material.dart';
import 'package:image_picker/image_picker.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../../../core/format/money_format.dart';
import '../../../../core/widgets/karryam_ui.dart';
import '../../../auth/models/provider_auth_models.dart';
import '../../state/provider_home_controller.dart';
import '../widgets/provider_app_colors.dart';
import '../widgets/provider_home_map.dart';
import '../widgets/provider_request_card.dart';

/// Full-screen active delivery: the map with a status-driven sheet that
/// always offers exactly the next step. "Complete delivery" opens the proof
/// screen in place.
class ProviderActiveTripScreen extends StatefulWidget {
  const ProviderActiveTripScreen({
    super.key,
    required this.controller,
    required this.booking,
    this.onSos,
  });

  final ProviderHomeController controller;
  final ProviderBooking booking;

  /// Opens the emergency options; the SOS button is hidden when null.
  final VoidCallback? onSos;

  @override
  State<ProviderActiveTripScreen> createState() => _ProviderActiveTripScreenState();
}

class _ProviderActiveTripScreenState extends State<ProviderActiveTripScreen> {
  bool _showCompletion = false;

  Future<void> _confirmCancel() async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Cancel this trip?'),
        content: const Text(
          "The customer will be told you can't make it, and the booking goes back to the queue.",
          style: TextStyle(height: 1.45, color: kProviderMuted),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: const Text('Keep trip'),
          ),
          TextButton(
            onPressed: () => Navigator.of(context).pop(true),
            style: TextButton.styleFrom(foregroundColor: kProviderRejectText),
            child: const Text('Cancel trip'),
          ),
        ],
      ),
    );
    if (ok == true) await widget.controller.cancelActiveTrip();
  }

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
            onBack: booking.status == 'delivered' ? null : () => setState(() => _showCompletion = false),
          );
        }

        final topPadding = MediaQuery.of(context).padding.top;
        return Scaffold(
          body: Stack(
            children: [
              const Positioned.fill(child: ProviderHomeMap()),
              if (widget.onSos != null)
                Positioned(
                  top: topPadding + 12,
                  right: 16,
                  child: Material(
                    color: kProviderRejectText,
                    shape: const StadiumBorder(),
                    elevation: 3,
                    shadowColor: const Color(0x55AD263A),
                    child: InkWell(
                      customBorder: const StadiumBorder(),
                      onTap: widget.onSos,
                      child: const Padding(
                        padding: EdgeInsets.symmetric(horizontal: 16, vertical: 10),
                        child: Row(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            Icon(Icons.sos_rounded, color: Colors.white, size: 18),
                            SizedBox(width: 6),
                            Text(
                              'SOS',
                              style: TextStyle(color: Colors.white, fontWeight: FontWeight.w800, letterSpacing: 1),
                            ),
                          ],
                        ),
                      ),
                    ),
                  ),
                ),
              DraggableScrollableSheet(
                initialChildSize: booking.status == 'arrived_at_pickup' ? 0.78 : 0.6,
                minChildSize: 0.36,
                maxChildSize: 0.94,
                snap: true,
                builder: (context, scroll) => _TripSheet(
                  scrollController: scroll,
                  booking: booking,
                  isLoading: state.isLoading,
                  error: state.error,
                  currentLat: state.currentLat,
                  currentLng: state.currentLng,
                  pickupCredential: state.pickupCredential,
                  isResendingPickupCredential: state.isResendingPickupCredential,
                  onEnRoutePickup: widget.controller.markEnRoutePickup,
                  onArrived: widget.controller.markArrived,
                  onConfirmPickup: widget.controller.confirmPickup,
                  onResendPickupCredential: widget.controller.resendPickupCredential,
                  onEnRouteDelivery: widget.controller.markEnRouteDelivery,
                  onEndTrip: () => setState(() => _showCompletion = true),
                  onCancel: _confirmCancel,
                ),
              ),
            ],
          ),
        );
      },
    );
  }
}

// ─── Trip sheet ───────────────────────────────────────────────────────────────

class _TripSheet extends StatelessWidget {
  const _TripSheet({
    required this.scrollController,
    required this.booking,
    required this.isLoading,
    this.error,
    this.currentLat,
    this.currentLng,
    this.pickupCredential,
    required this.isResendingPickupCredential,
    required this.onEnRoutePickup,
    required this.onArrived,
    required this.onConfirmPickup,
    required this.onResendPickupCredential,
    required this.onEnRouteDelivery,
    required this.onEndTrip,
    required this.onCancel,
  });

  final ScrollController scrollController;
  final ProviderBooking booking;
  final bool isLoading;
  final String? error;
  final double? currentLat;
  final double? currentLng;
  final PickupCredential? pickupCredential;
  final bool isResendingPickupCredential;
  final VoidCallback onEnRoutePickup;
  final VoidCallback onArrived;
  final VoidCallback onConfirmPickup;
  final VoidCallback onResendPickupCredential;
  final VoidCallback onEnRouteDelivery;
  final VoidCallback onEndTrip;
  final VoidCallback onCancel;

  /// Cargo is aboard: from here the trip can only be completed, never
  /// cancelled from the app.
  bool get _cargoAboard => booking.status == 'picked_up' || booking.status == 'en_route_delivery';

  @override
  Widget build(BuildContext context) {
    final status = booking.status;
    final action = _actionFor(status);

    final heading = switch (status) {
      'accepted' => 'Trip assigned',
      'en_route_pickup' => 'On the way to pickup',
      'arrived_at_pickup' => booking.pickupVerified ? 'Ready to load' : "You're at pickup",
      'picked_up' => 'Cargo loaded',
      'en_route_delivery' => 'On the way to delivery',
      _ => 'Trip in progress',
    };

    final subtitle = switch (status) {
      'accepted' => 'Head to the pickup location when you are ready.',
      'en_route_pickup' => 'Tap "I\'ve arrived" when you reach the pickup point.',
      'arrived_at_pickup' => booking.pickupVerified
          ? 'The customer has verified you. Load the cargo, then start the trip.'
          : 'Let the customer verify your identity before loading.',
      'picked_up' => 'Everything aboard? Start the delivery.',
      'en_route_delivery' => 'Drive safely. Complete the delivery at the drop-off.',
      _ => 'Keep the customer updated on your progress.',
    };

    return DecoratedBox(
      decoration: const BoxDecoration(
        color: kCanvas,
        borderRadius: BorderRadius.vertical(top: Radius.circular(26)),
        boxShadow: [BoxShadow(color: Color(0x1F0B1F44), blurRadius: 24, offset: Offset(0, -6))],
      ),
      child: Column(
        children: [
          Expanded(
            child: ListView(
              controller: scrollController,
              padding: const EdgeInsets.fromLTRB(20, 10, 20, 16),
              children: [
                Center(
                  child: Container(
                    width: 40,
                    height: 4,
                    decoration: BoxDecoration(color: kProviderGreenSoft, borderRadius: BorderRadius.circular(4)),
                  ),
                ),
                const SizedBox(height: 16),
                Row(
                  children: [
                    const KStatusChip('Active delivery'),
                    const Spacer(),
                    Text(
                      '#${booking.shortId}',
                      style: const TextStyle(fontSize: 12, color: kProviderMuted),
                    ),
                  ],
                ),
                const SizedBox(height: 12),
                KHeading(title: heading, subtitle: subtitle),
                const SizedBox(height: 18),
                _StageTracker(status: status),
                if (_cargoAboard) ...[
                  const SizedBox(height: 14),
                  _ProgressBar(progress: _tripProgress(status)),
                ],
                if (status == 'arrived_at_pickup') ...[
                  const SizedBox(height: 16),
                  if (booking.pickupVerified)
                    const KNotice(
                      'The customer has verified you. You can load the cargo.',
                      tone: KTone.success,
                      icon: Icons.verified_rounded,
                    )
                  else
                    CosmicforgeLogisticsVerificationCodeCard(
                      title: 'Show this code to the customer',
                      subtitle: 'They will scan or enter it to verify you before pickup.',
                      code: pickupCredential?.code,
                      expiresAt: pickupCredential?.expiresAt,
                      onResend: onResendPickupCredential,
                      canResend: !isResendingPickupCredential,
                    ),
                ],
                const SizedBox(height: 16),
                KCard(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      BookingRoute(full: true, pickupAddress: booking.pickupAddress, dropoffAddress: booking.dropoffAddress),
                      const SizedBox(height: 14),
                      const Divider(),
                      const SizedBox(height: 12),
                      Row(
                        children: [
                          Expanded(
                            child: BookingMetricsRow(
                              distanceKm: booking.distanceKm,
                              minutes: booking.estimatedMinutes,
                              fareNaira: null,
                            ),
                          ),
                          Text(
                            formatNairaShort(booking.fareNaira),
                            style: const TextStyle(fontSize: 16, fontWeight: FontWeight.w700, color: kProviderText),
                          ),
                        ],
                      ),
                    ],
                  ),
                ),
                const SizedBox(height: 12),
                BookingCargoRow(booking: booking),
                if (booking.receiverName.trim().isNotEmpty) ...[
                  const SizedBox(height: 12),
                  _ContactCard(booking: booking),
                ],
                if (error != null) ...[
                  const SizedBox(height: 12),
                  KNotice(error!, tone: KTone.danger),
                ],
              ],
            ),
          ),
          _ActionBar(
            action: action,
            isLoading: isLoading,
            canCancel: !_cargoAboard,
            onCancel: onCancel,
          ),
        ],
      ),
    );
  }

  /// Maps the current booking status to the next forward transition. A status
  /// this app version doesn't know renders a disabled button instead of
  /// re-firing an already-passed transition against the server's status guard.
  _TripAction _actionFor(String status) {
    return switch (status) {
      'accepted' => _TripAction(label: 'Start driving', onPressed: onEnRoutePickup),
      'en_route_pickup' => _TripAction(label: "I've arrived", onPressed: onArrived),
      // The server refuses the pickup until the customer has verified the
      // driver, so the button waits for that instead of failing on tap.
      'arrived_at_pickup' => booking.pickupVerified
          ? _TripAction(label: 'Cargo loaded — start trip', onPressed: onConfirmPickup)
          : const _TripAction(label: 'Waiting for customer to verify', onPressed: null),
      'picked_up' => _TripAction(label: 'Start delivery', onPressed: onEnRouteDelivery),
      'en_route_delivery' => _TripAction(label: 'Complete delivery', onPressed: onEndTrip),
      _ => const _TripAction(label: 'Updating trip…', onPressed: null),
    };
  }

  /// Fraction (0..1) of the pickup→destination leg covered, from the live
  /// location; a stage estimate when there is no usable fix.
  double _tripProgress(String status) {
    final lat = currentLat;
    final lng = currentLng;
    final total = _haversineKm(booking.pickupLat, booking.pickupLng, booking.dropoffLat, booking.dropoffLng);
    if (lat != null && lng != null && total > 0.05) {
      final remaining = _haversineKm(lat, lng, booking.dropoffLat, booking.dropoffLng);
      return ((total - remaining) / total).clamp(0.0, 1.0);
    }
    return status == 'en_route_delivery' ? 0.5 : 0.0;
  }

  double _haversineKm(double lat1, double lng1, double lat2, double lng2) {
    const earthRadiusKm = 6371.0;
    final dLat = _toRad(lat2 - lat1);
    final dLng = _toRad(lng2 - lng1);
    final a = math.sin(dLat / 2) * math.sin(dLat / 2) +
        math.cos(_toRad(lat1)) * math.cos(_toRad(lat2)) * math.sin(dLng / 2) * math.sin(dLng / 2);
    return earthRadiusKm * 2 * math.atan2(math.sqrt(a), math.sqrt(1 - a));
  }

  double _toRad(double deg) => deg * math.pi / 180.0;
}

/// Four stages, filled up to the current one: Pickup · Loading · Delivery · Proof.
class _StageTracker extends StatelessWidget {
  const _StageTracker({required this.status});
  final String status;

  static const _labels = ['To pickup', 'Loading', 'To delivery', 'Proof'];

  int get _current => switch (status) {
        'accepted' || 'en_route_pickup' => 0,
        'arrived_at_pickup' => 1,
        'picked_up' || 'en_route_delivery' => 2,
        _ => 3,
      };

  @override
  Widget build(BuildContext context) {
    final current = _current;
    return Row(
      children: [
        for (var i = 0; i < _labels.length; i++) ...[
          if (i > 0) const SizedBox(width: 6),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                AnimatedContainer(
                  duration: const Duration(milliseconds: 250),
                  height: 4,
                  decoration: BoxDecoration(
                    color: i < current ? kTeal : (i == current ? kNavy : kProviderBorder),
                    borderRadius: BorderRadius.circular(4),
                  ),
                ),
                const SizedBox(height: 6),
                Text(
                  _labels[i],
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: TextStyle(
                    fontSize: 11,
                    fontWeight: i == current ? FontWeight.w700 : FontWeight.w500,
                    color: i <= current ? kProviderText : kProviderMuted,
                  ),
                ),
              ],
            ),
          ),
        ],
      ],
    );
  }
}

/// Receiver contact with a call button when a phone number was given.
class _ContactCard extends StatelessWidget {
  const _ContactCard({required this.booking});
  final ProviderBooking booking;

  @override
  Widget build(BuildContext context) {
    final phone = booking.receiverPhone.trim();
    return KCard(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
      child: Row(
        children: [
          KAvatar(name: booking.receiverName),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  booking.receiverName.trim(),
                  style: const TextStyle(fontSize: 14.5, fontWeight: FontWeight.w600),
                ),
                const SizedBox(height: 2),
                const Text('Receiver at delivery', style: TextStyle(fontSize: 12.5, color: kProviderMuted)),
              ],
            ),
          ),
          if (phone.isNotEmpty)
            KCircleButton(
              icon: Icons.call_rounded,
              tooltip: 'Call receiver',
              background: kTealTint,
              onTap: () => launchUrl(Uri(scheme: 'tel', path: phone)),
            ),
        ],
      ),
    );
  }
}

/// White footer holding the forward step and, before pickup, Cancel.
class _ActionBar extends StatelessWidget {
  const _ActionBar({
    required this.action,
    required this.isLoading,
    required this.canCancel,
    required this.onCancel,
  });

  final _TripAction action;
  final bool isLoading;
  final bool canCancel;
  final VoidCallback onCancel;

  @override
  Widget build(BuildContext context) {
    return DecoratedBox(
      decoration: const BoxDecoration(
        color: kCard,
        border: Border(top: BorderSide(color: kProviderBorder)),
      ),
      child: SafeArea(
        top: false,
        child: Padding(
          padding: const EdgeInsets.fromLTRB(20, 12, 20, 12),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              KPrimaryButton(
                label: action.label,
                arrow: action.onPressed != null,
                loading: isLoading,
                onPressed: action.onPressed,
              ),
              if (canCancel) ...[
                const SizedBox(height: 4),
                TextButton(
                  onPressed: isLoading ? null : onCancel,
                  style: TextButton.styleFrom(foregroundColor: kProviderRejectText),
                  child: const Text('Cancel trip'),
                ),
              ],
            ],
          ),
        ),
      ),
    );
  }
}

// ─── Progress bar ─────────────────────────────────────────────────────────────

class _ProgressBar extends StatelessWidget {
  const _ProgressBar({required this.progress});

  /// 0..1 fraction of the pickup→destination leg covered.
  final double progress;

  @override
  Widget build(BuildContext context) {
    final value = progress.clamp(0.0, 1.0);
    return KCard(
      padding: const EdgeInsets.fromLTRB(16, 14, 16, 12),
      child: Column(
        children: [
          SizedBox(
            height: 22,
            child: LayoutBuilder(
              builder: (context, constraints) {
                const marker = 22.0;
                final travel = (constraints.maxWidth - marker).clamp(0.0, double.infinity);
                return Stack(
                  alignment: Alignment.centerLeft,
                  children: [
                    Container(
                      height: 4,
                      decoration: BoxDecoration(color: kProviderBorder, borderRadius: BorderRadius.circular(4)),
                    ),
                    FractionallySizedBox(
                      widthFactor: value,
                      child: Container(
                        height: 4,
                        decoration: BoxDecoration(color: kTeal, borderRadius: BorderRadius.circular(4)),
                      ),
                    ),
                    Positioned(
                      left: travel * value,
                      child: Container(
                        width: marker,
                        height: marker,
                        decoration: BoxDecoration(
                          color: kNavy,
                          shape: BoxShape.circle,
                          border: Border.all(color: Colors.white, width: 2),
                        ),
                        child: const Icon(Icons.local_shipping_rounded, size: 11, color: Colors.white),
                      ),
                    ),
                  ],
                );
              },
            ),
          ),
          const SizedBox(height: 8),
          Row(
            children: [
              const Text('Pickup', style: TextStyle(color: kProviderMuted, fontSize: 11.5)),
              const Spacer(),
              Text(
                '${(value * 100).round()}% of the way',
                style: const TextStyle(color: kTealText, fontSize: 11.5, fontWeight: FontWeight.w600),
              ),
              const Spacer(),
              const Text('Drop-off', style: TextStyle(color: kProviderMuted, fontSize: 11.5)),
            ],
          ),
        ],
      ),
    );
  }
}

// ─── Trip action model ────────────────────────────────────────────────────────

/// The single forward step for the current status. A null [onPressed]
/// renders the button disabled (waiting on the customer or a poll).
class _TripAction {
  const _TripAction({required this.label, required this.onPressed});

  final String label;
  final VoidCallback? onPressed;
}

// ─── Proof of delivery ───────────────────────────────────────────────────────

class _CompletionScreen extends StatefulWidget {
  const _CompletionScreen({required this.booking, required this.controller, this.onBack});

  final ProviderBooking booking;
  final ProviderHomeController controller;
  final VoidCallback? onBack;

  @override
  State<_CompletionScreen> createState() => _CompletionScreenState();
}

class _CompletionScreenState extends State<_CompletionScreen> {
  XFile? _photo;
  bool _pickingPhoto = false;
  final List<Offset> _signaturePoints = [];
  final GlobalKey _signaturePadKey = GlobalKey();

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

  /// Confirms delivery: takes the customer's delivery code (scanned or typed)
  /// as the final step, then submits it with the photo and signature.
  Future<void> _confirm() async {
    final code = await showModalBottomSheet<String>(
      context: context,
      isScrollControlled: true,
      showDragHandle: false,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(24)),
      ),
      builder: (_) => const CosmicforgeLogisticsCodeEntrySheet(
        title: 'Verify the customer',
        subtitle: "Scan the QR code on the customer's screen, or enter the code they read out.",
      ),
    );
    if (code == null || code.isEmpty || !mounted) return;
    final signature = await _renderSignaturePng();
    await widget.controller.confirmDelivery(
      code: code,
      proofPhoto: _photo,
      signaturePng: signature,
    );
  }

  /// Rasterizes the drawn signature strokes to a PNG (white background), sized
  /// to the on-screen pad so coordinates match what the receiver drew.
  Future<Uint8List?> _renderSignaturePng() async {
    if (_signaturePoints.isEmpty) return null;
    final box = _signaturePadKey.currentContext?.findRenderObject() as RenderBox?;
    final size = box?.size ?? const Size(360, 160);
    final recorder = ui.PictureRecorder();
    final canvas = Canvas(recorder);
    canvas.drawRect(Offset.zero & size, Paint()..color = Colors.white);
    _SignaturePainter(points: _signaturePoints).paint(canvas, size);
    final image = await recorder.endRecording().toImage(size.width.ceil(), size.height.ceil());
    final data = await image.toByteData(format: ui.ImageByteFormat.png);
    return data?.buffer.asUint8List();
  }

  @override
  Widget build(BuildContext context) {
    final booking = widget.booking;

    return AnimatedBuilder(
      animation: widget.controller,
      builder: (context, _) {
        final state = widget.controller.state;
        return PopScope(
          canPop: widget.onBack == null,
          onPopInvokedWithResult: (didPop, _) {
            if (!didPop) widget.onBack?.call();
          },
          child: Scaffold(
            appBar: KTopBar(title: 'Proof of delivery', onBack: widget.onBack, showBack: widget.onBack != null),
            body: Column(
              children: [
                Expanded(
                  child: ListView(
                    padding: const EdgeInsets.fromLTRB(20, 8, 20, 24),
                    children: [
                      const KHeading(
                        eyebrow: 'Almost done',
                        title: "You've arrived.",
                        subtitle: 'Get the receiver to confirm the delivery before you leave.',
                      ),
                      const SizedBox(height: 18),
                      KCard(
                        child: Column(
                          children: [
                            BookingRoute(
                              full: true,
                              pickupAddress: booking.pickupAddress,
                              dropoffAddress: booking.dropoffAddress,
                            ),
                            const SizedBox(height: 14),
                            BookingCargoRow(booking: booking),
                            const SizedBox(height: 8),
                            KInfoRow('Trip', '#${booking.shortId}'),
                            if (booking.receiverName.trim().isNotEmpty)
                              KInfoRow('Receiver', booking.receiverName.trim()),
                            KInfoRow('Trip fare', formatNairaShort(booking.fareNaira), bold: true),
                          ],
                        ),
                      ),
                      const SizedBox(height: 22),
                      const _StepTitle(
                        number: 1,
                        title: 'Photo of the delivered cargo',
                        subtitle: 'Take a clear picture where it was left.',
                      ),
                      const SizedBox(height: 10),
                      _PhotoBox(photo: _photo, busy: _pickingPhoto, onTap: _pickPhoto),
                      const SizedBox(height: 22),
                      _StepTitle(
                        number: 2,
                        title: "Receiver's signature",
                        subtitle: 'Hand the phone to the receiver to sign below.',
                        trailing: _signaturePoints.isEmpty
                            ? null
                            : TextButton(
                                onPressed: () => setState(_signaturePoints.clear),
                                child: const Text('Clear'),
                              ),
                      ),
                      const SizedBox(height: 10),
                      Container(
                        height: 160,
                        decoration: BoxDecoration(
                          color: kCard,
                          borderRadius: BorderRadius.circular(18),
                          border: Border.all(color: kProviderBorder, width: 1.5),
                        ),
                        child: ClipRRect(
                          borderRadius: BorderRadius.circular(18),
                          child: Stack(
                            children: [
                              if (_signaturePoints.isEmpty)
                                const Center(
                                  child: Text(
                                    'Sign here',
                                    style: TextStyle(color: kProviderGreenSoft, fontSize: 18, fontWeight: FontWeight.w600),
                                  ),
                                ),
                              Positioned(
                                left: 20,
                                right: 20,
                                bottom: 34,
                                child: Container(height: 1, color: kProviderBorder),
                              ),
                              GestureDetector(
                                onPanStart: (d) => setState(() => _signaturePoints.add(d.localPosition)),
                                onPanUpdate: (d) => setState(() => _signaturePoints.add(d.localPosition)),
                                onPanEnd: (_) => setState(() => _signaturePoints.add(const Offset(-1, -1))),
                                child: CustomPaint(
                                  key: _signaturePadKey,
                                  painter: _SignaturePainter(points: _signaturePoints),
                                  size: const Size(double.infinity, 160),
                                ),
                              ),
                            ],
                          ),
                        ),
                      ),
                      const SizedBox(height: 22),
                      const _StepTitle(
                        number: 3,
                        title: "Customer's delivery code",
                        subtitle: "You'll scan or type the code from the customer's app next.",
                      ),
                      if (state.error != null) ...[
                        const SizedBox(height: 16),
                        KNotice(state.error!, tone: KTone.danger),
                      ],
                    ],
                  ),
                ),
                DecoratedBox(
                  decoration: const BoxDecoration(
                    color: kCard,
                    border: Border(top: BorderSide(color: kProviderBorder)),
                  ),
                  child: SafeArea(
                    top: false,
                    child: Padding(
                      padding: const EdgeInsets.fromLTRB(20, 12, 20, 12),
                      child: KPrimaryButton(
                        label: 'Verify code & complete',
                        icon: Icons.qr_code_scanner_rounded,
                        loading: state.isLoading,
                        onPressed: _confirm,
                      ),
                    ),
                  ),
                ),
              ],
            ),
          ),
        );
      },
    );
  }
}

/// Numbered step heading on the proof screen.
class _StepTitle extends StatelessWidget {
  const _StepTitle({required this.number, required this.title, required this.subtitle, this.trailing});
  final int number;
  final String title;
  final String subtitle;
  final Widget? trailing;

  @override
  Widget build(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Container(
          width: 24,
          height: 24,
          alignment: Alignment.center,
          decoration: const BoxDecoration(color: kNavy, shape: BoxShape.circle),
          child: Text('$number', style: const TextStyle(color: Colors.white, fontSize: 12, fontWeight: FontWeight.w700)),
        ),
        const SizedBox(width: 12),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(title, style: const TextStyle(fontSize: 15, fontWeight: FontWeight.w700, color: kProviderText)),
              const SizedBox(height: 2),
              Text(subtitle, style: const TextStyle(fontSize: 12.5, height: 1.4, color: kProviderMuted)),
            ],
          ),
        ),
        ?trailing,
      ],
    );
  }
}

/// Dashed capture area, or the taken photo with a retake action.
class _PhotoBox extends StatelessWidget {
  const _PhotoBox({required this.photo, required this.busy, required this.onTap});
  final XFile? photo;
  final bool busy;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final p = photo;
    if (p != null) {
      return ClipRRect(
        borderRadius: BorderRadius.circular(18),
        child: Stack(
          children: [
            Image.file(
              File(p.path),
              height: 180,
              width: double.infinity,
              fit: BoxFit.cover,
              errorBuilder: (_, _, _) => Container(
                height: 180,
                color: kTealTint,
                child: const Center(child: Icon(Icons.image_rounded, color: kTealText, size: 40)),
              ),
            ),
            Positioned(
              right: 10,
              bottom: 10,
              child: FilledButton.icon(
                onPressed: busy ? null : onTap,
                style: FilledButton.styleFrom(
                  minimumSize: const Size(0, 38),
                  backgroundColor: kCard,
                  foregroundColor: kProviderText,
                  shape: const StadiumBorder(),
                ),
                icon: const Icon(Icons.refresh_rounded, size: 18),
                label: const Text('Retake'),
              ),
            ),
          ],
        ),
      );
    }
    return InkWell(
      onTap: busy ? null : onTap,
      borderRadius: BorderRadius.circular(18),
      child: CustomPaint(
        painter: _DashedBorderPainter(),
        child: SizedBox(
          height: 128,
          width: double.infinity,
          child: Center(
            child: busy
                ? const CircularProgressIndicator()
                : const Column(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      KIconTile(Icons.photo_camera_outlined, size: 44),
                      SizedBox(height: 10),
                      Text('Take photo', style: TextStyle(fontWeight: FontWeight.w600, color: kProviderText)),
                    ],
                  ),
          ),
        ),
      ),
    );
  }
}

class _DashedBorderPainter extends CustomPainter {
  @override
  void paint(Canvas canvas, Size size) {
    final rrect = RRect.fromRectAndRadius(Offset.zero & size, const Radius.circular(18));
    final paint = Paint()
      ..color = kProviderGreenSoft
      ..style = PaintingStyle.stroke
      ..strokeWidth = 1.5;
    final path = Path()..addRRect(rrect);
    for (final metric in path.computeMetrics()) {
      var d = 0.0;
      while (d < metric.length) {
        canvas.drawPath(metric.extractPath(d, d + 6), paint);
        d += 11;
      }
    }
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}

// ─── Signature painter ────────────────────────────────────────────────────────

class _SignaturePainter extends CustomPainter {
  _SignaturePainter({required this.points});
  final List<Offset> points;

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = kNavy
      ..strokeWidth = 2.6
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
