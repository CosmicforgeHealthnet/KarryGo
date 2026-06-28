import 'package:flutter/material.dart';

import '../../../core/format/money_format.dart';
import '../../auth/models/provider_auth_models.dart';
import '../../home/ui/widgets/provider_app_colors.dart';
import '../models/earnings_models.dart';
import '../state/provider_earnings_controller.dart';

/// Transaction Detail (Figma 2275-2280): status-coloured amount + a commission
/// breakdown, plus the trip section (2276/2278/2280) for trip-linked credits.
class ProviderTransactionDetailScreen extends StatefulWidget {
  const ProviderTransactionDetailScreen({
    super.key,
    required this.txn,
    required this.controller,
    this.onContactSupport,
  });

  final EarningsTransaction txn;
  final ProviderEarningsController controller;
  final VoidCallback? onContactSupport;

  @override
  State<ProviderTransactionDetailScreen> createState() => _ProviderTransactionDetailScreenState();
}

class _ProviderTransactionDetailScreenState extends State<ProviderTransactionDetailScreen> {
  // The platform commission shown in the breakdown. Display-only: the earnings
  // balances elsewhere are reported gross.
  static const _commissionRate = 0.10;

  ProviderBooking? _booking;
  bool _loadingTrip = false;

  @override
  void initState() {
    super.initState();
    if (widget.txn.isTrip && widget.txn.bookingId.isNotEmpty) {
      _loadingTrip = true;
      WidgetsBinding.instance.addPostFrameCallback((_) async {
        final b = await widget.controller.fetchTripDetail(widget.txn.bookingId);
        if (!mounted) return;
        setState(() {
          _booking = b;
          _loadingTrip = false;
        });
      });
    }
  }

  ({String label, Color color}) get _status {
    switch (widget.txn.status) {
      case EarningsTransaction.statusPending:
        return (label: 'Pending', color: const Color(0xFFE8A21B));
      case EarningsTransaction.statusFailed:
        return (label: 'Failed', color: kProviderRejectText);
      default:
        return (label: 'Successful', color: kProviderGreen);
    }
  }

  void _contactSupport() {
    if (widget.onContactSupport != null) {
      widget.onContactSupport!();
    } else {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Support is coming soon'),
          duration: Duration(seconds: 1),
          behavior: SnackBarBehavior.floating,
        ),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    final txn = widget.txn;
    final status = _status;
    final gross = txn.amountNaira.abs();
    final commission = gross * _commissionRate;
    final net = gross - commission;
    final sign = txn.isCredit ? '+' : '-';

    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        child: Column(
          children: [
            _AppBar(),
            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 8, 20, 24),
                children: [
                  const SizedBox(height: 12),
                  Center(
                    child: Column(
                      children: [
                        const Text('Total Amount', style: TextStyle(color: kProviderMuted, fontSize: 13)),
                        const SizedBox(height: 10),
                        Text(
                          '$sign₦${formatNaira(gross)}',
                          style: TextStyle(color: status.color, fontSize: 30, fontWeight: FontWeight.w800),
                        ),
                        const SizedBox(height: 12),
                        _StatusBadge(label: status.label, color: status.color),
                      ],
                    ),
                  ),
                  const SizedBox(height: 28),
                  const Text(
                    'Transaction Details',
                    style: TextStyle(color: kProviderText, fontSize: 18, fontWeight: FontWeight.w800),
                  ),
                  const SizedBox(height: 14),
                  _DetailRow(label: 'Transaction ID', value: '#${_ref(txn.id)}'),
                  _DetailRow(
                    label: 'Transaction Amount',
                    value: '$sign₦${formatNaira(gross)}',
                    valueColor: status.color,
                  ),
                  const _DetailRow(label: 'Commission', value: '-10%', valueColor: kProviderRejectText),
                  _DetailRow(label: 'Total', value: '₦${formatNaira(net)}', bold: true),

                  if (txn.isTrip) ...[
                    const SizedBox(height: 8),
                    if (_loadingTrip)
                      const Padding(
                        padding: EdgeInsets.symmetric(vertical: 24),
                        child: Center(child: CircularProgressIndicator(color: kProviderGreen)),
                      )
                    else if (_booking != null)
                      _TripSection(booking: _booking!, title: txn.title)
                    else
                      const SizedBox.shrink(),
                  ],
                  const SizedBox(height: 20),
                  _ContactSupportCard(onTap: _contactSupport),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }

  String _ref(String id) {
    final clean = id.replaceAll('-', '');
    return clean.length >= 9 ? clean.substring(0, 9) : clean;
  }
}

class _AppBar extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 12, 16, 0),
      child: Row(
        children: [
          GestureDetector(
            onTap: () => Navigator.of(context).maybePop(),
            child: Container(
              width: 44,
              height: 44,
              decoration: const BoxDecoration(
                color: Colors.white,
                shape: BoxShape.circle,
                boxShadow: [BoxShadow(color: Color(0x22000000), blurRadius: 10, offset: Offset(0, 3))],
              ),
              child: const Icon(Icons.arrow_back, color: kProviderText, size: 20),
            ),
          ),
          const SizedBox(width: 14),
          const Expanded(
            child: Text(
              'Transaction Details',
              style: TextStyle(color: kProviderText, fontSize: 21, fontWeight: FontWeight.w800),
            ),
          ),
        ],
      ),
    );
  }
}

class _StatusBadge extends StatelessWidget {
  const _StatusBadge({required this.label, required this.color});
  final String label;
  final Color color;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 7),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.14),
        borderRadius: BorderRadius.circular(20),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(width: 8, height: 8, decoration: BoxDecoration(color: color, shape: BoxShape.circle)),
          const SizedBox(width: 8),
          Text(label, style: TextStyle(color: color, fontSize: 13, fontWeight: FontWeight.w700)),
        ],
      ),
    );
  }
}

