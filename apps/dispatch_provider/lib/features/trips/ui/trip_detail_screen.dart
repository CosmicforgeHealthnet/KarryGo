import 'package:flutter/material.dart';

import '../data/trip_model.dart';

class TripDetailScreen extends StatelessWidget {
  const TripDetailScreen({super.key, required this.trip});
  final TripModel trip;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
      appBar: AppBar(
        backgroundColor: Colors.white,
        elevation: 0,
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
          'Trip Detail',
          style: TextStyle(
            fontSize: 16,
            fontWeight: FontWeight.w700,
            color: Color(0xFF1A1A1A),
          ),
        ),
        centerTitle: false,
      ),
      body: SingleChildScrollView(
        padding: const EdgeInsets.fromLTRB(20, 8, 20, 40),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            // ── Avatar + name + stats ───────────────────────────────────────
            Center(
              child: Column(
                children: [
                  Container(
                    width: 72,
                    height: 72,
                    decoration: const BoxDecoration(
                      color: Color(0xFFD0D0D0),
                      shape: BoxShape.circle,
                    ),
                    child: const Icon(
                      Icons.person,
                      size: 40,
                      color: Colors.white,
                    ),
                  ),
                  const SizedBox(height: 12),
                  Text(
                    trip.customerName,
                    style: const TextStyle(
                      fontSize: 22,
                      fontWeight: FontWeight.w800,
                      color: Color(0xFF1A1A1A),
                    ),
                  ),
                  const SizedBox(height: 8),
                  Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      const Icon(
                        Icons.av_timer,
                        size: 14,
                        color: Color(0xFF888888),
                      ),
                      const SizedBox(width: 4),
                      Text(
                        trip.distanceDisplay,
                        style: const TextStyle(
                          fontSize: 12,
                          color: Color(0xFF888888),
                        ),
                      ),
                      const SizedBox(width: 12),
                      const Icon(
                        Icons.attach_money,
                        size: 14,
                        color: Color(0xFF888888),
                      ),
                      const SizedBox(width: 4),
                      Text(
                        trip.fareDisplay,
                        style: const TextStyle(
                          fontSize: 12,
                          color: Color(0xFF888888),
                        ),
                      ),
                    ],
                  ),
                ],
              ),
            ),

            const SizedBox(height: 20),
            const Divider(color: Color(0xFFF0F0F0)),
            const SizedBox(height: 16),

            // ── Booking + Date ──────────────────────────────────────────────
            _InfoRow(
              label: 'Booking ID:',
              value: trip.bookingId.isNotEmpty ? trip.bookingId : '—',
            ),
            const SizedBox(height: 12),
            _InfoRow(label: 'Date:', value: _formatDate(trip.createdAt)),
            if (trip.completedAt != null) ...[
              const SizedBox(height: 12),
              _InfoRow(
                label: 'Completed:',
                value: _formatDate(trip.completedAt!),
              ),
            ],

            const SizedBox(height: 20),

            // ── Status Timeline ─────────────────────────────────────────────
            const Text(
              'Trip Status',
              style: TextStyle(
                fontSize: 16,
                fontWeight: FontWeight.w800,
                color: Color(0xFF1A1A1A),
              ),
            ),
            const SizedBox(height: 14),
            _TripTimeline(statusCode: trip.statusCode),

            const SizedBox(height: 24),

            // ── Route card ──────────────────────────────────────────────────
            Container(
              padding: const EdgeInsets.all(16),
              decoration: BoxDecoration(
                color: const Color(0xFFF9F9F9),
                borderRadius: BorderRadius.circular(16),
                border: Border.all(color: const Color(0xFFEEEEEE)),
              ),
              child: Column(
                children: [
                  _RouteInfo(
                    pickup: trip.pickupAddress,
                    dropoff: trip.dropoffAddress,
                  ),
                  const SizedBox(height: 16),
                  const Divider(color: Color(0xFFEEEEEE), height: 1),
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
                        trip.fareDisplay,
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
            ),

            const SizedBox(height: 24),

            // ── Receiver Information ────────────────────────────────────────
            const Text(
              'Receiver Information',
              style: TextStyle(
                fontSize: 18,
                fontWeight: FontWeight.w800,
                color: Color(0xFF1A1A1A),
              ),
            ),
            const SizedBox(height: 16),
            _LabelValue(
              label: 'Full Name',
              value: trip.receiverName.isNotEmpty ? trip.receiverName : '—',
            ),
            const SizedBox(height: 14),
            _LabelValue(
              label: 'Phone Number',
              value: trip.receiverPhone.isNotEmpty ? trip.receiverPhone : '—',
            ),

            if (trip.notes != null && trip.notes!.isNotEmpty) ...[
              const SizedBox(height: 24),
              const Text(
                'Notes',
                style: TextStyle(
                  fontSize: 18,
                  fontWeight: FontWeight.w800,
                  color: Color(0xFF1A1A1A),
                ),
              ),
              const SizedBox(height: 12),
              Text(
                trip.notes!,
                style: const TextStyle(
                  fontSize: 14,
                  color: Color(0xFF555555),
                  height: 1.5,
                ),
              ),
            ],

            // ── Proof Status ────────────────────────────────────────────────
            if (trip.statusCode == TripStatusCode.proofSubmitted ||
                trip.statusCode == TripStatusCode.completed) ...[
              const SizedBox(height: 24),
              const Text(
                'Proof of Delivery',
                style: TextStyle(
                  fontSize: 16,
                  fontWeight: FontWeight.w800,
                  color: Color(0xFF1A1A1A),
                ),
              ),
              const SizedBox(height: 12),
              Container(
                padding: const EdgeInsets.all(14),
                decoration: BoxDecoration(
                  color: const Color(0xFFE8F5E9),
                  borderRadius: BorderRadius.circular(12),
                ),
                child: Row(
                  children: [
                    const Icon(
                      Icons.check_circle,
                      color: Color(0xFF4CAF50),
                      size: 24,
                    ),
                    const SizedBox(width: 12),
                    Expanded(
                      child: Text(
                        trip.proofStatus == 'submitted'
                            ? 'Proof submitted and pending review.'
                            : trip.proofUrl != null
                            ? 'Proof of delivery uploaded.'
                            : 'Proof of delivery on record.',
                        style: const TextStyle(
                          fontSize: 13,
                          color: Color(0xFF2E7D32),
                          fontWeight: FontWeight.w600,
                        ),
                      ),
                    ),
                  ],
                ),
              ),
            ],
          ],
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
    final h = dt.hour.toString().padLeft(2, '0');
    final m = dt.minute.toString().padLeft(2, '0');
    return '${dt.day.toString().padLeft(2, '0')} ${months[dt.month - 1]} ${dt.year}, $h:$m';
  }
}

