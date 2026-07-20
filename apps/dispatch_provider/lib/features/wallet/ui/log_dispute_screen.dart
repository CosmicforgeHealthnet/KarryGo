import 'package:flutter/material.dart';

import '../../disputes/models/dispute_models.dart';
import '../../disputes/state/rider_dispute_controller.dart';
import '../../trips/data/trip_model.dart';
import '../../trips/state/trips_controller.dart';
import 'dispute_detail_screen.dart';

/// Log Disputes — select one of your trips, pick a dispute type, and file it
/// as a complaint on support-dispute-service. Existing disputes are listed at
/// the top and open their detail/status screen.
class LogDisputeScreen extends StatefulWidget {
  const LogDisputeScreen({
    super.key,
    required this.disputeController,
    required this.tripsController,
  });

  final RiderDisputeController disputeController;
  final TripsController tripsController;

  @override
  State<LogDisputeScreen> createState() => _LogDisputeScreenState();
}

class _LogDisputeScreenState extends State<LogDisputeScreen> {
  @override
  void initState() {
    super.initState();
    widget.disputeController.startNewDispute();
    widget.disputeController.loadComplaints();
    widget.tripsController.loadTrips();
  }

  bool get _canConfirm =>
      widget.disputeController.selectedTrip != null &&
      widget.disputeController.selectedType != null &&
      !widget.disputeController.submitting;

