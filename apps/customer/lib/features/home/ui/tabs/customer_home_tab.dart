import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../../auth/state/customer_auth_controller.dart';
import '../../../hauling/data/places_api.dart';
import '../../../hauling/state/hauling_booking_controller.dart';
import '../../../hauling/ui/hauling_flow_screen.dart';
import '../widgets/customer_home_map.dart';

class CustomerHomeTab extends StatelessWidget {
  const CustomerHomeTab({
    super.key,
    required this.state,
    required this.controller,
    required this.haulingController,
    required this.placesApi,
  });

  final CustomerAuthState state;
  final CustomerAuthController controller;
  final HaulingBookingController haulingController;
  final PlacesApi placesApi;

  void _onServiceTap(BuildContext context, int index) {
    switch (index) {
      case 2:
        haulingController.reset();
        Navigator.of(context).push(
          MaterialPageRoute(
            fullscreenDialog: true,
            builder: (_) => HaulingFlowScreen(
              controller: haulingController,
              placesApi: placesApi,
            ),
          ),
        );
      default:
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(
            content: Text('Coming soon!'),
            duration: Duration(seconds: 2),
          ),
        );
    }
  }

  @override
  Widget build(BuildContext context) {
    return Stack(
      children: [
        // Full-screen live map background with the green locate-me button
        // floating just above the service panel.
        const Positioned.fill(
          child: CustomerHomeMap(locateButtonBottomInset: 372),
        ),

        // Top-left hamburger button
        Positioned(
          top: 0,
          left: 0,
          right: 0,
          child: SafeArea(
            bottom: false,
            child: Padding(
              padding: const EdgeInsets.fromLTRB(16, 8, 16, 0),
              child: Align(
                alignment: Alignment.centerLeft,
                child: Container(
                  width: 44,
                  height: 44,
                  decoration: const BoxDecoration(
                    color: Colors.white,
                    shape: BoxShape.circle,
                    boxShadow: [
                      BoxShadow(
                        color: Color(0x14000000),
                        blurRadius: 12,
                        offset: Offset(0, 4),
                      ),
                    ],
                  ),
                  child: const Icon(
                    Icons.menu_rounded,
                    color: CustomerFigmaColors.text,
                    size: 22,
                  ),
                ),
              ),
            ),
          ),
        ),

        // Bottom service panel
        Positioned(
          left: 0,
          right: 0,
          bottom: 0,
          child: _ServicePanel(onServiceTap: (i) => _onServiceTap(context, i)),
        ),
      ],
    );
  }
}

// ─── Service panel ────────────────────────────────────────────────────────────

class _ServicePanel extends StatelessWidget {
  const _ServicePanel({required this.onServiceTap});

  final ValueChanged<int> onServiceTap;

  static const _services = [
    _ServiceOption(
      emoji: '🚖',
      icon: Icons.directions_car_filled_rounded,
      title: 'Car Ride',
      subtitle: 'Book a ride.',
    ),
    _ServiceOption(
      emoji: '🛵',
      icon: Icons.electric_bike_rounded,
      title: 'Bike Delivery',
      subtitle: 'Send a package.',
    ),
    _ServiceOption(
      emoji: '🚛',
      icon: Icons.local_shipping_rounded,
      title: 'Truck / Hauling',
      subtitle: 'Book truck or move heavy items.',
    ),
  ];

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: const BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.vertical(top: Radius.circular(28)),
        boxShadow: [
          BoxShadow(
            color: Color(0x1A000000),
            blurRadius: 24,
            offset: Offset(0, -6),
          ),
        ],
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // Drag handle
          Center(
            child: Container(
              margin: const EdgeInsets.only(top: 10),
              width: 36,
              height: 4,
              decoration: BoxDecoration(
                color: CustomerFigmaColors.border,
                borderRadius: BorderRadius.circular(99),
              ),
            ),
          ),
          const SizedBox(height: 12),

