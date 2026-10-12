import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'package:vendor/core/theme/vendor_theme.dart';
import 'package:vendor/features/taxonomy/state/taxonomy_workspace_controller.dart';
import 'package:vendor/features/taxonomy/ui/taxonomy_workspace_screen.dart';
import 'finance_test.dart' show FinanceTransport, financeApp;
import 'order_support_test.dart' show SupportRuntime;
import 'dashboard_test.dart' show flush;

class TaxonomyTransport extends FinanceTransport {
  bool loseMapping = false;
  final attempts = <JsonObject>[];
  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    if (request.method != 'PUT' || !request.url.path.contains('/taxonomy/')) {
      return super.send(request);
    }
    final bytes = await request.finalize().toBytes();
    final copy = http.Request(request.method, request.url)
      ..headers.addAll(request.headers)
      ..bodyBytes = bytes;
    attempts.add({
      'body': jsonDecode(utf8.decode(bytes)),
      'key': request.headers.entries
          .firstWhere((e) => e.key.toLowerCase() == 'idempotency-key')
          .value,
    });
    final response = await super.send(copy);
    if (loseMapping) {
      loseMapping = false;
      await response.stream.drain<void>();
      throw http.ClientException('Lost accepted mapping');
    }
    return response;
  }
}

List<JsonObject> candidate(TaxonomyWorkspaceController c, String facet) => [
  {
    'node_code': c.nodes.firstWhere(
      (n) => n['FacetID'] == facet,
    )['CanonicalCode'],
    'mapping_type': 'PRIMARY',
    'confidence_basis': 'MANUAL',
  },
];
void main() {
  test(
    'lost mapping response retries frozen payload and key after reopening',
    () async {
      final transport = TaxonomyTransport();
      final app = await financeApp(transport);
      final runtime = SupportRuntime(app);
      final c = TaxonomyWorkspaceController(
        runtime,
        'FOOD_ITEM',
        'demo-food-0',
      );
      await c.load();
      expect(c.error, isNull);
      final f = c.facets.first;
      final body = candidate(c, f['ID']);
      transport.loseMapping = true;
      expect(
        await c.save(
          facetCode: f['Code'],
          candidates: body,
          reviewed: c.reviewToken(),
        ),
        isFalse,
      );
      body.first['node_code'] = 'MUTATED';
      expect(c.pending, isNotNull);
      final restored = TaxonomyWorkspaceController(
        runtime,
        'FOOD_ITEM',
        'demo-food-0',
      );
      await restored.load();
      expect(await restored.save(), isTrue);
      expect(restored.pending, isNull);
      expect(transport.attempts, hasLength(2));
      expect(transport.attempts[0], transport.attempts[1]);
      expect(transport.attempts.first['body'].keys.toSet(), {
        'facet_code',
        'source_type',
        'source_ref',
        'mappings',
      });
      expect(restored.activeMappings(f['ID']), hasLength(1));
      c.dispose();
      restored.dispose();
      runtime.dispose();
      app.dispose();
    },
  );
  test(
    'facet replacement clears only its categories and refuses multiple primaries',
    () async {
      final transport = TaxonomyTransport();
      final app = await financeApp(transport);
      final runtime = SupportRuntime(app);
      final c = TaxonomyWorkspaceController(
        runtime,
        'FOOD_ITEM',
        'demo-food-0',
      );
      await c.load();
      expect(c.error, isNull);
      final first = c.facets[0], second = c.facets[1];
      expect(
        await c.save(
          facetCode: first['Code'],
          candidates: candidate(c, first['ID']),
          reviewed: c.reviewToken(),
        ),
        isTrue,
      );
      expect(
        await c.save(
          facetCode: second['Code'],
          candidates: candidate(c, second['ID']),
          reviewed: c.reviewToken(),
        ),
        isTrue,
      );
      final count = transport.attempts.length;
      final two = c.nodes
          .where((n) => n['FacetID'] == first['ID'])
          .take(2)
          .map(
            (n) => <String, dynamic>{
              'node_code': n['CanonicalCode'],
              'mapping_type': 'PRIMARY',
              'confidence_basis': 'MANUAL',
            },
          )
          .toList();
      expect(
        await c.save(
          facetCode: first['Code'],
          candidates: two,
          reviewed: c.reviewToken(),
        ),
        isFalse,
      );
      expect(transport.attempts, hasLength(count));
      expect(
        await c.save(
          facetCode: first['Code'],
          candidates: [],
          reviewed: c.reviewToken(),
        ),
        isTrue,
      );
      expect(c.activeMappings(first['ID']), isEmpty);
      expect(c.activeMappings(second['ID']), hasLength(1));
      c.dispose();
      runtime.dispose();
      app.dispose();
    },
  );
  test(
    'changed classifications and mismatched sources never submit stale edits',
    () async {
      final transport = TaxonomyTransport();
      final app = await financeApp(transport);
      final runtime = SupportRuntime(app);
      final a = TaxonomyWorkspaceController(
        runtime,
        'FOOD_ITEM',
        'demo-food-0',
      );
      final b = TaxonomyWorkspaceController(
        runtime,
        'FOOD_ITEM',
        'demo-food-0',
      );
      await a.load();
      await b.load();
      final old = a.reviewToken();
      final f = a.facets.first;
      expect(
        await b.save(
          facetCode: f['Code'],
          candidates: candidate(b, f['ID']),
          reviewed: b.reviewToken(),
        ),
        isTrue,
      );
      expect(
        await a.save(facetCode: f['Code'], candidates: [], reviewed: old),
        isFalse,
      );
      expect(transport.attempts, hasLength(1));
      expect(a.error, contains('changed'));
      final wrong = TaxonomyWorkspaceController(
        runtime,
        'FOOD_ITEM',
        'demo-product',
      );
      await wrong.load();
      expect(wrong.stale, isTrue);
      expect(wrong.writable, isFalse);
      a.dispose();
      b.dispose();
      wrong.dispose();
      runtime.dispose();
      app.dispose();
    },
  );
  testWidgets('classification and category selection fit a small dark phone', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(360, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final app = (await tester.runAsync(() => financeApp(TaxonomyTransport())))!;
    final runtime = SupportRuntime(app);
    await tester.pumpWidget(
      MaterialApp(
        theme: vendorTheme(brightness: Brightness.dark),
        home: TaxonomyWorkspaceScreen(
          runtime: runtime,
          sourceType: 'FOOD_ITEM',
          sourceRef: 'demo-food-0',
          name: 'Smoky jollof rice',
        ),
      ),
    );
    await flush(tester);
    expect(find.text('Smoky jollof rice'), findsOneWidget);
    expect(tester.takeException(), isNull);
    await tester.scrollUntilVisible(
      find.byTooltip('Edit Provider Type'),
      200,
      scrollable: find.byType(Scrollable).first,
    );
    await tester.pumpAndSettle();
    await tester.tap(find.byTooltip('Edit Provider Type'));
    await tester.pumpAndSettle();
    expect(find.text('Review categories'), findsOneWidget);
    await tester.tap(find.byType(Checkbox).first);
    await tester.pumpAndSettle();
    expect(find.text('Primary'), findsOneWidget);
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox.shrink());
    runtime.dispose();
    app.dispose();
  });
}
