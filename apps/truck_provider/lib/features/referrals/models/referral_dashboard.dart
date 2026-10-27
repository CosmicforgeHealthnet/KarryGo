/// The driver's referral dashboard, as referral-service reports it.
class ReferralTotals {
  const ReferralTotals({this.earnedKobo = 0, this.pendingKobo = 0, this.heldKobo = 0, this.takenBackKobo = 0, this.netKobo = 0});

  final int earnedKobo;
  final int pendingKobo;
  final int heldKobo;
  final int takenBackKobo;
  final int netKobo;

  factory ReferralTotals.fromJson(Map<String, dynamic>? j) => ReferralTotals(
        earnedKobo: _int(j?['earned_kobo']),
        pendingKobo: _int(j?['pending_kobo']),
        heldKobo: _int(j?['held_kobo']),
        takenBackKobo: _int(j?['taken_back_kobo']),
        netKobo: _int(j?['net_kobo']),
      );
}

/// One person this driver referred. Nothing identifies them: only how far
/// they have got and what they have earned the driver.
class ReferralEntry {
  const ReferralEntry({
    required this.id,
    required this.kind,
    required this.joinedAt,
    required this.stage,
    required this.jobsRequired,
    required this.jobsCounted,
    this.earningUntil,
    required this.earnedKobo,
    required this.pendingKobo,
  });

  final String id;

  /// "driver" or "customer".
  final String kind;
  final DateTime joinedAt;

  /// signed_up, in_progress, earning, qualified, ended, under_review or not_eligible.
  final String stage;
  final int jobsRequired;
  final int jobsCounted;
  final DateTime? earningUntil;
  final int earnedKobo;
  final int pendingKobo;

  bool get isDriver => kind == 'driver';

  factory ReferralEntry.fromJson(Map<String, dynamic> j) => ReferralEntry(
        id: j['id'] as String? ?? '',
        kind: j['kind'] as String? ?? 'customer',
        joinedAt: DateTime.tryParse(j['joined_at'] as String? ?? '')?.toLocal() ?? DateTime.now(),
        stage: j['stage'] as String? ?? 'signed_up',
        jobsRequired: _int(j['jobs_required']),
        jobsCounted: _int(j['jobs_counted']),
        earningUntil: DateTime.tryParse(j['earning_until'] as String? ?? '')?.toLocal(),
        earnedKobo: _int(j['earned_kobo']),
        pendingKobo: _int(j['pending_kobo']),
      );
}

class ReferralMilestone {
  const ReferralMilestone({required this.referrals, required this.amountKobo, required this.badge, this.achievedAt});

  final int referrals;
  final int amountKobo;
  final bool badge;
  final DateTime? achievedAt;

  bool get achieved => achievedAt != null;

  factory ReferralMilestone.fromJson(Map<String, dynamic> j) => ReferralMilestone(
        referrals: _int(j['referrals']),
        amountKobo: _int(j['amount_kobo']),
        badge: j['badge'] as bool? ?? false,
        achievedAt: DateTime.tryParse(j['achieved_at'] as String? ?? '')?.toLocal(),
      );
}

class ReferralDashboard {
  const ReferralDashboard({
    required this.code,
    required this.shareLink,
    required this.status,
    required this.qualifiedCustomerReferrals,
    required this.powerUser,
    required this.earnings,
    required this.promotional,
    required this.referrals,
    required this.milestones,
    required this.customerBonusKobo,
    required this.commissionBps,
    required this.activationJobs,
    required this.windowDays,
  });

  final String code;
  final String shareLink;

  /// active, frozen (under review) or blocked.
  final String status;
  final int qualifiedCustomerReferrals;
  final bool powerUser;
  final ReferralTotals earnings;
  final ReferralTotals promotional;
  final List<ReferralEntry> referrals;
  final List<ReferralMilestone> milestones;
  final int customerBonusKobo;
  final int commissionBps;
  final int activationJobs;
  final int windowDays;

  bool get frozen => status == 'frozen';
  bool get blocked => status == 'blocked';

  factory ReferralDashboard.fromJson(Map<String, dynamic> j) {
    final rules = j['rules'] is Map ? Map<String, dynamic>.from(j['rules'] as Map) : const <String, dynamic>{};
    List<Map<String, dynamic>> list(String key) => (j[key] as List? ?? const [])
        .whereType<Map>()
        .map((m) => Map<String, dynamic>.from(m))
        .toList();
    Map<String, dynamic>? map(String key) => j[key] is Map ? Map<String, dynamic>.from(j[key] as Map) : null;
    return ReferralDashboard(
      code: j['code'] as String? ?? '',
      shareLink: j['share_link'] as String? ?? '',
      status: j['status'] as String? ?? 'active',
      qualifiedCustomerReferrals: _int(j['qualified_customer_referrals']),
      powerUser: j['power_user'] as bool? ?? false,
      earnings: ReferralTotals.fromJson(map('earnings')),
      promotional: ReferralTotals.fromJson(map('promotional')),
      referrals: list('referrals').map(ReferralEntry.fromJson).toList(),
      milestones: list('milestones').map(ReferralMilestone.fromJson).toList(),
      customerBonusKobo: _int(rules['customer_referral_bonus_kobo']),
      commissionBps: _int(rules['provider_commission_bps']),
      activationJobs: _int(rules['provider_activation_jobs']),
      windowDays: _int(rules['provider_window_days']),
    );
  }
}

int _int(Object? v) => (v as num?)?.toInt() ?? 0;
