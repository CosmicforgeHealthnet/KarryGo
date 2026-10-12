import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/features/orders/state/order_detail_controller.dart';
import 'dashboard_test.dart' show dashboardRuntime;

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  test(
    'Measured quantities use their own version and preserve payment state',
    () async {
      final runtime = await dashboardRuntime();
      addTearDown(runtime.dispose);
      final controller = OrderDetailController(
        runtime.backend,
        'demo-order-1047',
      );
      addTearDown(controller.dispose);
      await controller.load();
      final row = controller.picking.single;
      final line = '${row['OrderLineID']}';
      final before = row['Measurement'] as Map;
      expect(before['version'], isNot(row['Version']));
      expect(
        await controller.line(line, 'actual-quantity', {
          'actual_quantity': 24,
        }, row['Version'] as int),
        isFalse,
      );
      await controller.load();
      expect(
        await controller.line(line, 'actual-quantity', {
          'actual_quantity': 24,
        }, before['version'] as int),
        isTrue,
      );
      final measured = controller.picking.single['Measurement'] as Map;
      expect(measured['actual_quantity'], 24);
      expect(measured['version'], 8);
      expect(measured['state'], 'ACTUAL_RECORDED');
      expect(
        await controller.line(line, 'actual-quantity', {
          'actual_quantity': 26,
        }, 8),
        isTrue,
      );
      expect(
        (controller.picking.single['Measurement'] as Map)['state'],
        'REAPPROVAL_REQUIRED',
      );
      expect(
        await controller.line(line, 'actual-quantity', {
          'actual_quantity': 27,
        }, 9),
        isFalse,
      );
    },
  );
  test(
    'Saved quality results retain observations, reasons and revision',
    () async {
      final runtime = await dashboardRuntime();
      addTearDown(runtime.dispose);
      final controller = OrderDetailController(
        runtime.backend,
        'demo-order-1045',
      );
      addTearDown(controller.dispose);
      await controller.load();
      final line = '${controller.picking.single['OrderLineID']}';
      expect(controller.picking.single['QualityResult'], isNull);
      expect(
        await controller.line(line, 'availability', {
          'picked_quantity': 2,
          'unavailable_quantity': 0,
          'outcome': 'FULLY_AVAILABLE',
        }, 1),
        isTrue,
      );
      expect(
        await controller.line(line, 'quality', {
          'result': 'PARTIAL',
          'observed_value': 'One pack needs sorting',
          'reason': 'Damaged produce removed',
        }, 1),
        isTrue,
      );
      final results = controller.picking.single['QualityResults'] as List;
      expect(results.single['observed_value'], 'One pack needs sorting');
      expect(results.single['reason'], 'Damaged produce removed');
      expect(results.single['version'], 1);
      expect(
        await controller.line(line, 'quality', {
          'result': 'MET',
          'observed_value': 'Both packs sorted',
        }, controller.picking.single['Version'] as int),
        isTrue,
      );
      await controller.load();
      expect(controller.picking.single['QualityResult'], 'MET');
      expect(
        (controller.picking.single['QualityResults'] as List).single['version'],
        2,
      );
    },
  );
}
