import 'package:flutter/material.dart';

import '../../../../core/theme/theme_extensions.dart';
import '../../data/request_model.dart';

/// Card used in the Opportunities inbox. Every section renders conditionally
/// on the presence of its data — mock supplies everything, broadcast supplies
/// most, assignment and commerce supply progressively less. Nothing is
/// fabricated: a source that sends no fare shows no fare row.
class OpportunityCard extends StatelessWidget {
  const OpportunityCard({
    super.key,
    required this.request,
    required this.onTap,
    required this.onAccept,
    required this.onDecline,
  });

  final RequestModel request;
  final VoidCallback onTap;
  final VoidCallback onAccept;
  final VoidCallback onDecline;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    final lifecycle = request.lifecycle;
    final isDimmed = !request.isAcceptable;
    final isRequoted = lifecycle == RequestLifecycleState.requoted;
    final showInlineActions = _shouldShowInlineActions();

    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Opacity(
        opacity: isDimmed ? 0.72 : 1.0,
        child: Container(
          decoration: BoxDecoration(
            color: colors.surface,
            borderRadius: BorderRadius.circular(16),
            border: Border.all(
              color: isRequoted
                  ? colors.danger.withValues(alpha: 0.4)
                  : colors.divider,
              width: isRequoted ? 1.4 : 1,
            ),
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              if (isRequoted) const _RequoteBanner(),

              Padding(
                padding: const EdgeInsets.all(16),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    // ── Top row: badge + urgency ──────────────────────
                    Row(
                      children: [
                        if (lifecycle == RequestLifecycleState.available)
                          _SourceBadge(source: request.source)
                        else
                          _LifecycleBadge(lifecycle: lifecycle),
                        const Spacer(),
                        if (lifecycle == RequestLifecycleState.available &&
                            request.minutesRemaining != null &&
                            request.isAcceptable)
                          _UrgencyChip(
                            minutes: request.minutesRemaining!,
                            urgent: (request.minutesRemaining ?? 999) <= 5,
                          ),
                      ],
                    ),

                    const SizedBox(height: 12),

                    // ── Fare row (broadcast only) ─────────────────────
                    if (request.hasFare) ...[
                      Row(
                        crossAxisAlignment: CrossAxisAlignment.end,
                        children: [
                          Text(
                            request.fareDisplayGrouped,
                            style: TextStyle(
                              fontSize: 26,
                              fontWeight: FontWeight.w800,
                              color: colors.text,
                              letterSpacing: -0.8,
                              height: 1.0,
                            ),
                          ),
                          if (isRequoted &&
                              request.previousFareDisplayGrouped != null) ...[
                            const SizedBox(width: 8),
                            Padding(
                              padding: const EdgeInsets.only(bottom: 3),
                              child: Text(
                                request.previousFareDisplayGrouped!,
                                style: TextStyle(
                                  fontSize: 13,
                                  color: colors.textMuted,
                                  decoration: TextDecoration.lineThrough,
                                ),
                              ),
                            ),
                          ],
                        ],
                      ),
                      const SizedBox(height: 6),
                    ],

                    // ── Package summary ────────────────────────────────
                    if (request.hasPackage) ...[
                      Text(
                        request.itemSummary!,
                        style: TextStyle(
                          fontSize: 13.5,
                          fontWeight: FontWeight.w500,
                          color: colors.textMuted,
                          height: 1.3,
                        ),
                      ),
                      const SizedBox(height: 12),
                    ],

                    // ── Fallback line for bare offers ──────────────────
                    if (!request.hasFare && !request.hasRoute) ...[
                      Text(
                        request.bookingId.isNotEmpty
                            ? '#${request.bookingId}'
                            : 'Offer ${request.id}',
                        style: TextStyle(
                          fontSize: 14,
                          fontWeight: FontWeight.w700,
                          color: colors.text,
                          letterSpacing: 0.1,
                        ),
                      ),
                      const SizedBox(height: 6),
                      Text(
                        'Details available after you accept.',
                        style: TextStyle(
                          fontSize: 12.5,
                          color: colors.textMuted,
                          height: 1.35,
                        ),
                      ),
                      const SizedBox(height: 6),
                    ],

                    // ── Route ─────────────────────────────────────────
                    if (request.hasRoute) ...[
                      _RouteLine(
                        icon: Icons.radio_button_checked,
                        color: colors.brandNavy900,
                        label: request.pickupAddress,
                      ),
                      const SizedBox(height: 8),
                      _RouteLine(
                        icon: Icons.place_outlined,
                        color: colors.brandTeal500,
                        label: request.dropoffAddress,
                      ),
                    ],

                    // ── Handling tags (never on live data today) ──────
                    if (request.hasHandling) ...[
                      const SizedBox(height: 12),
                      Wrap(
                        spacing: 6,
                        runSpacing: 6,
                        children: request.handlingTags
                            .map((t) => _HandlingPill(tag: t))
                            .toList(),
                      ),
                    ],

                    // ── Metrics row (distance only) ───────────────────
                    if (request.hasDistance) ...[
                      const SizedBox(height: 14),
                      Row(
                        children: [
                          _Metric(
                            icon: Icons.straighten_rounded,
                            label: request.distanceDisplay,
                          ),
                          const SizedBox(width: 14),
                          _Metric(
                            icon: Icons.schedule_rounded,
                            label:
                                '${(request.distanceKm * 2.5).round()} min',
                          ),
                        ],
                      ),
                    ],

                    // ── Assignment info box ───────────────────────────
                    if (request.source == RequestSource.assignment &&
                        lifecycle == RequestLifecycleState.available) ...[
                      const SizedBox(height: 14),
                      Container(
                        padding: const EdgeInsets.symmetric(
                          horizontal: 12,
                          vertical: 10,
                        ),
                        decoration: BoxDecoration(
                          color: colors.navy10,
                          borderRadius: BorderRadius.circular(10),
                        ),
                        child: Row(
                          children: [
                            Icon(
                              Icons.assignment_ind_outlined,
                              size: 16,
                              color: colors.brandNavy900,
                            ),
                            const SizedBox(width: 8),
                            Expanded(
                              child: Text(
                                'This offer has been assigned to you based on your profile and location.',
                                style: TextStyle(
                                  fontSize: 12,
                                  height: 1.4,
                                  fontWeight: FontWeight.w500,
                                  color: colors.text,
                                ),
                              ),
                            ),
                          ],
                        ),
                      ),
                    ],

                    // ── Lifecycle hint ────────────────────────────────
                    if (lifecycle == RequestLifecycleState.expired) ...[
                      const SizedBox(height: 12),
                      const _LifecycleHint(
                        icon: Icons.info_outline_rounded,
                        text:
                            'This opportunity is no longer available. Check for new requests near you.',
                      ),
                    ],
                    if (lifecycle == RequestLifecycleState.taken) ...[
                      const SizedBox(height: 12),
                      const _LifecycleHint(
                        icon: Icons.info_outline_rounded,
                        text:
                            'This opportunity is no longer available. It may have been taken by another provider.',
                      ),
                    ],
                    if (isRequoted) ...[
                      const SizedBox(height: 12),
                      const _LifecycleHint(
                        icon: Icons.info_outline_rounded,
                        text:
                            'This offer has been updated with a new rate. Review the details and choose an action.',
                      ),
                    ],
                  ],
                ),
              ),

              if (showInlineActions) _buildActions(context, isRequoted),
            ],
          ),
        ),
      ),
    );
  }

  /// Inline accept / decline is shown when:
  ///   • the rider can act on it directly (assignment / commerce), OR
  ///   • the offer was requoted (rider needs to explicitly re-accept).
  bool _shouldShowInlineActions() {
    if (!request.isAcceptable) return false;
    if (request.lifecycle == RequestLifecycleState.requoted) return true;
    if (request.lifecycle != RequestLifecycleState.available) return false;
    return request.source == RequestSource.assignment ||
        request.source == RequestSource.commerce;
  }

  Widget _buildActions(BuildContext context, bool isRequoted) {
    final colors = context.palette;
    final acceptLabel = isRequoted ? 'Accept new offer' : 'Accept offer';

    return Container(
      padding: const EdgeInsets.fromLTRB(16, 0, 16, 16),
      child: Row(
        children: [
          Expanded(
            child: SizedBox(
              height: 48,
              child: FilledButton(
                onPressed: request.isAcceptable ? onAccept : null,
                style: FilledButton.styleFrom(
                  shape: RoundedRectangleBorder(
                    borderRadius: BorderRadius.circular(12),
                  ),
                  textStyle: const TextStyle(
                    fontSize: 14.5,
                    fontWeight: FontWeight.w700,
                  ),
                ),
                child: Text(acceptLabel),
              ),
            ),
          ),
          const SizedBox(width: 10),
          Expanded(
            child: SizedBox(
              height: 48,
              child: OutlinedButton(
                onPressed: onDecline,
                style: OutlinedButton.styleFrom(
                  foregroundColor: colors.text,
                  side: BorderSide(color: colors.divider, width: 1.2),
                  shape: RoundedRectangleBorder(
                    borderRadius: BorderRadius.circular(12),
                  ),
                  textStyle: const TextStyle(
                    fontSize: 14.5,
                    fontWeight: FontWeight.w700,
                  ),
                ),
                child: const Text('Decline'),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

// ── Requote banner ────────────────────────────────────────────────────────

class _RequoteBanner extends StatelessWidget {
  const _RequoteBanner();

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
      decoration: BoxDecoration(
        color: colors.dangerSoft,
        borderRadius: const BorderRadius.vertical(
          top: Radius.circular(15),
        ),
      ),
      child: Row(
        children: [
          Icon(
            Icons.error_outline_rounded,
            size: 16,
            color: colors.danger,
          ),
          const SizedBox(width: 8),
          Expanded(
            child: Text(
              'Offer updated',
              style: TextStyle(
                fontSize: 12.5,
                fontWeight: FontWeight.w800,
                color: colors.danger,
              ),
            ),
          ),
          Text(
            'New details available',
            style: TextStyle(
              fontSize: 11.5,
              fontWeight: FontWeight.w500,
              color: colors.danger,
            ),
          ),
        ],
      ),
    );
  }
}

// ── Lifecycle hint ────────────────────────────────────────────────────────

class _LifecycleHint extends StatelessWidget {
  const _LifecycleHint({required this.icon, required this.text});
  final IconData icon;
  final String text;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: const EdgeInsets.only(top: 1),
          child: Icon(icon, size: 14, color: colors.textMuted),
        ),
        const SizedBox(width: 8),
        Expanded(
          child: Text(
            text,
            style: TextStyle(
              fontSize: 12,
              height: 1.4,
              color: colors.textMuted,
            ),
          ),
        ),
      ],
    );
  }
}

// ── Source badge ──────────────────────────────────────────────────────────

class _SourceBadge extends StatelessWidget {
  const _SourceBadge({required this.source});
  final RequestSource source;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;

    final (IconData icon, String label, Color fg, Color bg) = switch (source) {
      RequestSource.broadcast => (
          Icons.podcasts_rounded,
          'Broadcast',
          colors.brandTeal600,
          colors.teal10,
        ),
      RequestSource.assignment => (
          Icons.assignment_ind_outlined,
          'Assignment offer',
          colors.brandNavy900,
          colors.navy10,
        ),
      RequestSource.commerce => (
          Icons.storefront_outlined,
          'Commerce',
          colors.warning,
          colors.warningSoft,
        ),
    };

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      decoration: BoxDecoration(
        color: bg,
        borderRadius: BorderRadius.circular(999),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(icon, size: 12, color: fg),
          const SizedBox(width: 5),
          Text(
            label,
            style: TextStyle(
              fontSize: 11,
              fontWeight: FontWeight.w700,
              color: fg,
            ),
          ),
        ],
      ),
    );
  }
}

