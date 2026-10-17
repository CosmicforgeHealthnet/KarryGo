import '../../../shared/widgets/account_components.dart';
import '../../../shared/widgets/journey_icon.dart';
import 'package:flutter/material.dart';

import '../../wallet/state/wallet_controller.dart';
import 'new_phone_number_screen.dart';
import 'session_guidance_screen.dart';

class SecurityScreen extends StatelessWidget {
  const SecurityScreen({super.key, this.walletController});

  final WalletController? walletController;

  void _openPhoneNumber(BuildContext context) {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => const NewPhoneNumberScreen(
          countryFlag: '🇳🇬',
          countryCode: '+234',
        ),
      ),
    );
  }

  void _openSecurityTips(BuildContext context) {
    Navigator.of(
      context,
    ).push(MaterialPageRoute(builder: (_) => const SessionGuidanceScreen()));
  }

  @override
  Widget build(BuildContext context) => AccountPage(
    title: 'Security',
    description: 'Keep your sign-in details safe.',
    symbol: JourneySymbol.key,
    children: [
      AccountSection(
        title: 'Sign-in and safety',
        children: [
          AccountMenuTile(
            title: 'Phone number',
            subtitle: 'Verify and update your sign-in number',
            symbol: JourneySymbol.contact,
            onTap: () => _openPhoneNumber(context),
          ),
          AccountMenuTile(
            title: 'Security tips',
            subtitle: 'Protect your account and verification codes',
            symbol: JourneySymbol.shield,
            onTap: () => _openSecurityTips(context),
          ),
        ],
      ),
    ],
  );
}
