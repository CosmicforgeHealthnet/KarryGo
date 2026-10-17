import 'dart:async';
import 'package:flutter/material.dart';
import '../../../core/app_mode.dart';
import '../../../core/network/provider_session.dart';
import '../../../core/theme/theme_extensions.dart';
import '../../../shared/widgets/async_content.dart';
import '../state/offline_updates_controller.dart';

class OfflineUpdatesScreen extends StatefulWidget {
  const OfflineUpdatesScreen({super.key, required this.controller});
  final OfflineUpdatesController controller;
  @override
  State<OfflineUpdatesScreen> createState() => _OfflineUpdatesScreenState();
}

class _OfflineUpdatesScreenState extends State<OfflineUpdatesScreen> {
  bool _loading = true;
  @override
  void initState() {
    super.initState();
    unawaited(_start());
  }

  Future<void> _start() async {
    await widget.controller.start();
    if (mounted) setState(() => _loading = false);
    unawaited(widget.controller.sync());
  }

  Future<void> _remove(Map<String, dynamic> item) async {
    if (item['state'] != 'ACCEPTED') {
      final confirmed = await showDialog<bool>(
        context: context,
        builder: (dialog) => AlertDialog(
          title: const Text('Remove from this phone?'),
          content: const Text(
            'An unsynced update may never reach the server. Removing an evidence reminder does not change the delivery record.',
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(dialog, false),
              child: const Text('Keep update'),
            ),
            TextButton(
              onPressed: () => Navigator.pop(dialog, true),
              child: const Text('Remove'),
            ),
          ],
        ),
      );
      if (confirmed != true) return;
    }
    await widget.controller.dismiss(item['id'] as String);
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final queue = widget.controller;
    return AnimatedBuilder(
      animation: queue,
      builder: (context, _) => Scaffold(
        appBar: AppBar(title: const Text('Saved updates')),
        body: _loading
            ? const ContentSkeleton()
            : RefreshIndicator(
                onRefresh: () => queue.sync(force: true),
                child: ListView(
                  physics: const AlwaysScrollableScrollPhysics(),
                  padding: const EdgeInsets.all(20),
                  children: [
                    Text(
                      queue.pending > 0
                          ? '${queue.pending} updates to sync'
                          : 'Saved updates',
                      style: Theme.of(context).textTheme.headlineSmall,
                    ),
                    const SizedBox(height: 12),
                    const Text(
                      'Location history syncs automatically. Evidence still needs confirmation in the delivery.',
                    ),
                    const SizedBox(height: 20),
                    if (queue.error != null) ...[
                      ActionNotice(
                        message: queue.error!,
                        onRetry: () => queue.sync(force: true),
                      ),
                      const SizedBox(height: 16),
                    ],
                    FilledButton.icon(
                      onPressed: queue.syncing
                          ? null
                          : () => queue.sync(force: true),
                      icon: queue.syncing
                          ? const SizedBox(
                              width: 18,
                              height: 18,
                              child: CircularProgressIndicator(strokeWidth: 2),
                            )
                          : const Icon(Icons.sync_rounded),
                      label: Text(
                        queue.syncing ? 'Syncing updates' : 'Sync now',
                      ),
                    ),
                    const SizedBox(height: 24),
                    if (queue.records.isEmpty)
                      Padding(
                        padding: const EdgeInsets.symmetric(vertical: 44),
                        child: Column(
                          children: [
                            Icon(
                              Icons.cloud_done_outlined,
                              size: 64,
                              color: c.brandTeal600,
                            ),
                            const SizedBox(height: 16),
                            Text(
                              'No saved updates',
                              style: Theme.of(context).textTheme.titleLarge,
                            ),
                            const SizedBox(height: 8),
                            const Text(
                              'Updates saved during a connection problem will appear here.',
                              textAlign: TextAlign.center,
                            ),
                          ],
                        ),
                      ),
                    for (final item in queue.records.reversed) ...[
                      Card(
                        child: Padding(
                          padding: const EdgeInsets.all(18),
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.stretch,
                            children: [
                              Row(
                                children: [
                                  Container(
                                    padding: const EdgeInsets.all(12),
                                    decoration: BoxDecoration(
                                      color:
                                          (item['state'] == 'ACCEPTED'
                                                  ? c.success
                                                  : item['state'] == 'REJECTED'
                                                  ? c.danger
                                                  : c.warning)
                                              .withValues(alpha: .1),
                                      borderRadius: BorderRadius.circular(16),
                                    ),
                                    child: Icon(
                                      item['kind'] == 'LOCATION_FIX'
                                          ? Icons.location_on_outlined
                                          : Icons.fact_check_outlined,
                                      color: item['state'] == 'ACCEPTED'
                                          ? c.success
                                          : item['state'] == 'REJECTED'
                                          ? c.danger
                                          : c.warning,
                                    ),
                                  ),
                                  const SizedBox(width: 12),
                                  Expanded(
                                    child: Column(
                                      crossAxisAlignment:
                                          CrossAxisAlignment.start,
                                      children: [
                                        Text(
                                          item['kind'] == 'LOCATION_FIX'
                                              ? 'Location history'
                                              : 'Delivery evidence',
                                          style: Theme.of(
                                            context,
                                          ).textTheme.titleMedium,
                                        ),
                                        const SizedBox(height: 4),
                                        Text(
                                          switch (item['state']) {
                                            'ACCEPTED' => 'Received by server',
                                            'PENDING_EVIDENCE_SYNC' =>
                                              'Needs delivery confirmation',
                                            'REJECTED' => 'Not accepted',
                                            'EXPIRED' =>
                                              'Replay window expired',
                                            _ => 'Saved on this phone',
                                          },
                                          style: TextStyle(color: c.textMuted),
                                        ),
                                      ],
                                    ),
                                  ),
                                  IconButton(
                                    tooltip: 'Remove from phone',
                                    onPressed: queue.syncing
                                        ? null
                                        : () => _remove(item),
                                    icon: const Icon(Icons.close_rounded),
                                  ),
                                ],
                              ),
                              if ([
                                'REJECTED',
                                'EXPIRED',
                              ].contains(item['state']))
                                Padding(
                                  padding: const EdgeInsets.only(top: 12),
                                  child: Text(
                                    item['reason']?.toString() ??
                                        switch (item['reason_code']) {
                                          'STALE_SERVER_VERSION' =>
                                            'The delivery changed. Open it to review the latest status.',
                                          'ONLINE_MANIFEST_VALIDATION_REQUIRED' =>
                                            'Parcel scans must be confirmed online.',
                                          'ONLINE_HANDOVER_VERIFICATION_REQUIRED' =>
                                            'This handover needs online verification.',
                                          _ =>
                                            'Open the delivery to review this update.',
                                        },
                                  ),
                                ),
                              if (item['kind'] == 'EVIDENCE_REFERENCE' &&
                                  item['state'] != 'ACCEPTED') ...[
                                const SizedBox(height: 12),
                                const Text(
                                  'This reminder does not mean the evidence or handover is complete.',
                                ),
                                if (ProviderSession.of(
                                      context,
                                    ).openCurrentDelivery !=
                                    null)
                                  TextButton.icon(
                                    onPressed: ProviderSession.of(
                                      context,
                                    ).openCurrentDelivery,
                                    icon: const Icon(
                                      Icons.arrow_forward_rounded,
                                    ),
                                    label: const Text('Open current delivery'),
                                  ),
                              ],
                            ],
                          ),
                        ),
                      ),
                      const SizedBox(height: 8),
                    ],
                    if (AppModeConfig.instance.isMock)
                      OutlinedButton.icon(
                        onPressed: queue.syncing
                            ? null
                            : () async {
                                await queue.saveLocation(
                                  latitude: 6.45,
                                  longitude: 3.4,
                                  heading: 0,
                                  speed: 0,
                                  accuracy: 10,
                                  observedAt: DateTime.now(),
                                );
                              },
                        icon: const Icon(Icons.science_outlined),
                        label: const Text('Add demo location update'),
                      ),
                  ],
                ),
              ),
      ),
    );
  }
}

class OfflineSyncNotice extends StatelessWidget {
  const OfflineSyncNotice({super.key});
  @override
  Widget build(BuildContext context) {
    final queue = ProviderSession.of(context).offlineUpdates;
    if (queue == null) return const SizedBox.shrink();
    return AnimatedBuilder(
      animation: queue,
      builder: (context, _) => !queue.hasUpdates
          ? const SizedBox.shrink()
          : Padding(
              padding: const EdgeInsets.only(top: 12),
              child: Card(
                child: ListTile(
                  leading: const Icon(Icons.cloud_upload_outlined),
                  title: Text(
                    queue.pending > 0
                        ? '${queue.pending} saved updates'
                        : 'Review saved updates',
                  ),
                  subtitle: Text(
                    queue.pending > 0
                        ? 'Syncing when the server is reachable'
                        : 'Some updates need your attention',
                  ),
                  trailing: const Icon(Icons.chevron_right_rounded),
                  onTap: () => Navigator.of(context).push(
                    MaterialPageRoute(
                      builder: (_) => OfflineUpdatesScreen(controller: queue),
                    ),
                  ),
                ),
              ),
            ),
    );
  }
}