  void _openTripPicker() {
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      backgroundColor: Colors.white,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(20)),
      ),
      builder: (_) => _TripPickerSheet(
        tripsController: widget.tripsController,
        onSelect: (trip) {
          widget.disputeController.selectTrip(trip);
          Navigator.of(context).pop();
        },
      ),
    );
  }

  Future<void> _submit() async {
    final complaint = await widget.disputeController.submitDispute();
    if (complaint == null || !mounted) return;
    Navigator.of(context).pushReplacement(
      MaterialPageRoute(
        builder: (_) => DisputeDetailScreen(
          complaint: complaint,
          disputeController: widget.disputeController,
        ),
      ),
    );
  }

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
          'Log Disputes',
          style: TextStyle(
            fontSize: 16,
            fontWeight: FontWeight.w700,
            color: Color(0xFF1A1A1A),
          ),
        ),
        centerTitle: false,
      ),
      body: AnimatedBuilder(
        animation: Listenable.merge([
          widget.disputeController,
          widget.tripsController,
        ]),
        builder: (context, _) {
          final controller = widget.disputeController;
          final selectedTrip = controller.selectedTrip;
          final showDisputeType = selectedTrip != null;

          return Column(
            children: [
              Expanded(
                child: SingleChildScrollView(
                  padding: const EdgeInsets.fromLTRB(20, 12, 20, 24),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      if (controller.complaints.isNotEmpty) ...[
                        const Text(
                          'My Disputes',
                          style: TextStyle(
                            fontSize: 15,
                            fontWeight: FontWeight.w700,
                            color: Color(0xFF1A1A1A),
                          ),
                        ),
                        const SizedBox(height: 10),
                        ...controller.complaints
                            .where(
                              (c) =>
                                  c.subject !=
                                  RiderDisputeController.supportChatSubject,
                            )
                            .map(
                              (c) => _ComplaintCard(
                                complaint: c,
                                onTap: () => Navigator.of(context).push(
                                  MaterialPageRoute(
                                    builder: (_) => DisputeDetailScreen(
                                      complaint: c,
                                      disputeController: controller,
                                    ),
                                  ),
                                ),
                              ),
                            ),
                        const SizedBox(height: 24),
                      ],

                      // ── Select trip ─────────────────────────────────────
                      const Text(
                        'Select a Trip',
                        style: TextStyle(
                          fontSize: 15,
                          fontWeight: FontWeight.w700,
                          color: Color(0xFF1A1A1A),
                        ),
                      ),
                      const SizedBox(height: 10),
                      GestureDetector(
                        onTap: _openTripPicker,
                        child: Container(
                          padding: const EdgeInsets.symmetric(
                            horizontal: 16,
                            vertical: 14,
                          ),
                          decoration: BoxDecoration(
                            color: selectedTrip != null
                                ? const Color(0xFFF0FAF0)
                                : const Color(0xFFF5F5F5),
                            borderRadius: BorderRadius.circular(12),
                            border: Border.all(
                              color: selectedTrip != null
                                  ? const Color(0xFF4CAF50)
                                  : const Color(0xFFDDDDDD),
                            ),
                          ),
                          child: selectedTrip != null
                              ? Column(
                                  crossAxisAlignment: CrossAxisAlignment.start,
                                  children: [
                                    Text(
                                      selectedTrip.dropoffAddress.isNotEmpty
                                          ? selectedTrip.dropoffAddress
                                          : 'Delivery Trip',
                                      style: const TextStyle(
                                        fontSize: 14,
                                        fontWeight: FontWeight.w700,
                                        color: Color(0xFF1A1A1A),
                                      ),
                                    ),
                                    const SizedBox(height: 2),
                                    Text(
                                      _formatDate(selectedTrip.createdAt),
                                      style: const TextStyle(
                                        fontSize: 12,
                                        color: Color(0xFF888888),
                                      ),
                                    ),
                                    const SizedBox(height: 4),
                                    Text(
                                      _formatFare(
                                        selectedTrip.estimatedFareKobo,
                                      ),
                                      style: const TextStyle(
                                        fontSize: 14,
                                        fontWeight: FontWeight.w700,
                                        color: Color(0xFF4CAF50),
                                      ),
                                    ),
                                  ],
                                )
                              : const Row(
                                  mainAxisAlignment:
                                      MainAxisAlignment.spaceBetween,
                                  children: [
                                    Text(
                                      'Select Trip',
                                      style: TextStyle(
                                        fontSize: 14,
                                        color: Color(0xFF888888),
                                      ),
                                    ),
                                    Icon(
                                      Icons.chevron_right,
                                      color: Color(0xFF888888),
                                    ),
                                  ],
                                ),
                        ),
                      ),

                      if (showDisputeType) ...[
                        const SizedBox(height: 24),
                        const Text(
                          'Select Dispute Type',
                          style: TextStyle(
                            fontSize: 15,
                            fontWeight: FontWeight.w700,
                            color: Color(0xFF1A1A1A),
                          ),
                        ),
                        const SizedBox(height: 10),
                        Container(
                          decoration: BoxDecoration(
                            color: Colors.white,
                            borderRadius: BorderRadius.circular(12),
                            border: Border.all(color: const Color(0xFFEEEEEE)),
                          ),
                          child: Column(
                            children: disputeTypes.asMap().entries.map((e) {
                              final i = e.key;
                              final type = e.value;
                              final isLast = i == disputeTypes.length - 1;
                              final isSelected =
                                  controller.selectedType == type;
                              return GestureDetector(
                                onTap: () => controller.selectType(type),
                                child: Container(
                                  padding: const EdgeInsets.symmetric(
                                    horizontal: 16,
                                    vertical: 14,
                                  ),
                                  decoration: BoxDecoration(
                                    color: isSelected
                                        ? const Color(0xFFF0FAF0)
                                        : Colors.transparent,
                                    border: isLast
                                        ? null
                                        : const Border(
                                            bottom: BorderSide(
                                              color: Color(0xFFF0F0F0),
                                              width: 1,
                                            ),
                                          ),
                                  ),
                                  child: Row(
                                    children: [
                                      Expanded(
                                        child: Text(
                                          type,
                                          style: TextStyle(
                                            fontSize: 14,
                                            color: isSelected
                                                ? const Color(0xFF4CAF50)
                                                : const Color(0xFF1A1A1A),
                                          ),
                                        ),
                                      ),
                                      if (isSelected)
                                        const Icon(
                                          Icons.check,
                                          size: 16,
                                          color: Color(0xFF4CAF50),
                                        ),
                                    ],
                                  ),
                                ),
                              );
                            }).toList(),
                          ),
                        ),
                      ],

                      if (controller.submitError != null) ...[
                        const SizedBox(height: 16),
                        Container(
                          width: double.infinity,
                          padding: const EdgeInsets.all(12),
                          decoration: BoxDecoration(
                            color: const Color(0xFFFFF0F0),
                            borderRadius: BorderRadius.circular(10),
                            border: Border.all(color: const Color(0xFFE53935)),
                          ),
                          child: Text(
                            controller.submitError!,
                            style: const TextStyle(
                              fontSize: 13,
                              color: Color(0xFFE53935),
                            ),
                          ),
                        ),
                      ],
                    ],
                  ),
                ),
              ),

              // ── Confirm button ────────────────────────────────────────────
              SafeArea(
                child: Padding(
                  padding: const EdgeInsets.fromLTRB(20, 12, 20, 16),
                  child: SizedBox(
                    width: double.infinity,
                    height: 52,
                    child: FilledButton(
                      onPressed: _canConfirm ? _submit : null,
                      style: FilledButton.styleFrom(
                        backgroundColor: const Color(0xFF4CAF50),
                        disabledBackgroundColor: const Color(
                          0xFF4CAF50,
                        ).withValues(alpha: 0.4),
                        shape: RoundedRectangleBorder(
                          borderRadius: BorderRadius.circular(999),
                        ),
                      ),
                      child: controller.submitting
                          ? const SizedBox(
                              width: 22,
                              height: 22,
                              child: CircularProgressIndicator(
                                strokeWidth: 2,
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
                ),
              ),
            ],
          );
        },
      ),
    );
  }
}

String _formatDate(DateTime time) {
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
  final hour12 = time.hour % 12 == 0 ? 12 : time.hour % 12;
  final minute = time.minute.toString().padLeft(2, '0');
  final period = time.hour < 12 ? 'AM' : 'PM';
  return '${time.day} ${months[time.month - 1]} ${time.year}, $hour12:$minute $period';
}

