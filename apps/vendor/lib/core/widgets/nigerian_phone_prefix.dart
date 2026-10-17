import 'package:flutter/material.dart';

import '../config/nigerian_phone.dart';

/// Whether a form field key holds a phone number.
bool isPhoneFieldKey(String key) => key == 'phone' || key.endsWith('_phone');

/// The 🇳🇬 +234 prefix shown inside every phone input in the app.
const nigerianPhonePrefix = Padding(
  padding: EdgeInsets.symmetric(horizontal: 14),
  child: Row(
    mainAxisSize: MainAxisSize.min,
    children: [
      Text('\u{1F1F3}\u{1F1EC}', style: TextStyle(fontSize: 22)),
      SizedBox(width: 8),
      Text('+234', style: TextStyle(fontWeight: FontWeight.w600)),
      SizedBox(width: 8),
    ],
  ),
);

const nigerianPhoneHint = '801 234 5678';
const nigerianPhoneError = 'Enter a valid Nigerian phone number.';

/// Converts a stored `+234...` number into what the user sees after the prefix.
String nigerianPhoneLocal(String stored) {
  final value = stored.trim();
  if (value.startsWith('+234')) return value.substring(4);
  return value;
}

/// Validator for an optional or required phone input typed after the prefix.
String? validateNigerianPhone(String? value, {bool required = false}) {
  final text = value?.trim() ?? '';
  if (text.isEmpty) return required ? 'Enter a phone number.' : null;
  return normalizeNigerianPhone(text) == null ? nigerianPhoneError : null;
}

/// The value to send to the server: `+234...`, or the trimmed text when empty.
String nigerianPhoneForSubmit(String value) =>
    normalizeNigerianPhone(value) ?? value.trim();
