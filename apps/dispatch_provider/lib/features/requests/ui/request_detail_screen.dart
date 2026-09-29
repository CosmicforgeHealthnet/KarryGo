import 'package:flutter/material.dart';
import 'package:flutter_map/flutter_map.dart';
import 'package:latlong2/latlong.dart' hide Path;

import '../../../core/theme/theme_extensions.dart';
import '../data/request_model.dart';

class RequestDetailScreen extends StatelessWidget {
  const RequestDetailScreen({
    super.key,
    required this.request,
    required this.onAccept,
    required this.onDecline,
    required this.onBrowseNew,
    required this.onBackToOpportunities,
  });

  final RequestModel request;
  final VoidCallback onAccept;
  final VoidCallback onDecline;
  final VoidCallback onBrowseNew;
  final VoidCallback onBackToOpportunities;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Scaffold(
      backgroundColor: colors.background,
      body: SafeArea(
        child: Column(
          children: [
            _buildTopBar(context),
            Expanded(
              child: SingleChildScrollView(
                padding: const EdgeInsets.fromLTRB(20, 8, 20, 24),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    if (request.lifecycle != RequestLifecycleState.available)
                      _buildLifecycleBanner(context),

                    if (request.hasMap) _buildMapCard(context),

                    if (request.hasFare) ...[
                      const SizedBox(height: 16),
                      _buildOfferSummary(context),
                    ],

                    if (request.hasRoute) ...[
                      const SizedBox(height: 16),
                      _buildRouteCard(context),
                    ],

                    if (request.hasNotes) ...[
                      const SizedBox(height: 20),
                      _buildNotesCard(context),
                    ],

                    if (request.hasSourceOrg) ...[
                      const SizedBox(height: 20),
                      _buildSourceOrgCard(context),
                    ],

                    if (request.hasHandling) ...[
                      const SizedBox(height: 20),
                      _buildHandlingCard(context),
                    ],

                    // ── Fallback detail for bare offers ─────────────
                    if (!request.hasFare &&
                        !request.hasRoute &&
                        !request.hasNotes &&
                        !request.hasSourceOrg) ...[
                      const SizedBox(height: 16),
                      _buildBareOfferCard(context),
                    ],

                    const SizedBox(height: 24),
                  ],
                ),
              ),
            ),
            _buildStickyActions(context),
          ],
        ),
      ),
    );
  }

  // ── Top bar ───────────────────────────────────────────────────────────

  Widget _buildTopBar(BuildContext context) {
    final colors = context.palette;
    return Padding(
      padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
      child: Stack(
        alignment: Alignment.center,
        children: [
          Align(
            alignment: Alignment.centerLeft,
            child: GestureDetector(
              onTap: () => Navigator.of(context).pop(),
              behavior: HitTestBehavior.opaque,
              child: Padding(
                padding: const EdgeInsets.all(8),
                child: Icon(
                  Icons.arrow_back_rounded,
                  size: 22,
                  color: colors.text,
                ),
              ),
            ),
          ),
          Text(
            'Opportunity details',
            style: TextStyle(
              fontSize: 16,
              fontWeight: FontWeight.w700,
              color: colors.text,
              letterSpacing: -0.2,
            ),
          ),
          Align(
            alignment: Alignment.centerRight,
            child: GestureDetector(
              onTap: () => Navigator.of(context).pop(),
              behavior: HitTestBehavior.opaque,
              child: Padding(
                padding: const EdgeInsets.all(8),
                child: Icon(
                  Icons.close_rounded,
                  size: 22,
                  color: colors.text,
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }

  // ── Lifecycle banner ──────────────────────────────────────────────────

  Widget _buildLifecycleBanner(BuildContext context) {
    final colors = context.palette;

    final (IconData icon, String title, String body, Color fg, Color bg) =
        switch (request.lifecycle) {
      RequestLifecycleState.expired => (
          Icons.timer_off_outlined,
          'This request has expired',
          'The offer window has closed. Check for new requests near you.',
          colors.danger,
          colors.dangerSoft,
        ),
      RequestLifecycleState.taken => (
          Icons.lock_clock_outlined,
          'No longer available',
          'This opportunity has been taken by another provider.',
          colors.warning,
          colors.warningSoft,
        ),
      RequestLifecycleState.requoted => (
          Icons.error_outline_rounded,
          'Offer updated',
          'This offer has a new rate. Review the details and choose an action.',
          colors.danger,
          colors.dangerSoft,
        ),
      _ => (
          Icons.info_outline_rounded,
          '',
          '',
          colors.textMuted,
          colors.navy10,
        ),
    };

    return Padding(
      padding: const EdgeInsets.only(bottom: 16),
      child: Container(
        padding: const EdgeInsets.all(14),
        decoration: BoxDecoration(
          color: bg,
          borderRadius: BorderRadius.circular(14),
          border: Border.all(color: fg.withValues(alpha: 0.3)),
        ),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Icon(icon, size: 20, color: fg),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    title,
                    style: TextStyle(
                      fontSize: 14.5,
                      fontWeight: FontWeight.w800,
                      color: fg,
                      letterSpacing: -0.2,
                    ),
                  ),
                  const SizedBox(height: 3),
                  Text(
                    body,
                    style: TextStyle(
                      fontSize: 12.5,
                      height: 1.4,
                      color: fg,
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }

  // ── Map card ──────────────────────────────────────────────────────────

  Widget _buildMapCard(BuildContext context) {
    final colors = context.palette;
    final pickup = LatLng(request.pickupLat!, request.pickupLng!);
    final dropoff = LatLng(request.dropoffLat!, request.dropoffLng!);
    final center = LatLng(
      (pickup.latitude + dropoff.latitude) / 2,
      (pickup.longitude + dropoff.longitude) / 2,
    );

    return ClipRRect(
      borderRadius: BorderRadius.circular(16),
      child: SizedBox(
        height: 220,
        child: Stack(
          children: [
            FlutterMap(
              options: MapOptions(
                initialCenter: center,
                initialZoom: 12.5,
                interactionOptions: const InteractionOptions(
                  flags: InteractiveFlag.none,
                ),
              ),
              children: [
                TileLayer(
                  urlTemplate: 'https://tile.openstreetmap.org/{z}/{x}/{y}.png',
                  userAgentPackageName:
                      'com.cosmicforge.logistics.dispatchprovider',
                ),
                PolylineLayer(
                  polylines: [
                    Polyline(
                      points: [pickup, dropoff],
                      color: colors.brandNavy900,
                      strokeWidth: 3.5,
                      pattern: StrokePattern.dashed(segments: const [10, 6]),
                    ),
                  ],
                ),
                MarkerLayer(
                  markers: [
                    Marker(
                      point: pickup,
                      width: 32,
                      height: 32,
                      child: _DotMarker(
                        color: colors.brandNavy900,
                        icon: Icons.radio_button_checked,
                      ),
                    ),
                    Marker(
                      point: dropoff,
                      width: 32,
                      height: 32,
                      child: _DotMarker(
                        color: colors.brandTeal500,
                        icon: Icons.place_rounded,
                      ),
                    ),
                  ],
                ),
              ],
            ),
            Positioned(
              top: 12,
              left: 12,
              child: Container(
                padding:
                    const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
                decoration: BoxDecoration(
                  color: colors.surface,
                  borderRadius: BorderRadius.circular(999),
                  boxShadow: colors.cardShadow,
                ),
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Icon(
                      Icons.podcasts_rounded,
                      size: 13,
                      color: colors.brandTeal600,
                    ),
                    const SizedBox(width: 5),
                    Text(
                      request.source.label,
                      style: TextStyle(
                        fontSize: 12,
                        fontWeight: FontWeight.w700,
                        color: colors.text,
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  // ── Offer summary (fare) ─────────────────────────────────────────────

  Widget _buildOfferSummary(BuildContext context) {
    final colors = context.palette;
    final minutes = request.minutesRemaining;
    final isUrgent = (minutes ?? 999) <= 5;

    return Container(
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
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      request.fareDisplayGrouped,
                      style: TextStyle(
                        fontSize: 28,
                        fontWeight: FontWeight.w800,
                        color: colors.text,
                        letterSpacing: -0.8,
                        height: 1.0,
                      ),
                    ),
                    if (request.isFareUpdate &&
                        request.previousFareDisplayGrouped != null) ...[
                      const SizedBox(height: 6),
                      Row(
                        children: [
                          Text(
                            request.previousFareDisplayGrouped!,
                            style: TextStyle(
                              fontSize: 13,
                              color: colors.textMuted,
                              decoration: TextDecoration.lineThrough,
                            ),
                          ),
                          const SizedBox(width: 6),
                          Container(
                            padding: const EdgeInsets.symmetric(
                              horizontal: 6,
                              vertical: 2,
                            ),
                            decoration: BoxDecoration(
                              color: colors.successSoft,
                              borderRadius: BorderRadius.circular(4),
                            ),
                            child: Text(
                              'Updated',
                              style: TextStyle(
                                fontSize: 10.5,
                                fontWeight: FontWeight.w700,
                                color: colors.success,
                              ),
                            ),
                          ),
                        ],
                      ),
                    ],
                  ],
                ),
              ),
              if (minutes != null &&
                  request.lifecycle == RequestLifecycleState.available)
                Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 10,
                    vertical: 6,
                  ),
                  decoration: BoxDecoration(
                    color: isUrgent ? colors.dangerSoft : colors.warningSoft,
                    borderRadius: BorderRadius.circular(999),
                  ),
                  child: Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Icon(
                        Icons.access_time_rounded,
                        size: 13,
                        color: isUrgent ? colors.danger : colors.warning,
                      ),
                      const SizedBox(width: 4),
                      Text(
                        '$minutes min left',
                        style: TextStyle(
                          fontSize: 11.5,
                          fontWeight: FontWeight.w700,
                          color: isUrgent ? colors.danger : colors.warning,
                        ),
                      ),
                    ],
                  ),
                ),
            ],
          ),
          if (request.hasPackage) ...[
            const SizedBox(height: 10),
            Text(
              request.itemSummary!,
              style: TextStyle(
                fontSize: 14,
                fontWeight: FontWeight.w500,
                color: colors.textMuted,
                height: 1.4,
              ),
            ),
          ],
        ],
      ),
    );
  }

  // ── Route card ────────────────────────────────────────────────────────

  Widget _buildRouteCard(BuildContext context) {
    final colors = context.palette;
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: colors.surface,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: colors.divider),
      ),
      child: Column(
        children: [
          _StopRow(
            label: 'Pick-up',
            address: request.pickupAddress,
            color: colors.brandNavy900,
            icon: Icons.radio_button_checked,
          ),
          Padding(
            padding: const EdgeInsets.only(left: 9, top: 4, bottom: 4),
            child: Container(width: 2, height: 20, color: colors.divider),
          ),
          _StopRow(
            label: 'Drop-off',
            address: request.dropoffAddress,
            color: colors.brandTeal500,
            icon: Icons.place_rounded,
          ),
          if (request.hasDistance) ...[
            const SizedBox(height: 16),
            Container(height: 1, color: colors.divider),
            const SizedBox(height: 14),
            Row(
              children: [
                _MetricBlock(
                  icon: Icons.straighten_rounded,
                  value: request.distanceDisplay,
                  label: 'Distance',
                ),
                Container(width: 1, height: 36, color: colors.divider),
                _MetricBlock(
                  icon: Icons.schedule_rounded,
                  value: '${(request.distanceKm * 2.5).round()} min',
                  label: 'Est. time',
                ),
                Container(width: 1, height: 36, color: colors.divider),
                const _MetricBlock(
                  icon: Icons.today_rounded,
                  value: 'Today',
                  label: 'Window',
                ),
              ],
            ),
          ],
        ],
      ),
    );
  }

  // ── Notes ─────────────────────────────────────────────────────────────

  Widget _buildNotesCard(BuildContext context) {
    final colors = context.palette;
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: colors.surface,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: colors.divider),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            'Notes',
            style: TextStyle(
              fontSize: 14,
              fontWeight: FontWeight.w800,
              color: colors.text,
            ),
          ),
          const SizedBox(height: 8),
          Text(
            request.notes!,
            style: TextStyle(
              fontSize: 13.5,
              color: colors.textMuted,
              height: 1.5,
            ),
          ),
        ],
      ),
    );
  }

  // ── Source org ────────────────────────────────────────────────────────

  Widget _buildSourceOrgCard(BuildContext context) {
    final colors = context.palette;
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: colors.surface,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: colors.divider),
      ),
      child: Row(
        children: [
          Container(
            width: 40,
            height: 40,
            decoration: BoxDecoration(
              color: colors.navy10,
              borderRadius: BorderRadius.circular(10),
            ),
            child: Icon(
              Icons.storefront_outlined,
              size: 20,
              color: colors.text,
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  request.sourceOrgName!,
                  style: TextStyle(
                    fontSize: 14.5,
                    fontWeight: FontWeight.w700,
                    color: colors.text,
                  ),
                ),
                const SizedBox(height: 2),
                Text(
                  'This request is from a verified Karryam Commerce customer.',
                  style: TextStyle(
                    fontSize: 12,
                    color: colors.textMuted,
                    height: 1.35,
                  ),
                ),
              ],
            ),
          ),
          Icon(
            Icons.verified_rounded,
            size: 18,
            color: colors.brandTeal500,
          ),
        ],
      ),
    );
  }

  // ── Handling card ─────────────────────────────────────────────────────

  Widget _buildHandlingCard(BuildContext context) {
    final colors = context.palette;
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: colors.warningSoft,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(
          color: colors.warning.withValues(alpha: 0.35),
        ),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(
                Icons.info_outline_rounded,
                size: 16,
                color: colors.warning,
              ),
              const SizedBox(width: 6),
              Text(
                'Handling notes',
                style: TextStyle(
                  fontSize: 13.5,
                  fontWeight: FontWeight.w800,
                  color: colors.warning,
                ),
              ),
            ],
          ),
          const SizedBox(height: 8),
          Wrap(
            spacing: 6,
            runSpacing: 6,
            children: request.handlingTags
                .map((t) => Container(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 8,
                        vertical: 4,
                      ),
                      decoration: BoxDecoration(
                        color: colors.surface,
                        borderRadius: BorderRadius.circular(6),
                      ),
                      child: Text(
                        _handlingLabel(t),
                        style: TextStyle(
                          fontSize: 12,
                          fontWeight: FontWeight.w600,
                          color: colors.warning,
                        ),
                      ),
                    ))
                .toList(),
          ),
        ],
      ),
    );
  }

  // ── Bare offer fallback ───────────────────────────────────────────────

  Widget _buildBareOfferCard(BuildContext context) {
    final colors = context.palette;
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: colors.surface,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: colors.divider),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            request.source == RequestSource.assignment
                ? 'Assignment offer'
                : 'Offer',
            style: TextStyle(
              fontSize: 14,
              fontWeight: FontWeight.w800,
              color: colors.text,
            ),
          ),
          const SizedBox(height: 6),
          Text(
            'This offer doesn\'t include route or fare details yet. '
            'Accept to see full mission information.',
            style: TextStyle(
              fontSize: 13,
              height: 1.45,
              color: colors.textMuted,
            ),
          ),
        ],
      ),
    );
  }

  static String _handlingLabel(String tag) => switch (tag) {
        'fragile' => 'Fragile',
        'refrigerated' => 'Refrigerated',
        'handle_with_care' => 'Handle with care',
        'palletised' => 'Palletised',
        'documents' => 'Documents',
        _ => tag,
      };

  // ── Sticky actions ────────────────────────────────────────────────────

  Widget _buildStickyActions(BuildContext context) {
    final colors = context.palette;
    final lifecycle = request.lifecycle;

    if (lifecycle == RequestLifecycleState.expired) {
      return _stickyButton(
        context,
        label: 'Browse new opportunities',
        onPressed: () {
          Navigator.of(context).pop();
          onBrowseNew();
        },
      );
    }

    if (lifecycle == RequestLifecycleState.taken) {
      return _stickyButton(
        context,
        label: 'Back to opportunities',
        onPressed: () {
          Navigator.of(context).pop();
          onBackToOpportunities();
        },
      );
    }

    if (lifecycle == RequestLifecycleState.requoted) {
      return _stickyPair(
        context,
        primaryLabel: 'Accept new offer',
        secondaryLabel: 'Decline',
        onPrimary: onAccept,
        onSecondary: onDecline,
      );
    }

    return _stickyPair(
      context,
      primaryLabel: 'Accept request',
      secondaryLabel: 'Decline',
      onPrimary: onAccept,
      onSecondary: onDecline,
    );
  }

  Widget _stickyButton(
    BuildContext context, {
    required String label,
    required VoidCallback onPressed,
  }) {
    final colors = context.palette;
    return SafeArea(
      top: false,
      child: Container(
        padding: const EdgeInsets.fromLTRB(20, 12, 20, 16),
        decoration: BoxDecoration(
          color: colors.surface,
          border: Border(top: BorderSide(color: colors.divider)),
        ),
        child: SizedBox(
          width: double.infinity,
          height: 52,
          child: FilledButton(
            onPressed: onPressed,
            style: FilledButton.styleFrom(
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(999),
              ),
              textStyle: const TextStyle(
                fontSize: 15,
                fontWeight: FontWeight.w700,
              ),
            ),
            child: Text(label),
          ),
        ),
      ),
    );
  }

  Widget _stickyPair(
    BuildContext context, {
    required String primaryLabel,
    required String secondaryLabel,
    required VoidCallback onPrimary,
    required VoidCallback onSecondary,
  }) {
    final colors = context.palette;
    return SafeArea(
      top: false,
      child: Container(
        padding: const EdgeInsets.fromLTRB(20, 12, 20, 16),
        decoration: BoxDecoration(
          color: colors.surface,
          border: Border(top: BorderSide(color: colors.divider)),
        ),
        child: Row(
          children: [
            Expanded(
              child: SizedBox(
                height: 52,
                child: OutlinedButton(
                  onPressed: onSecondary,
                  style: OutlinedButton.styleFrom(
                    foregroundColor: colors.danger,
                    backgroundColor: colors.dangerSoft,
                    side: BorderSide.none,
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(999),
                    ),
                    textStyle: const TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  child: Text(secondaryLabel),
                ),
              ),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: SizedBox(
                height: 52,
                child: FilledButton(
                  onPressed: onPrimary,
                  style: FilledButton.styleFrom(
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(999),
                    ),
                    textStyle: const TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  child: Text(primaryLabel),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

// ── Small widgets ─────────────────────────────────────────────────────────

class _DotMarker extends StatelessWidget {
  const _DotMarker({required this.color, required this.icon});
  final Color color;
  final IconData icon;

  @override
  Widget build(BuildContext context) {
    return Container(
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: color,
        shape: BoxShape.circle,
        border: Border.all(color: Colors.white, width: 2.5),
        boxShadow: [
          BoxShadow(
            color: Colors.black.withValues(alpha: 0.18),
            blurRadius: 6,
            offset: const Offset(0, 2),
          ),
        ],
      ),
      child: Icon(icon, size: 16, color: Colors.white),
    );
  }
}

class _StopRow extends StatelessWidget {
  const _StopRow({
    required this.label,
    required this.address,
    required this.color,
    required this.icon,
  });

  final String label;
  final String address;
  final Color color;
  final IconData icon;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: const EdgeInsets.only(top: 1),
          child: Icon(icon, size: 18, color: color),
        ),
        const SizedBox(width: 12),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                label,
                style: TextStyle(
                  fontSize: 11,
                  fontWeight: FontWeight.w600,
                  color: colors.textMuted,
                  letterSpacing: 0.3,
                ),
              ),
              const SizedBox(height: 2),
              Text(
                address,
                style: TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w700,
                  color: colors.text,
                  height: 1.3,
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }
}

class _MetricBlock extends StatelessWidget {
  const _MetricBlock({
    required this.icon,
    required this.value,
    required this.label,
  });

  final IconData icon;
  final String value;
  final String label;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Expanded(
      child: Column(
        children: [
          Icon(icon, size: 16, color: colors.textMuted),
          const SizedBox(height: 6),
          Text(
            value,
            style: TextStyle(
              fontSize: 14,
              fontWeight: FontWeight.w800,
              color: colors.text,
              letterSpacing: -0.2,
            ),
          ),
          const SizedBox(height: 2),
          Text(
            label,
            style: TextStyle(fontSize: 11, color: colors.textMuted),
          ),
        ],
      ),
    );
  }
}