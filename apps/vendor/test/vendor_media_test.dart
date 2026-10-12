import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'package:vendor/core/network/vendor_command.dart';
import 'package:vendor/core/theme/vendor_theme.dart';
import 'package:vendor/features/media/ui/vendor_media_screen.dart';
import 'package:vendor/features/onboarding/data/document_upload_client.dart';
import 'dashboard_test.dart' show dashboardRuntime, flush;

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  test(
    'Mock upload binds content to its record, verifies bytes and attaches with version checks',
    () async {
      final r = await dashboardRuntime();
      addTearDown(r.dispose);
      final app = r.backend;
      const target = {'target_type': 'FOOD_ITEM', 'target_ref': 'demo-food-0'};
      final bytes = (await rootBundle.load(
        'assets/catalogue/pizza.jpg',
      )).buffer.asUint8List();
      final body = VendorPayload({
        ...target,
        'file_name': 'pizza.jpg',
        'mime_type': 'image/jpeg',
        'byte_size': bytes.length,
      });
      final command = VendorCommand.create();
      final first = await app.businessApi.request(
        'POST',
        '/api/v1/vendor/media/uploads',
        body: body,
        command: command,
      );
      final retry = await app.businessApi.request(
        'POST',
        '/api/v1/vendor/media/uploads',
        body: body,
        command: command,
      );
      expect(retry.object['media_ref'], first.object['media_ref']);
      await DocumentUploadClient().upload(
        first.object,
        bytes,
        'image/jpeg',
        transport: r.mockUploadTransport,
      );
      final ref = first.object['media_ref'].toString();
      final status = await app.businessApi.request(
        'GET',
        '/api/v1/vendor/media/objects/{mediaRef}',
        pathParameters: {'mediaRef': ref},
        query: target,
      );
      expect(status.object['usable'], true);
      expect(
        await app.client.mediaFile(ref, 'FOOD_ITEM', 'demo-food-0'),
        bytes,
      );
      await expectLater(
        app.client.mediaFile(ref, 'FOOD_ITEM', 'demo-food-1'),
        throwsA(isA<Exception>()),
      );
      await app.foodCatalogueApi.request(
        'PUT',
        '/api/v1/vendor/food/items/{itemId}/media',
        pathParameters: {'itemId': 'demo-food-0'},
        body: VendorPayload({
          'expected_version': 1,
          'media': [
            {'media_ref': ref, 'is_primary': true},
          ],
        }),
        command: VendorCommand.create(),
      );
      final item = await app.foodCatalogueApi.request(
        'GET',
        '/api/v1/vendor/food/items/{itemId}',
        pathParameters: {'itemId': 'demo-food-0'},
      );
      expect(item.object['version'], 2);
      expect((item.object['media'] as List).single['media_ref'], ref);
      await expectLater(
        app.foodCatalogueApi.request(
          'PUT',
          '/api/v1/vendor/food/items/{itemId}/media',
          pathParameters: {'itemId': 'demo-food-0'},
          body: VendorPayload({'expected_version': 1, 'media': []}),
          command: VendorCommand.create(),
        ),
        throwsA(isA<Exception>()),
      );
    },
  );
  testWidgets(
    'Dark media screen uploads a real sample and saves a primary product photo',
    (tester) async {
      tester.view.physicalSize = const Size(360, 800);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final r = (await tester.runAsync(() => dashboardRuntime()))!;
      addTearDown(r.dispose);
      await tester.pumpWidget(
        MaterialApp(
          theme: vendorTheme(brightness: Brightness.dark),
          home: VendorMediaScreen(
            runtime: r,
            targetType: 'FOOD_ITEM',
            targetRef: 'demo-food-0',
            entries: const [],
            save: (media) async => MediaSave(
              r.backend.foodCatalogueApi,
              'PUT',
              '/api/v1/vendor/food/items/{itemId}/media',
              {'itemId': 'demo-food-0'},
              {'expected_version': 1, 'media': media},
            ),
          ),
        ),
      );
      await flush(tester);
      await tester.tap(find.text('Use sample image'));
      await flush(tester);
      await tester.scrollUntilVisible(find.text('Add this file'), 150);
      expect(find.text('Ready to add'), findsOneWidget);
      expect(find.byType(Image), findsWidgets);
      await tester.ensureVisible(find.text('Add this file'));
      await tester.tap(find.text('Add this file'));
      await flush(tester);
      expect(find.text('Primary photo'), findsOneWidget);
      await tester.ensureVisible(find.text('Save media'));
      await tester.tap(find.text('Save media'));
      await flush(tester);
      expect(find.text('Media saved'), findsOneWidget);
      expect(tester.takeException(), isNull);
    },
  );
}
