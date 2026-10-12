import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/ui/premium_loading.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../dashboard/ui/dashboard_empty_state.dart';
import '../../settings/ui/settings_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../../onboarding/ui/setup_design.dart';
import '../state/delivery_policy_controller.dart';
import '../state/delivery_policy_rules.dart';
import 'delivery_policy_advanced_screen.dart';

class DeliveryPolicyScreen extends StatefulWidget {
  const DeliveryPolicyScreen({
    super.key,
    required this.runtime,
    required this.branch,
  });
  final VendorRuntime runtime;
  final JsonObject branch;
  @override
  State<DeliveryPolicyScreen> createState() => _DeliveryPolicyScreenState();
}

class _DeliveryPolicyScreenState extends State<DeliveryPolicyScreen> {
  late final c = DeliveryPolicyController(widget.runtime, widget.branch);
  final currency = TextEditingController(), reason = TextEditingController();
  final fields = {
    for (final key in [
      'base_fee_minor',
      'per_km_fee_minor',
      'minimum_order_minor',
      'free_delivery_threshold_minor',
      'max_delivery_radius_meters',
      'max_package_weight_grams',
      'max_package_count',
      'max_volume_ml',
      'estimated_minutes_base',
      'estimated_minutes_per_km',
    ])
      key: TextEditingController(),
  };
  bool enabled = false, filled = false;
  JsonObject advanced = {};
  String mode = 'VENDOR', fee = 'DISTANCE_TIER';
  @override
  void initState() {
    super.initState();
    c.load();
  }

  @override
  void dispose() {
    currency.dispose();
    reason.dispose();
    for (final t in fields.values) {
      t.dispose();
    }
    c.dispose();
    super.dispose();
  }

  void fill() {
    if (filled || c.loading || c.stale) return;
    filled = true;
    final record = c.record ?? {};
    currency.text =
        record['currency']?.toString() ??
        c.business?['currency']?.toString() ??
        '';
    enabled = record['enabled'] == true;
    mode = record['delivery_mode']?.toString() ?? 'VENDOR';
    fee = record['fee_rule']?.toString() ?? 'DISTANCE_TIER';
    advanced = {
      'operating_timezone':
          record['operating_timezone'] ?? c.business?['timezone'] ?? 'UTC',
      for (final key in [
        'service_zones',
        'distance_tiers',
        'operating_periods',
        'supported_vehicle_classes',
        'fallback_triggers',
      ])
        key: record[key] ?? [],
      'rider_availability': record['rider_availability'] ?? 'UNKNOWN',
      'hybrid_fallback_funding_sponsor':
          record['hybrid_fallback_funding_sponsor'] ?? 'VENDOR',
      'hybrid_vendor_share_bps': record['hybrid_vendor_share_bps'] ?? 10000,
      'hybrid_platform_approval_ref':
          record['hybrid_platform_approval_ref'] ?? '',
    };
    for (final e in fields.entries) {
      final value = record[e.key];
      e.value.text = value == null
          ? (widget.runtime.mode == VendorDataMode.mock &&
                    (e.key == 'base_fee_minor' ||
                        e.key == 'minimum_order_minor' ||
                        e.key == 'per_km_fee_minor')
                ? '0.00'
                : '')
          : e.key.endsWith('_minor')
          ? ((value as num) / 100).toStringAsFixed(2)
          : value.toString();
    }
  }

  int? money(String value) {
    if (!RegExp(r'^\d+(?:\.\d{1,2})?$').hasMatch(value)) return null;
    final pieces = value.split('.');
    final whole = int.tryParse(pieces[0]);
    if (whole == null) return null;
    return whole * 100 +
        (pieces.length == 1 ? 0 : int.parse(pieces[1].padRight(2, '0')));
  }

