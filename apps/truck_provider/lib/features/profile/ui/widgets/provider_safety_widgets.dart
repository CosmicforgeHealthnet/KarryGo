import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../../home/ui/widgets/provider_app_colors.dart';

// Shared building blocks for the Safety & Emergency section (emergency contact
// + guarantor screens).

/// Light field label used on the safety forms (the Figma labels here are
/// regular weight, unlike the bold ProviderFieldLabel used elsewhere).
class SafetyFieldLabel extends StatelessWidget {
  const SafetyFieldLabel(this.text, {super.key});
  final String text;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 8),
      child: Text(
        text,
        style: TextStyle(color: kProviderText, fontSize: 14, fontWeight: FontWeight.w500),
      ),
    );
  }
}

/// Gray phone input with the Nigeria flag + (+234) prefix. Digits only; the
/// caller assembles the full `+234...` value on save.
class SafetyPhoneField extends StatelessWidget {
  const SafetyPhoneField({
    super.key,
    required this.controller,
    required this.onChanged,
  });

  final TextEditingController controller;
  final ValueChanged<String> onChanged;

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: BoxDecoration(
        color: kProviderSurface,
        borderRadius: BorderRadius.circular(12),
      ),
      child: Row(
        children: [
          Padding(
            padding: EdgeInsets.only(left: 16),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Text('🇳🇬', style: TextStyle(fontSize: 18)),
                SizedBox(width: 4),
                Icon(Icons.keyboard_arrow_down_rounded, size: 18, color: kProviderMuted),
                SizedBox(width: 8),
                Text(
                  '(+234)',
                  style: TextStyle(color: kProviderMuted, fontSize: 14, fontWeight: FontWeight.w500),
                ),
              ],
            ),
          ),
          Expanded(
            child: TextField(
              controller: controller,
              onChanged: onChanged,
              keyboardType: TextInputType.phone,
              inputFormatters: [FilteringTextInputFormatter.digitsOnly],
              style: TextStyle(color: kProviderText, fontSize: 14, fontWeight: FontWeight.w500),
              decoration: const InputDecoration(
                border: InputBorder.none,
                contentPadding: EdgeInsets.symmetric(horizontal: 10, vertical: 16),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

/// Green "+ Add New" pill shown above the saved-contact card.
class SafetyAddNewPill extends StatelessWidget {
  const SafetyAddNewPill({super.key, required this.onTap});
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 8),
        decoration: BoxDecoration(
          color: kProviderGreen,
          borderRadius: BorderRadius.circular(999),
        ),
        child: Text(
          '+ Add New',
          style: TextStyle(color: kOnPrimary, fontSize: 13, fontWeight: FontWeight.w700),
        ),
      ),
    );
  }
}

/// Saved contact card: bold name, muted phone, optional muted relationship,
/// trailing chevron. Tapping opens the pre-filled form.
class SafetyContactCard extends StatelessWidget {
  const SafetyContactCard({
    super.key,
    required this.name,
    required this.phone,
    this.relationship,
    required this.onTap,
  });

  final String name;
  final String phone;
  final String? relationship;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Material(
      color: kCard,
      borderRadius: BorderRadius.circular(12),
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(12),
        child: Ink(
          decoration: BoxDecoration(
            color: kCard,
            borderRadius: BorderRadius.circular(12),
            boxShadow: const [BoxShadow(color: Color(0x0F000000), blurRadius: 16, offset: Offset(0, 4))],
          ),
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
          child: Row(
            children: [
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      name,
                      style: TextStyle(color: kProviderText, fontSize: 14, fontWeight: FontWeight.w700),
                    ),
                    const SizedBox(height: 2),
                    Text(phone, style: TextStyle(color: kProviderMuted, fontSize: 13)),
                    if (relationship != null && relationship!.isNotEmpty) ...[
                      const SizedBox(height: 2),
                      Text(relationship!, style: TextStyle(color: kProviderMuted, fontSize: 13)),
                    ],
                  ],
                ),
              ),
              Icon(Icons.chevron_right_rounded, color: kProviderMuted, size: 22),
            ],
          ),
        ),
      ),
    );
  }
}

/// Green helper paragraph shown at the top of the safety forms.
class SafetyHelperText extends StatelessWidget {
  const SafetyHelperText(this.text, {super.key});
  final String text;

  @override
  Widget build(BuildContext context) {
    return Text(
      text,
      style: TextStyle(color: kTealText, fontSize: 13, height: 1.5),
    );
  }
}

/// Strips a leading 0 from the typed local digits and prefixes +234. The
/// backend stores guarantor/emergency phones as sent (trim only), so this is
/// where the canonical format is produced.
String safetyFullPhone(String digits) {
  var d = digits.trim();
  if (d.startsWith('0')) d = d.substring(1);
  return '+234$d';
}

/// Local digits for pre-filling the form from a stored `+234...` value.
String safetyLocalDigits(String storedPhone) =>
    storedPhone.replaceFirst(RegExp(r'^\+234'), '').replaceAll(RegExp(r'\D'), '');
