import 'package:flutter/material.dart';

import '../../design_system/karryam_design_system.dart';

enum KarryamHomePreviewState {
  offline,
  blocked,
  locationPermission,
  online,
  busy,
  sessionStats,
  degradedLocation,
}

class KarryamHomePreview extends StatelessWidget {
  const KarryamHomePreview({super.key, required this.state});
  final KarryamHomePreviewState state;

  @override
  Widget build(BuildContext context) => switch (state) {
        KarryamHomePreviewState.offline => const _HomeBody(mode: _HomeMode.offline),
        KarryamHomePreviewState.blocked => const _BlockedHome(),
        KarryamHomePreviewState.locationPermission => const _LocationPermission(),
        KarryamHomePreviewState.online => const _HomeBody(mode: _HomeMode.online),
        KarryamHomePreviewState.busy => const _HomeBody(mode: _HomeMode.busy),
        KarryamHomePreviewState.sessionStats => const _SessionStats(),
        KarryamHomePreviewState.degradedLocation => const _DegradedLocation(),
      };
}

enum _HomeMode { offline, online, busy }

class _Shell extends StatelessWidget {
  const _Shell({required this.child, this.notificationDot = true});
  final Widget child;
  final bool notificationDot;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: KarryamColors.backgroundSubtle,
      body: SafeArea(
        child: Center(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 430),
            child: Column(
              children: [
                Padding(
                  padding: const EdgeInsets.fromLTRB(20, 14, 16, 10),
                  child: Row(
                    children: [
                      Container(
                        width: 42,
                        height: 42,
                        decoration: const BoxDecoration(color: KarryamColors.midnightNavy, shape: BoxShape.circle),
                        alignment: Alignment.center,
                        child: const Text('M', style: TextStyle(color: Colors.white, fontWeight: FontWeight.w900)),
                      ),
                      const SizedBox(width: 12),
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text('Good morning, Malik', style: Theme.of(context).textTheme.titleMedium),
                            Text('Dispatch provider', style: Theme.of(context).textTheme.bodySmall?.copyWith(color: KarryamColors.indigoGray)),
                          ],
                        ),
                      ),
                      Stack(
                        children: [
                          IconButton(onPressed: () {}, icon: const Icon(Icons.notifications_none_rounded)),
                          if (notificationDot)
                            const Positioned(right: 11, top: 10, child: CircleAvatar(radius: 4, backgroundColor: KarryamColors.auroraTeal)),
                        ],
                      ),
                    ],
                  ),
                ),
                Expanded(child: child),
                const _BottomNav(),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class _HomeBody extends StatelessWidget {
  const _HomeBody({required this.mode});
  final _HomeMode mode;

  @override
  Widget build(BuildContext context) {
    final online = mode != _HomeMode.offline;
    final busy = mode == _HomeMode.busy;
    return _Shell(
      child: ListView(
        padding: const EdgeInsets.fromLTRB(20, 10, 20, 24),
        children: [
          Container(
            padding: const EdgeInsets.all(20),
            decoration: BoxDecoration(
              color: KarryamColors.midnightNavy,
              borderRadius: BorderRadius.circular(KarryamRadius.xl),
              boxShadow: KarryamElevation.soft,
            ),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  mainAxisAlignment: MainAxisAlignment.spaceBetween,
                  children: [
                    KarryamStatusChip(
                      label: busy ? 'BUSY' : online ? 'ONLINE' : 'OFFLINE',
                      tone: busy ? KarryamStatusTone.warning : online ? KarryamStatusTone.active : KarryamStatusTone.neutral,
                    ),
                    Icon(online ? Icons.gps_fixed_rounded : Icons.location_off_outlined, color: KarryamColors.pureWhite),
                  ],
                ),
                const SizedBox(height: 24),
                Text(
                  busy ? 'Mission in progress' : online ? 'You’re ready for opportunities' : 'You’re currently offline',
                  style: Theme.of(context).textTheme.headlineSmall?.copyWith(color: KarryamColors.pureWhite, fontWeight: FontWeight.w900),
                ),
                const SizedBox(height: 8),
                Text(
                  busy
                      ? 'Complete your active mission before becoming available again.'
                      : online
                          ? 'Karryam is using your live location to match nearby delivery work.'
                          : 'Go online when you’re ready to receive dispatch opportunities.',
                  style: Theme.of(context).textTheme.bodyMedium?.copyWith(color: KarryamColors.pureWhite.withValues(alpha: .75)),
                ),
                const SizedBox(height: 22),
                KarryamButton(
                  label: busy ? 'Open active mission' : online ? 'Go offline' : 'Go online',
                  variant: online && !busy ? KarryamButtonVariant.secondary : KarryamButtonVariant.accent,
                  onPressed: () {},
                ),
              ],
            ),
          ),
          const SizedBox(height: 20),
          if (busy) ...[
            _ActiveMissionCard(),
            const SizedBox(height: 20),
          ],
          Text('Today', style: Theme.of(context).textTheme.titleMedium),
          const SizedBox(height: 12),
          const Row(
            children: [
              Expanded(child: _MetricCard(label: 'Trips', value: '4')),
              SizedBox(width: 10),
              Expanded(child: _MetricCard(label: 'Completed', value: '3')),
              SizedBox(width: 10),
              Expanded(child: _MetricCard(label: 'Online', value: '1h 22m')),
            ],
          ),
          const SizedBox(height: 20),
          Text('Readiness', style: Theme.of(context).textTheme.titleMedium),
          const SizedBox(height: 12),
          const _ReadinessRow(icon: Icons.verified_user_outlined, title: 'Provider verification', status: 'Verified', tone: KarryamStatusTone.success),
          const SizedBox(height: 10),
          const _ReadinessRow(icon: Icons.two_wheeler_rounded, title: 'Vehicle', status: 'Verified', tone: KarryamStatusTone.success),
          const SizedBox(height: 10),
          _ReadinessRow(icon: Icons.location_on_outlined, title: 'Location', status: online ? 'Live' : 'Paused', tone: online ? KarryamStatusTone.active : KarryamStatusTone.neutral),
        ],
      ),
    );
  }
}

