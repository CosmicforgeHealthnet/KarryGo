import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/foundation.dart';

import '../data/referral_api.dart';
import '../models/referral_dashboard.dart';

/// Loads the driver's referral dashboard.
class ProviderReferralController extends ChangeNotifier {
  ProviderReferralController({required ReferralApi api, required String? Function() accessToken})
      : _api = api,
        _token = accessToken;

  final ReferralApi _api;
  final String? Function() _token;

  ReferralDashboard? _dashboard;
  ReferralDashboard? get dashboard => _dashboard;

  bool _loading = false;
  bool get loading => _loading;

  String? _error;
  String? get error => _error;

  Future<void> load() async {
    final token = _token();
    if (token == null || _loading) return;
    _loading = true;
    _error = null;
    notifyListeners();
    try {
      _dashboard = await _api.getDashboard(accessToken: token);
    } catch (e) {
      _error = e is ApiException ? e.message : 'Could not load your referrals. Please try again.';
    } finally {
      _loading = false;
      notifyListeners();
    }
  }
}
