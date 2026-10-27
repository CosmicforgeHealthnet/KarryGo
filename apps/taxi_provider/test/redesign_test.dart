import 'dart:convert';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:taxi_provider/app/driver_services.dart';
import 'package:taxi_provider/core/config/taxi_provider_config.dart';
import 'package:taxi_provider/core/theme/karryam_theme.dart';
import 'package:taxi_provider/core/widgets/driver_code_sheet.dart';
import 'package:taxi_provider/features/auth/data/provider_auth_api.dart';
import 'package:taxi_provider/features/auth/data/provider_session_store.dart';
import 'package:taxi_provider/features/auth/models/provider_auth_models.dart';
import 'package:taxi_provider/features/auth/state/provider_auth_controller.dart';
import 'package:taxi_provider/features/home/data/provider_api.dart';
import 'package:taxi_provider/features/home/models/provider_booking.dart';
import 'package:taxi_provider/features/home/state/provider_home_controller.dart';
import 'package:taxi_provider/features/home/ui/screens/provider_active_trip_screen.dart';
import 'package:taxi_provider/features/home/ui/screens/provider_dashboard_screen.dart';
import 'package:taxi_provider/features/profile/ui/provider_profile_screen.dart';

class SignedInAuth extends ProviderAuthController {
  SignedInAuth(ProviderAuthApi api)
    : super(api: api, sessionStore: MemoryProviderSessionStore());
  @override
  ProviderAuthState get state => const ProviderAuthState(
    status: ProviderAuthStatus.authenticated,
    session: ProviderSession(
      accessToken: 'test',
      refreshToken: 'test',
      provider: TaxiProvider(
        id: 'driver',
        phone: '+2348000000000',
        firstName: 'Long driver name',
        lastName: 'For accessibility review',
        accountStatus: 'suspended',
        onboardingStatus: 'pending',
        acceptsParcels: false,
      ),
    ),
  );
}

void main() {
  const config = ApiCoreConfig(baseUrl: 'http://localhost/api/v1/taxi');
  late ProviderAuthApi authApi;
  late ProviderApi api;
  late SignedInAuth auth;
  late ProviderHomeController home;
  setUp(() {
    final client = MockClient(
      (request) async => http.Response(
        jsonEncode({
          'success': true,
          'data': {'vehicles': []},
        }),
        200,
      ),
    );
    authApi = ProviderAuthApi(config: config, client: client);
    api = ProviderApi(config: config, client: client);
    auth = SignedInAuth(authApi);
    home = ProviderHomeController(
      api: api,
      authApi: authApi,
      authController: auth,
    );
  });
  tearDown(() {
    home.dispose();
    auth.dispose();
    api.close();
    authApi.close();
  });
  Widget host(Widget child, Brightness brightness) => MaterialApp(
    theme: karryamTheme(brightness),
    home: MediaQuery(
      data: const MediaQueryData(
        size: Size(360, 800),
        textScaler: TextScaler.linear(1.5),
        disableAnimations: true,
      ),
      child: child,
    ),
  );
  for (final brightness in Brightness.values) {
    testWidgets(
      'profile preserves real pending/suspended status and fits large text in $brightness',
      (tester) async {
        tester.view.physicalSize = const Size(360, 800);
        tester.view.devicePixelRatio = 1;
        addTearDown(tester.view.resetPhysicalSize);
        addTearDown(tester.view.resetDevicePixelRatio);
        await tester.pumpWidget(
          host(
            ProviderProfileScreen(
              authApi: authApi,
              authController: auth,
              homeController: home,
              services: DriverServices(
                config: const TaxiProviderConfig(
                  taxiApiBaseUrl: 'http://localhost/api/v1/taxi',
                  walletApiBaseUrl: 'http://localhost/api/v1/payment-wallet',
                  supportApiBaseUrl: 'http://localhost/api/v1/support-disputes',
                  referralApiBaseUrl: 'http://localhost/api/v1/referrals',
                ),
                auth: auth,
              ),
            ),
            brightness,
          ),
        );
        await tester.pumpAndSettle();
        expect(find.text('Suspended'), findsOneWidget);
        expect(find.text('No ratings yet'), findsOneWidget);
        // The profile is long (account menu, vehicles, settings); scroll to
        // the bottom, checking every frame lays out at large text.
        for (var i = 0; i < 16; i++) {
          await tester.drag(find.byType(ListView), const Offset(0, -400));
          await tester.pumpAndSettle();
          expect(tester.takeException(), isNull);
        }
        expect(find.text('Pending'), findsOneWidget);
      },
    );
    testWidgets(
      'map-first dashboard remains usable without Maps keys in $brightness',
      (tester) async {
        tester.view.physicalSize = const Size(360, 800);
        tester.view.devicePixelRatio = 1;
        addTearDown(tester.view.resetPhysicalSize);
        addTearDown(tester.view.resetDevicePixelRatio);
        await tester.pumpWidget(
          host(
            ProviderDashboardScreen(controller: home, authController: auth),
            brightness,
          ),
        );
        await tester.pumpAndSettle();
        expect(find.text('Map unavailable'), findsOneWidget);
        expect(find.text('Go online'), findsOneWidget);
        expect(tester.takeException(), isNull);
      },
    );
    testWidgets(
      'in-progress parcel requires handover and hides cancel in $brightness',
      (tester) async {
        const booking = ProviderBooking(
          id: 'job',
          status: BookingStatus.inProgress,
          pickupVerification: PickupVerification.verified,
          deliveryVerification: DeliveryVerification.pending,
          isParcel: true,
          vehicleClass: 'sedan',
          stops: [],
        );
        await tester.pumpWidget(
          host(
            ProviderActiveTripScreen(controller: home, booking: booking),
            brightness,
          ),
        );
        await tester.pumpAndSettle();
        expect(find.text('Scan receiver code'), findsOneWidget);
        expect(find.text('Cancel trip'), findsNothing);
        expect(tester.takeException(), isNull);
      },
    );
  }
  testWidgets('manual handover rejects short code without closing sheet', (
    tester,
  ) async {
    await tester.pumpWidget(
      host(
        const Scaffold(
          body: DriverCodeSheet(
            title: 'Verify pickup',
            subtitle: 'Ask for the code.',
          ),
        ),
        Brightness.dark,
      ),
    );
    await tester.enterText(find.byType(TextField), '12');
    await tester.tap(find.text('Verify code'));
    await tester.pump();
    expect(find.text('Enter a six-digit code.'), findsOneWidget);
  });
}
