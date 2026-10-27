import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../../core/format/money_format.dart';
import '../../../core/theme/karryam_theme.dart';
import '../../../core/widgets/karryam_ui.dart';
import '../../wallet/data/wallet_api.dart';
import '../../wallet/ui/withdraw_screen.dart';
import '../data/referral_api.dart';
import '../data/referral_dashboard.dart';

/// The minimum referral cash-out, matching payment-wallet's default.
const _minCashOutKobo = 500000;

/// The driver's referral dashboard: their code, what they have earned, each
/// person they referred and how far that person has got.
class ReferralScreen extends StatefulWidget {
  const ReferralScreen({super.key, required this.api, required this.wallet});

  final ReferralApi api;
  final WalletApi wallet;

  @override
  State<ReferralScreen> createState() => _ReferralScreenState();
}

class _ReferralScreenState extends State<ReferralScreen> {
  ReferralDashboard? _dashboard;
  WalletBalance? _balance;
  String? _error;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() => _error = null);
    try {
      final results = await Future.wait([
        widget.api.dashboard(),
        widget.wallet.balance(),
      ]);
      if (!mounted) return;
      setState(() {
        _dashboard = results[0] as ReferralDashboard;
        _balance = results[1] as WalletBalance;
      });
    } on ApiException catch (e) {
      if (mounted) setState(() => _error = e.message);
    } catch (_) {
      if (mounted) {
        setState(
          () => _error = 'Could not load your referrals. Please try again.',
        );
      }
    }
  }

  Future<void> _copy(String text, String what) async {
    await Clipboard.setData(ClipboardData(text: text));
    if (!mounted) return;
    ScaffoldMessenger.of(
      context,
    ).showSnackBar(SnackBar(content: Text('$what copied')));
  }

  Future<void> _share(ReferralDashboard d) async {
    final message =
        'Join me on Karryam. Sign up with my code ${d.code}: ${d.shareLink}';
    final uri = Uri.parse(
      'https://wa.me/?text=${Uri.encodeComponent(message)}',
    );
    final opened = await launchUrl(
      uri,
      mode: LaunchMode.externalApplication,
    ).catchError((_) => false);
    if (!opened) await _copy(message, 'Invite message');
  }

  Future<void> _cashOut(int referralKobo) async {
    final accounts = await widget.wallet.bankAccounts();
    if (!mounted) return;
    if (accounts.isEmpty) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Add a bank account in your wallet first.'),
        ),
      );
      return;
    }
    await Navigator.of(context).push(
      MaterialPageRoute<bool>(
        builder: (_) => WithdrawScreen(
          api: widget.wallet,
          availableKobo: referralKobo,
          accounts: accounts,
          referral: true,
        ),
      ),
    );
    if (mounted) _load();
  }

  @override
  Widget build(BuildContext context) {
    final d = _dashboard;
    final Widget body;
    if (d == null) {
      body = _error != null
          ? KarryamPlaceholder.error(message: _error!, onRetry: _load)
          : const KarryamPlaceholder.loading();
    } else {
      body = RefreshIndicator(onRefresh: _load, child: _content(d));
    }
    return KarryamPage(
      title: 'Refer & earn',
      subtitle: 'Invite drivers and customers',
      body: body,
    );
  }

  Widget _content(ReferralDashboard d) {
    final colors = KarryamColors.of(context);
    final referralKobo = _balance?.referralKobo ?? 0;
    final canCashOut = referralKobo >= _minCashOutKobo;
    final percent = d.commissionBps / 100;
    final pct = percent == percent.roundToDouble()
        ? percent.toStringAsFixed(0)
        : percent.toStringAsFixed(1);
    return ListView(
      padding: const EdgeInsets.fromLTRB(20, 8, 20, 32),
      children: [
        if (d.frozen || d.blocked)
          Padding(
            padding: const EdgeInsets.only(bottom: 14),
            child: KarryamPanel(
              child: Text(
                d.blocked
                    ? 'Your referral account is closed. Contact support if you think this is a mistake.'
                    : 'Your referrals are under review. Rewards are held until the review is done.',
                style: TextStyle(color: colors.warning, height: 1.4),
              ),
            ),
          ),
        Container(
          padding: const EdgeInsets.all(20),
          decoration: BoxDecoration(
            color: KarryamColors.navy,
            borderRadius: BorderRadius.circular(20),
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const Text(
                'Your referral code',
                style: TextStyle(color: Color(0xCCFFFFFF), fontSize: 13),
              ),
              const SizedBox(height: 6),
              Row(
                children: [
                  Expanded(
                    child: Text(
                      d.code,
                      style: const TextStyle(
                        color: Colors.white,
                        fontSize: 28,
                        fontWeight: FontWeight.w800,
                        letterSpacing: 2,
                      ),
                    ),
                  ),
                  IconButton(
                    tooltip: 'Copy code',
                    onPressed: () => _copy(d.code, 'Code'),
                    icon: const Icon(Icons.copy_rounded, color: Colors.white),
                  ),
                ],
              ),
              if (d.powerUser)
                const Padding(
                  padding: EdgeInsets.only(top: 4),
                  child: Text(
                    '★ Power User',
                    style: TextStyle(
                      color: Color(0xFFF2A900),
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                ),
              const SizedBox(height: 14),
              Row(
                children: [
                  Expanded(
                    child: FilledButton.icon(
                      onPressed: () => _share(d),
                      style: FilledButton.styleFrom(
                        backgroundColor: KarryamColors.teal,
                        foregroundColor: KarryamColors.navy,
                        minimumSize: const Size.fromHeight(46),
                      ),
                      icon: const Icon(Icons.share_rounded, size: 18),
                      label: const Text(
                        'Invite',
                        style: TextStyle(fontWeight: FontWeight.w700),
                      ),
                    ),
                  ),
                  const SizedBox(width: 10),
                  Expanded(
                    child: OutlinedButton.icon(
                      onPressed: () => _copy(d.shareLink, 'Link'),
                      style: OutlinedButton.styleFrom(
                        foregroundColor: Colors.white,
                        side: const BorderSide(color: Color(0x66FFFFFF)),
                        minimumSize: const Size.fromHeight(46),
                      ),
                      icon: const Icon(Icons.link_rounded, size: 18),
                      label: const Text('Copy link'),
                    ),
                  ),
                ],
              ),
            ],
          ),
        ),
        const SizedBox(height: 16),
        KarryamPanel(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  Expanded(
                    child: _figure(
                      'Ready to cash out',
                      referralKobo,
                      strong: true,
                    ),
                  ),
                  Expanded(child: _figure('Earned so far', d.earnings.netKobo)),
                ],
              ),
              const SizedBox(height: 12),
              Row(
                children: [
                  Expanded(
                    child: _figure(
                      'Waiting to qualify',
                      d.earnings.pendingKobo + d.earnings.heldKobo,
                    ),
                  ),
                  Expanded(
                    child: _figure(
                      'Taken back after refunds',
                      d.earnings.takenBackKobo,
                    ),
                  ),
                ],
              ),
              if (d.promotional.netKobo > 0) ...[
                const SizedBox(height: 12),
                Text(
                  'Plus ${formatNaira(d.promotional.netKobo)} of bonus credit for rides (not withdrawable).',
                  style: TextStyle(
                    fontSize: 12.5,
                    color: colors.textMuted,
                    height: 1.4,
                  ),
                ),
              ],
              const SizedBox(height: 14),
              KarryamButton(
                label: 'Cash out',
                onPressed: canCashOut ? () => _cashOut(referralKobo) : null,
              ),
              if (!canCashOut) ...[
                const SizedBox(height: 8),
                Text(
                  'You can cash out from ${formatNaira(_minCashOutKobo)}.',
                  style: TextStyle(fontSize: 12, color: colors.textMuted),
                ),
              ],
            ],
          ),
        ),
        const KarryamSectionTitle('How it works'),
        KarryamPanel(
          child: Column(
            children: [
              _step(
                Icons.local_taxi_outlined,
                'Refer a driver: once they finish ${d.activationJobs} paid trips, you earn $pct% of Karryam\'s fee on their trips for ${d.windowDays} days.',
              ),
              const SizedBox(height: 12),
              _step(
                Icons.person_add_alt_1_outlined,
                'Refer a customer: you earn ${formatNaira(d.customerBonusKobo)} after their first paid booking.',
              ),
              const SizedBox(height: 12),
              _step(
                Icons.bolt_outlined,
                'Stay active to keep earning: rewards are paid while you are driving on Karryam yourself.',
              ),
            ],
          ),
        ),
        if (d.milestones.isNotEmpty) ...[
          const KarryamSectionTitle('Milestones'),
          KarryamPanel(child: _milestones(d)),
        ],
        KarryamSectionTitle('Your referrals (${d.referrals.length})'),
        if (d.referrals.isEmpty)
          KarryamPanel(
            child: Text(
              'No one has joined with your code yet. Share it with drivers and customers you know.',
              style: TextStyle(color: colors.textMuted, height: 1.4),
            ),
          )
        else
          for (final r in d.referrals)
            Padding(
              padding: const EdgeInsets.only(bottom: 10),
              child: _ReferralTile(entry: r),
            ),
      ],
    );
  }

  Widget _figure(String label, int kobo, {bool strong = false}) {
    final colors = KarryamColors.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(label, style: TextStyle(fontSize: 12, color: colors.textMuted)),
        const SizedBox(height: 4),
        Text(
          formatNaira(kobo),
          style: TextStyle(
            fontSize: strong ? 20 : 16,
            fontWeight: FontWeight.w800,
            color: colors.text,
          ),
        ),
      ],
    );
  }

  Widget _step(IconData icon, String text) => Row(
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      Icon(icon, size: 20, color: KarryamColors.teal),
      const SizedBox(width: 12),
      Expanded(
        child: Text(text, style: const TextStyle(height: 1.4, fontSize: 13.5)),
      ),
    ],
  );

  Widget _milestones(ReferralDashboard d) {
    final colors = KarryamColors.of(context);
    final count = d.qualifiedCustomerReferrals;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          '$count qualified customer referral${count == 1 ? '' : 's'}',
          style: const TextStyle(fontWeight: FontWeight.w700),
        ),
        for (final m in d.milestones) ...[
          const SizedBox(height: 12),
          Row(
            children: [
              Icon(
                m.achieved ? Icons.check_circle_rounded : Icons.flag_outlined,
                color: m.achieved ? KarryamColors.teal : colors.textMuted,
                size: 20,
              ),
              const SizedBox(width: 10),
              Expanded(
                child: Text(
                  '${m.referrals} referrals: ${formatNaira(m.amountKobo)}${m.badge ? ' + Power User badge' : ''}',
                  style: const TextStyle(fontSize: 13.5),
                ),
              ),
              Text(
                m.achieved
                    ? 'Reached'
                    : '${count.clamp(0, m.referrals)}/${m.referrals}',
                style: TextStyle(
                  fontSize: 12.5,
                  fontWeight: FontWeight.w700,
                  color: colors.textMuted,
                ),
              ),
            ],
          ),
          const SizedBox(height: 6),
          ClipRRect(
            borderRadius: BorderRadius.circular(4),
            child: LinearProgressIndicator(
              value: m.referrals == 0
                  ? 0
                  : (count / m.referrals).clamp(0.0, 1.0),
              minHeight: 6,
              backgroundColor: colors.border,
              color: KarryamColors.teal,
            ),
          ),
        ],
      ],
    );
  }
}

