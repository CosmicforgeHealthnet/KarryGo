import '../core/config/taxi_provider_config.dart';
import '../core/data/driver_client.dart';
import '../features/auth/state/provider_auth_controller.dart';
import '../features/onboarding/data/onboarding_api.dart';
import '../features/support/data/support_api.dart';
import '../features/trips/data/trips_api.dart';
import '../features/wallet/data/wallet_api.dart';

/// The signed-in driver's APIs beyond the ride loop: account setup, trip
/// history, wallet and support. Each talks to its own service as the driver.
class DriverServices {
  DriverServices({
    required TaxiProviderConfig config,
    required ProviderAuthController auth,
  }) : taxi = DriverClient(baseUrl: config.taxiApiBaseUrl, auth: auth),
       _wallet = DriverClient(baseUrl: config.walletApiBaseUrl, auth: auth),
       _support = DriverClient(baseUrl: config.supportApiBaseUrl, auth: auth);

  final DriverClient taxi;
  final DriverClient _wallet;
  final DriverClient _support;

  late final onboarding = OnboardingApi(taxi);
  late final trips = TripsApi(taxi);
  late final wallet = WalletApi(_wallet);
  late final support = SupportApi(_support);

  void close() {
    taxi.close();
    _wallet.close();
    _support.close();
  }
}
