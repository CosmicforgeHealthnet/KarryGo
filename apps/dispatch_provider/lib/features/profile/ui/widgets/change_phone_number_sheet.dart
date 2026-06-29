import 'package:flutter/material.dart';
import '../new_phone_number_screen.dart';

/// Confirmation sheet shown over [ProfileInfoScreen] before starting the
/// change-phone-number flow. On confirmation it pushes the full flow
/// (new number → OTP → success) and, if completed, closes itself returning
/// the updated `{flag, code, number}` map to the caller.
class ChangePhoneNumberSheet extends StatelessWidget {
  const ChangePhoneNumberSheet({
    super.key,
    required this.currentCountryFlag,
    required this.currentCountryCode,
    required this.currentPhoneNumber,
  });

  final String currentCountryFlag;
  final String currentCountryCode;
  final String currentPhoneNumber;

  Future<void> _startFlow(BuildContext context) async {
    final result = await Navigator.of(context).push<Map<String, String>>(
      MaterialPageRoute(
        builder: (_) => NewPhoneNumberScreen(
          countryFlag: currentCountryFlag,
          countryCode: currentCountryCode,
        ),
      ),
    );

    if (!context.mounted) return;
    if (result != null) {
      Navigator.of(context).pop(result);
    }
  }

  @override
  Widget build(BuildContext context) {
    final bottomInset = MediaQuery.of(context).padding.bottom;

    return Container(
      padding: EdgeInsets.fromLTRB(24, 32, 24, 24 + bottomInset),
      decoration: const BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.vertical(top: Radius.circular(24)),
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Image.asset(
            'assets/figma/figma/profile_submitted.png',
            height: 140,
            errorBuilder: (_, _, _) => const Icon(
              Icons.phone_iphone,
              size: 96,
              color: Color(0xFF4CAF50),
            ),
          ),
          const SizedBox(height: 24),
          const Text(
            'Change Phone Number?',
            style: TextStyle(
              fontSize: 18,
              fontWeight: FontWeight.w800,
              color: Color(0xFF1A1A1A),
            ),
          ),
          const SizedBox(height: 8),
          const Text(
            'You are about to change your phone number, do you '
            'confirm this decision? If yes proceed confirm to OTP.',
            textAlign: TextAlign.center,
            style: TextStyle(
              fontSize: 13,
              color: Color(0xFF888888),
              height: 1.5,
            ),
          ),
          const SizedBox(height: 24),
          SizedBox(
            width: double.infinity,
            height: 52,
            child: FilledButton(
              onPressed: () => _startFlow(context),
              style: FilledButton.styleFrom(
                backgroundColor: const Color(0xFF4CAF50),
                shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(999),
                ),
              ),
              child: const Text(
                'Change Phone Number',
                style: TextStyle(
                  fontSize: 16,
                  fontWeight: FontWeight.w700,
                  color: Colors.white,
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}
