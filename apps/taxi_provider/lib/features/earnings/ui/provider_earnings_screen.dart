import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:cosmicforge_logistics_ui_kit/cosmicforge_logistics_ui_kit.dart';
import 'package:flutter/material.dart';

import '../../../core/format/money_format.dart';
import '../../auth/state/provider_auth_controller.dart';
import '../../home/data/provider_api.dart';
import '../models/earnings_insights.dart';

/// The driver's report for a chosen window.
class ProviderEarningsScreen extends StatefulWidget {
  const ProviderEarningsScreen({
    super.key,
    required this.api,
    required this.authController,
  });

  final ProviderApi api;
  final ProviderAuthController authController;

  @override
  State<ProviderEarningsScreen> createState() => _ProviderEarningsScreenState();
}

class _ProviderEarningsScreenState extends State<ProviderEarningsScreen> {
  EarningsInsights? _insights;
  bool _loading = true;
  String? _error;
  int _days = 7;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    final token = widget.authController.state.accessToken;
    if (token == null) return;
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final now = DateTime.now();
      final data = await widget.api.getEarningsInsights(
        accessToken: token,
        from: now.subtract(Duration(days: _days)),
        until: now,
      );
      if (!mounted) return;
      setState(() {
        _insights = EarningsInsights.fromJson(data);
        _loading = false;
      });
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() {
        _error = e.message;
        _loading = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: CosmicforgeLogisticsColors.surface,
      body: SafeArea(
        child: RefreshIndicator(
          color: CosmicforgeLogisticsColors.primary,
          onRefresh: _load,
          child: ListView(
            padding: const EdgeInsets.fromLTRB(20, 16, 20, 32),
            children: [
              const Text(
                'Earnings',
                style: TextStyle(
                  fontSize: 22,
                  fontWeight: FontWeight.w800,
                  color: CosmicforgeLogisticsColors.text,
                ),
              ),
              const SizedBox(height: 14),
              Row(
                children: [
                  for (final option in const [1, 7, 30]) ...[
                    _RangeChip(
                      label: option == 1 ? 'Today' : 'Last $option days',
                      selected: _days == option,
                      onTap: () {
                        setState(() => _days = option);
                        _load();
                      },
                    ),
                    if (option != 30) const SizedBox(width: 8),
                  ],
                ],
              ),
              const SizedBox(height: 18),

              if (_loading)
                const Padding(
                  padding: EdgeInsets.symmetric(vertical: 60),
                  child: Center(child: CircularProgressIndicator()),
                )
              else if (_error != null)
                Text(
                  _error!,
                  style: const TextStyle(
                    color: CosmicforgeLogisticsColors.danger,
                    fontSize: 13,
                  ),
                )
              else if (_insights != null) ...[
                _GrossCard(insights: _insights!),
                const SizedBox(height: 14),
                _StatGrid(insights: _insights!),
              ],
            ],
          ),
        ),
      ),
    );
  }
}

class _RangeChip extends StatelessWidget {
  const _RangeChip({
    required this.label,
    required this.selected,
    required this.onTap,
  });

  final String label;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return InkWell(
      onTap: onTap,
      borderRadius: BorderRadius.circular(20),
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 8),
        decoration: BoxDecoration(
          color: selected
              ? CosmicforgeLogisticsColors.primary
              : CosmicforgeLogisticsColors.card,
          borderRadius: BorderRadius.circular(20),
          border: Border.all(color: CosmicforgeLogisticsColors.border),
        ),
        child: Text(
          label,
          style: TextStyle(
            color: selected ? Colors.white : CosmicforgeLogisticsColors.text,
            fontSize: 12.5,
            fontWeight: FontWeight.w700,
          ),
        ),
      ),
    );
  }
}

class _GrossCard extends StatelessWidget {
  const _GrossCard({required this.insights});

  final EarningsInsights insights;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.all(20),
      decoration: BoxDecoration(
        gradient: CosmicforgeLogisticsColors.radialBlue,
        borderRadius: BorderRadius.circular(18),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            'Gross earnings',
            style: TextStyle(
              color: Colors.white.withValues(alpha: 0.85),
              fontSize: 13,
            ),
          ),
          const SizedBox(height: 6),
          Text(
            formatNaira(insights.grossEarningsKobo),
            style: const TextStyle(
              color: Colors.white,
              fontSize: 30,
              fontWeight: FontWeight.w900,
            ),
          ),
          const SizedBox(height: 8),
          Text(
            '${insights.tripsCompleted} completed '
            '${insights.tripsCompleted == 1 ? 'trip' : 'trips'}',
            style: TextStyle(
              color: Colors.white.withValues(alpha: 0.85),
              fontSize: 12.5,
            ),
          ),
          if (!insights.isLedger) ...[
            const SizedBox(height: 10),
            Text(
              'Trip totals, not a withdrawable balance.',
              style: TextStyle(
                color: Colors.white.withValues(alpha: 0.7),
                fontSize: 11.5,
              ),
            ),
          ],
        ],
      ),
    );
  }
}

class _StatGrid extends StatelessWidget {
  const _StatGrid({required this.insights});

  final EarningsInsights insights;

  @override
  Widget build(BuildContext context) {
    final tiles = <(String, String)>[
      ('Online', '${insights.onlineHours.toStringAsFixed(1)} h'),
      if (insights.utilizationPercent != null)
        ('Utilisation', '${insights.utilizationPercent!.toStringAsFixed(0)}%'),
      if (insights.earningsPerOnlineHourKobo != null)
        (
          'Per online hour',
          formatNaira(insights.earningsPerOnlineHourKobo!.round()),
        ),
      if (insights.averageTripDurationMinutes != null)
        (
          'Avg trip',
          '${insights.averageTripDurationMinutes!.toStringAsFixed(0)} min',
        ),
      ('Customer cancels', '${insights.cancelledByCustomer}'),
      ('Your cancels', '${insights.cancelledByProvider}'),
    ];

    return Wrap(
      spacing: 12,
      runSpacing: 12,
      children: [
        for (final tile in tiles)
          SizedBox(
            width: (MediaQuery.sizeOf(context).width - 52) / 2,
            child: Container(
              padding: const EdgeInsets.all(14),
              decoration: BoxDecoration(
                color: CosmicforgeLogisticsColors.card,
                borderRadius: BorderRadius.circular(14),
                border: Border.all(color: CosmicforgeLogisticsColors.border),
              ),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    tile.$1,
                    style: const TextStyle(
                      color: CosmicforgeLogisticsColors.textMuted,
                      fontSize: 12,
                    ),
                  ),
                  const SizedBox(height: 6),
                  Text(
                    tile.$2,
                    style: const TextStyle(
                      color: CosmicforgeLogisticsColors.text,
                      fontSize: 17,
                      fontWeight: FontWeight.w800,
                    ),
                  ),
                ],
              ),
            ),
          ),
      ],
    );
  }
}
