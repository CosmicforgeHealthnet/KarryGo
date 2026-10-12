import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/core/ui/premium_loading.dart';
import 'package:vendor/core/theme/vendor_theme.dart';
import 'package:vendor/features/inventory/ui/stock_screen.dart';
import 'package:vendor/features/orders/ui/order_feedback_dialog.dart';

void main() {
  testWidgets(
    'stock figures remain readable on narrow phones and larger text',
    (tester) async {
      tester.view.physicalSize = const Size(320, 800);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      await tester.pumpWidget(
        MaterialApp(
          theme: vendorTheme(),
          home: MediaQuery(
            data: const MediaQueryData(
              size: Size(320, 800),
              textScaler: TextScaler.linear(1.4),
            ),
            child: const Scaffold(
              body: SingleChildScrollView(
                padding: EdgeInsets.all(22),
                child: StockFigures(
                  record: {
                    'on_hand_quantity': 1000000000,
                    'reserved_quantity': 3,
                    'quarantined_quantity': 2,
                    'in_transit_quantity': 7,
                    'backorder_quantity': 0,
                  },
                ),
              ),
            ),
          ),
        ),
      );
      expect(find.text('On hand'), findsOneWidget);
      expect(find.text('1000000000'), findsOneWidget);
      expect(tester.getTopLeft(find.text('On hand')).dx, lessThan(80));
      expect(tester.takeException(), isNull);
    },
  );
  testWidgets('archive filtering is discreet and remains selectable', (
    tester,
  ) async {
    var selected = false;
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: StatefulBuilder(
            builder: (context, setState) => ArchiveFilter(
              selected: selected,
              onChanged: (v) => setState(() => selected = v),
            ),
          ),
        ),
      ),
    );
    expect(find.text('Include archived'), findsNothing);
    await tester.tap(find.byType(PopupMenuButton<bool>));
    await tester.pumpAndSettle();
    await tester.tap(find.byType(CheckedPopupMenuItem<bool>).last);
    await tester.pumpAndSettle();
    expect(selected, true);
    expect(find.byTooltip('Filters: archived included'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
  testWidgets('skeleton content and animated feedback respect reduced motion', (
    tester,
  ) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: vendorTheme(),
        home: MediaQuery(
          data: const MediaQueryData(disableAnimations: true),
          child: Scaffold(
            body: Builder(
              builder: (context) => Column(
                children: [
                  const PremiumSkeleton(rows: 1),
                  TextButton(
                    onPressed: () => showOrderFeedback(
                      context,
                      title: 'Saved',
                      message: 'Your changes are saved.',
                      success: true,
                    ),
                    child: const Text('Show feedback'),
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.bySemanticsLabel('Loading content'), findsOneWidget);
    await tester.tap(find.text('Show feedback'));
    await tester.pumpAndSettle();
    expect(find.text('Saved'), findsOneWidget);
    expect(find.bySemanticsLabel('Success'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
}
