import 'dart:convert';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/ui/premium_loading.dart';
import '../../dashboard/ui/dashboard_empty_state.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../onboarding/ui/setup_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../../settings/ui/settings_design.dart';
import '../state/order_support_controller.dart';

IconData supportIcon(Object? category) => switch (category) {
  'MISSING_ITEM' => Icons.playlist_remove_rounded,
  'WRONG_ITEM_OR_MEAL' ||
  'UNAUTHORIZED_SUBSTITUTION' => Icons.swap_horiz_rounded,
  'DAMAGED_OR_SPOILT_PRODUCT' => Icons.broken_image_outlined,
  'VENDOR_DELAY' || 'DELIVERY_DELAY' => Icons.schedule_rounded,
  'PICKUP_DISPUTE' || 'RECEIVER_HANDOVER_DISPUTE' => Icons.handshake_outlined,
  'REFUND_PROBLEM' => Icons.currency_exchange_rounded,
  'VENDOR_OWNED_DELIVERY_ISSUE' => Icons.local_shipping_outlined,
  _ => Icons.fact_check_outlined,
};
Color supportStateColor(Object? value) =>
    switch (value?.toString().toLowerCase()) {
      'resolved' || 'closed' => KarryamColors.tealInk,
      'failed' || 'rejected' => KarryamColors.error,
      _ => const Color(0xFF986013),
    };

class OrderSupportScreen extends StatefulWidget {
  const OrderSupportScreen({
    super.key,
    required this.runtime,
    required this.order,
  });
  final VendorRuntime runtime;
  final JsonObject order;
  @override
  State<OrderSupportScreen> createState() => _OrderSupportScreenState();
}

class _OrderSupportScreenState extends State<OrderSupportScreen> {
  late final OrderSupportController c;
  @override
  void initState() {
    super.initState();
    c = OrderSupportController(
      widget.runtime,
      widget.order['vendor_order_id'] as String,
    );
    c.initialize();
  }

  @override
  void dispose() {
    c.dispose();
    super.dispose();
  }

  Future<void> report() async {
    await Navigator.push(
      context,
      setupRoute<void>(_ReportIssueScreen(controller: c, order: widget.order)),
    );
  }