class _ReferralTile extends StatelessWidget {
  const _ReferralTile({required this.entry});
  final ReferralEntry entry;

  (String, KarryamTone) get _stage => switch (entry.stage) {
    'signed_up' => ('Signed up', KarryamTone.neutral),
    'in_progress' => ('In progress', KarryamTone.neutral),
    'earning' => ('Earning', KarryamTone.good),
    'qualified' => ('Qualified', KarryamTone.good),
    'ended' => ('Ended', KarryamTone.neutral),
    'under_review' => ('Under review', KarryamTone.warning),
    _ => ('Not eligible', KarryamTone.bad),
  };

  String get _detail {
    if (entry.isDriver) {
      return switch (entry.stage) {
        'signed_up' || 'in_progress' =>
          '${entry.jobsCounted} of ${entry.jobsRequired} paid trips done',
        'earning' when entry.earningUntil != null =>
          'Earning until ${_date(entry.earningUntil!)}',
        'ended' => 'Earning window ended',
        _ => '',
      };
    }
    return switch (entry.stage) {
      'signed_up' || 'in_progress' => 'Waiting for their first paid booking',
      'qualified' => 'Booked and qualified',
      _ => '',
    };
  }

  @override
  Widget build(BuildContext context) {
    final colors = KarryamColors.of(context);
    final (label, tone) = _stage;
    final showProgress =
        entry.isDriver &&
        entry.jobsRequired > 0 &&
        (entry.stage == 'signed_up' || entry.stage == 'in_progress');
    return KarryamPanel(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(
                entry.isDriver
                    ? Icons.local_taxi_outlined
                    : Icons.person_outline_rounded,
                color: colors.textMuted,
                size: 20,
              ),
              const SizedBox(width: 10),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      entry.isDriver ? 'Driver' : 'Customer',
                      style: const TextStyle(fontWeight: FontWeight.w700),
                    ),
                    Text(
                      'Joined ${_date(entry.joinedAt)}',
                      style: TextStyle(fontSize: 12, color: colors.textMuted),
                    ),
                  ],
                ),
              ),
              KarryamChip(label, tone: tone),
            ],
          ),
          if (_detail.isNotEmpty) ...[
            const SizedBox(height: 10),
            Text(_detail, style: const TextStyle(fontSize: 13)),
          ],
          if (showProgress) ...[
            const SizedBox(height: 8),
            ClipRRect(
              borderRadius: BorderRadius.circular(4),
              child: LinearProgressIndicator(
                value: (entry.jobsCounted / entry.jobsRequired).clamp(0.0, 1.0),
                minHeight: 6,
                backgroundColor: colors.border,
                color: KarryamColors.teal,
              ),
            ),
          ],
          if (entry.earnedKobo > 0 || entry.pendingKobo > 0) ...[
            const SizedBox(height: 10),
            Text(
              'Earned ${formatNaira(entry.earnedKobo)}'
              '${entry.pendingKobo > 0 ? '  ·  ${formatNaira(entry.pendingKobo)} waiting' : ''}',
              style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w600),
            ),
          ],
        ],
      ),
    );
  }
}

String _date(DateTime d) {
  const months = [
    'Jan',
    'Feb',
    'Mar',
    'Apr',
    'May',
    'Jun',
    'Jul',
    'Aug',
    'Sep',
    'Oct',
    'Nov',
    'Dec',
  ];
  return '${d.day} ${months[d.month - 1]} ${d.year}';
}
