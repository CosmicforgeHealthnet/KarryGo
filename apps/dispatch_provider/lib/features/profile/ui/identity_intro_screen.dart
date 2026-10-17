import 'package:flutter/material.dart';
import '../../../shared/widgets/account_components.dart';
import '../../../shared/widgets/journey_icon.dart';
import '../../verification/state/verification_controller.dart';
import 'identity_details_screen.dart';

class IdentityIntroScreen extends StatelessWidget {
  const IdentityIntroScreen({super.key, required this.verificationController});
  final VerificationController verificationController;
  @override
  Widget build(BuildContext context) => AccountPage(
    title: 'Verify your identity',
    symbol: JourneySymbol.shield,
    description:
        'Confirm who you are with your government-issued identity document.',
    bottom: FilledButton(
      onPressed: () async {
        final result = await Navigator.of(context).push<bool>(
          MaterialPageRoute(
            builder: (_) => IdentityDetailsScreen(
              verificationController: verificationController,
            ),
          ),
        );
        if (context.mounted && result == true) Navigator.of(context).pop(true);
      },
      child: const Text('Continue'),
    ),
    children: const [
      AccountSection(
        title: 'Before you start',
        children: [
          AccountMenuTile(
            title: 'Have your document ready',
            subtitle: 'Use your NIN or international passport details.',
            symbol: JourneySymbol.contact,
          ),
          AccountMenuTile(
            title: 'Keep every detail visible',
            subtitle: 'Use a clear, readable copy of the document.',
            symbol: JourneySymbol.photo,
          ),
          AccountMenuTile(
            title: 'Match your personal details',
            subtitle:
                'Enter your details exactly as they appear on your document.',
            symbol: JourneySymbol.profile,
          ),
        ],
      ),
    ],
  );
}
