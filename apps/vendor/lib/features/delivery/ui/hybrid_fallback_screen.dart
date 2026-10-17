import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/ui/premium_loading.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../settings/ui/settings_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../state/hybrid_fallback_controller.dart';

class HybridFallbackScreen extends StatefulWidget {
  const HybridFallbackScreen({
    super.key,
    required this.runtime,
    required this.mission,
  });
  final VendorRuntime runtime;
  final JsonObject mission;
  @override
  State<HybridFallbackScreen> createState() => _HybridFallbackScreenState();
}

class _HybridFallbackScreenState extends State<HybridFallbackScreen> {
  late final c = HybridFallbackController(widget.runtime, widget.mission);
  String? reason;
  @override
  void initState() {
    super.initState();
    if (widget.runtime.mode == VendorDataMode.mock) {
      reason = 'PROVIDER_UNAVAILABLE';
    }
    c.load();
  }

  @override
  void dispose() {
    c.dispose();
    super.dispose();
  }

  Future<void> submit() async {
    if (c.attempt.pending == null) {
      final confirm = await confirmSettings(
        context,
        'Request platform fallback',
        '${dashboardLabel(reason)}. This requests a new delivery plan and its authorized funding transition. The server confirms whether the fallback can proceed.',
      );
      if (!confirm || !mounted) return;
    }
    final ok = await c.request(reason);
    if (!mounted) return;
    await showOrderFeedback(
      context,
      title: ok ? 'Fallback updated' : 'Fallback needs attention',
      message: ok
          ? dashboardLabel(c.record!['State'])
          : c.error ?? 'Refresh and retry the same request.',
      success: ok,
    );
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => PopScope(
      canPop: !c.busy,
      child: Scaffold(
        appBar: AppBar(
          title: const Text('Platform fallback'),
          actions: [
            IconButton(
              tooltip: 'Refresh fallback',
              onPressed: c.loading || c.busy ? null : c.load,
              icon: const Icon(Icons.refresh_rounded),
            ),
          ],
        ),
        body: ListView(
          padding: const EdgeInsets.all(20),
          children: [
            if (c.loading)
              const PremiumSkeleton(detail: true)
            else ...[
              if (c.error != null) ...[
                Text(c.error!, style: TextStyle(color: Theme.of(context).colorScheme.error)),
                const SizedBox(height: 16),
              ],
              SettingsCard(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    SettingsHeading(
                      c.record == null
                          ? 'Request delivery fallback'
                          : dashboardLabel(c.record!['State']),
                      subtitle:
                          'Change delivery authority before pickup custody.',
                    ),
                    const SizedBox(height: 20),
                    if (c.record == null)
                      DropdownButtonFormField<String>(
                        initialValue: reason,
                        isExpanded: true,
                        decoration: const InputDecoration(
                          labelText: 'Fallback reason',
                        ),
                        items: [
                          for (final r in fallbackReasons)
                            DropdownMenuItem(
                              value: r,
                              child: Text(dashboardLabel(r)),
                            ),
                        ],
                        onChanged: c.attempt.pending != null
                            ? null
                            : (v) => setState(() => reason = v),
                      ),
                    if (c.record != null)
                      for (final field in {
                        'OperationRef': 'Operation',
                        'ReasonCode': 'Reason',
                        'FailureReason': 'Failure reason',
                        'PlatformMissionRefID': 'Platform mission',
                        'TargetPlanVersion': 'New plan version',
                      }.entries)
                        if (c.record![field.key]?.toString().isNotEmpty == true)
                          Padding(
                            padding: const EdgeInsets.only(bottom: 14),
                            child: SettingsFact(
                              field.value,
                              field.key == 'ReasonCode'
                                  ? dashboardLabel(c.record![field.key])
                                  : c.record![field.key].toString(),
                            ),
                          ),
                  ],
                ),
              ),
              const SizedBox(height: 20),
              if (c.canRequest || c.attempt.pending != null)
                SettingsAction(
                  c.attempt.pending == null
                      ? 'Review fallback'
                      : 'Retry original fallback',
                  busy: c.busy,
                  icon: Icons.alt_route_rounded,
                  onPressed:
                      c.attempt.blocked ||
                          c.attempt.pending == null && reason == null
                      ? null
                      : submit,
                ),
              if (c.attempt.pending != null)
                const Padding(
                  padding: EdgeInsets.only(top: 16),
                  child: Text(
                    'Keep the original request while its result is being confirmed.',
                  ),
                ),
              if (c.attempt.blocked)
                Text(c.attempt.error ?? 'Recovery unavailable.'),
            ],
          ],
        ),
      ),
    ),
  );
}
