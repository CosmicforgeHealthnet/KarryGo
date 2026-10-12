import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'package:vendor/features/scheduled_commerce/state/rfq_workspace_controller.dart';
import 'finance_test.dart' show financeApp;
import 'order_support_test.dart' show SupportRuntime;
import 'crm_workspace_test.dart' show CrmTransport;

void main() {
  test(
    'RFQ inbox, frozen quote recovery and immutable revision history',
    () async {
      final transport = CrmTransport(),
          app = await financeApp(transport),
          runtime = SupportRuntime(app);
      final inbox = RfqWorkspaceController(runtime, rfqBase);
      await inbox.load();
      expect(inbox.error, isNull);
      expect(inbox.rows.length, 2);
      final rfq = inbox.rows.first,
          params = {'rfqId': '${inbox.rows.first['ID']}'};
      final c = RfqWorkspaceController(
        runtime,
        '$rfqBase/{rfqId}',
        parameters: params,
      );
      await c.load();
      final body = <String, dynamic>{
        'branch_id':
            app.scope.branchId ??
            (await app.businessApi.request(
              'GET',
              '/api/v1/vendor/businesses/{businessId}/branches',
              pathParameters: {'businessId': app.scope.vendorId!},
            )).object['items'][0]['id'],
        'price_minor': 12000000,
        'currency': rfq['Currency'],
        'snapshot': {},
        'capacity': {'required': false},
        'expires_at': DateTime.now()
            .toUtc()
            .add(const Duration(days: 1))
            .toIso8601String(),
      };
      transport.lose = true;
      transport.suffix = '/quotes';
      expect(
        await c.save(
          method: 'POST',
          route: '$rfqBase/{rfqId}/quotes',
          params: params,
          body: body,
        ),
        isNull,
      );
      expect(c.pending, isNotNull);
      final saved = jsonObject(c.pending!['body']);
      body['price_minor'] = 999;
      c.dispose();
      final restored = RfqWorkspaceController(
        runtime,
        '$rfqBase/{rfqId}',
        parameters: params,
      );
      await restored.load();
      expect(
        restored.pending!['body']['idempotency_key'],
        saved['idempotency_key'],
      );
      expect(restored.pending!['body']['price_minor'], 12000000);
      final first = await restored.save();
      expect(first, isNotNull, reason: restored.error);
      expect(first!['Revision'], 1);
      await restored.load();
      expect(restored.record!['Status'], 'QUOTED');
      final second = await restored.save(
        method: 'POST',
        route: '$rfqBase/{rfqId}/quotes',
        params: params,
        body: {...saved, 'price_minor': 14000000},
      );
      expect(second, isNotNull, reason: restored.error);
      expect(second!['QuoteID'], first['QuoteID']);
      expect(second['Revision'], 2);
      final history = restored.collection(
        await restored.fetch('$rfqBase/{rfqId}/quotes', params: params),
      );
      expect(history.length, 2);
      expect(
        history.singleWhere((q) => q['Revision'] == 1)['PriceMinor'],
        12000000,
      );
      expect(history.every((q) => q['VendorID'] == app.scope.vendorId), isTrue);
      await restored.load();
      final denied = await restored.save(
        method: 'POST',
        route: '$rfqBase/{rfqId}/quotes',
        params: {'rfqId': 'demo-rfq-1'},
        body: saved,
      );
      expect(denied, isNull);
      expect(restored.pending, isNull);
      restored.dispose();
      inbox.dispose();
      app.client.close();
    },
  );
}
