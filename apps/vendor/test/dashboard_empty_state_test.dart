import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/core/theme/vendor_theme.dart';
import 'package:vendor/features/dashboard/ui/dashboard_empty_state.dart';

void main() {
  testWidgets(
    'empty stories fit a phone and keep actions usable at large text sizes',
    (tester) async {
      tester.view.physicalSize = const Size(360, 800);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      var refreshes = 0;
      for (final story in EmptyStory.values) {
        await tester.pumpWidget(
          MaterialApp(
            theme: vendorTheme(),
            home: Scaffold(
              body: MediaQuery(
                data: const MediaQueryData(textScaler: TextScaler.linear(1.6)),
                child: ListView(
                  padding: const EdgeInsets.all(20),
                  children: [
                    DashboardEmptyState(
                      story: story,
                      title: 'No saved entries yet',
                      message: 'Saved entries will appear here when available.',
                      action: 'Refresh details',
                      onAction: () => refreshes++,
                    ),
                  ],
                ),
              ),
            ),
          ),
        );
        await tester.tap(find.text('Refresh details'));
        expect(tester.takeException(), isNull);
      }
      expect(refreshes, EmptyStory.values.length);
    },
  );
  testWidgets(
    'permission and failure stories have distinct icons and colours',
    (tester) async {
      for (final story in [EmptyStory.restricted, EmptyStory.unavailable]) {
        await tester.pumpWidget(
          MaterialApp(
            home: Scaffold(
              body: DashboardEmptyState(
                story: story,
                title: 'Unavailable',
                message: 'Please try again later.',
              ),
            ),
          ),
        );
        final icon = tester.widget<Icon>(
          find.byIcon(
            story == EmptyStory.restricted
                ? Icons.lock_outline_rounded
                : Icons.wifi_off_rounded,
          ),
        );
        expect(
          icon.color,
          story == EmptyStory.restricted
              ? const Color(0xFF9A650D)
              : KarryamColors.error,
        );
        expect(find.text('Try again'), findsNothing);
      }
    },
  );
}
