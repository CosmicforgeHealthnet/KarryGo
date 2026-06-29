import 'package:flutter/material.dart';

// ── Transaction model ─────────────────────────────────────────────────────────

enum TransactionStatus { successful, failed, pending }

enum TransactionType { trip, debit }

class TransactionData {
  const TransactionData({
    required this.id,
    required this.amount,
    required this.commission,
    required this.total,
    required this.status,
    required this.type,
    this.customerName,
    this.customerAvatarUrl,
    this.distanceKm,
    this.durationMin,
    this.tripFee,
    this.tripReference,
    this.date,
    this.pickup,
    this.dropoff,
  });

  final String id;
  final double amount;
  final double commission;
  final double total;
  final TransactionStatus status;
  final TransactionType type;

  final String? customerName;
  final String? customerAvatarUrl;
  final int? distanceKm;
  final int? durationMin;
  final double? tripFee;
  final String? tripReference;
  final String? date;
  final String? pickup;
  final String? dropoff;

  bool get isTripLinked => type == TransactionType.trip;
}

// ── Screen ────────────────────────────────────────────────────────────────────

class TransactionDetailScreen extends StatelessWidget {
  const TransactionDetailScreen({super.key, required this.transaction});

  final TransactionData transaction;

  Color get _amountColor {
    switch (transaction.status) {
      case TransactionStatus.successful:
        return const Color(0xFF4CAF50);
      case TransactionStatus.pending:
        return const Color(0xFFFFB300);
      case TransactionStatus.failed:
        return const Color(0xFFE53935);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
      appBar: AppBar(
        backgroundColor: Colors.white,
        elevation: 0,
        scrolledUnderElevation: 0,
        leading: GestureDetector(
          onTap: () => Navigator.of(context).pop(),
          behavior: HitTestBehavior.opaque,
          child: const Padding(
            padding: EdgeInsets.all(12),
            child: Icon(
              Icons.arrow_back_ios_new,
              size: 18,
              color: Color(0xFF1A1A1A),
            ),
          ),
        ),
        title: const Text(
          'Transaction Details',
          style: TextStyle(
            fontSize: 16,
            fontWeight: FontWeight.w700,
            color: Color(0xFF1A1A1A),
          ),
        ),
        centerTitle: false,
      ),
      body: SingleChildScrollView(
        physics: const BouncingScrollPhysics(),
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 20),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const SizedBox(height: 24),

              // ── Amount hero ────────────────────────────────────────────
              Center(
                child: Column(
                  children: [
                    const Text(
                      'Total Amount',
                      style: TextStyle(
                        fontSize: 13,
                        color: Color(0xFF888888),
                        fontWeight: FontWeight.w400,
                      ),
                    ),
                    const SizedBox(height: 8),
                    Text(
                      '+₦${_formatAmount(transaction.amount)}',
                      style: TextStyle(
                        fontSize: 36,
                        fontWeight: FontWeight.w800,
                        color: _amountColor,
                        letterSpacing: -0.5,
                      ),
                    ),
                    const SizedBox(height: 12),
                    _StatusBadge(status: transaction.status),
                  ],
                ),
              ),

              const SizedBox(height: 32),

              // ── Transaction Details section ────────────────────────────
              const Text(
                'Transaction Details',
                style: TextStyle(
                  fontSize: 17,
                  fontWeight: FontWeight.w800,
                  color: Color(0xFF1A1A1A),
                ),
              ),
              const SizedBox(height: 16),
              _DetailsTable(transaction: transaction),

              // ── Trip-linked content (Type A) ───────────────────────────
              if (transaction.isTripLinked) ...[
                const SizedBox(height: 24),
                _CustomerCard(transaction: transaction),
                const SizedBox(height: 20),
                _TripMeta(transaction: transaction),
                const SizedBox(height: 20),
                _RouteCard(
                  pickup: transaction.pickup!,
                  dropoff: transaction.dropoff!,
                  tripFee: transaction.tripFee!,
                ),
                const SizedBox(height: 16),
                _GoToTripsLink(),
              ],

              const SizedBox(height: 24),

              // ── Support card (always) ──────────────────────────────────
              _SupportCard(),

              const SizedBox(height: 40),
            ],
          ),
        ),
      ),
    );
  }

  String _formatAmount(double v) {
    final parts = v.toStringAsFixed(2).split('.');
    final intPart = parts[0].replaceAllMapped(
      RegExp(r'(\d{1,3})(?=(\d{3})+(?!\d))'),
      (m) => '${m[1]},',
    );
    return '$intPart.${parts[1]}';
  }
}

