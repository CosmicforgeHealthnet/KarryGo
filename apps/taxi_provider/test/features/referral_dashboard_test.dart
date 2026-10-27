import 'package:flutter_test/flutter_test.dart';
import 'package:taxi_provider/core/config/taxi_provider_config.dart';
import 'package:taxi_provider/features/referrals/data/referral_dashboard.dart';

void main() {
  test('referral-service sits next to taxi behind the gateway and on its own dev port', () {
    expect(
      TaxiProviderConfig.siblingBaseUrl('https://api-gateway.staging.karryam.com/api/v1/taxi', '/api/v1/referrals', 8114),
      'https://api-gateway.staging.karryam.com/api/v1/referrals',
    );
    expect(
      TaxiProviderConfig.siblingBaseUrl('http://192.168.1.112:8102/api/v1/taxi', '/api/v1/referrals', 8114),
      'http://192.168.1.112:8114/api/v1/referrals',
    );
  });

  test('parses the dashboard, defaulting anything missing', () {
    final d = ReferralDashboard.fromJson({
      'code': 'TUN3D9',
      'share_link': 'https://karryam.com/r/TUN3D9',
      'status': 'frozen',
      'earnings': {'net_kobo': 120000, 'pending_kobo': 30000},
      'referrals': [
        {'id': 'r1', 'kind': 'driver', 'joined_at': '2026-10-01T09:00:00Z', 'stage': 'earning',
          'jobs_required': 5, 'jobs_counted': 5, 'earning_until': '2026-12-30T09:00:00Z', 'earned_kobo': 120000},
      ],
      'rules': {'provider_commission_bps': 1000, 'provider_activation_jobs': 5, 'provider_window_days': 90},
    });
    expect(d.frozen, isTrue);
    expect(d.earnings.netKobo, 120000);
    expect(d.promotional.netKobo, 0);
    expect(d.referrals.single.isDriver, isTrue);
    expect(d.referrals.single.earningUntil, isNotNull);
    expect(d.milestones, isEmpty);
  });
}
