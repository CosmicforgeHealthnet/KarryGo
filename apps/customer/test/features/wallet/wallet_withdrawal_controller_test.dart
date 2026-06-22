import 'package:customer/features/wallet/state/wallet_withdrawal_controller.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  group('WalletWithdrawalController', () {
    test('builds amount from keypad input and validates against balance', () {
      final c = WalletWithdrawalController(availableKobo: 500000); // ₦5,000

      c.appendAmount('2');
      c.appendAmount('0');
      c.appendAmount('0'); // ₦200
      expect(c.amountKobo, 20000);
      expect(c.amountValid, isTrue);

      c.backspaceAmountKey(); // ₦20
      expect(c.amountKobo, 2000);
      expect(c.amountValid, isFalse); // below ₦100 minimum
    });

    test('rejects amount above available balance', () {
      final c = WalletWithdrawalController(availableKobo: 10000); // ₦100
      c.appendAmount('5');
      c.appendAmount('0');
      c.appendAmount('0'); // ₦500
      c.confirmAmount();
      // Stays on amount step with an error.
      expect(c.status, WalletWithdrawalStatus.enterAmount);
      expect(c.amountError, isNotNull);
    });

    test('advances through account + PIN to mocked success', () async {
      final c = WalletWithdrawalController(availableKobo: 500000);
      c.appendAmount('2');
      c.appendAmount('0');
      c.appendAmount('0');
      c.confirmAmount();
      expect(c.status, WalletWithdrawalStatus.selectAccount);

      c.selectAccount(c.bankAccounts.first);
      c.confirmAccount();
      expect(c.status, WalletWithdrawalStatus.authorize);

      c.appendPin('1');
      c.appendPin('2');
      c.appendPin('3');
      c.appendPin('4'); // 4th digit triggers submit
      expect(c.status, WalletWithdrawalStatus.submitting);

      // Mock submission resolves to success.
      await Future<void>.delayed(const Duration(seconds: 3));
      expect(c.status, WalletWithdrawalStatus.success);
    });
  });
}
