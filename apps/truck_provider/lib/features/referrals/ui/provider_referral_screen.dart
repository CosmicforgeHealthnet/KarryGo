import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../../core/format/money_format.dart';
import '../../../core/widgets/karryam_ui.dart';
import '../../home/ui/widgets/provider_app_colors.dart';
import '../../wallet/state/provider_withdrawal_controller.dart';
import '../../wallet/ui/provider_withdrawal_form_screen.dart';
import '../models/referral_dashboard.dart';
import '../state/referral_controller.dart';

/// The minimum referral cash-out, matching payment-wallet's default.
const _minCashOutKobo = 500000;

/// The driver's referral dashboard: their code, what they have earned, each
/// person they referred and how far that person has got.
class ProviderReferralScreen extends StatefulWidget {
  const ProviderReferralScreen({
    super.key,
    required this.controller,
    required this.withdrawalController,
  });

  final ProviderReferralController controller;
  final ProviderWithdrawalController withdrawalController;

  @override
  State<ProviderReferralScreen> createState() => _ProviderReferralScreenState();
}

class _ProviderReferralScreenState extends State<ProviderReferralScreen> {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      widget.controller.load();
      widget.withdrawalController.loadBalanceAndAccounts();
    });
  }

  Future<void> _copy(String text, String what) async {
    await Clipboard.setData(ClipboardData(text: text));
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('$what copied')));
  }

  Future<void> _share(ReferralDashboard d) async {
    final message = 'Join me on Karryam. Sign up with my code ${d.code}: ${d.shareLink}';
    final uri = Uri.parse('https://wa.me/?text=${Uri.encodeComponent(message)}');
    final opened = await launchUrl(uri, mode: LaunchMode.externalApplication).catchError((_) => false);
    if (!opened) await _copy(message, 'Invite message');
  }

  Future<void> _cashOut() async {
    await Navigator.of(context).push(MaterialPageRoute<void>(
      builder: (_) => ProviderWithdrawalFormScreen(controller: widget.withdrawalController, referral: true),
    ));
    if (!mounted) return;
    widget.controller.load();
    widget.withdrawalController.loadBalanceAndAccounts();
  }

  Future<void> _refresh() async {
    await Future.wait([widget.controller.load(), widget.withdrawalController.loadBalanceAndAccounts()]);
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: KTopBar(title: 'Refer & earn'),
      body: AnimatedBuilder(
        animation: Listenable.merge([widget.controller, widget.withdrawalController]),
        builder: (context, _) {
          final c = widget.controller;
          final d = c.dashboard;
          if (d == null) {
            if (c.error != null) {
              return KEmptyState(
                icon: Icons.group_add_outlined,
                art: KArt.cloud,
                title: "Couldn't load your referrals",
                message: c.error,
                actionLabel: 'Try again',
                onAction: c.load,
              );
            }
            return const Center(child: CircularProgressIndicator());
          }
          final walletKobo = widget.withdrawalController.balance.referralKobo;
          return RefreshIndicator(
            onRefresh: _refresh,
            child: ListView(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 32),
              children: [
                if (d.frozen || d.blocked) ...[
                  KNotice(
                    d.blocked
                        ? 'Your referral account is closed. Contact support if you think this is a mistake.'
                        : 'Your referrals are under review. Rewards are held until the review is done.',
                    tone: KTone.warning,
                  ),
                  const SizedBox(height: 14),
                ],
                _CodeCard(
                  dashboard: d,
                  onCopyCode: () => _copy(d.code, 'Code'),
                  onCopyLink: () => _copy(d.shareLink, 'Link'),
                  onShare: () => _share(d),
                ),
                const SizedBox(height: 16),
                _EarningsCard(
                  dashboard: d,
                  walletKobo: walletKobo,
                  onCashOut: walletKobo >= _minCashOutKobo ? _cashOut : null,
                ),
                const SizedBox(height: 20),
                KEyebrow('How it works'),
                const SizedBox(height: 10),
                _HowItWorks(dashboard: d),
                const SizedBox(height: 20),
                if (d.milestones.isNotEmpty) ...[
                  KEyebrow('Milestones'),
                  const SizedBox(height: 10),
                  _Milestones(dashboard: d),
                  const SizedBox(height: 20),
                ],
                KEyebrow('Your referrals (${d.referrals.length})'),
                const SizedBox(height: 10),
                if (d.referrals.isEmpty)
                  KCard(
                    child: Text(
                      'No one has joined with your code yet. Share it with drivers and customers you know.',
                      style: TextStyle(color: kProviderMuted, height: 1.4),
                    ),
                  )
                else
                  for (final r in d.referrals) ...[
                    _ReferralTile(entry: r, dashboard: d),
                    const SizedBox(height: 10),
                  ],
              ],
            ),
          );
        },
      ),
    );
  }
}

class _CodeCard extends StatelessWidget {
  const _CodeCard({required this.dashboard, required this.onCopyCode, required this.onCopyLink, required this.onShare});
  final ReferralDashboard dashboard;
  final VoidCallback onCopyCode;
  final VoidCallback onCopyLink;
  final VoidCallback onShare;

