import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/ui/premium_loading.dart';
import '../../dashboard/ui/dashboard_empty_state.dart';
import '../../settings/ui/settings_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../state/rider_assignment_controller.dart';

class RiderAssignmentScreen extends StatefulWidget {
  const RiderAssignmentScreen({
    super.key,
    required this.runtime,
    required this.mission,
  });
  final VendorRuntime runtime;
  final JsonObject mission;
  @override
  State<RiderAssignmentScreen> createState() => _RiderAssignmentScreenState();
}

class _RiderAssignmentScreenState extends State<RiderAssignmentScreen> {
  late final c = RiderAssignmentController(widget.runtime, widget.mission);
  String? selected;
  @override
  void initState() {
    super.initState();
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
        'Assign rider',
        'Assign this delivery to the selected rider? Availability and branch access will be checked before assignment.',
      );
      if (!confirm || !mounted) return;
    }
    final ok = await c.assign(selected);
    if (!mounted) return;
    await showOrderFeedback(
      context,
      title: ok ? 'Rider assigned' : 'Assignment needs attention',
      message: ok
          ? 'The rider can now open their assigned delivery.'
          : c.error ?? 'Refresh and try again.',
      success: ok,
    );
    if (ok && mounted) Navigator.pop(context, true);
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => PopScope(
      canPop: !c.busy,
      child: Scaffold(
        appBar: AppBar(title: const Text('Assign rider')),
        body: ListView(
          padding: const EdgeInsets.all(20),
          children: [
            const SettingsHeading(
              'Choose a branch rider',
              subtitle:
                  'Active staff with the Vendor Rider role. Availability is confirmed by the server.',
            ),
            const SizedBox(height: 20),
            if (c.loading)
              const PremiumSkeleton(rows: 3)
            else ...[
              if (c.error != null) ...[
                Text(c.error!, style: TextStyle(color: Theme.of(context).colorScheme.error)),
                const SizedBox(height: 16),
                SettingsAction(
                  'Refresh riders',
                  secondary: true,
                  onPressed: c.busy ? null : c.load,
                ),
                const SizedBox(height: 16),
              ],
              if (c.riders.isEmpty && c.error == null)
                const DashboardEmptyState(
                  story: EmptyStory.setup,
                  title: 'No eligible riders',
                  message:
                      'Assign the Vendor Rider role to an active team member at this branch.',
                  iconOverride: Icons.delivery_dining_rounded,
                  detailIconOverride: Icons.badge_outlined,
                ),
              for (final rider in c.riders)
                Padding(
                  padding: const EdgeInsets.only(bottom: 12),
                  child: SettingsCard(
                    child: ListTile(
                      contentPadding: EdgeInsets.zero,
                      leading: const Icon(Icons.person_outline_rounded),
                      selected: selected == rider['id'],
                      trailing: Icon(
                        selected == rider['id']
                            ? Icons.radio_button_checked_rounded
                            : Icons.radio_button_off_rounded,
                        color: selected == rider['id']
                            ? KarryamColors.tealInk.forInk(context)
                            : null,
                      ),
                      onTap: c.busy || c.attempt.pending != null
                          ? null
                          : () => setState(
                              () => selected = rider['id'] as String,
                            ),
                      title: Text(
                        rider['display_name']?.toString() ??
                            rider['id'].toString(),
                      ),
                      subtitle: const Text('Vendor Rider'),
                    ),
                  ),
                ),
              if (c.attempt.pending != null)
                const Padding(
                  padding: EdgeInsets.only(bottom: 16),
                  child: Text(
                    'The previous assignment was not confirmed. Retry the same assignment.',
                  ),
                ),
              if (c.attempt.blocked)
                Text(c.attempt.error ?? 'Recovery unavailable.'),
              SettingsAction(
                c.attempt.pending == null ? 'Assign rider' : 'Retry assignment',
                icon: Icons.delivery_dining_rounded,
                busy: c.busy,
                onPressed:
                    c.attempt.blocked ||
                        c.error != null && c.attempt.pending == null ||
                        selected == null && c.attempt.pending == null
                    ? null
                    : submit,
              ),
            ],
          ],
        ),
      ),
    ),
  );
}
