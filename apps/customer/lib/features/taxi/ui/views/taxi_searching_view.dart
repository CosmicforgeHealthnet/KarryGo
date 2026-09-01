import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../state/taxi_booking_controller.dart';
import '../widgets/taxi_flow_helpers.dart';

/// Shown while the matcher is offering the ride to nearby drivers.
class TaxiSearchingView extends StatefulWidget {
  const TaxiSearchingView({
    super.key,
    required this.controller,
    required this.state,
  });

  final TaxiBookingController controller;
  final TaxiBookingState state;

  @override
  State<TaxiSearchingView> createState() => _TaxiSearchingViewState();
}

class _TaxiSearchingViewState extends State<TaxiSearchingView>
    with SingleTickerProviderStateMixin {
  late final AnimationController _pulse = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 1200),
  )..repeat(reverse: true);

  @override
  void dispose() {
    _pulse.dispose();
    super.dispose();
  }

  Future<void> _confirmCancel() async {
    final confirmed = await showModalBottomSheet<bool>(
      context: context,
      backgroundColor: Colors.white,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(20)),
      ),
      builder: (sheetContext) => SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(20),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Text(
                'Cancel this ride?',
                style: TextStyle(
                  fontSize: 17,
                  fontWeight: FontWeight.w800,
                  color: CustomerFigmaColors.text,
                ),
              ),
              const SizedBox(height: 8),
              const Text(
                'We will stop looking for a driver.',
                textAlign: TextAlign.center,
                style: TextStyle(
                  color: CustomerFigmaColors.muted,
                  fontSize: 13,
                ),
              ),
              const SizedBox(height: 18),
              FigmaPrimaryButton(
                label: 'Yes, cancel',
                onPressed: () => Navigator.of(sheetContext).pop(true),
              ),
              const SizedBox(height: 8),
              FigmaSecondaryButton(
                label: 'Keep looking',
                onPressed: () => Navigator.of(sheetContext).pop(false),
              ),
            ],
          ),
        ),
      ),
    );
    if (confirmed == true) {
      await widget.controller.cancelBooking();
    }
  }

  @override
  Widget build(BuildContext context) {
    final state = widget.state;
    return taxiFlowScaffold(
      body: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          ScaleTransition(
            scale: Tween<double>(begin: 0.88, end: 1.06).animate(
              CurvedAnimation(parent: _pulse, curve: Curves.easeInOut),
            ),
            child: Container(
              width: 120,
              height: 120,
              decoration: const BoxDecoration(
                color: CustomerFigmaColors.primaryTint,
                shape: BoxShape.circle,
              ),
              child: const Icon(
                Icons.local_taxi_rounded,
                size: 52,
                color: CustomerFigmaColors.primary,
              ),
            ),
          ),
          const SizedBox(height: 26),
          const Text(
            'Finding you a driver...',
            style: TextStyle(
              color: CustomerFigmaColors.text,
              fontSize: 19,
              fontWeight: FontWeight.w800,
            ),
          ),
          const SizedBox(height: 8),
          const Text(
            'We are offering your ride to drivers nearby.',
            textAlign: TextAlign.center,
            style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
          ),
          const SizedBox(height: 28),
          TaxiRouteSummary(
            pickupAddress: state.pickupAddress,
            dropoffAddress: state.dropoffAddress,
          ),
          if (state.error != null) ...[
            const SizedBox(height: 16),
            Container(
              padding: const EdgeInsets.all(12),
              decoration: BoxDecoration(
                color: const Color(0xFFFDECEC),
                borderRadius: BorderRadius.circular(10),
                border: Border.all(color: const Color(0xFFF5B5B5)),
              ),
              child: Text(
                state.error!,
                textAlign: TextAlign.center,
                style: const TextStyle(color: Color(0xFFB3261E), fontSize: 12),
              ),
            ),
          ],
        ],
      ),
      // FigmaSecondaryButton takes a non-null callback, so an in-flight
      // cancel is swallowed here rather than by a disabled button.
      bottom: FigmaSecondaryButton(
        label: state.isLoading ? 'Cancelling...' : 'Cancel ride',
        onPressed: () {
          if (state.isLoading) return;
          _confirmCancel();
        },
      ),
    );
  }
}
