import 'dart:convert';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:vendor/features/settings/state/settings_controller.dart';
import 'finance_test.dart' show FinanceTransport, financeApp;

class SettingsTransport extends FinanceTransport {
  bool lose = false, reject = false;
  int staffPosts = 0;
  final List<String?> staffKeys = [];
  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    if (request.method != 'POST' || !request.url.path.endsWith('/staff')) {
      return super.send(request);
    }
    staffPosts++;
    staffKeys.add(
      request.headers.entries
          .where((e) => e.key.toLowerCase() == 'idempotency-key')
          .firstOrNull
          ?.value,
    );
    if (reject) {
      return http.StreamedResponse(
        Stream.value(
          utf8.encode(
            jsonEncode({
              'success': false,
              'error': {'code': 'UNAUTHORIZED', 'message': 'Session expired.'},
            }),
          ),
        ),
        401,
      );
    }
    final response = await inner.send(request);
    if (lose) {
      lose = false;
      await response.stream.drain<void>();
      throw http.ClientException('Response lost after acceptance');
    }
    return response;
  }
}

void main() {
  test(
    'IAM accepted response loss blocks replay until an explicit refresh',
    () async {
      final transport = SettingsTransport();
      final app = await financeApp(transport);
      addTearDown(app.dispose);
      final c = SettingsController(app, SettingsKind.staff);
      addTearDown(c.dispose);
      await c.load();
      final count = c.rows.length;
      transport.lose = true;
      expect(
        await c.mutate('POST', c.collection, {
          'email': 'staff@example.com',
          'role_id': 'role_branch_manager',
          'branch_id': null,
        }),
        isFalse,
      );
      expect(transport.staffPosts, 1);
      expect(transport.staffKeys, [null]);
      expect(c.stale, isTrue);
      expect(c.rows.length, count);
      expect(
        await c.mutate('POST', c.collection, {
          'email': 'staff@example.com',
          'role_id': 'role_branch_manager',
          'branch_id': null,
        }),
        isFalse,
      );
      expect(transport.staffPosts, 1);
      await c.load();
      expect(c.rows.length, count + 1);
      expect(c.stale, isFalse);
    },
  );
  test('IAM write 401 does not refresh authentication or retry', () async {
    final transport = SettingsTransport();
    final app = await financeApp(transport);
    addTearDown(app.dispose);
    final c = SettingsController(app, SettingsKind.staff);
    addTearDown(c.dispose);
    await c.load();
    transport.reject = true;
    expect(
      await c.mutate('POST', c.collection, {
        'email': 'staff@example.com',
        'role_id': 'role_branch_manager',
        'branch_id': null,
      }),
      isFalse,
    );
    expect(transport.staffPosts, 1);
    expect(transport.refreshes, 0);
    expect(transport.staffKeys, [null]);
  });
}