          // Safety banner
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 16),
            child: Container(
              padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
              decoration: BoxDecoration(
                color: Colors.white,
                borderRadius: BorderRadius.circular(14),
                border: Border.all(color: CustomerFigmaColors.border),
              ),
              child: Row(
                children: [
                  Container(
                    width: 36,
                    height: 36,
                    decoration: BoxDecoration(
                      color: CustomerFigmaColors.primaryTint,
                      borderRadius: BorderRadius.circular(10),
                    ),
                    child: const Icon(
                      Icons.verified_user_rounded,
                      color: CustomerFigmaColors.primary,
                      size: 20,
                    ),
                  ),
                  const SizedBox(width: 12),
                  const Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          'Your safety comes first',
                          style: TextStyle(
                            color: CustomerFigmaColors.text,
                            fontSize: 13,
                            fontWeight: FontWeight.w700,
                          ),
                        ),
                        SizedBox(height: 2),
                        Text(
                          'verified drivers and secure trips every time you ride or send.',
                          style: TextStyle(
                            color: CustomerFigmaColors.muted,
                            fontSize: 11,
                            height: 1.4,
                          ),
                        ),
                      ],
                    ),
                  ),
                  const SizedBox(width: 8),
                  const Icon(Icons.arrow_forward_ios_rounded,
                      color: CustomerFigmaColors.muted, size: 14),
                ],
              ),
            ),
          ),
          const SizedBox(height: 14),

          // Heading
          const Padding(
            padding: EdgeInsets.symmetric(horizontal: 20),
            child: Text(
              'What do you want to do?',
              style: TextStyle(
                color: CustomerFigmaColors.text,
                fontSize: 17,
                fontWeight: FontWeight.w800,
              ),
            ),
          ),
          const SizedBox(height: 2),
          const Padding(
            padding: EdgeInsets.symmetric(horizontal: 20),
            child: Text(
              'Choose an action to continue.',
              style: TextStyle(
                color: CustomerFigmaColors.muted,
                fontSize: 13,
              ),
            ),
          ),
          const SizedBox(height: 12),

          // Service rows
          ...List.generate(_services.length, (i) => _ServiceCard(
            service: _services[i],
            onTap: () => onServiceTap(i),
          )),

          const SizedBox(height: 16),
        ],
      ),
    );
  }
}

// ─── Service card ─────────────────────────────────────────────────────────────

class _ServiceCard extends StatelessWidget {
  const _ServiceCard({required this.service, required this.onTap});

  final _ServiceOption service;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Container(
        margin: const EdgeInsets.symmetric(horizontal: 16, vertical: 4),
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
        decoration: BoxDecoration(
          color: Colors.white,
          borderRadius: BorderRadius.circular(14),
          border: Border.all(color: CustomerFigmaColors.border),
        ),
        child: Row(
          children: [
            Container(
              width: 48,
              height: 48,
              decoration: BoxDecoration(
                color: CustomerFigmaColors.primaryTint,
                borderRadius: BorderRadius.circular(12),
              ),
              child: Center(
                child: Text(service.emoji, style: const TextStyle(fontSize: 24)),
              ),
            ),
            const SizedBox(width: 14),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    service.title,
                    style: const TextStyle(
                      color: CustomerFigmaColors.text,
                      fontSize: 14,
                      fontWeight: FontWeight.w800,
                    ),
                  ),
                  const SizedBox(height: 2),
                  Text(
                    service.subtitle,
                    style: const TextStyle(
                      color: CustomerFigmaColors.muted,
                      fontSize: 12,
                    ),
                  ),
                ],
              ),
            ),
            const Icon(
              Icons.arrow_forward_ios_rounded,
              color: CustomerFigmaColors.muted,
              size: 16,
            ),
          ],
        ),
      ),
    );
  }
}

// ─── Service option data ──────────────────────────────────────────────────────

class _ServiceOption {
  const _ServiceOption({
    required this.emoji,
    required this.icon,
    required this.title,
    required this.subtitle,
  });

  final String emoji;
  final IconData icon;
  final String title;
  final String subtitle;
}