class _DetailRow extends StatelessWidget {
  const _DetailRow({required this.label, required this.value, this.valueColor, this.bold = false});
  final String label;
  final String value;
  final Color? valueColor;
  final bool bold;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 9),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.spaceBetween,
        children: [
          Expanded(
            child: Text(label, style: const TextStyle(color: kProviderMuted, fontSize: 14)),
          ),
          const SizedBox(width: 12),
          Text(
            value,
            style: TextStyle(
              color: valueColor ?? kProviderText,
              fontSize: 14,
              fontWeight: bold ? FontWeight.w800 : FontWeight.w600,
            ),
          ),
        ],
      ),
    );
  }
}

class _TripSection extends StatelessWidget {
  const _TripSection({required this.booking, required this.title});
  final ProviderBooking booking;
  final String title;

  @override
  Widget build(BuildContext context) {
    final name = title.isEmpty ? booking.displayName : title;
    final km = booking.distanceKm ?? 0;
    final mins = (km * 2.5).round(); // rough ETA — duration isn't stored
    final fare = booking.fareEstimateNaira;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const SizedBox(height: 8),
        const Divider(color: kProviderBorder),
        const SizedBox(height: 12),
        Row(
          children: [
            CircleAvatar(
              radius: 24,
              backgroundColor: kProviderGreenTint,
              child: Text(
                name.isNotEmpty ? name[0].toUpperCase() : '?',
                style: const TextStyle(color: kProviderGreen, fontSize: 20, fontWeight: FontWeight.w800),
              ),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    name,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: const TextStyle(color: kProviderText, fontSize: 18, fontWeight: FontWeight.w800),
                  ),
                  const SizedBox(height: 4),
                  Text(
                    '${km.toStringAsFixed(0)} km  ·  $mins min  ·  ₦${formatNaira(fare)}',
                    style: const TextStyle(color: kProviderMuted, fontSize: 13),
                  ),
                ],
              ),
            ),
          ],
        ),
        const SizedBox(height: 16),
        _kv('Trip Completed:', '#${booking.shortId}'),
        const SizedBox(height: 10),
        _kv('Date:', _formatDate(booking.createdAt)),
        const SizedBox(height: 14),
        _RouteCard(pickup: booking.pickupAddress, dropoff: booking.dropoffAddress, fare: fare),
      ],
    );
  }

  Widget _kv(String k, String v) {
    return Row(
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: [
        Text(k, style: const TextStyle(color: kProviderText, fontSize: 14, fontWeight: FontWeight.w700)),
        Flexible(
          child: Text(
            v,
            textAlign: TextAlign.right,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(color: kProviderMuted, fontSize: 13.5),
          ),
        ),
      ],
    );
  }

  static String _formatDate(DateTime d) {
    const weekdays = ['Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday'];
    const months = [
      'Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun',
      'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec',
    ];
    return '${weekdays[d.weekday - 1]}, ${d.day} ${months[d.month - 1]} ${d.year}';
  }
}

class _RouteCard extends StatelessWidget {
  const _RouteCard({required this.pickup, required this.dropoff, required this.fare});
  final String pickup;
  final String dropoff;
  final double fare;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: kProviderBorder),
      ),
      child: Column(
        children: [
          _point(color: kProviderGreen, label: 'Pick-up', address: pickup),
          const Padding(
            padding: EdgeInsets.only(left: 5),
            child: SizedBox(
              height: 22,
              child: VerticalDivider(color: kProviderBorder, thickness: 1.5, width: 12),
            ),
          ),
          _point(color: const Color(0xFFE8A21B), label: 'Drop off', address: dropoff),
          const SizedBox(height: 12),
          const Divider(color: kProviderBorder),
          const SizedBox(height: 8),
          Row(
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: [
              const Text('Trip Fee:', style: TextStyle(color: kProviderText, fontSize: 14, fontWeight: FontWeight.w700)),
              Text('₦${formatNaira(fare)}', style: const TextStyle(color: kProviderText, fontSize: 14, fontWeight: FontWeight.w800)),
            ],
          ),
        ],
      ),
    );
  }

  Widget _point({required Color color, required String label, required String address}) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: const EdgeInsets.only(top: 3),
          child: Icon(Icons.circle, size: 11, color: color),
        ),
        const SizedBox(width: 12),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(label, style: const TextStyle(color: kProviderMuted, fontSize: 12)),
              const SizedBox(height: 2),
              Text(
                address.isEmpty ? '—' : address,
                style: const TextStyle(color: kProviderText, fontSize: 14, fontWeight: FontWeight.w600),
              ),
            ],
          ),
        ),
      ],
    );
  }
}

class _ContactSupportCard extends StatelessWidget {
  const _ContactSupportCard({required this.onTap});
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: kProviderSurface,
        borderRadius: BorderRadius.circular(14),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text(
            'Any Question about this transaction?',
            style: TextStyle(color: kProviderMuted, fontSize: 13.5, fontWeight: FontWeight.w600),
          ),
          const SizedBox(height: 10),
          GestureDetector(
            onTap: onTap,
            behavior: HitTestBehavior.opaque,
            child: const Row(
              children: [
                Icon(Icons.headset_mic_outlined, color: kProviderGreen, size: 20),
                SizedBox(width: 8),
                Text(
                  'Contact Support',
                  style: TextStyle(color: kProviderGreen, fontSize: 15, fontWeight: FontWeight.w700),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}