// ── Status Badge ──────────────────────────────────────────────────────────────

class _StatusBadge extends StatelessWidget {
  const _StatusBadge({required this.status});
  final TransactionStatus status;

  @override
  Widget build(BuildContext context) {
    final Color dotColor;
    final Color bgColor;
    final String label;

    switch (status) {
      case TransactionStatus.successful:
        dotColor = const Color(0xFF4CAF50);
        bgColor = const Color(0xFFE8F5E9);
        label = 'Successful';
        break;
      case TransactionStatus.failed:
        dotColor = const Color(0xFFE53935);
        bgColor = const Color(0xFFFFF0F0);
        label = 'Failed';
        break;
      case TransactionStatus.pending:
        dotColor = const Color(0xFFFFB300);
        bgColor = const Color(0xFFFFF8E1);
        label = 'Pending';
        break;
    }

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 7),
      decoration: BoxDecoration(
        color: bgColor,
        borderRadius: BorderRadius.circular(999),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(
            width: 8,
            height: 8,
            decoration: BoxDecoration(color: dotColor, shape: BoxShape.circle),
          ),
          const SizedBox(width: 7),
          Text(
            label,
            style: const TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w600,
              color: Color(0xFF1A1A1A),
            ),
          ),
        ],
      ),
    );
  }
}

// ── Details Table ─────────────────────────────────────────────────────────────

class _DetailsTable extends StatelessWidget {
  const _DetailsTable({required this.transaction});
  final TransactionData transaction;

  @override
  Widget build(BuildContext context) {
    final isDebit = transaction.type == TransactionType.debit;

    final amountColor = isDebit
        ? const Color(0xFFE53935)
        : const Color(0xFF4CAF50);

    final commissionColor = isDebit
        ? const Color(0xFFE53935)
        : const Color(0xFF1A1A1A);

    return Column(
      children: [
        _TableRow(
          label: 'Transaction ID',
          value: '#${transaction.id}',
          valueColor: const Color(0xFF1A1A1A),
          valueFontWeight: FontWeight.w500,
        ),
        _TableRow(
          label: 'Transaction Amount',
          value: isDebit
              ? '-₦${_fmt(transaction.amount)}'
              : '+₦${_fmt(transaction.amount)}',
          valueColor: amountColor,
          valueFontWeight: FontWeight.w600,
        ),
        _TableRow(
          label: 'Commission',
          value: isDebit
              ? '-${transaction.commission.toStringAsFixed(0)}%'
              : '${transaction.commission.toStringAsFixed(0)}%',
          valueColor: commissionColor,
          valueFontWeight: FontWeight.w500,
        ),
        _TableRow(
          label: 'Total',
          value: '-₦${_fmt(transaction.total.abs())}',
          valueColor: const Color(0xFF1A1A1A),
          valueFontWeight: FontWeight.w800,
          isLast: true,
        ),
      ],
    );
  }

  String _fmt(double v) {
    final parts = v.toStringAsFixed(2).split('.');
    final intPart = parts[0].replaceAllMapped(
      RegExp(r'(\d{1,3})(?=(\d{3})+(?!\d))'),
      (m) => '${m[1]},',
    );
    return '$intPart.${parts[1]}';
  }
}

