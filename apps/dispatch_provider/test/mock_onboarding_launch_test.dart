import 'package:dispatch_provider/app/dispatch_provider_app.dart';
import 'package:dispatch_provider/core/app_mode.dart';
import 'package:dispatch_provider/core/theme/theme_controller.dart';
import 'package:dispatch_provider/features/auth/ui/phone_entry_screen.dart';
import 'package:dispatch_provider/features/account_setup/ui/emergency_contact_screen.dart';
import 'package:dispatch_provider/features/account_setup/ui/profile_details_screen.dart';
import 'package:dispatch_provider/features/account_setup/ui/operation_mode_screen.dart';
import 'package:dispatch_provider/features/onboarding/ui/onboarding_screen.dart';
import 'package:dispatch_provider/features/profile/mock/profile_mock.dart';
import 'package:dispatch_provider/shared/widgets/mock_badge.dart';
import 'package:flutter/material.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  setUp(() async {
    AppModeConfig.instance.resetMemoryForTesting();
    FlutterSecureStorage.setMockInitialValues({});
    await AppModeConfig.instance.load();
    ProfileMock.reset();
  });

  Future<void> advance(WidgetTester tester, [int count = 4]) async {
    for (var i = 0; i < count; i++) {
      await tester.pump(const Duration(milliseconds: 500));
    }
  }

  test('fresh mock launch ignores the obsolete dashboard default', () async {
    FlutterSecureStorage.setMockInitialValues({
      'dispatch_mock_entry': 'dashboard',
    });
    await AppModeConfig.instance.load();
    expect(AppModeConfig.instance.mockEntry, MockEntry.introduction);
    expect(AppModeConfig.instance.mockJourney, isEmpty);
  });

  test(
    'saved mock setup and filled details survive reload; replay resets them',
    () async {
      await AppModeConfig.instance.setMode(
        AppMode.mock,
        mockEntry: MockEntry.setup,
      );
      ProfileMock.updateMe(
        fullName: 'Tobi Adeyemi',
        state: 'Lagos',
        city: 'Ikeja',
      );
      ProfileMock.saveEmergencyContact(
        fullName: 'Ada Adeyemi',
        phone: '+2348000000001',
        relationship: 'Sibling',
      );
      await AppModeConfig.instance.saveMockJourney({
        'screen': 'guarantor',
        'profile': ProfileMock.snapshot(),
        'operation_type': 'individual',
      });
      ProfileMock.reset();
      await AppModeConfig.instance.load();
      ProfileMock.restore(
        Map<String, dynamic>.from(
          AppModeConfig.instance.mockJourney['profile'] as Map,
        ),
      );
      expect(ProfileMock.profile.fullName, 'Tobi Adeyemi');
      expect(ProfileMock.profile.hasEmergencyContact, isTrue);
      expect(ProfileMock.emergencyContact?.relationship, 'Sibling');
      expect(AppModeConfig.instance.mockJourney['screen'], 'guarantor');
      final revision = AppModeConfig.instance.previewRevision;
      await AppModeConfig.instance.setMode(
        AppMode.mock,
        mockEntry: MockEntry.setup,
      );
      expect(AppModeConfig.instance.previewRevision, greaterThan(revision));
      expect(AppModeConfig.instance.mockJourney, isEmpty);
    },
  );

  testWidgets(
    'first launch shows intro, then prefilled signup; restart does not replay intro',
    (tester) async {
      final theme = ThemeController();
      await tester.pumpWidget(DispatchProviderApp(themeController: theme));
      await tester.pump(const Duration(seconds: 2));
      await advance(tester);
      expect(find.byType(OnboardingScreen), findsOneWidget);
      await tester.tap(find.text('Skip'));
      await advance(tester);
      expect(find.byType(PhoneEntryScreen), findsOneWidget);
      final fields = tester.widgetList<TextField>(find.byType(TextField));
      expect(
        fields.map((field) => field.controller?.text),
        contains('8000000000'),
      );
      expect(
        fields.map((field) => field.controller?.text),
        contains('rider@example.com'),
      );
      await tester.pumpWidget(const SizedBox());
      await advance(tester);
      await tester.pumpWidget(DispatchProviderApp(themeController: theme));
      await tester.pump(const Duration(seconds: 2));
      await advance(tester);
      expect(find.byType(OnboardingScreen), findsNothing);
      expect(find.byType(PhoneEntryScreen), findsOneWidget);
      await tester.pumpWidget(const SizedBox());
      await advance(tester);
    },
  );

  testWidgets(
    'onboarding preview is prefilled and resumes the next saved step',
    (tester) async {
      await tester.runAsync(
        () => AppModeConfig.instance.setMode(
          AppMode.mock,
          mockEntry: MockEntry.setup,
        ),
      );
      final theme = ThemeController();
      await tester.pumpWidget(DispatchProviderApp(themeController: theme));
      await tester.pump(const Duration(seconds: 2));
      await advance(tester);
      final continueButton = find.widgetWithText(
        FilledButton,
        'Continue setup',
      );
      if (continueButton.evaluate().isNotEmpty) {
        await tester.ensureVisible(continueButton);
        await tester.tap(continueButton);
      } else {
        final button = find.byType(FilledButton).last;
        await tester.ensureVisible(button);
        await tester.tap(button);
      }
      await advance(tester);
      expect(find.byType(ProfileDetailsScreen), findsOneWidget);
      final values = tester
          .widgetList<TextField>(find.byType(TextField))
          .map((field) => field.controller?.text);
      expect(values, contains('Tobi Adeyemi'));
      expect(values, contains('rider@example.com'));
      await tester.ensureVisible(find.widgetWithText(FilledButton, 'Continue'));
      await tester.tap(find.widgetWithText(FilledButton, 'Continue'));
      await advance(tester);
      expect(find.byType(OperationModeScreen), findsOneWidget);
      await tester.ensureVisible(find.widgetWithText(FilledButton, 'Continue'));
      await tester.tap(find.widgetWithText(FilledButton, 'Continue'));
      await advance(tester);
      expect(find.byType(EmergencyContactScreen), findsOneWidget);
      await tester.pumpWidget(const SizedBox());
      await advance(tester);
      await tester.pumpWidget(DispatchProviderApp(themeController: theme));
      await tester.pump(const Duration(seconds: 2));
      await advance(tester);
      expect(find.byType(EmergencyContactScreen), findsOneWidget);
      expect(ProfileMock.profile.fullName, 'Tobi Adeyemi');
      await tester.pumpWidget(const SizedBox());
      await advance(tester);
    },
  );

  testWidgets(
    'data source offers explicit intro, prefilled onboarding and dashboard choices',
    (tester) async {
      await tester.pumpWidget(
        const MaterialApp(home: Scaffold(body: MockBadge())),
      );
      await tester.tap(find.byKey(const Key('data-mode-button')));
      await advance(tester);
      await tester.tap(find.byType(DropdownButtonFormField<MockEntry>));
      await advance(tester);
      expect(find.text('Introduction'), findsWidgets);
      expect(find.text('Onboarding \u00b7 prefilled'), findsOneWidget);
      expect(find.text('Dashboard \u00b7 skip setup'), findsOneWidget);
      await tester.pumpWidget(const SizedBox());
      await advance(tester);
    },
  );
}