  int? number(String key) => key.endsWith('_minor')
      ? money(fields[key]!.text)
      : int.tryParse(fields[key]!.text);
  bool get valid =>
      RegExp(r'^[A-Z]{3}$').hasMatch(currency.text.trim().toUpperCase()) &&
      [
        'base_fee_minor',
        'per_km_fee_minor',
        'minimum_order_minor',
      ].every((key) => number(key) != null) &&
      fields.entries.every(
        (e) => e.value.text.isEmpty || number(e.key) != null,
      ) &&
      validateDeliveryPolicy(draft()) == null;
  JsonObject draft() => {
    ...advanced,
    'enabled': enabled,
    'delivery_mode': mode,
    'fee_rule': fee,
    'currency': currency.text.trim().toUpperCase(),
    'reason': reason.text.trim(),
    for (final e in fields.entries)
      e.key: e.value.text.isNotEmpty
          ? number(e.key)
          : e.key.startsWith('estimated_minutes_')
          ? 0
          : null,
  };
  Future<void> editAdvanced() async {
    final value = await Navigator.push<JsonObject>(
      context,
      setupRoute<JsonObject>(
        DeliveryPolicyAdvancedScreen(
          values: advanced,
          currency: currency.text.trim().toUpperCase(),
          hybrid: mode == 'HYBRID',
          mock: widget.runtime.mode == VendorDataMode.mock,
          readOnly: !c.canWrite,
        ),
      ),
    );
    if (value != null && mounted) setState(() => advanced = value);
  }

