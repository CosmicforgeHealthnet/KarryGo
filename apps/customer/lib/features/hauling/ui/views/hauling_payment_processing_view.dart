import 'package:flutter/material.dart';

import '../../state/hauling_booking_controller.dart';

class HaulingPaymentProcessingView extends StatelessWidget {
  const HaulingPaymentProcessingView({super.key, required this.controller});

  final HaulingBookingController controller;

  @override
  Widget build(BuildContext context) {
    final fareKobo = controller.state.fareEstimate?.fareEstimateKobo ?? 0;
    final fareNaira = fareKobo / 100;

    return Scaffold(
      backgroundColor: const Color(0xFF1F7A4D),
      body: Center(
        child: Padding(
          padding: const EdgeInsets.all(32),
          child: Column(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              const Text(
                'Amount to pay',
                style: TextStyle(color: Colors.white70, fontSize: 14),
              ),
              const SizedBox(height: 4),
              const Text(
                'Total',
                style: TextStyle(color: Colors.white, fontSize: 16, fontWeight: FontWeight.w600),
              ),
              const SizedBox(height: 16),
              Text(
                '₦${fareNaira.toStringAsFixed(2)}',
                style: const TextStyle(
                  color: Colors.white,
                  fontSize: 40,
                  fontWeight: FontWeight.w800,
                  letterSpacing: -1,
                ),
              ),
              const SizedBox(height: 48),
              const SizedBox(
                width: 48, height: 48,
                child: CircularProgressIndicator(
                  color: Colors.white,
                  strokeWidth: 3,
                ),
              ),
              const SizedBox(height: 20),
              const Text(
                'Payment Processing...',
                style: TextStyle(
                  color: Colors.white,
                  fontSize: 16,
                  fontWeight: FontWeight.w600,
                ),
              ),
              const SizedBox(height: 8),
              const Text(
                'Please wait while we process your payment\nand find you a truck.',
                style: TextStyle(color: Colors.white70, fontSize: 13, height: 1.5),
                textAlign: TextAlign.center,
              ),
            ],
          ),
        ),
      ),
    );
  }
}