// ── Timeline ──────────────────────────────────────────────────────────────────

class _TripTimeline extends StatelessWidget {
  const _TripTimeline({required this.statusCode});
  final TripStatusCode statusCode;

  static const _steps = [
    (TripStatusCode.assigned, 'Assigned', 'Request accepted by provider'),
    (
      TripStatusCode.arrivedPickup,
      'At Pickup',
      'Provider arrived at pickup point',
    ),
    (TripStatusCode.inProgress, 'In Progress', 'Package collected, en route'),
    (
      TripStatusCode.proofSubmitted,
      'Proof Submitted',
      'Delivery proof uploaded',
    ),
    (TripStatusCode.completed, 'Completed', 'Package delivered successfully'),
  ];

  int get _currentIndex {
    if (statusCode == TripStatusCode.cancelled) return -1;
    return _steps.indexWhere((s) => s.$1 == statusCode);
  }

  @override
  Widget build(BuildContext context) {
    if (statusCode == TripStatusCode.cancelled) {
      return Container(
        padding: const EdgeInsets.all(14),
        decoration: BoxDecoration(
          color: const Color(0xFFFFEBEE),
          borderRadius: BorderRadius.circular(12),
        ),
        child: const Row(
          children: [
            Icon(Icons.cancel_outlined, color: Color(0xFFE53935), size: 22),
            SizedBox(width: 10),
            Text(
              'This trip was cancelled.',
              style: TextStyle(
                fontSize: 13,
                color: Color(0xFFE53935),
                fontWeight: FontWeight.w600,
              ),
            ),
          ],
        ),
      );
    }

    final current = _currentIndex;
    return Column(
      children: List.generate(_steps.length, (i) {
        final (_, label, desc) = _steps[i];
        final isDone = i < current;
        final isCurrent = i == current;
        final isLast = i == _steps.length - 1;

        return IntrinsicHeight(
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              SizedBox(
                width: 28,
                child: Column(
                  children: [
                    Container(
                      width: 20,
                      height: 20,
                      decoration: BoxDecoration(
                        shape: BoxShape.circle,
                        color: isDone || isCurrent
                            ? const Color(0xFF4CAF50)
                            : const Color(0xFFE0E0E0),
                      ),
                      child: Icon(
                        isDone ? Icons.check : Icons.circle,
                        size: isDone ? 12 : 8,
                        color: Colors.white,
                      ),
                    ),
                    if (!isLast)
                      Expanded(
                        child: Container(
                          width: 2,
                          color: isDone
                              ? const Color(0xFF4CAF50)
                              : const Color(0xFFE0E0E0),
                        ),
                      ),
                  ],
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Padding(
                  padding: EdgeInsets.only(bottom: isLast ? 0 : 16),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        label,
                        style: TextStyle(
                          fontSize: 13,
                          fontWeight: FontWeight.w700,
                          color: isCurrent
                              ? const Color(0xFF4CAF50)
                              : isDone
                              ? const Color(0xFF1A1A1A)
                              : const Color(0xFFAAAAAA),
                        ),
                      ),
                      const SizedBox(height: 2),
                      Text(
                        desc,
                        style: TextStyle(
                          fontSize: 12,
                          color: isDone || isCurrent
                              ? const Color(0xFF555555)
                              : const Color(0xFFCCCCCC),
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ],
          ),
        );
      }),
    );
  }
}

// ── Shared widgets ─────────────────────────────────────────────────────────────

class _InfoRow extends StatelessWidget {
  const _InfoRow({required this.label, required this.value});
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
        const Spacer(),
        Text(
          value,
          style: const TextStyle(fontSize: 13, color: Color(0xFF888888)),
        ),
      ],
    );
  }
}