  Future<void> save() async {
    if (c.attempt.pending == null) {
      final confirm = await confirmSettings(
        context,
        'Save delivery policy',
        '${dashboardLabel(mode)} delivery · ${dashboardLabel(fee)} pricing. '
            '${(advanced['service_zones'] as List?)?.length ?? 0} zones, '
            '${(advanced['distance_tiers'] as List?)?.length ?? 0} distance tiers and '
            '${(advanced['operating_periods'] as List?)?.length ?? 0} delivery windows. '
            'These settings determine delivery eligibility and charges.',
      );
      if (!confirm || !mounted) return;
    }
    final ok = await c.save(draft());
    if (!mounted) return;
    if (ok) {
      setState(() {
        filled = false;
        fill();
      });
    }
    await showOrderFeedback(
      context,
      title: ok ? 'Policy saved' : 'Policy needs attention',
      message: ok
          ? 'The branch delivery policy has been updated.'
          : c.error ?? 'Refresh and try again.',
      success: ok,
    );
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) {
      fill();
      return PopScope(
        canPop: !c.busy,
        child: Scaffold(
          appBar: AppBar(
            title: const Text('Delivery policy'),
            actions: [
              IconButton(
                tooltip: 'Refresh policy',
                onPressed: c.loading || c.busy
                    ? null
                    : () async {
                        filled = false;
                        await c.load();
                      },
                icon: const Icon(Icons.refresh_rounded),
              ),
            ],
          ),
          body: ListView(
            padding: const EdgeInsets.all(20),
            children: [
              if (c.loading)
                const PremiumSkeleton(detail: true)
              else if (c.stale && c.attempt.pending == null)
                DashboardEmptyState(
                  story: EmptyStory.unavailable,
                  title: 'Delivery policy unavailable',
                  message: c.error ?? 'Refresh to load the current policy.',
                  action: 'Refresh policy',
                  onAction: () async {
                    filled = false;
                    await c.load();
                  },
                )
              else ...[
                SettingsHeading(
                  (widget.branch['display_name'] ??
                          widget.branch['name'] ??
                          'Branch delivery')
                      .toString(),
                ),
                const SizedBox(height: 16),
                if (c.error != null) ...[
                  Text(c.error!),
                  const SizedBox(height: 16),
                ],
                SettingsCard(
                  child: Column(
                    children: [
                      SwitchListTile(
                        contentPadding: EdgeInsets.zero,
                        title: const Text('Enable delivery policy'),
                        value: enabled,
                        onChanged: c.canWrite
                            ? (v) => setState(() => enabled = v)
                            : null,
                      ),
                      const SizedBox(height: 12),
                      DropdownButtonFormField<String>(
                        initialValue: mode,
                        isExpanded: true,
                        decoration: const InputDecoration(
                          labelText: 'Delivery mode',
                        ),
                        items: [
                          for (final m in ['PLATFORM', 'VENDOR', 'HYBRID'])
                            DropdownMenuItem(
                              value: m,
                              child: Text(dashboardLabel(m)),
                            ),
                        ],
                        onChanged: c.canWrite
                            ? (v) => setState(() => mode = v!)
                            : null,
                      ),
                    ],
                  ),
                ),
                const SizedBox(height: 24),
                const SettingsHeading('Delivery charges'),
                const SizedBox(height: 16),
                TextField(
                  controller: currency,
                  maxLength: 3,
                  textCapitalization: TextCapitalization.characters,
                  readOnly: !c.canWrite,
                  decoration: const InputDecoration(labelText: 'Currency'),
                  onChanged: (_) => setState(() {}),
                ),
                const SizedBox(height: 16),
                DropdownButtonFormField<String>(
                  initialValue: fee,
                  decoration: const InputDecoration(labelText: 'Fee rule'),
                  items: [
                    for (final f in ['FLAT', 'DISTANCE_TIER', 'ZONE_BASED'])
                      DropdownMenuItem(
                        value: f,
                        child: Text(dashboardLabel(f)),
                      ),
                  ],
                  onChanged: c.canWrite
                      ? (v) => setState(() => fee = v!)
                      : null,
                ),
                const SizedBox(height: 16),
                for (final e in fields.entries)
                  Padding(
                    padding: const EdgeInsets.only(bottom: 16),
                    child: TextField(
                      controller: e.value,
                      readOnly: !c.canWrite,
                      keyboardType: TextInputType.numberWithOptions(
                        decimal: e.key.endsWith('_minor'),
                      ),
                      inputFormatters: [
                        FilteringTextInputFormatter.allow(
                          RegExp(
                            e.key.endsWith('_minor') ? r'[0-9.]' : r'[0-9]',
                          ),
                        ),
                      ],
                      decoration: InputDecoration(
                        labelText: switch (e.key) {
                          'base_fee_minor' => 'Base delivery fee',
                          'per_km_fee_minor' => 'Fee per kilometre',
                          'minimum_order_minor' => 'Minimum order amount',
                          'free_delivery_threshold_minor' =>
                            'Free delivery above (optional)',
                          'max_delivery_radius_meters' =>
                            'Maximum radius (metres, optional)',
                          'max_package_weight_grams' =>
                            'Maximum weight (grams, optional)',
                          'max_package_count' => 'Maximum packages (optional)',
                          'max_volume_ml' => 'Maximum volume (ml, optional)',
                          'estimated_minutes_base' =>
                            'Base delivery time (minutes)',
                          _ => 'Extra minutes per kilometre',
                        },
                      ),
                      onChanged: (_) => setState(() {}),
                    ),
                  ),
                TextField(
                  controller: reason,
                  readOnly: !c.canWrite,
                  maxLines: 2,
                  decoration: const InputDecoration(
                    labelText: 'Change reason (optional)',
                  ),
                ),
                const SizedBox(height: 20),
                SettingsLink(
                  'Advanced delivery settings',
                  Icons.tune_rounded,
                  color: const Color(0xFF346BA6),
                  subtitle: 'Zones, pricing, hours, vehicles & fallback',
                  onTap:
                      c.loading ||
                          c.stale ||
                          c.busy ||
                          c.attempt.pending != null
                      ? null
                      : editAdvanced,
                ),
                if (fee == 'ZONE_BASED' &&
                    (advanced['service_zones'] as List? ?? []).isEmpty)
                  Padding(
                    padding: const EdgeInsets.only(bottom: 16),
                    child: Text(
                      'Zone pricing needs an eligible enabled zone. Add zones in Advanced delivery settings.',
                      style: TextStyle(
                        color: const Color(0xFF986013).forInk(context),
                      ),
                    ),
                  ),
                if (c.record?['hybrid_fallback_funding_sponsor'] != null &&
                    c.record?['hybrid_fallback_funding_sponsor'] != 'VENDOR')
                  const Text(
                    'This platform-funded policy is managed by the platform.',
                  ),
                if (c.attempt.pending != null)
                  const Padding(
                    padding: EdgeInsets.only(bottom: 16),
                    child: Text(
                      'Retry the original policy update to confirm its result.',
                    ),
                  ),
                SettingsAction(
                  c.attempt.pending == null
                      ? 'Review policy'
                      : 'Retry original policy',
                  busy: c.busy,
                  icon: Icons.local_shipping_outlined,
                  onPressed: c.attempt.blocked
                      ? null
                      : c.attempt.pending != null
                      ? save
                      : c.canWrite && valid
                      ? save
                      : null,
                ),
                if (c.attempt.blocked)
                  Text(c.attempt.error ?? 'Recovery unavailable.'),
              ],
            ],
          ),
        ),
      );
    },
  );
}
