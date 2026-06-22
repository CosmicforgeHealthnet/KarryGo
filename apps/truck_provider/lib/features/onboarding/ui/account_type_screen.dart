import 'package:flutter/material.dart';

import '../../home/ui/widgets/provider_app_colors.dart';
import '../../auth/state/provider_auth_controller.dart';
import 'onboarding_shared_widgets.dart';

class AccountTypeScreen extends StatefulWidget {
  const AccountTypeScreen({super.key, required this.controller});
  final ProviderAuthController controller;

  @override
  State<AccountTypeScreen> createState() => _AccountTypeScreenState();
}

class _AccountTypeScreenState extends State<AccountTypeScreen> {
  String _selected = '';

  static const _types = [
    (
      value: 'customer',
      icon: Icons.directions_car_rounded,
      title: 'Send, Ride or Request',
      subtitle: 'Book deliveries, rides or transport services anytime.',
    ),
    (
      value: 'business',
      icon: Icons.business_center_rounded,
      title: 'Business Deliveries',
      subtitle: 'Dispatch your customer goods or orders.',
    ),
    (
      value: 'driver',
      icon: Icons.local_shipping_rounded,
      title: 'Drive, Deliver or Haul',
      subtitle: 'Earn by completing deliveries, trips or transport jobs.',
    ),
  ];

  @override
  Widget build(BuildContext context) {
    return OnboardingScaffold(
      title: 'How do you want to use KarryGo?',
      subtitle: 'By selecting your preferred account type, you have automatically set your user role.',
      showBack: false,
      content: Column(
        children: _types.map((t) => Padding(
          padding: const EdgeInsets.only(bottom: 12),
          child: _TypeCard(
            icon: t.icon,
            title: t.title,
            subtitle: t.subtitle,
            selected: _selected == t.value,
            onTap: () => setState(() {
              _selected = t.value;
              widget.controller.selectAccountType(t.value);
            }),
          ),
        )).toList(),
      ),
      onContinue: _selected.isNotEmpty ? _proceed : null,
      continueLabel: 'Continue',
    );
  }

  void _proceed() {
    if (_selected != 'driver') {
      _showDriverOnlyDialog(context);
      return;
    }
    widget.controller.proceedToServiceType();
  }

  void _showDriverOnlyDialog(BuildContext context) {
    showDialog(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('Provider App', style: TextStyle(fontWeight: FontWeight.w800)),
        content: const Text(
          'This app is for service providers. Please select "Drive, Deliver or Haul" to continue, or download the KarryGo customer app for other account types.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('OK', style: TextStyle(color: kProviderGreen)),
          ),
        ],
      ),
    );
  }
}

class _TypeCard extends StatelessWidget {
  const _TypeCard({
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
                  Text(
                    title,
                    style: TextStyle(
                      color: kProviderText,
                      fontWeight: FontWeight.w700,
                      fontSize: 14,
                    ),
                  ),
                  const SizedBox(height: 3),
                  Text(
                    subtitle,
                    style: const TextStyle(color: kProviderMuted, fontSize: 12, height: 1.4),
                  ),
                ],
              ),
            ),
            const SizedBox(width: 8),
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
