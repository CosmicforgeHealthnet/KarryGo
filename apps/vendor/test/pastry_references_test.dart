import 'package:flutter/material.dart';
import 'package:vendor/features/pastry/ui/pastry_configuration_screen.dart';
import 'package:vendor/core/theme/vendor_theme.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/features/pastry/state/pastry_workspace_controller.dart';
import 'finance_test.dart' show FinanceTransport, financeApp;
import 'order_support_test.dart' show SupportRuntime;
import 'package:vendor/core/contracts/vendor_contract.dart';

void main() {
  testWidgets(
    'Saved requirement edits preserve unknown fields and existing value types',
    (tester) async {
      JsonObject? saved;
      const original = {
        'servings': 24,
        'fragile': true,
        'legacy_entries': ['keep-me'],
        'unknown': null,
      };
      await tester.pumpWidget(
        MaterialApp(
          theme: vendorTheme(brightness: Brightness.dark),
          home: Builder(
            builder: (context) => Scaffold(
              body: TextButton(
                onPressed: () async {
                  saved = await Navigator.push<JsonObject>(
                    context,
                    MaterialPageRoute(
                      builder: (_) => const PastrySavedRequirementsScreen(
                        title: 'Saved requirements',
                        initial: original,
                      ),
                    ),
                  );
                },
                child: const Text('Open'),
              ),
            ),
          ),
        ),
      );
      await tester.tap(find.text('Open'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Servings'));
      await tester.pumpAndSettle();
      await tester.enterText(find.byType(TextFormField), '30');
      await tester.tap(find.text('Save details'));
      await tester.pumpAndSettle();
      await tester.scrollUntilVisible(find.text('Save requirements'), 200);
      await tester.tap(find.text('Save requirements'));
      await tester.pumpAndSettle();
      expect(saved, {
        'servings': 30,
        'fragile': true,
        'legacy_entries': ['keep-me'],
        'unknown': null,
      });
      expect(original['servings'], 24);
      expect(tester.takeException(), isNull);
    },
  );
  test(
    'Pastry references use canonical customers and reject unrelated or archived selections',
    () async {
      final app = await financeApp(FinanceTransport()),
          runtime = SupportRuntime(app);
      final c = PastryWorkspaceController(
        runtime,
        '$pastryBase/custom-requests',
      );
      await c.load();
      final branch = c.rows.first['branch_id'] as String;
      final choices = jsonObject(
        await c.fetch(
          '$pastryBase/request-references',
          query: {
            'branch_id': branch,
            'kind': 'CUSTOMER',
            'limit': '50',
            'offset': '0',
          },
        ),
      );
      expect(choices['items'].length, 1);
      final ref = choices['items'].first['reference'];
      expect(ref, 'mock-customer-celebration');
      expect(ref, isNot('mock-crm-customer-1'));
      final request = await c.save(
        method: 'POST',
        route: '$pastryBase/custom-requests',
        body: {
          'branch_id': branch,
          'title': 'Birthday cake',
          'customer_ref': ref,
        },
      );
      expect(request, isNotNull, reason: c.error);
      await c.load();
      expect(
        await c.save(
          method: 'POST',
          route: '$pastryBase/custom-requests',
          body: {
            'branch_id': branch,
            'title': 'Wrong reference',
            'customer_ref': 'mock-crm-customer-1',
          },
        ),
        isNull,
      );
      expect(c.pending, isNull);
      final customer = await app.crmApi.request(
        'GET',
        '/api/v1/vendor/crm/customers',
      );
      final profiles = customer.object['items'] as List;
      final profile = jsonObject(profiles.first['profile']);
      await app.crmApi.request(
        'POST',
        '/api/v1/vendor/crm/customers/{profileId}/archive',
        pathParameters: {'profileId': profile['profile_ref']},
        contractHeaders: {'If-Match': '${profile['version']}'},
      );
      final updated = jsonObject(
        await c.fetch(
          '$pastryBase/request-references',
          query: {'branch_id': branch, 'kind': 'CUSTOMER'},
        ),
      );
      expect(updated['items'], isEmpty);
      await c.load();
      expect(
        await c.save(
          method: 'POST',
          route: '$pastryBase/custom-requests',
          body: {
            'branch_id': branch,
            'title': 'Stale selection',
            'customer_ref': ref,
          },
        ),
        isNull,
      );
      c.dispose();
      app.client.close();
    },
  );
}
