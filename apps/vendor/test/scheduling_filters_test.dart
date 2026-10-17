import 'dart:convert';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'package:vendor/features/scheduled_commerce/state/scheduling_workspace_controller.dart';
import 'finance_test.dart' show FinanceTransport, financeApp;
import 'order_support_test.dart' show SupportRuntime;

class FilteredSchedulingTransport extends FinanceTransport {
  List<JsonObject> records = [];
  final reads = <Map<String, String>>[];
  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    if (request.method != 'GET' ||
        !request.url.path.endsWith('/scheduled-orders')) {
      return super.send(request);
    }
    final query = request.url.queryParameters;
    reads.add(Map.of(query));
    final view = query['view'] ?? 'ALL';
    final now = DateTime.now();
    final matching = records.where((r) {
      if (view == 'ALL') return true;
      final future = DateTime.parse(r['DesiredDeliveryAt']).isAfter(now);
      return view == 'UPCOMING' ? future : !future;
    }).toList();
    final offset = int.parse(query['offset'] ?? '0');
    final limit = int.parse(query['limit'] ?? '50');
    return http.StreamedResponse(
      Stream.value(
        utf8.encode(
          jsonEncode({
            'success': true,
            'data': {
              'items': matching.skip(offset).take(limit).toList(),
              'pagination': {'has_more': offset + limit < matching.length},
            },
          }),
        ),
      ),
      200,
    );
  }
}

void main() {
  test(
    'schedule filters reach the server before pagination and reset the page',
    () async {
      final transport = FilteredSchedulingTransport();
      final app = await financeApp(transport);
      addTearDown(app.dispose);
      addTearDown(transport.close);
      final runtime = SupportRuntime(app);
      addTearDown(runtime.dispose);
      final now = DateTime.now().toUtc();
      transport.records = [
        for (var i = 0; i < 83; i++)
          {
            'ID': 'schedule-$i',
            'BusinessID': app.scope.vendorId,
            'BranchID': 'branch',
            'SchedulingStatus': 'ACCEPTED',
            'DesiredDeliveryAt': now
                .add(Duration(days: i < 80 ? -2 : 2))
                .toIso8601String(),
          },
      ];
      final controller = SchedulingWorkspaceController(runtime);
      addTearDown(controller.dispose);
      await controller.selectView('UPCOMING');
      expect(controller.error, isNull);
      expect(controller.rows.map((r) => r['ID']), [
        'schedule-80',
        'schedule-81',
        'schedule-82',
      ]);
      expect(transport.reads.last['view'], 'UPCOMING');
      await controller.selectView('ALL');
      expect(controller.rows, hasLength(50));
      expect(controller.hasMore, isTrue);
      await controller.load(more: true);
      expect(controller.rows, hasLength(83));
      expect(transport.reads.last['offset'], '50');
      await controller.selectView('PAST');
      expect(transport.reads.last['offset'], '0');
      expect(controller.rows, hasLength(50));
      expect(
        controller.rows.every(
          (r) => DateTime.parse(r['DesiredDeliveryAt']).isBefore(now),
        ),
        isTrue,
      );
    },
  );
}