class _TableRow extends StatelessWidget {
  const _TableRow({
    required this.label,
    required this.value,
    required this.valueColor,
    required this.valueFontWeight,
    this.isLast = false,
  });

  final String label;
  final String value;
  final Color valueColor;
  final FontWeight valueFontWeight;
  final bool isLast;

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        Padding(
          padding: const EdgeInsets.symmetric(vertical: 13),
          child: Row(
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: [
              Text(
                label,
                style: const TextStyle(
                  fontSize: 14,
                  color: Color(0xFF555555),
                  fontWeight: FontWeight.w400,
                ),
              ),
              Text(
                value,
                style: TextStyle(
                  fontSize: 14,
                  color: valueColor,
                  fontWeight: valueFontWeight,
                ),
              ),
            ],
          ),
        ),
        if (!isLast) const Divider(height: 1, color: Color(0xFFF2F2F2)),
      ],
    );
  }
}

// ── Customer Card (Type A) — centered ────────────────────────────────────────

class _CustomerCard extends StatelessWidget {
  const _CustomerCard({required this.transaction});
  final TransactionData transaction;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Column(
        children: [
          // Avatar
          Container(
            width: 64,
            height: 64,
            decoration: BoxDecoration(
              color: const Color(0xFFD0D0D0),
              shape: BoxShape.circle,
              image: transaction.customerAvatarUrl != null
                  ? DecorationImage(
                      image: NetworkImage(transaction.customerAvatarUrl!),
                      fit: BoxFit.cover,
                    )
                  : null,
            ),
            child: transaction.customerAvatarUrl == null
                ? const Icon(Icons.person, size: 32, color: Colors.white)
                : null,
          ),
          const SizedBox(height: 10),
          Text(
            transaction.customerName ?? '',
            style: const TextStyle(
              fontSize: 18,
              fontWeight: FontWeight.w800,
              color: Color(0xFF1A1A1A),
            ),
          ),
          const SizedBox(height: 8),
          Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(Icons.av_timer, size: 14, color: Color(0xFF888888)),
              const SizedBox(width: 4),
              Text(
                '${transaction.distanceKm} km',
                style: const TextStyle(fontSize: 12, color: Color(0xFF888888)),
              ),
              const SizedBox(width: 12),
              const Icon(Icons.access_time, size: 14, color: Color(0xFF888888)),
              const SizedBox(width: 4),
              Text(
                '${transaction.durationMin} min',
                style: const TextStyle(fontSize: 12, color: Color(0xFF888888)),
              ),
              const SizedBox(width: 12),
              const Icon(
                Icons.monetization_on_outlined,
                size: 14,
                color: Color(0xFF888888),
              ),
              const SizedBox(width: 4),
              Text(
                '₦${transaction.tripFee?.toStringAsFixed(2) ?? ''}',
                style: const TextStyle(fontSize: 12, color: Color(0xFF888888)),
              ),
            ],
          ),
        ],
      ),
    );
  }
}

// ── Trip Meta rows (Type A) ───────────────────────────────────────────────────

class _TripMeta extends StatelessWidget {
  const _TripMeta({required this.transaction});
  final TransactionData transaction;

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        _MetaRow(
          label: 'Trip Completed:',
          value: transaction.tripReference ?? '',
        ),
        const SizedBox(height: 10),
        _MetaRow(label: 'Date:', value: transaction.date ?? ''),
      ],
    );
  }
}

class _MetaRow extends StatelessWidget {
  const _MetaRow({required this.label, required this.value});
  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          label,
          style: const TextStyle(
            fontSize: 14,
            fontWeight: FontWeight.w700,
            color: Color(0xFF1A1A1A),
          ),
        ),
        const SizedBox(width: 8),
        Expanded(
          child: Text(
            value,
            textAlign: TextAlign.right,
            style: const TextStyle(
              fontSize: 13,
              color: Color(0xFF666666),
              fontWeight: FontWeight.w400,
            ),
          ),
        ),
      ],
    );
  }
}

