import 'dart:convert';
import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/features/crm/state/crm_export_controller.dart';
import 'finance_test.dart' show financeApp, grant;
import 'crm_workspace_test.dart' show CrmTransport;
import 'order_support_test.dart' show SupportRuntime;

void main() {
  test(
    'Export recovery uses fresh verification without duplicating the job; downloads require their own grant',
    () async {
      final transport = CrmTransport()..suffix = '/exports';
      final app = await financeApp(transport);
      final runtime = SupportRuntime(app);
      final controller = CrmExportController(runtime);
      await controller.load();
      final createGrant = await grant(app, purpose: 'crm.export.create');
      transport.lose = true;
      expect(
        await controller.createExport(
          createGrant,
          settings: {
            'export_type': 'CUSTOMER_SUMMARY',
            'format': 'CSV',
            'scope_type': 'BUSINESS',
            'row_limit': 100,
          },
        ),
        isNull,
      );
      expect(controller.pending, isNotNull, reason: controller.error);
      expect(
        controller.pending!['body'].containsKey('step_up_grant_id'),
        isFalse,
      );
      final key = controller.pending!['key'];
      controller.dispose();
      final restored = CrmExportController(runtime);
      await restored.load();
      expect(restored.pending!['key'], key);
      final row = await restored.createExport(
        await grant(app, purpose: 'crm.export.create'),
      );
      expect(row, isNotNull, reason: restored.error);
      await restored.load();
      await restored.load();
      expect(restored.rows.length, 1);
      expect(restored.rows.single['status'], 'READY');
      final id = row!['export_ref'] as String;
      await expectLater(
        app.client.crmExportFile(
          id,
          await grant(app, purpose: 'crm.export.create'),
        ),
        throwsA(anything),
      );
      final downloadGrant = await grant(app, purpose: 'crm.export.download');
      final bytes = await app.client.crmExportFile(id, downloadGrant);
      final csv = utf8.decode(bytes);
      expect(csv, contains('profile_ref'));
      expect(csv, isNot(contains('email')));
      await expectLater(
        app.client.crmExportFile(id, downloadGrant),
        throwsA(anything),
      );
      restored.dispose();
      app.client.close();
    },
  );
}