// ── Lifecycle badge ───────────────────────────────────────────────────────

class _LifecycleBadge extends StatelessWidget {
  const _LifecycleBadge({required this.lifecycle});
  final RequestLifecycleState lifecycle;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;

    final (IconData icon, String label, Color fg, Color bg) =
        switch (lifecycle) {
      RequestLifecycleState.expired => (
          Icons.timer_off_outlined,
          'Expired',
          colors.danger,
          colors.dangerSoft,
        ),
      RequestLifecycleState.taken => (
          Icons.lock_clock_outlined,
          'Not available',
          colors.warning,
          colors.warningSoft,
        ),
      _ => (
          Icons.error_outline_rounded,
          'Offer updated',
          colors.danger,
          colors.dangerSoft,
        ),
    };

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      decoration: BoxDecoration(
        color: bg,
        borderRadius: BorderRadius.circular(999),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(icon, size: 12, color: fg),
          const SizedBox(width: 5),
          Text(
            label,
            style: TextStyle(
              fontSize: 11,
              fontWeight: FontWeight.w700,
              color: fg,
            ),
          ),
        ],
      ),
    );
  }
}

// ── Urgency chip ──────────────────────────────────────────────────────────

class _UrgencyChip extends StatelessWidget {
  const _UrgencyChip({required this.minutes, required this.urgent});
  final int minutes;
  final bool urgent;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    final fg = urgent ? colors.danger : colors.warning;
    final bg = urgent ? colors.dangerSoft : colors.warningSoft;

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      decoration: BoxDecoration(
        color: bg,
        borderRadius: BorderRadius.circular(999),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(Icons.access_time_rounded, size: 12, color: fg),
          const SizedBox(width: 4),
          Text(
            '$minutes min left',
            style: TextStyle(
              fontSize: 11,
              fontWeight: FontWeight.w700,
              color: fg,
            ),
          ),
        ],
      ),
    );
  }
}

