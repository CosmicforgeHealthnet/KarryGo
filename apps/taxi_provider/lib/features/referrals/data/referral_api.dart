import '../../../core/data/driver_client.dart';
import 'referral_dashboard.dart';

/// The driver's referral dashboard from referral-service, which accepts the
/// taxi driver's own sign-in.
class ReferralApi {
  ReferralApi(this._client);
  final DriverClient _client;

  Future<ReferralDashboard> dashboard() async =>
      ReferralDashboard.fromJson(await _client.getMap('/me/dashboard'));
}