  @override
  Widget build(BuildContext context) {
    return KMoneyCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text('Your referral code', style: TextStyle(color: Color(0xCCFFFFFF), fontSize: 13)),
          const SizedBox(height: 6),
          Row(
            children: [
              Expanded(
                child: Text(
                  dashboard.code,
                  style: const TextStyle(color: Colors.white, fontSize: 28, fontWeight: FontWeight.w800, letterSpacing: 2),
                ),
              ),
              IconButton(
                tooltip: 'Copy code',
                onPressed: onCopyCode,
                icon: const Icon(Icons.copy_rounded, color: Colors.white),
              ),
            ],
          ),
          if (dashboard.powerUser) ...[
            const SizedBox(height: 4),
            const Row(
              children: [
                Icon(Icons.workspace_premium_rounded, color: kAmber, size: 18),
                SizedBox(width: 6),
                Text('Power User', style: TextStyle(color: kAmber, fontWeight: FontWeight.w700)),
              ],
            ),
          ],
          const SizedBox(height: 14),
          Row(
            children: [
              Expanded(
                child: FilledButton.icon(
                  onPressed: onShare,
                  style: FilledButton.styleFrom(backgroundColor: kTeal, foregroundColor: kNavy, minimumSize: const Size.fromHeight(46)),
                  icon: const Icon(Icons.share_rounded, size: 18),
                  label: const Text('Invite', style: TextStyle(fontWeight: FontWeight.w700)),
                ),
              ),
              const SizedBox(width: 10),
              Expanded(
                child: OutlinedButton.icon(
                  onPressed: onCopyLink,
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
    );
  }
}

class _EarningsCard extends StatelessWidget {
  const _EarningsCard({required this.dashboard, required this.walletKobo, required this.onCashOut});
  final ReferralDashboard dashboard;
  final int walletKobo;
  final VoidCallback? onCashOut;

  @override
  Widget build(BuildContext context) {
    final e = dashboard.earnings;
    return KCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Expanded(child: _Figure(label: 'Ready to cash out', kobo: walletKobo, strong: true)),
              Expanded(child: _Figure(label: 'Earned so far', kobo: e.netKobo)),
            ],
          ),
          const SizedBox(height: 12),
          Row(
            children: [
              Expanded(child: _Figure(label: 'Waiting to qualify', kobo: e.pendingKobo + e.heldKobo)),
              Expanded(child: _Figure(label: 'Taken back after refunds', kobo: e.takenBackKobo)),
            ],
          ),
          if (dashboard.promotional.netKobo > 0) ...[
            const SizedBox(height: 12),
            Text(
              'Plus ${formatNairaShort(dashboard.promotional.netKobo / 100)} of bonus credit for bookings (not withdrawable).',
              style: TextStyle(fontSize: 12.5, color: kProviderMuted, height: 1.4),
            ),
          ],
          const SizedBox(height: 14),
          KPrimaryButton(label: 'Cash out', onPressed: onCashOut, icon: Icons.account_balance_outlined),
          if (onCashOut == null) ...[
            const SizedBox(height: 8),
            Text(
              'You can cash out from ${formatNairaShort(_minCashOutKobo / 100)}.',
              style: TextStyle(fontSize: 12, color: kProviderMuted),
            ),
          ],
        ],
      ),
    );
  }
}

class _Figure extends StatelessWidget {
  const _Figure({required this.label, required this.kobo, this.strong = false});
  final String label;
  final int kobo;
  final bool strong;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(label, style: TextStyle(fontSize: 12, color: kProviderMuted)),
        const SizedBox(height: 4),
        Text(
          formatNairaShort(kobo / 100),
          style: TextStyle(
            fontSize: strong ? 20 : 16,
            fontWeight: FontWeight.w800,
            color: strong ? kTealText : kProviderText,
          ),
        ),
      ],
    );
  }
}

class _HowItWorks extends StatelessWidget {
  const _HowItWorks({required this.dashboard});
  final ReferralDashboard dashboard;

  @override
  Widget build(BuildContext context) {
    final percent = dashboard.commissionBps / 100;
    final pct = percent == percent.roundToDouble() ? percent.toStringAsFixed(0) : percent.toStringAsFixed(1);
    final steps = <(IconData, String)>[
      (Icons.local_shipping_outlined,
          'Refer a driver: once they finish ${dashboard.activationJobs} paid jobs, you earn $pct% of Karryam\'s fee on their jobs for ${dashboard.windowDays} days.'),
      (Icons.person_add_alt_1_outlined,
          'Refer a customer: you earn ${formatNairaShort(dashboard.customerBonusKobo / 100)} after their first paid booking.'),
      (Icons.bolt_outlined, 'Stay active to keep earning: rewards are paid while you are working on Karryam yourself.'),
    ];
    return KCard(
      child: Column(
        children: [
          for (var i = 0; i < steps.length; i++) ...[
            if (i > 0) const SizedBox(height: 12),
            Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Icon(steps[i].$1, size: 20, color: kTealText),
                const SizedBox(width: 12),
                Expanded(child: Text(steps[i].$2, style: TextStyle(color: kProviderText, height: 1.4, fontSize: 13.5))),
              ],
            ),
          ],
        ],
      ),
    );
  }
}