class _BlockedHome extends StatelessWidget {
  const _BlockedHome();

  @override
  Widget build(BuildContext context) {
    return _Shell(
      child: ListView(
        padding: const EdgeInsets.fromLTRB(20, 10, 20, 24),
        children: [
          const KarryamNotice(
            title: 'You’re not ready to go online yet',
            message: 'Complete the required provider checks below before Karryam can start sending opportunities.',
            tone: KarryamNoticeTone.warning,
          ),
          const SizedBox(height: 20),
          const _ReadinessRow(icon: Icons.badge_outlined, title: 'Identity', status: 'Verified', tone: KarryamStatusTone.success),
          const SizedBox(height: 10),
          const _ReadinessRow(icon: Icons.credit_card_outlined, title: 'Driver licence', status: 'Expired', tone: KarryamStatusTone.danger),
          const SizedBox(height: 10),
          const _ReadinessRow(icon: Icons.two_wheeler_rounded, title: 'Vehicle', status: 'Verified', tone: KarryamStatusTone.success),
          const SizedBox(height: 24),
          KarryamButton(label: 'Renew driver licence', onPressed: () {}),
        ],
      ),
    );
  }
}

class _LocationPermission extends StatelessWidget {
  const _LocationPermission();

  @override
  Widget build(BuildContext context) {
    return _Shell(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Container(
              width: 90,
              height: 90,
              decoration: const BoxDecoration(color: KarryamColors.teal10, shape: BoxShape.circle),
              alignment: Alignment.center,
              child: const Icon(Icons.my_location_rounded, size: 42, color: KarryamColors.auroraTeal),
            ),
            const SizedBox(height: 24),
            Text('Allow precise location', style: Theme.of(context).textTheme.headlineSmall, textAlign: TextAlign.center),
            const SizedBox(height: 10),
            Text(
              'Karryam needs precise location while you’re online so nearby opportunities, route progress and safety checks work correctly.',
              textAlign: TextAlign.center,
              style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: KarryamColors.indigoGray),
            ),
            const SizedBox(height: 24),
            KarryamButton(label: 'Allow precise location', onPressed: () {}),
            const SizedBox(height: 10),
            KarryamButton(label: 'Not now', variant: KarryamButtonVariant.secondary, onPressed: () {}),
          ],
        ),
      ),
    );
  }
}

