import 'package:flutter/material.dart';

import '../../home/ui/widgets/provider_app_colors.dart';
import '../../auth/state/provider_auth_controller.dart';
import 'onboarding_shared_widgets.dart';

class OperationModeScreen extends StatefulWidget {
  const OperationModeScreen({super.key, required this.controller});
  final ProviderAuthController controller;

  @override
  State<OperationModeScreen> createState() => _OperationModeScreenState();
}

class _OperationModeScreenState extends State<OperationModeScreen> {
  String _selected = '';

  static const _modes = [
    (
      value: 'individual',
      icon: Icons.person_rounded,
      title: 'Individual',
      subtitle: 'I work alone and handle my own jobs.',
    ),
    (
      value: 'fleet',
      icon: Icons.groups_rounded,
      title: 'Fleet',
      subtitle: 'I manage multiple drivers or vehicles.',
    ),
  ];

  @override
  Widget build(BuildContext context) {
    return OnboardingScaffold(
      title: 'How do you operate?',
      subtitle: 'This helps us tailor jobs and tools for you.',
      step: 2,
      content: Column(
        children: _modes.map((m) => Padding(
          padding: const EdgeInsets.only(bottom: 12),
          child: GestureDetector(
            onTap: () => setState(() {
              _selected = m.value;
              widget.controller.selectOperationMode(m.value);
            }),
            child: AnimatedContainer(
              duration: const Duration(milliseconds: 150),
              padding: const EdgeInsets.all(18),
              decoration: BoxDecoration(
                color: _selected == m.value ? kProviderGreenTint : Colors.white,
                borderRadius: BorderRadius.circular(14),
                border: Border.all(
                  color: _selected == m.value ? kProviderGreen : kProviderBorder,
                  width: _selected == m.value ? 2 : 1,
                ),
              ),
              child: Row(
                children: [
                  Container(
                    width: 48,
                    height: 48,
                    decoration: BoxDecoration(
                      color: _selected == m.value ? kProviderGreen : kProviderSurface,
                      shape: BoxShape.circle,
                    ),
                    child: Icon(m.icon, color: _selected == m.value ? Colors.white : kProviderMuted, size: 24),
                  ),
                  const SizedBox(width: 14),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          m.title,
                          style: const TextStyle(color: kProviderText, fontWeight: FontWeight.w700, fontSize: 16),
                        ),
                        const SizedBox(height: 4),
                        Text(
                          m.subtitle,
                          style: const TextStyle(color: kProviderMuted, fontSize: 13, height: 1.4),
                        ),
                      ],
                    ),
                  ),
                  AnimatedContainer(
                    duration: const Duration(milliseconds: 150),
                    width: 22,
                    height: 22,
                    decoration: BoxDecoration(
                      shape: BoxShape.circle,
                      color: _selected == m.value ? kProviderGreen : Colors.transparent,
                      border: Border.all(
                        color: _selected == m.value ? kProviderGreen : kProviderBorder,
                        width: 2,
                      ),
                    ),
                    child: _selected == m.value
                        ? const Icon(Icons.check, color: Colors.white, size: 14)
                        : null,
                  ),
                ],
              ),
            ),
          ),
        )).toList(),
      ),
      onContinue: _selected.isNotEmpty ? widget.controller.proceedToPersonalInfo : null,
    );
  }
}
