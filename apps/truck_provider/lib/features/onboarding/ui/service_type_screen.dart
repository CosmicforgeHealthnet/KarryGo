import 'package:flutter/material.dart';

import '../../home/ui/widgets/provider_app_colors.dart';
import '../../auth/state/provider_auth_controller.dart';
import 'onboarding_shared_widgets.dart';

class ServiceTypeScreen extends StatefulWidget {
  const ServiceTypeScreen({super.key, required this.controller});
  final ProviderAuthController controller;

  @override
  State<ServiceTypeScreen> createState() => _ServiceTypeScreenState();
}

class _ServiceTypeScreenState extends State<ServiceTypeScreen> {
  String _selected = '';

  static const _services = [
    (
      value: 'delivery',
      icon: Icons.pedal_bike_rounded,
      title: 'Package Delivery',
      subtitle: 'Deliver parcels and small items to customers.',
    ),
    (
      value: 'taxi',
      icon: Icons.directions_car_rounded,
      title: 'Ride or Taxi Services',
      subtitle: 'Drive passengers to their destinations.',
    ),
    (
      value: 'hauling',
      icon: Icons.local_shipping_rounded,
      title: 'Heavy Hauling',
      subtitle: 'Transport goods, equipment or bulk loads.',
    ),
  ];

  @override
  Widget build(BuildContext context) {
    return OnboardingScaffold(
      title: 'What service do you provide?',
      subtitle: 'Select how you want to use this platform.',
      step: 1,
      content: Column(
        children: _services.map((s) => Padding(
          padding: const EdgeInsets.only(bottom: 12),
          child: _ServiceCard(
            icon: s.icon,
            title: s.title,
            subtitle: s.subtitle,
            selected: _selected == s.value,
            onTap: () => setState(() {
              _selected = s.value;
              widget.controller.selectServiceType(s.value);
            }),
          ),
        )).toList(),
      ),
      onContinue: _selected.isNotEmpty ? widget.controller.proceedToOperationMode : null,
    );
  }
}

class _ServiceCard extends StatelessWidget {
  const _ServiceCard({
    required this.icon,
    required this.title,
    required this.subtitle,
    required this.selected,
    required this.onTap,
  });

  final IconData icon;
  final String title;
  final String subtitle;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 150),
        padding: const EdgeInsets.all(16),
        decoration: BoxDecoration(
          color: selected ? kProviderGreenTint : Colors.white,
          borderRadius: BorderRadius.circular(14),
          border: Border.all(
            color: selected ? kProviderGreen : kProviderBorder,
            width: selected ? 2 : 1,
          ),
        ),
        child: Row(
          children: [
            Container(
              width: 44,
              height: 44,
              decoration: BoxDecoration(
                color: selected ? kProviderGreen : kProviderSurface,
                borderRadius: BorderRadius.circular(10),
              ),
              child: Icon(icon, color: selected ? Colors.white : kProviderMuted, size: 22),
            ),
            const SizedBox(width: 14),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(title, style: const TextStyle(color: kProviderText, fontWeight: FontWeight.w700, fontSize: 14)),
                  const SizedBox(height: 3),
                  Text(subtitle, style: const TextStyle(color: kProviderMuted, fontSize: 12, height: 1.4)),
                ],
              ),
            ),
            Icon(
              selected ? Icons.check_circle_rounded : Icons.chevron_right_rounded,
              color: selected ? kProviderGreen : kProviderMuted,
              size: 22,
            ),
          ],
        ),
      ),
    );
  }
}