class _SessionStats extends StatelessWidget {
  const _SessionStats();

  @override
  Widget build(BuildContext context) {
    return _Shell(
      child: ListView(
        padding: const EdgeInsets.fromLTRB(20, 10, 20, 24),
        children: [
          Text('Today’s session', style: Theme.of(context).textTheme.headlineSmall),
          const SizedBox(height: 6),
          Text('A live operational summary for this online session.', style: Theme.of(context).textTheme.bodyMedium?.copyWith(color: KarryamColors.indigoGray)),
          const SizedBox(height: 20),
          const _LargeMetric(icon: Icons.schedule_rounded, label: 'Time online', value: '1h 22m'),
          const SizedBox(height: 12),
          const _LargeMetric(icon: Icons.route_rounded, label: 'Trips accepted', value: '4'),
          const SizedBox(height: 12),
          const _LargeMetric(icon: Icons.check_circle_outline_rounded, label: 'Completed', value: '3'),
          const SizedBox(height: 12),
          const _LargeMetric(icon: Icons.cancel_outlined, label: 'Cancelled', value: '0'),
        ],
      ),
    );
  }
}

class _DegradedLocation extends StatelessWidget {
  const _DegradedLocation();

  @override
  Widget build(BuildContext context) {
    return _Shell(
      child: ListView(
        padding: const EdgeInsets.fromLTRB(20, 10, 20, 24),
        children: [
          const KarryamNotice(
            title: 'Location accuracy is degraded',
            message: 'Your last precise location update is stale. Keep GPS on and move to an area with a clearer signal.',
            tone: KarryamNoticeTone.warning,
          ),
          const SizedBox(height: 20),
          Container(
            height: 230,
            decoration: BoxDecoration(color: KarryamColors.navy10, borderRadius: BorderRadius.circular(KarryamRadius.xl)),
            alignment: Alignment.center,
            child: const Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(Icons.location_searching_rounded, size: 50, color: KarryamColors.midnightNavy),
                SizedBox(height: 10),
                Text('Trying to restore precise location'),
              ],
            ),
          ),
          const SizedBox(height: 20),
          KarryamButton(label: 'Open location settings', onPressed: () {}),
          const SizedBox(height: 10),
          KarryamButton(label: 'Go offline', variant: KarryamButtonVariant.secondary, onPressed: () {}),
        ],
      ),
    );
  }
}

class _ActiveMissionCard extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(color: KarryamColors.pureWhite, borderRadius: BorderRadius.circular(KarryamRadius.lg), border: Border.all(color: KarryamColors.border)),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Row(mainAxisAlignment: MainAxisAlignment.spaceBetween, children: [Text('ACTIVE MISSION', style: TextStyle(fontSize: 11, fontWeight: FontWeight.w800, letterSpacing: 1.1, color: KarryamColors.auroraTeal)), KarryamStatusChip(label: 'IN PROGRESS', tone: KarryamStatusTone.active)]),
          const SizedBox(height: 12),
          Text('Ikeja GRA → Victoria Island', style: Theme.of(context).textTheme.titleMedium),
          const SizedBox(height: 6),
          Text('2 of 4 parcels verified • 18 min remaining', style: Theme.of(context).textTheme.bodySmall?.copyWith(color: KarryamColors.indigoGray)),
        ],
      ),
    );
  }
}

