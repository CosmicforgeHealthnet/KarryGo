/// "₦2,714.50"-style fare: thousands separators plus two decimals.
///
/// Local rather than `intl`: this app has no other need for that dependency,
/// and the truck provider app already formats money the same way.
String formatNaira(int kobo) {
  final naira = kobo / 100;
  final whole = naira.truncate();
  final cents = ((naira - whole) * 100).round().toString().padLeft(2, '0');
  final digits = whole.abs().toString();
  final buf = StringBuffer();
  for (var i = 0; i < digits.length; i++) {
    if (i > 0 && (digits.length - i) % 3 == 0) buf.write(',');
    buf.write(digits[i]);
  }
  final sign = kobo < 0 ? '-' : '';
  return '$sign₦$buf.$cents';
}
