import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/app/data/vendor_preferences.dart';
import 'package:vendor/features/inventory/state/stock_controller.dart';
import 'crm_workspace_test.dart' show CrmTransport;
import 'finance_test.dart' show financeApp;

void main() {
  test(
    'accepted stock update survives reopening without adding stock twice',
    () async {
      final transport = CrmTransport()..suffix = '/adjustments';
      final app = await financeApp(transport);
      addTearDown(app.dispose);
      addTearDown(transport.close);
      final preferences = VendorPreferences(inMemory: true);
      final first = StockController(
        app,
        'mock-branch',
        preferences: preferences,
      );
      final before = await first.balance('stock-demo-listing');
      final body = {
        'stock_basis_id': before['stock_basis_ref'],
        'expected_revision': before['revision'],
        'quantity_delta': 2,
        'reason_code': 'FOUND_STOCK',
      };
      transport.lose = true;
      expect(
        await first.mutate(
          'POST',
          '${StockController.root}/adjustments',
          body: body,
        ),
        isNull,
      );
      expect(first.uncertainOutcome, isTrue);
      final originalKey = first.pending!['key'];
      first.dispose();
      final reopened = StockController(
        app,
        'mock-branch',
        preferences: preferences,
      );
      addTearDown(reopened.dispose);
      await reopened.restorePending();
      expect(reopened.pending!['key'], originalKey);
      expect(
        await reopened.mutate(
          'POST',
          '${StockController.root}/adjustments',
          body: {...body, 'quantity_delta': 10},
        ),
        isNull,
      );
      expect(reopened.pending!['key'], originalKey);
      expect(await reopened.retryPending(), isNotNull, reason: reopened.error);
      final after = await reopened.balance('stock-demo-listing');
      expect(after['on_hand_quantity'], before['on_hand_quantity'] + 2);
      expect(reopened.pending, isNull);
      expect(reopened.commands, isEmpty);
      expect(await preferences.read(reopened.recoveryKey), '{}');
    },
  );
}
