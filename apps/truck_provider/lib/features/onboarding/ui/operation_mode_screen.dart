import 'package:flutter/material.dart';

import '../../../core/widgets/karryam_ui.dart';
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
      title: 'Work your way.',
      subtitle: 'Choose how you operate, so we can tailor jobs and tools for you.',
      step: 2,
      content: Column(
        children: _modes.map((m) => Padding(
          padding: const EdgeInsets.only(bottom: 12),
          child: KChoiceCard(
            icon: m.icon,
            title: m.title,
            subtitle: m.subtitle,
            selected: _selected == m.value,
            onTap: () => setState(() {
              _selected = m.value;
              widget.controller.selectOperationMode(m.value);
            }),
          ),
        )).toList(),
      ),
      onContinue: _selected.isNotEmpty ? widget.controller.proceedToPersonalInfo : null,
    );
  }
}