String _formatFare(int kobo) => '₦${(kobo / 100).toStringAsFixed(2)}';

class _ComplaintCard extends StatelessWidget {
  const _ComplaintCard({required this.complaint, required this.onTap});

  final Complaint complaint;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final statusColor = switch (complaint.stage) {
      2 => const Color(0xFF4CAF50),
      1 => const Color(0xFFFFA000),
      _ => const Color(0xFF888888),
    };
    return GestureDetector(
      onTap: onTap,
      child: Container(
        margin: const EdgeInsets.only(bottom: 10),
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
        decoration: BoxDecoration(
          color: Colors.white,
          borderRadius: BorderRadius.circular(12),
          border: Border.all(color: const Color(0xFFEEEEEE)),
        ),
        child: Row(
          children: [
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    complaint.subject,
                    style: const TextStyle(
                      fontSize: 14,
                      fontWeight: FontWeight.w700,
                      color: Color(0xFF1A1A1A),
                    ),
                  ),
                  const SizedBox(height: 2),
                  Text(
                    _formatDate(complaint.createdAt),
                    style: const TextStyle(
                      fontSize: 12,
                      color: Color(0xFF888888),
                    ),
                  ),
                ],
              ),
            ),
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
              decoration: BoxDecoration(
                color: statusColor.withValues(alpha: 0.12),
                borderRadius: BorderRadius.circular(999),
              ),
              child: Text(
                complaint.statusLabel,
                style: TextStyle(
                  fontSize: 11,
                  fontWeight: FontWeight.w700,
                  color: statusColor,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _TripPickerSheet extends StatelessWidget {
  const _TripPickerSheet({required this.tripsController, required this.onSelect});

  final TripsController tripsController;
  final ValueChanged<TripModel> onSelect;

  @override
  Widget build(BuildContext context) {
    return DraggableScrollableSheet(
      initialChildSize: 0.7,
      maxChildSize: 0.9,
      minChildSize: 0.5,
      expand: false,
      builder: (_, scrollController) => AnimatedBuilder(
        animation: tripsController,
        builder: (context, _) {
          final trips = tripsController.trips;
          return Column(
            children: [
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 16, 20, 8),
                child: Row(
                  mainAxisAlignment: MainAxisAlignment.spaceBetween,
                  children: [
                    const Text(
                      'Select the trip',
                      style: TextStyle(
                        fontSize: 16,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                    GestureDetector(
                      onTap: () => Navigator.of(context).pop(),
                      child: const Icon(Icons.close, size: 22),
                    ),
                  ],
                ),
              ),
              if (tripsController.isLoading && trips.isEmpty)
                const Expanded(
                  child: Center(
                    child: CircularProgressIndicator(color: Color(0xFF4CAF50)),
                  ),
                )
              else if (trips.isEmpty)
                const Expanded(
                  child: Center(
                    child: Text(
                      'No trips yet.\nCompleted trips appear here.',
                      textAlign: TextAlign.center,
                      style: TextStyle(fontSize: 13, color: Color(0xFF888888)),
                    ),
                  ),
                )
              else
                Expanded(
                  child: ListView.builder(
                    controller: scrollController,
                    padding: const EdgeInsets.fromLTRB(0, 0, 0, 24),
                    itemCount: trips.length,
                    itemBuilder: (context, index) {
                      final trip = trips[index];
                      return GestureDetector(
                        onTap: () => onSelect(trip),
                        child: Container(
                          padding: const EdgeInsets.fromLTRB(20, 12, 20, 12),
                          decoration: const BoxDecoration(
                            border: Border(
                              bottom: BorderSide(
                                color: Color(0xFFF5F5F5),
                                width: 1,
                              ),
                            ),
                          ),
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              Text(
                                trip.dropoffAddress.isNotEmpty
                                    ? trip.dropoffAddress
                                    : 'Delivery Trip',
                                style: const TextStyle(
                                  fontSize: 14,
                                  fontWeight: FontWeight.w700,
                                  color: Color(0xFF1A1A1A),
                                ),
                              ),
                              const SizedBox(height: 2),
                              Text(
                                trip.rawStatus,
                                style: const TextStyle(
                                  fontSize: 12,
                                  color: Color(0xFF888888),
                                ),
                              ),
                              const SizedBox(height: 2),
                              Text(
                                _formatDate(trip.createdAt),
                                style: const TextStyle(
                                  fontSize: 12,
                                  color: Color(0xFF888888),
                                ),
                              ),
                              const SizedBox(height: 4),
                              Text(
                                _formatFare(trip.estimatedFareKobo),
                                style: const TextStyle(
                                  fontSize: 14,
                                  fontWeight: FontWeight.w700,
                                  color: Color(0xFF4CAF50),
                                ),
                              ),
                            ],
                          ),
                        ),
                      );
                    },
                  ),
                ),
            ],
          );
        },
      ),
    );
  }
}