// ── Route Card (Type A) ───────────────────────────────────────────────────────

class _RouteCard extends StatelessWidget {
  const _RouteCard({
    required this.pickup,
    required this.dropoff,
    required this.tripFee,
  });
  final String pickup;
  final String dropoff;
  final double tripFee;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: const Color(0xFFE8E8E8)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          IntrinsicHeight(
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                SizedBox(
                  width: 18,
                  child: Column(
                    children: [
                      Container(
                        width: 13,
                        height: 13,
                        decoration: const BoxDecoration(
                          color: Color(0xFF4CAF50),
                          shape: BoxShape.circle,
                        ),
                      ),
                      Expanded(
                        child: Center(
                          child: Container(
                            width: 2,
                            color: const Color(0xFF4CAF50),
                          ),
                        ),
                      ),
                      Container(
                        width: 13,
                        height: 13,
                        decoration: BoxDecoration(
                          shape: BoxShape.circle,
                          border: Border.all(
                            color: const Color(0xFF4CAF50),
                            width: 2.5,
                          ),
                        ),
                      ),
                    ],
                  ),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      const Text(
                        'Pick-up',
                        style: TextStyle(
                          fontSize: 11,
                          color: Color(0xFF888888),
                        ),
                      ),
                      const SizedBox(height: 2),
                      Text(
                        pickup,
                        style: const TextStyle(
                          fontSize: 13,
                          fontWeight: FontWeight.w700,
                          color: Color(0xFF1A1A1A),
                        ),
                      ),
                      const SizedBox(height: 14),
                      const Text(
                        'Drop off (optional)',
                        style: TextStyle(
                          fontSize: 11,
                          color: Color(0xFF888888),
                        ),
                      ),
                      const SizedBox(height: 2),
                      Text(
                        dropoff,
                        style: const TextStyle(
                          fontSize: 13,
                          fontWeight: FontWeight.w700,
                          color: Color(0xFF1A1A1A),
                        ),
                      ),
                    ],
                  ),
                ),
              ],
            ),
          ),

          const SizedBox(height: 16),
          const Divider(height: 1, color: Color(0xFFF0F0F0)),
          const SizedBox(height: 14),

          Row(
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: [
              const Text(
                'Trip Fee:',
                style: TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w600,
                  color: Color(0xFF1A1A1A),
                ),
              ),
              Text(
                '₦${tripFee.toStringAsFixed(2)}',
                style: const TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w700,
                  color: Color(0xFF1A1A1A),
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }
}

// ── Go to Trips link (Type A) ─────────────────────────────────────────────────

class _GoToTripsLink extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: () {
        Navigator.of(context).popUntil((route) => route.isFirst);
      },
      child: const Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(Icons.arrow_forward, size: 16, color: Color(0xFF4CAF50)),
          SizedBox(width: 6),
          Text(
            'Go to Trips',
            style: TextStyle(
              fontSize: 14,
              fontWeight: FontWeight.w600,
              color: Color(0xFF4CAF50),
            ),
          ),
        ],
      ),
    );
  }
}

// ── Support Card ──────────────────────────────────────────────────────────────

class _SupportCard extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 16),
      decoration: BoxDecoration(
        color: const Color(0xFFF7F7F7),
        borderRadius: BorderRadius.circular(16),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text(
            'Any Question about this transaction?',
            style: TextStyle(
              fontSize: 13,
              color: Color(0xFF888888),
              fontWeight: FontWeight.w400,
            ),
          ),
          const SizedBox(height: 10),
          GestureDetector(
            onTap: () {},
            child: const Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(
                  Icons.headset_mic_outlined,
                  size: 20,
                  color: Color(0xFF4CAF50),
                ),
                SizedBox(width: 8),
                Text(
                  'Contact Support',
                  style: TextStyle(
                    fontSize: 14,
                    fontWeight: FontWeight.w700,
                    color: Color(0xFF4CAF50),
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}
