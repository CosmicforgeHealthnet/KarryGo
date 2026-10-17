String? normalizeNigerianPhone(String input) {
  var digits = input.trim().replaceAll(RegExp(r'[\s()\-]'), '');
  if (digits.startsWith('+234')) {
    digits = digits.substring(4);
  } else if (digits.startsWith('234') && digits.length == 13) {
    digits = digits.substring(3);
  }
  if (digits.startsWith('0')) digits = digits.substring(1);
  return RegExp(r'^[789]\d{9}$').hasMatch(digits) ? '+234$digits' : null;
}
