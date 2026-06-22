import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../state/hauling_booking_controller.dart';

class HaulingAvailabilityCheckView extends StatefulWidget {
  const HaulingAvailabilityCheckView({super.key, required this.controller});

  final HaulingBookingController controller;

  @override
  State<HaulingAvailabilityCheckView> createState() => _HaulingAvailabilityCheckViewState();
}

class _HaulingAvailabilityCheckViewState extends State<HaulingAvailabilityCheckView> {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      widget.controller.startHaulingFlow();
    });
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: CustomerFigmaColors.surface,
      appBar: AppBar(
        backgroundColor: CustomerFigmaColors.surface,
        elevation: 0,
        leading: IconButton(
          onPressed: () => Navigator.of(context).pop(),
          icon: const Icon(Icons.close_rounded),
          color: CustomerFigmaColors.text,
        ),
      ),
      body: const Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            CircularProgressIndicator(color: CustomerFigmaColors.primary),
            SizedBox(height: 20),
            Text(
              'Checking truck availability...',
              style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 14),
            ),
          ],
        ),
      ),
    );
  }
}