  Future<void> details(JsonObject record) => settingsSheet<void>(
    context,
    Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        SettingsHeading(dashboardLabel(record['category'])),
        const SizedBox(height: 16),
        for (final entry in <String, String>{
          'case_ref': 'Case reference',
          'status': 'Status',
          'resolution': 'Resolution',
          'refund_status': 'Refund status',
          'financial_action_ref': 'Financial action reference',
          'created_at': 'Created',
          'updated_at': 'Last updated',
        }.entries)
          if (record[entry.key]?.toString().isNotEmpty == true)
            Padding(
              padding: const EdgeInsets.only(bottom: 14),
              child: SettingsFact(
                entry.value,
                entry.key.endsWith('_at')
                    ? supportDate(record[entry.key])
                    : entry.key == 'status' ||
                          entry.key == 'resolution' ||
                          entry.key == 'refund_status'
                    ? dashboardLabel(record[entry.key])
                    : record[entry.key].toString(),
              ),
            ),
      ],
    ),
  );
  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => PopScope(
      canPop: !c.busy,
      child: Scaffold(
        appBar: AppBar(
          title: const Text('Order support'),
          actions: [
            IconButton(
              tooltip: 'Refresh reports',
              onPressed: c.loading || c.busy ? null : c.load,
              icon: const Icon(Icons.refresh_rounded),
            ),
          ],
        ),
        body: RefreshIndicator(
          onRefresh: c.load,
          child: ListView(
            physics: const AlwaysScrollableScrollPhysics(),
            padding: const EdgeInsets.fromLTRB(22, 10, 22, 24),
            children: [
              SettingsCard(
                child: Row(
                  children: [
                    Container(
                      padding: const EdgeInsets.all(14),
                      decoration: BoxDecoration(
                        color: Theme.of(context).colorScheme.surfaceContainer,
                        borderRadius: BorderRadius.circular(18),
                      ),
                      child: Icon(
                        Icons.support_agent_rounded,
                        color: KarryamColors.tealInk.forInk(context),
                        size: 28,
                      ),
                    ),
                    const SizedBox(width: 16),
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          const Text(
                            'Get help with this order',
                            style: TextStyle(
                              fontSize: 18,
                              fontWeight: FontWeight.w700,
                            ),
                          ),
                          const SizedBox(height: 6),
                          Text(
                            '#${c.orderId}',
                            style: const TextStyle(fontSize: 12),
                          ),
                          const SizedBox(height: 8),
                          const Text(
                            'Report an issue and follow its latest status.',
                            style: TextStyle(fontSize: 13),
                          ),
                        ],
                      ),
                    ),
                  ],
                ),
              ),
              const SizedBox(height: 20),
              if (c.commandError != null)
                SettingsNote(c.commandError!, warning: c.pending != null),
              if (c.pending != null && !c.journalBlocked) ...[
                const SettingsNote(
                  'The last report has an unconfirmed outcome. Refresh to check whether it was recorded, or retry the same report.',
                  warning: true,
                ),
                SettingsAction(
                  'Retry original report',
                  secondary: true,
                  busy: c.busy,
                  icon: Icons.refresh_rounded,
                  onPressed: c.loading
                      ? null
                      : () async {
                          final result = await c.create(null);
                          if (result != null && context.mounted) {
                            await showOrderFeedback(
                              context,
                              title: 'Issue reported',
                              message:
                                  'Your report has been recorded. You can follow its status here.',
                              success: true,
                              action: 'View reports',
                            );
                          }
                        },
                ),
                const SizedBox(height: 18),
              ],
              if (c.loading && c.cases.isEmpty)
                const PremiumSkeleton(rows: 2)
              else if (c.error != null)
                DashboardEmptyState(
                  story: c.readDenied
                      ? EmptyStory.restricted
                      : EmptyStory.unavailable,
                  title: c.readDenied
                      ? 'Report access required'
                      : 'Reports unavailable',
                  message: c.error!,
                  action: 'Try again',
                  onAction: c.load,
                )
              else if (c.cases.isEmpty)
                const DashboardEmptyState(
                  story: EmptyStory.activity,
                  iconOverride: Icons.forum_outlined,
                  detailIconOverride: Icons.receipt_long_outlined,
                  title: 'No issues reported',
                  message: 'Reports for this order will appear here.',
                )
              else ...[
                if (c.loading) const ContentRefresh(),
                for (final record in c.cases)
                  Padding(
                    padding: const EdgeInsets.only(bottom: 12),
                    child: SettingsLink(
                      dashboardLabel(record['category']),
                      supportIcon(record['category']),
                      subtitle:
                          '${dashboardLabel(record['status'])}${record['case_ref'] != null ? ' · ${record['case_ref']}' : ''}',
                      color: supportStateColor(record['status']),
                      onTap: () => details(record),
                    ),
                  ),
              ],
            ],
          ),
        ),
        bottomNavigationBar: SafeArea(
          child: Padding(
            padding: const EdgeInsets.fromLTRB(22, 12, 22, 16),
            child: SettingsAction(
              'Report an issue',
              icon: Icons.add_comment_outlined,
              onPressed: c.canCreate ? report : null,
            ),
          ),
        ),
      ),
    ),
  );
}

String supportDate(Object? value) {
  final date = DateTime.tryParse(value?.toString() ?? '')?.toLocal();
  if (date == null) return value?.toString() ?? '';
  return '${date.day.toString().padLeft(2, '0')}/${date.month.toString().padLeft(2, '0')}/${date.year} · ${date.hour.toString().padLeft(2, '0')}:${date.minute.toString().padLeft(2, '0')}';
}

class _ReportIssueScreen extends StatefulWidget {
  const _ReportIssueScreen({required this.controller, required this.order});
  final OrderSupportController controller;
  final JsonObject order;
  @override
  State<_ReportIssueScreen> createState() => _ReportIssueScreenState();
}

class _ReportIssueScreenState extends State<_ReportIssueScreen> {
  final form = GlobalKey<FormState>();
  final description = TextEditingController();
  final selected = <String>{};
  String? category;
  OrderSupportController get c => widget.controller;
  List<JsonObject> get lines => (widget.order['lines'] as List? ?? [])
      .map(jsonObject)
      .where(
        (line) => line['id'] is String && (line['id'] as String).isNotEmpty,
      )
      .toList();
  @override
  void initState() {
    super.initState();
    if (c.runtime.mode == VendorDataMode.mock) {
      category = 'QUALITY_ISSUE';
      description.text =
          'The item quality did not meet the expected standard. Please review this order.';
    }
  }

  @override
  void dispose() {
    description.dispose();
    super.dispose();
  }

