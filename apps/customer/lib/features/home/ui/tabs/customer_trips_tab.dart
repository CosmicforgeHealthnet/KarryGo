import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';

class CustomerTripsTab extends StatelessWidget {
  const CustomerTripsTab({super.key});

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: CustomerFigmaColors.surface,
      appBar: AppBar(
        backgroundColor: Colors.white,
        elevation: 0,
        centerTitle: false,
        title: const Text(
          'My Trips',
          style: TextStyle(
            color: CustomerFigmaColors.text,
            fontWeight: FontWeight.w800,
            fontSize: 18,
          ),
        ),
      ),
      body: Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Container(
              width: 80,
              height: 80,
              decoration: const BoxDecoration(
                color: CustomerFigmaColors.primaryPale,
                shape: BoxShape.circle,
              ),
              child: const Icon(
                Icons.receipt_long_rounded,
                size: 38,
                color: CustomerFigmaColors.primary,
              ),
            ),
            const SizedBox(height: 20),
            const Text(
              'No trips yet',
              style: TextStyle(
                color: CustomerFigmaColors.text,
                fontSize: 17,
                fontWeight: FontWeight.w800,
              ),
            ),
            const SizedBox(height: 8),
            const Text(
              'Your completed trips will appear here.',
              style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
            ),
          ],
        ),
      ),
    );
  }
}
