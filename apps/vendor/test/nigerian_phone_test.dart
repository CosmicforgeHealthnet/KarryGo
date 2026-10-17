import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/core/config/nigerian_phone.dart';

void main() {
  test('normalizes local numbers and pasted country prefixes', () {
    for (final value in [
      '08012345678',
      '8012345678',
      '+234 801 234 5678',
      '2348012345678',
      '+234 (0)801-234-5678',
    ]) {
      expect(normalizeNigerianPhone(value), '+2348012345678');
    }
  });
  test(
    'rejects incomplete and foreign numbers without changing the country',
    () {
      for (final value in [
        '',
        '0801234',
        '+348012345678',
        '6012345678',
        '080123456789',
        'phone',
      ]) {
        expect(normalizeNigerianPhone(value), isNull);
      }
    },
  );
}
