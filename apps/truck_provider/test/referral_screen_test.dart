import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:truck_provider/features/referrals/data/referral_api.dart';
import 'package:truck_provider/features/referrals/state/referral_controller.dart';
import 'package:truck_provider/features/referrals/ui/provider_referral_screen.dart';
import 'package:truck_provider/features/wallet/data/provider_wallet_api.dart';
import 'package:truck_provider/features/wallet/state/provider_withdrawal_controller.dart';

final _dashboard = {
  'code': 'ADA7K2',
  'share_link': 'https://karryam.com/r/ADA7K2',
  'status': 'active',
  'qualified_customer_referrals': 2,
  'power_user': false,
  'earnings': {'earned_kobo': 450000, 'pending_kobo': 150000, 'held_kobo': 0, 'taken_back_kobo': 0, 'net_kobo': 450000},
  'promotional': {'net_kobo': 0},
  'referral_count': 2,
  'referrals': [
    {'id': 'r1', 'kind': 'driver', 'joined_at': '2026-10-01T09:00:00Z', 'status': 'active', 'stage': 'in_progress',
      'jobs_required': 5, 'jobs_counted': 3, 'earned_kobo': 0, 'pending_kobo': 150000},
    {'id': 'r2', 'kind': 'customer', 'joined_at': '2026-09-20T09:00:00Z', 'status': 'active', 'stage': 'qualified',
      'jobs_required': 1, 'jobs_counted': 1, 'earned_kobo': 50000, 'pending_kobo': 0},
  ],
  'milestones': [
    {'key': 'qualified_5', 'referrals': 5, 'amount_kobo': 250000, 'balance': 'promotional', 'badge': false},
    {'key': 'qualified_10', 'referrals': 10, 'amount_kobo': 750000, 'balance': 'earnings', 'badge': true},
  ],
  'rules': {'customer_referral_bonus_kobo': 50000, 'provider_commission_bps': 1000, 'provider_activation_jobs': 5, 'provider_window_days': 90},
};

Future<void> _pump(WidgetTester tester, {required int referralKobo}) async {
  final mock = MockClient((req) async {
    final path = req.url.path;
    Object data;
    if (path.endsWith('/me/dashboard')) {
      data = _dashboard;
    } else if (path.endsWith('/provider/earnings')) {
      data = {'available_kobo': 0, 'escrow_kobo': 0, 'pending_kobo': 0, 'referral_kobo': referralKobo, 'currency': 'NGN'};
    } else if (path.endsWith('/provider/bank-accounts')) {
      data = {'bank_accounts': []};
    } else {
      data = {'pin_set': true};
    }
    return http.Response(jsonEncode({'success': true, 'data': data}), 200);
  });
  final referrals = ProviderReferralController(
    api: ReferralApi(config: ApiCoreConfig(baseUrl: 'http://x/api/v1/referrals'), client: mock),
    accessToken: () => 'token',
  );
  final withdrawals = ProviderWithdrawalController(
    api: ProviderWalletApi(config: ApiCoreConfig(baseUrl: 'http://x/api/v1/payment-wallet'), client: mock),
    accessToken: () => 'token',
  );
  await tester.binding.setSurfaceSize(const Size(430, 2200));
  await tester.pumpWidget(MaterialApp(
    home: ProviderReferralScreen(controller: referrals, withdrawalController: withdrawals),
  ));
  await tester.pumpAndSettle();
}

void main() {
  testWidgets('shows the code, earnings and each referral\'s progress', (tester) async {
    await _pump(tester, referralKobo: 450000);
    expect(find.text('ADA7K2'), findsOneWidget);
    expect(find.text('3 of 5 paid jobs done'), findsOneWidget);
    expect(find.text('Booked and qualified'), findsOneWidget);
    expect(find.text('YOUR REFERRALS (2)'), findsOneWidget);
    expect(find.textContaining('10% of Karryam'), findsOneWidget);
    // Below the ₦5,000 minimum, cash-out stays off.
    expect(find.textContaining('You can cash out from'), findsOneWidget);
  });

  testWidgets('cash-out opens once the referral balance reaches the minimum', (tester) async {
    await _pump(tester, referralKobo: 600000);
    expect(find.textContaining('You can cash out from'), findsNothing);
    await tester.tap(find.text('Cash out'));
    await tester.pumpAndSettle();
    expect(find.text('Cash out referrals'), findsOneWidget);
    expect(find.text('Referral earnings'), findsOneWidget);
  });
}