class _Milestones extends StatelessWidget {
  const _Milestones({required this.dashboard});
  final ReferralDashboard dashboard;

  @override
  Widget build(BuildContext context) {
    final count = dashboard.qualifiedCustomerReferrals;
    return KCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            '$count qualified customer referral${count == 1 ? '' : 's'}',
            style: TextStyle(fontWeight: FontWeight.w700, color: kProviderText),
          ),
          for (final m in dashboard.milestones) ...[
            const SizedBox(height: 12),
            Row(
              children: [
                Icon(
                  m.achieved ? Icons.check_circle_rounded : Icons.flag_outlined,
                  color: m.achieved ? kTealText : kProviderMuted,
                  size: 20,
                ),
                const SizedBox(width: 10),
                Expanded(
                  child: Text(
                    '${m.referrals} referrals: ${formatNairaShort(m.amountKobo / 100)}${m.badge ? ' + Power User badge' : ''}',
                    style: TextStyle(color: kProviderText, fontSize: 13.5),
                  ),
                ),
                Text(
                  m.achieved ? 'Reached' : '${count.clamp(0, m.referrals)}/${m.referrals}',
                  style: TextStyle(fontSize: 12.5, fontWeight: FontWeight.w700, color: m.achieved ? kTealText : kProviderMuted),
                ),
              ],
            ),
            const SizedBox(height: 6),
            ClipRRect(
              borderRadius: BorderRadius.circular(4),
              child: LinearProgressIndicator(
                value: m.referrals == 0 ? 0 : (count / m.referrals).clamp(0.0, 1.0),
                minHeight: 6,
                backgroundColor: kProviderBorder,
                color: kTeal,
              ),
            ),
          ],
        ],
      ),
    );
  }
}

class _ReferralTile extends StatelessWidget {
  const _ReferralTile({required this.entry, required this.dashboard});
  final ReferralEntry entry;
  final ReferralDashboard dashboard;

  (String, KTone) get _stage => switch (entry.stage) {
        'signed_up' => ('Signed up', KTone.neutral),
        'in_progress' => ('In progress', KTone.info),
        'earning' => ('Earning', KTone.success),
        'qualified' => ('Qualified', KTone.success),
        'ended' => ('Ended', KTone.neutral),
        'under_review' => ('Under review', KTone.warning),
        _ => ('Not eligible', KTone.danger),
      };

  String get _detail {
    if (entry.isDriver) {
      return switch (entry.stage) {
        'signed_up' || 'in_progress' => '${entry.jobsCounted} of ${entry.jobsRequired} paid jobs done',
        'earning' when entry.earningUntil != null => 'Earning until ${_date(entry.earningUntil!)}',
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
    final (label, tone) = _stage;
    final showProgress = entry.isDriver && entry.jobsRequired > 0 && (entry.stage == 'signed_up' || entry.stage == 'in_progress');
    return KCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(entry.isDriver ? Icons.local_shipping_outlined : Icons.person_outline_rounded, color: kProviderMuted, size: 20),
              const SizedBox(width: 10),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      entry.isDriver ? 'Driver' : 'Customer',
                      style: TextStyle(fontWeight: FontWeight.w700, color: kProviderText),
                    ),
                    Text('Joined ${_date(entry.joinedAt)}', style: TextStyle(fontSize: 12, color: kProviderMuted)),
                  ],
                ),
              ),
              KStatusChip(label, tone: tone),
            ],
          ),
          if (_detail.isNotEmpty) ...[
            const SizedBox(height: 10),
            Text(_detail, style: TextStyle(fontSize: 13, color: kProviderText)),
          ],
          if (showProgress) ...[
            const SizedBox(height: 8),
            ClipRRect(
              borderRadius: BorderRadius.circular(4),
              child: LinearProgressIndicator(
                value: (entry.jobsCounted / entry.jobsRequired).clamp(0.0, 1.0),
                minHeight: 6,
                backgroundColor: kProviderBorder,
                color: kTeal,
              ),
            ),
          ],
          if (entry.earnedKobo > 0 || entry.pendingKobo > 0) ...[
            const SizedBox(height: 10),
            Row(
              children: [
                Text('Earned ${formatNairaShort(entry.earnedKobo / 100)}',
                    style: TextStyle(fontSize: 13, fontWeight: FontWeight.w700, color: kTealText)),
                if (entry.pendingKobo > 0) ...[
                  const SizedBox(width: 12),
                  Text('${formatNairaShort(entry.pendingKobo / 100)} waiting',
                      style: TextStyle(fontSize: 13, color: kProviderMuted)),
                ],
              ],
            ),
          ],
        ],
      ),
    );
  }
}

String _date(DateTime d) {
  const months = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
  return '${d.day} ${months[d.month - 1]} ${d.year}';
}
