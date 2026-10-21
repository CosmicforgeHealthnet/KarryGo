/// Formats a naira amount with thousands separators and 2 decimals, e.g.
/// `204000.0` → `204,000.00`.
String formatNaira(double naira) {
  final negative = naira < 0;
  final fixed = naira.abs().toStringAsFixed(2);
  final parts = fixed.split('.');
  final intPart = parts[0];
  final decPart = parts[1];

  final buf = StringBuffer();
  for (var i = 0; i < intPart.length; i++) {
    if (i > 0 && (intPart.length - i) % 3 == 0) buf.write(',');
    buf.write(intPart[i]);
  }
  return '${negative ? '-' : ''}$buf.$decPart';
}

/// Groups a whole-number digit string with thousands separators, e.g.
/// `204000` → `204,000`. Used by the withdrawal amount keypad.
String groupThousands(String digits) {
  if (digits.isEmpty) return '';
  final buf = StringBuffer();
  for (var i = 0; i < digits.length; i++) {
    if (i > 0 && (digits.length - i) % 3 == 0) buf.write(',');
    buf.write(digits[i]);
  }
  return buf.toString();
}

/// "₦32,500" for whole amounts and "₦32,500.50" otherwise — the headline
/// format for fares and balances.
String formatNairaShort(double naira) {
  final full = formatNaira(naira);
  return '₦${full.endsWith('.00') ? full.substring(0, full.length - 3) : full}';
}
