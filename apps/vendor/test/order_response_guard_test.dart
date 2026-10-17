import 'dart:convert';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'package:vendor/features/orders/state/order_detail_controller.dart';
import 'finance_test.dart' show FinanceTransport, financeApp;

class OrderResponseTransport extends FinanceTransport {
  JsonObject? replacement;
  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) {
    if (request.method == 'GET' &&
        request.url.path.endsWith('/orders/demo-order-1048') &&
        replacement != null) {
      return Future.value(
        http.StreamedResponse(
          Stream.value(
            utf8.encode(jsonEncode({'success': true, 'data': replacement})),
          ),
          200,
        ),
      );
    }
    return super.send(request);
  }
}

void main() {
  test(
    'unverified or foreign order responses cannot expose order actions',
    () async {
      final transport = OrderResponseTransport();
      final app = await financeApp(transport);
      addTearDown(app.dispose);
      addTearDown(transport.close);
      final controller = OrderDetailController(app, 'demo-order-1048');
      addTearDown(controller.dispose);
      expect(controller.can('accept'), isFalse);
      await controller.load();
      expect(controller.error, isNull);
      final original = Map<String, dynamic>.from(controller.order!);
      expect(controller.can('accept'), isTrue);
      for (final changes in [
        {'business_id': 'another-business'},
        {'vendor_order_id': 'another-order'},
        {'version': 0},
      ]) {
        transport.replacement = {...original, ...changes};
        await controller.load();
        expect(controller.order, isNull);
        expect(controller.can('accept'), isFalse);
        expect(controller.error, contains('incomplete order information'));
      }
      transport.replacement = original;
      await controller.load();
      expect(controller.error, isNull);
      expect(controller.can('accept'), isTrue);
    },
  );
}