  Future<void> chooseCategory() async {
    final value = await settingsSheet<String>(
      context,
      Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const SettingsHeading('What happened?'),
          const SizedBox(height: 16),
          for (final value in c.categories)
            Padding(
              padding: const EdgeInsets.only(bottom: 10),
              child: SettingsLink(
                dashboardLabel(value),
                supportIcon(value),
                onTap: () => Navigator.pop(context, value),
              ),
            ),
        ],
      ),
    );
    if (value != null && mounted) setState(() => category = value);
  }

  Future<void> submit() async {
    JsonObject? body;
    if (c.pending == null) {
      if (!form.currentState!.validate() || category == null) return;
      FocusScope.of(context).unfocus();
      final accepted = await settingsSheet<bool>(
        context,
        Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const SettingsHeading('Review your report'),
            const SizedBox(height: 18),
            SettingsFact('Issue', dashboardLabel(category)),
            const SizedBox(height: 16),
            SettingsFact(
              'Description',
              sanitizeSupportDescription(description.text),
            ),
            if (selected.isNotEmpty) ...[
              const SizedBox(height: 16),
              SettingsFact(
                'Affected items',
                lines
                    .where((line) => selected.contains(line['id']))
                    .map((line) => line['item_name'] ?? line['id'])
                    .join(', '),
              ),
            ],
          ],
        ),
        footer: SettingsAction(
          'Submit report',
          icon: Icons.send_outlined,
          onPressed: () => Navigator.pop(context, true),
        ),
      );
      if (accepted != true || !mounted) return;
      body = {
        'category': category,
        'description': description.text.trim(),
        if (selected.isNotEmpty) 'line_refs': selected.toList(),
      };
    }
    final result = await c.create(body);
    if (!mounted) return;
    if (result == null) {
      await showOrderFeedback(
        context,
        title: c.pending != null
            ? 'Check report status'
            : 'Report not submitted',
        message: c.commandError ?? 'Please try again.',
        warning: c.pending != null,
        action: 'Back to report',
      );
      return;
    }
    await showOrderFeedback(
      context,
      title: 'Issue reported',
      message:
          'Your report has been recorded. You can follow its status in Order support.',
      success: true,
      action: 'View reports',
    );
    if (mounted) Navigator.pop(context);
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => PopScope(
      canPop: !c.busy,
      child: Scaffold(
        appBar: AppBar(title: const Text('Report an issue')),
        body: Form(
          key: form,
          child: ListView(
            padding: const EdgeInsets.fromLTRB(22, 8, 22, 24),
            children: [
              Text(
                '#${c.orderId}',
                style: TextStyle(
                  color: Theme.of(context).colorScheme.onSurfaceVariant,
                  fontSize: 12,
                ),
              ),
              const SizedBox(height: 18),
              SettingsLink(
                category == null
                    ? 'Choose the issue'
                    : dashboardLabel(category),
                supportIcon(category),
                subtitle: 'Issue type',
                onTap: c.busy || c.pending != null ? null : chooseCategory,
              ),
              const SizedBox(height: 22),
              TextFormField(
                controller: description,
                enabled: !c.busy && c.pending == null,
                minLines: 5,
                maxLines: 8,
                maxLength: 1000,
                decoration: const InputDecoration(
                  labelText: 'What happened?',
                  alignLabelWithHint: true,
                  hintText: 'Describe the issue with this order.',
                ),
                validator: (value) {
                  if (value?.trim().isNotEmpty != true) {
                    return 'Describe what happened.';
                  }
                  return utf8
                              .encode(sanitizeSupportDescription(value!))
                              .length >
                          1000
                      ? 'Keep the description within 1,000 bytes.'
                      : null;
                },
              ),
              if (lines.isNotEmpty) ...[
                const SizedBox(height: 12),
                const SettingsHeading('Affected items', subtitle: 'Optional'),
                const SizedBox(height: 14),
                SettingsCard(
                  child: Column(
                    children: [
                      for (final line in lines)
                        CheckboxListTile(
                          contentPadding: EdgeInsets.zero,
                          controlAffinity: ListTileControlAffinity.leading,
                          title: Text(
                            (line['item_name'] ?? line['id']).toString(),
                          ),
                          value: selected.contains(line['id']),
                          onChanged:
                              c.busy ||
                                  c.pending != null ||
                                  selected.length >= 20 &&
                                      !selected.contains(line['id'])
                              ? null
                              : (value) => setState(() {
                                  if (value == true) {
                                    selected.add(line['id'] as String);
                                  } else {
                                    selected.remove(line['id']);
                                  }
                                }),
                        ),
                    ],
                  ),
                ),
              ],
              if (c.commandError != null) ...[
                const SizedBox(height: 18),
                SettingsNote(c.commandError!, warning: c.pending != null),
              ],
            ],
          ),
        ),
        bottomNavigationBar: SafeArea(
          child: Padding(
            padding: EdgeInsets.fromLTRB(
              22,
              12,
              22,
              16 + MediaQuery.viewInsetsOf(context).bottom,
            ),
            child: SettingsAction(
              c.pending == null ? 'Review report' : 'Retry original report',
              icon: c.pending == null
                  ? Icons.arrow_forward_rounded
                  : Icons.refresh_rounded,
              busy: c.busy,
              onPressed: c.writeDenied || c.journalBlocked || category == null
                  ? null
                  : submit,
            ),
          ),
        ),
      ),
    ),
  );
}