class _LabelValue extends StatelessWidget {
  const _LabelValue({required this.label, required this.value});
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
            fontSize: 14,
            fontWeight: FontWeight.w700,
            color: Color(0xFF1A1A1A),
          ),
        ),
        const SizedBox(height: 2),
        Text(
          value,
          style: const TextStyle(fontSize: 14, color: Color(0xFF555555)),
        ),
      ],
    );
  }
}

class _RouteInfo extends StatelessWidget {
  const _RouteInfo({required this.pickup, required this.dropoff});
  final String pickup;
  final String dropoff;

  @override
  Widget build(BuildContext context) {
    return IntrinsicHeight(
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          SizedBox(
            width: 18,
            child: Column(
              children: [
                Container(
                  width: 14,
                  height: 14,
                  decoration: const BoxDecoration(
                    color: Color(0xFF4CAF50),
                    shape: BoxShape.circle,
                  ),
                ),
                Expanded(
                  child: Center(
                    child: Container(width: 2, color: const Color(0xFF4CAF50)),
                  ),
                ),
                Container(
                  width: 14,
                  height: 14,
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
                  style: TextStyle(fontSize: 11, color: Color(0xFF888888)),
                ),
                const SizedBox(height: 2),
                Text(
                  pickup,
                  style: const TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w700,
                  ),
                ),
                const SizedBox(height: 14),
                const Text(
                  'Drop-off',
                  style: TextStyle(fontSize: 11, color: Color(0xFF888888)),
                ),
                const SizedBox(height: 2),
                Text(
                  dropoff,
                  style: const TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w700,
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
