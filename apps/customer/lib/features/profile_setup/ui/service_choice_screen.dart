import 'package:flutter/material.dart';

import '../../../app/app_routes.dart';
import '../../../shared/widgets/figma_customer_widgets.dart';
import '../../auth/state/customer_auth_controller.dart';

class ServiceChoiceScreen extends StatefulWidget {
  const ServiceChoiceScreen({
    super.key,
    required this.controller,
    required this.state,
  });

  final CustomerAuthController controller;
  final CustomerAuthState state;

  @override
  State<ServiceChoiceScreen> createState() => _ServiceChoiceScreenState();
}

class _ServiceChoiceScreenState extends State<ServiceChoiceScreen> {
  String? _selectedService;

  static const _options = [
    _ServiceOption(
      id: 'customer',
      title: 'Send, Ride or Request',
      subtitle: 'Book deliveries, rides or transport services anytime.',
      icon: Icons.person_pin_circle_rounded,
    ),
    _ServiceOption(
      id: 'business',
      title: 'Business Deliveries',
      subtitle: 'Dispatch your customer goods or orders.',
      icon: Icons.local_shipping_rounded,
    ),
    _ServiceOption(
      id: 'provider',
      title: 'Drive, Deliver or Haul',
      subtitle: 'Earn by completing deliveries, trips or transport jobs.',
      icon: Icons.delivery_dining_rounded,
    ),
  ];

  @override
  void initState() {
    super.initState();
    _selectedService = widget.state.selectedService;
  }

  @override
  Widget build(BuildContext context) {
    return FigmaPhoneScaffold(
      key: const ValueKey(CustomerAppRoutes.serviceChoice),
      bottom: FigmaPrimaryButton(
        label: 'Continue',
        onPressed: _selectedService == null
            ? null
            : () => widget.controller.selectServiceChoice(_selectedService!),
      ),
      child: SingleChildScrollView(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            const SizedBox(height: 34),
            const Text(
              'How do you want to use Cosmicforge Logistics?',
              style: TextStyle(
                color: CustomerFigmaColors.text,
                fontSize: 18,
                fontWeight: FontWeight.w900,
              ),
            ),
            const SizedBox(height: 12),
            const Text(
              'By selecting your preferred account type, you have automatically set your role.',
              style: TextStyle(
                color: CustomerFigmaColors.muted,
                fontSize: 12,
                height: 1.45,
              ),
            ),
            const SizedBox(height: 26),
            for (final option in _options) ...[
              _ServiceChoiceCard(
                option: option,
                selected: option.id == _selectedService,
                onTap: () => setState(() => _selectedService = option.id),
              ),
              const SizedBox(height: 16),
            ],
          ],
        ),
      ),
    );
  }
}

class _ServiceChoiceCard extends StatelessWidget {
  const _ServiceChoiceCard({
    required this.option,
    required this.selected,
    required this.onTap,
  });

  final _ServiceOption option;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Material(
      color: selected ? CustomerFigmaColors.primaryPale : Colors.white,
      borderRadius: BorderRadius.circular(14),
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(14),
        child: Container(
          constraints: const BoxConstraints(minHeight: 86),
          padding: const EdgeInsets.all(14),
          decoration: BoxDecoration(
            borderRadius: BorderRadius.circular(14),
            boxShadow: [
              BoxShadow(
                color: Colors.black.withValues(alpha: 0.07),
                blurRadius: 22,
                offset: const Offset(0, 10),
              ),
            ],
          ),
          child: Row(
            children: [
              Container(
                width: 48,
                height: 48,
                decoration: BoxDecoration(
                  color: selected
                      ? Colors.white.withValues(alpha: 0.72)
                      : CustomerFigmaColors.surface,
                  borderRadius: BorderRadius.circular(12),
                ),
                child: Icon(
                  option.icon,
                  color: CustomerFigmaColors.primary,
                  size: 28,
                ),
              ),
              const SizedBox(width: 14),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Text(
                      option.title,
                      style: const TextStyle(
                        color: CustomerFigmaColors.text,
                        fontSize: 15,
                        fontWeight: FontWeight.w900,
                      ),
                    ),
                    const SizedBox(height: 4),
                    Text(
                      option.subtitle,
                      style: const TextStyle(
                        color: CustomerFigmaColors.text,
                        fontSize: 11,
                        height: 1.35,
                      ),
                    ),
                  ],
                ),
              ),
              Icon(
                Icons.arrow_forward_rounded,
                size: 18,
                color: selected
                    ? CustomerFigmaColors.primary
                    : CustomerFigmaColors.border,
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _ServiceOption {
  const _ServiceOption({
    required this.id,
    required this.title,
    required this.subtitle,
    required this.icon,
  });

  final String id;
  final String title;
  final String subtitle;
  final IconData icon;
}
