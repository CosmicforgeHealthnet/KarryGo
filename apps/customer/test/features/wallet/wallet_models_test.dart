import 'package:customer/features/wallet/models/wallet_models.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  group('WalletSummary.fromJson', () {
    test('parses the payment-wallet-service summary contract', () {
      final summary = WalletSummary.fromJson(const {
        'owner_type': 'customer',
        'owner_id': 'cust-1',
        'currency': 'NGN',
        'available_kobo': 250000,
        'escrow_kobo': 50000,
        'pending_kobo': 10000,
      });

      expect(summary.availableKobo, 250000);
      expect(summary.escrowKobo, 50000);
      expect(summary.pendingKobo, 10000);
      expect(summary.balanceNaira, 2500.0);
      expect(summary.formattedBalance, '₦2,500.00');
    });

    test('defaults missing fields to zero rather than throwing', () {
      final summary = WalletSummary.fromJson(const {});
      expect(summary.availableKobo, 0);
      expect(summary.formattedBalance, '₦0.00');
      expect(summary.currency, 'NGN');
    });
  });

  group('WalletTransaction.fromJson', () {
    test('parses the ledger entry contract and derives credit from side', () {
      final txn = WalletTransaction.fromJson(const {
        'reference': 'pi_123',
        'transaction_type': 'paystack_charge_success',
        'side': 'credit',
        'amount_kobo': 100000,
        'currency': 'NGN',
        'memo': 'Wallet top-up',
        'created_at': '2026-06-21T10:00:00Z',
      });

      expect(txn.reference, 'pi_123');
      expect(txn.type, 'paystack_charge_success');
      expect(txn.isCredit, isTrue);
      expect(txn.description, 'Wallet top-up');
      expect(txn.formattedAmount, '+₦1,000.00');
      expect(txn.title, 'Wallet Top-up');
      expect(txn.status, WalletTxnStatus.success);
    });

    test('derives pending/failed status from transaction type', () {
      WalletTransaction txn(String type) => WalletTransaction.fromJson({
            'transaction_type': type,
            'side': 'debit',
            'amount_kobo': 1000,
            'created_at': '2026-06-21T10:00:00Z',
          });

      expect(txn('withdrawal_reserved').status, WalletTxnStatus.pending);
      expect(txn('refund_failed').status, WalletTxnStatus.failed);
      expect(txn('withdrawal_paid').status, WalletTxnStatus.success);
    });

    test('debit side renders as outgoing', () {
      final txn = WalletTransaction.fromJson(const {
        'transaction_type': 'wallet_payment_hold',
        'side': 'debit',
        'amount_kobo': 75000,
        'memo': 'Truck booking',
        'created_at': '2026-06-21T10:00:00Z',
      });

      expect(txn.isCredit, isFalse);
      expect(txn.formattedAmount, '-₦750.00');
    });
  });
}
