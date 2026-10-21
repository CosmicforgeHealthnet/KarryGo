import 'package:flutter/material.dart';

import '../../../core/widgets/karryam_ui.dart';
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
      title: 'What do you move?',
      subtitle: 'Set up your truck partner account.',
      step: 1,
      content: Column(
        children: _services.map((s) => Padding(
          padding: const EdgeInsets.only(bottom: 12),
          child: KChoiceCard(
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

