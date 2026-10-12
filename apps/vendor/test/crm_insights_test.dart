import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/features/crm/state/crm_insights_controller.dart';
import 'package:vendor/features/crm/state/crm_workspace_controller.dart';
import 'package:vendor/features/crm/ui/crm_insights_screen.dart';
import 'package:vendor/core/theme/vendor_theme.dart';
import 'finance_test.dart' show financeApp;
import 'crm_workspace_test.dart' show CrmTransport;
import 'order_support_test.dart' show SupportRuntime;
import 'dashboard_test.dart' show flush;

void main() {
  test(
    'Lost rebuild response retries the same job and preserves unavailable scores',
    () async {
      final transport = CrmTransport()..suffix = '/rebuild',
          app = await financeApp(transport),
          runtime = SupportRuntime(app);
      final c = CrmInsightsController(runtime);
      await c.load();
      expect(c.error, isNull);
      expect(c.record!['models'].first['status'], 'NOT_AVAILABLE');
      transport.lose = true;
      expect(
        await c.save(
          method: 'POST',
          route: '$crmBase/intelligence/rebuild',
          body: {'model_key': 'RFM_V1', 'scope_type': 'BUSINESS'},
        ),
        isNull,
      );
      final key = c.pending!['key'];
      c.dispose();
      final restored = CrmInsightsController(runtime);
      await restored.load();
      expect(restored.pending!['key'], key);
      final result = await restored.save();
      expect(result!['generation'], 1);
      await restored.load();
      expect(restored.record!['models'].first['status'], 'CURRENT');
      expect(restored.record!['models'].first['available_count'], 0);
      final detail = CrmInsightsController(runtime, model: 'RFM_V1');
      await detail.load();
      expect(detail.error, isNull);
      expect(detail.record!['composite_distribution'], isEmpty);
      expect(
        detail.record!['unavailable_reasons'].first['key'],
        'SOURCE_NOT_AVAILABLE',
      );
      detail.readQuery = {'branch_ref': 'outside-business'};
      await detail.load();
      expect(detail.stale, isTrue);
      expect(detail.error, isNotNull);
      restored.dispose();
      detail.dispose();
      app.client.close();
    },
  );
  testWidgets(
    'Insights overview and distributions fit a narrow phone in both themes',
    (tester) async {
      await tester.binding.setSurfaceSize(const Size(360, 800));
      addTearDown(() => tester.binding.setSurfaceSize(null));
      final app = (await tester.runAsync(() => financeApp(CrmTransport())))!,
          runtime = SupportRuntime(app);
      for (final brightness in [Brightness.light, Brightness.dark]) {
        for (final model in [null, 'RFM_V1', 'CHURN_V1']) {
          await tester.pumpWidget(
            MaterialApp(
              theme: vendorTheme(brightness: brightness),
              home: CrmInsightsScreen(
                key: ValueKey('$brightness$model'),
                runtime: runtime,
                model: model,
              ),
            ),
          );
          await flush(tester);
          expect(tester.takeException(), isNull);
          expect(find.text('Refresh insights'), findsWidgets);
        }
      }
      await tester.pumpWidget(const SizedBox());
      app.client.close();
    },
  );
}
