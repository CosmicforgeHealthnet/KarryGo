import 'package:flutter/material.dart';

import '../../../core/format/money_format.dart';
import '../../auth/models/provider_auth_models.dart';
import '../../../core/format/date_format.dart';
import '../../../core/widgets/karryam_ui.dart';
import '../../home/ui/widgets/provider_app_colors.dart';
import '../../home/ui/widgets/provider_request_card.dart';
import 'widgets/earnings_transaction_list.dart';
import '../models/earnings_models.dart';
import '../state/provider_earnings_controller.dart';

/// One earnings transaction: the amount exactly as the server reports it, its
/// status, and — for trip earnings — the trip it came from.
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

  (String, KTone) get _status => switch (widget.txn.status) {
        EarningsTransaction.statusPending => ('Pending', KTone.warning),
        EarningsTransaction.statusFailed => ('Failed', KTone.danger),
        _ => ('Successful', KTone.success),
      };

  void _contactSupport() {
    if (widget.onContactSupport != null) {
      widget.onContactSupport!();
    } else {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Support is coming soon'), duration: Duration(seconds: 1)),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    final txn = widget.txn;
    final (statusLabel, tone) = _status;
    final amount = '${txn.isCredit ? '+' : '-'}₦${formatNaira(txn.amountNaira.abs())}';
    final amountColor = switch (tone) {
      KTone.danger => kProviderRejectText,
      KTone.warning => kWarning,
      _ => txn.isCredit ? kTealText : kProviderText,
    };

    return Scaffold(
      appBar: KTopBar(title: 'Transaction Details'),
      body: ListView(
        padding: const EdgeInsets.fromLTRB(20, 8, 20, 32),
        children: [
          KCard(
            padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 24),
            child: Column(
              children: [
                KIconTile(
                  txn.isCredit ? Icons.south_west_rounded : Icons.north_east_rounded,
                  size: 52,
                  color: txn.isCredit ? kTealText : kProviderRejectText,
                  background: txn.isCredit ? kTealTint : kProviderRejectBg,
                ),
                const SizedBox(height: 14),
                Text(
                  amount,
                  style: TextStyle(fontSize: 30, fontWeight: FontWeight.w700, letterSpacing: -0.8, color: amountColor),
                ),
                const SizedBox(height: 6),
                Text(
                  txn.title,
                  textAlign: TextAlign.center,
                  style: TextStyle(fontSize: 14, color: kProviderMuted),
                ),
                const SizedBox(height: 12),
                KStatusChip(statusLabel, tone: tone),
              ],
            ),
          ),
          const SizedBox(height: 12),
          KCard(
            child: Column(
              children: [
                KInfoRow('Transaction ID', '#${_ref(txn.id)}'),
                KInfoRow('Amount', amount, valueColor: amountColor),
                KInfoRow('Type', txn.isTrip ? 'Trip earning' : (txn.isCredit ? 'Credit' : 'Payout')),
                if (txn.subtitle.isNotEmpty) KInfoRow('Details', txn.subtitle),
                KInfoRow('Date', formatTransactionDate(txn.occurredAt)),
              ],
            ),
          ),
          if (txn.isTrip) ...[
            const SizedBox(height: 22),
            const KSectionHeader('Trip'),
            const SizedBox(height: 10),
            if (_loadingTrip)
              KCard(child: Center(child: CircularProgressIndicator()))
            else if (_booking != null)
              _TripSection(booking: _booking!)
            else
              KCard(
                child: Text(
                  "We couldn't load this trip right now.",
                  style: TextStyle(fontSize: 13, color: kProviderMuted),
                ),
              ),
          ],
          const SizedBox(height: 22),
          KGroup(
            children: [
              KListRow(
                icon: Icons.headset_mic_outlined,
                title: 'Contact Support',
                subtitle: 'Any question about this transaction?',
                onTap: _contactSupport,
              ),
            ],
          ),
        ],
      ),
    );
  }

  String _ref(String id) {
    final clean = id.replaceAll('-', '');
    return clean.length >= 9 ? clean.substring(0, 9) : clean;
  }
}

/// The trip behind a trip earning: route, fare and when it happened.
class _TripSection extends StatelessWidget {
  const _TripSection({required this.booking});
  final ProviderBooking booking;

  @override
  Widget build(BuildContext context) {
    return KCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          if (booking.receiverName.trim().isNotEmpty) ...[
            Row(
              children: [
                KAvatar(name: booking.receiverName),
                const SizedBox(width: 12),
                Expanded(
                  child: Text(
                    booking.receiverName.trim(),
                    style: const TextStyle(fontSize: 15, fontWeight: FontWeight.w600),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 14),
          ],
          BookingRoute(full: true, pickupAddress: booking.pickupAddress, dropoffAddress: booking.dropoffAddress),
          const SizedBox(height: 12),
          const Divider(),
          const SizedBox(height: 4),
          KInfoRow('Trip', '#${booking.shortId}'),
          KInfoRow('Date', formatLongDate(booking.createdAt)),
          KInfoRow('Trip Fee:', formatNairaShort(booking.fareNaira), bold: true),
        ],
      ),
    );
  }
}