// ── Route line ────────────────────────────────────────────────────────────

class _RouteLine extends StatelessWidget {
  const _RouteLine({
    required this.icon,
    required this.color,
    required this.label,
  });

  final IconData icon;
  final Color color;
  final String label;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: const EdgeInsets.only(top: 2),
          child: Icon(icon, size: 14, color: color),
        ),
        const SizedBox(width: 10),
        Expanded(
          child: Text(
            label,
            style: TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w500,
              color: colors.text,
              height: 1.3,
            ),
          ),
        ),
      ],
    );
  }
}

// ── Handling pill ─────────────────────────────────────────────────────────

class _HandlingPill extends StatelessWidget {
  const _HandlingPill({required this.tag});
  final String tag;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;

    final (IconData icon, String label) = switch (tag) {
      'fragile' => (Icons.warning_amber_rounded, 'Fragile'),
      'refrigerated' => (Icons.ac_unit_rounded, 'Refrigerated'),
      'handle_with_care' => (
          Icons.volunteer_activism_outlined,
          'Handle with care'
        ),
      'palletised' => (Icons.inventory_2_outlined, 'Palletised'),
      'documents' => (Icons.description_outlined, 'Documents'),
      _ => (Icons.local_offer_outlined, tag),
    };

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      decoration: BoxDecoration(
        color: colors.navy10,
        borderRadius: BorderRadius.circular(6),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(icon, size: 12, color: colors.textMuted),
          const SizedBox(width: 4),
          Text(
            label,
            style: TextStyle(
              fontSize: 11,
              fontWeight: FontWeight.w600,
              color: colors.textMuted,
            ),
          ),
        ],
      ),
    );
  }
}

// ── Metric ────────────────────────────────────────────────────────────────

class _Metric extends StatelessWidget {
  const _Metric({required this.icon, required this.label});
  final IconData icon;
  final String label;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Icon(icon, size: 13, color: colors.textMuted),
        const SizedBox(width: 4),
        Text(
          label,
          style: TextStyle(
            fontSize: 12,
            fontWeight: FontWeight.w600,
            color: colors.textMuted,
          ),
        ),
      ],
    );
  }
}