class _MetricCard extends StatelessWidget {
  const _MetricCard({required this.label, required this.value});
  final String label;
  final String value;
  @override
  Widget build(BuildContext context) => Container(
        padding: const EdgeInsets.symmetric(vertical: 16, horizontal: 12),
        decoration: BoxDecoration(color: KarryamColors.pureWhite, borderRadius: BorderRadius.circular(KarryamRadius.lg), border: Border.all(color: KarryamColors.border)),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [Text(value, style: Theme.of(context).textTheme.titleLarge), const SizedBox(height: 4), Text(label, style: Theme.of(context).textTheme.bodySmall?.copyWith(color: KarryamColors.indigoGray))]),
      );
}

class _LargeMetric extends StatelessWidget {
  const _LargeMetric({required this.icon, required this.label, required this.value});
  final IconData icon;
  final String label;
  final String value;
  @override
  Widget build(BuildContext context) => Container(
        padding: const EdgeInsets.all(18),
        decoration: BoxDecoration(color: KarryamColors.pureWhite, borderRadius: BorderRadius.circular(KarryamRadius.lg), border: Border.all(color: KarryamColors.border)),
        child: Row(children: [Icon(icon, color: KarryamColors.midnightNavy), const SizedBox(width: 14), Expanded(child: Text(label, style: Theme.of(context).textTheme.titleSmall)), Text(value, style: Theme.of(context).textTheme.titleLarge)]),
      );
}

class _ReadinessRow extends StatelessWidget {
  const _ReadinessRow({required this.icon, required this.title, required this.status, required this.tone});
  final IconData icon;
  final String title;
  final String status;
  final KarryamStatusTone tone;
  @override
  Widget build(BuildContext context) => Container(
        padding: const EdgeInsets.all(15),
        decoration: BoxDecoration(color: KarryamColors.pureWhite, borderRadius: BorderRadius.circular(KarryamRadius.lg), border: Border.all(color: KarryamColors.border)),
        child: Row(children: [Icon(icon, color: KarryamColors.midnightNavy), const SizedBox(width: 12), Expanded(child: Text(title, style: Theme.of(context).textTheme.titleSmall)), KarryamStatusChip(label: status, tone: tone)]),
      );
}

class _BottomNav extends StatelessWidget {
  const _BottomNav();
  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.fromLTRB(12, 8, 12, 10),
      decoration: const BoxDecoration(color: KarryamColors.pureWhite, border: Border(top: BorderSide(color: KarryamColors.border))),
      child: const Row(mainAxisAlignment: MainAxisAlignment.spaceAround, children: [
        _NavItem(icon: Icons.home_rounded, label: 'Home', active: true),
        _NavItem(icon: Icons.bolt_rounded, label: 'Opportunities'),
        _NavItem(icon: Icons.route_rounded, label: 'Activity'),
        _NavItem(icon: Icons.account_balance_wallet_outlined, label: 'Wallet'),
        _NavItem(icon: Icons.person_outline_rounded, label: 'Account'),
      ]),
    );
  }
}

class _NavItem extends StatelessWidget {
  const _NavItem({required this.icon, required this.label, this.active = false});
  final IconData icon;
  final String label;
  final bool active;
  @override
  Widget build(BuildContext context) => SizedBox(
        width: 66,
        child: Column(mainAxisSize: MainAxisSize.min, children: [Icon(icon, size: 22, color: active ? KarryamColors.auroraTeal : KarryamColors.forgeGray), const SizedBox(height: 3), Text(label, style: TextStyle(fontSize: 10, fontWeight: active ? FontWeight.w800 : FontWeight.w600, color: active ? KarryamColors.midnightNavy : KarryamColors.forgeGray))]),
      );
}
