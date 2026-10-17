import 'package:flutter/material.dart';
import '../../../shared/widgets/account_components.dart';
import '../../../shared/widgets/journey_icon.dart';

class SessionGuidanceScreen extends StatelessWidget {
  const SessionGuidanceScreen({super.key});
  @override
  Widget build(BuildContext context) => AccountPage(
    title: 'Keep your account safe',
    symbol: JourneySymbol.shield,
    description: 'A few habits that protect your account.',
    bottom: FilledButton(
      onPressed: () => Navigator.of(context).pop(),
      child: const Text('Got it'),
    ),
    children: const [
      AccountSection(
        title: 'Stay in control',
        children: [
          AccountMenuTile(
            title: 'Keep codes private',
            subtitle: 'Never share a sign-in code or withdrawal PIN.',
            symbol: JourneySymbol.key,
          ),
          AccountMenuTile(
            title: 'Check links before opening',
            subtitle:
                'Avoid unexpected requests for account or payment details.',
            symbol: JourneySymbol.shield,
          ),
          AccountMenuTile(
            title: 'Use your own device',
            subtitle: 'Log out whenever you use a shared phone.',
            symbol: JourneySymbol.profile,
          ),
        ],
      ),
    ],
  );
}
