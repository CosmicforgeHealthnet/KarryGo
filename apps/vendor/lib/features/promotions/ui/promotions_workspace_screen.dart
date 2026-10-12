import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/ui/premium_loading.dart';
import '../../dashboard/ui/dashboard_empty_state.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../onboarding/ui/setup_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../state/promotion_rules.dart';
import '../state/promotions_workspace_controller.dart';
import 'promotion_editor.dart';
import 'promotions_design.dart';

class PromotionsWorkspaceScreen extends StatefulWidget {
  const PromotionsWorkspaceScreen({super.key, required this.runtime, this.id});
  final VendorRuntime runtime;
  final String? id;
  @override
  State<PromotionsWorkspaceScreen> createState() =>
      _PromotionsWorkspaceScreenState();
}

class _PromotionsWorkspaceScreenState extends State<PromotionsWorkspaceScreen> {
  late final c = PromotionsWorkspaceController(widget.runtime, id: widget.id);
  String? filter;
  @override
  void initState() {
    super.initState();
    unawaited(c.load());
  }

  @override
  void dispose() {
    c.dispose();
    super.dispose();
  }

  Future<void> open(JsonObject row) async {
    await Navigator.push(
      context,
      setupRoute<void>(
        PromotionsWorkspaceScreen(runtime: widget.runtime, id: row['id']),
      ),
    );
    await c.load();
  }

  Future<void> edit() async {
    await Navigator.push(
      context,
      setupRoute<bool>(
        PromotionEditor(
          runtime: widget.runtime,
          controller: c,
          record: c.record,
        ),
      ),
    );
    await c.load();
  }

  Future<void> feedback(JsonObject? result, {String? coupon}) async {
    if (!mounted) return;
    final ok = await showOrderFeedback(
      context,
      title: result == null
          ? 'Update not confirmed'
          : coupon != null
          ? 'Coupon created'
          : 'Promotion updated',
      message: result == null
          ? c.error ?? 'Refresh and try again.'
          : coupon != null
          ? '$coupon\nYour code is ready to share.'
          : c.error ?? 'Your changes have been saved.',
      success: result != null && c.error == null,
      warning: c.pending != null || result != null && c.error != null,
      action: result != null && coupon != null ? 'Copy code' : null,
    );
    if (ok == true && coupon != null) {
      await Clipboard.setData(ClipboardData(text: coupon));
    }
    if (result != null) await c.load();
  }

  Future<void> retry() async {
    final attempt = c.pending!;
    final code = (attempt['path'] as String).endsWith('/coupons')
        ? attempt['body']['code'] as String?
        : null;
    final confirm = await showOrderFeedback(
      context,
      title: 'Retry original update?',
      warning: true,
      message:
          'This sends the saved ${attempt['body']['name'] ?? attempt['body']['status'] ?? 'coupon'} update with its original retry key.',
      action: 'Retry update',
    );
    if (confirm != true || !mounted) return;
    await feedback(await c.save(), coupon: code);
  }

  Future<void> status() async {
    final row = c.record!;
    final next = await showModalBottomSheet<String>(
      context: context,
      showDragHandle: true,
      useSafeArea: true,
      builder: (context) => Padding(
        padding: const EdgeInsets.fromLTRB(20, 0, 20, 24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Text(
              'Change status',
              style: TextStyle(fontSize: 22, fontWeight: FontWeight.w700),
            ),
            const SizedBox(height: 14),
            for (final value
                in promotionTransitions[row['status']] ?? <String>[])
              ListTile(
                contentPadding: EdgeInsets.zero,
                leading: Icon(switch (value) {
                  'ACTIVE' => Icons.play_circle_outline_rounded,
                  'SCHEDULED' => Icons.event_available_outlined,
                  'PAUSED' => Icons.pause_circle_outline_rounded,
                  'ENDED' => Icons.stop_circle_outlined,
                  _ => Icons.archive_outlined,
                }, color: promotionStatusColor(value).forInk(context)),
                title: Text(dashboardLabel(value)),
                trailing: const Icon(Icons.chevron_right_rounded),
                onTap: () => Navigator.pop(context, value),
              ),
          ],
        ),
      ),
    );
    if (next == null || !mounted) return;
    final confirm = await showOrderFeedback(
      context,
      title: '${dashboardLabel(next)} this offer?',
      message: next == 'ARCHIVED'
          ? 'Archived offers cannot be reopened.'
          : next == 'ENDED'
          ? 'Ended offers can only be archived.'
          : next == 'ACTIVE' || next == 'SCHEDULED'
          ? 'Eligibility still follows the offer dates, audience and limits. Your business funds the configured discount.'
          : 'The offer will stop being eligible while paused.',
      warning: true,
      ending: ['ENDED', 'ARCHIVED'].contains(next),
      action: 'Confirm',
    );
    if (confirm != true || !mounted) return;
    await feedback(
      await c.save(
        method: 'POST',
        path: '$promotionBase/{promotionId}/status',
        body: {'status': next},
        promotionId: row['id'],
        version: row['version'],
      ),
    );
  }

  Future<void> coupon() async {
    final body = await showModalBottomSheet<JsonObject>(
      context: context,
      isScrollControlled: true,
      showDragHandle: true,
      useSafeArea: true,
      builder: (context) =>
          _CouponSheet(mock: widget.runtime.mode == VendorDataMode.mock),
    );
    if (body == null || !mounted) return;
    await feedback(
      await c.save(
        method: 'POST',
        path: '$promotionBase/{promotionId}/coupons',
        body: body,
        promotionId: c.record!['id'],
      ),
      coupon: body['code'],
    );
  }

  Widget recovery() => PromotionSurface(
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Icon(
          Icons.sync_problem_outlined,
          color: const Color(0xFF986013).forInk(context),
          size: 30,
        ),
        const SizedBox(height: 12),
        Text(
          c.blocked
              ? 'Recovery needs attention'
              : 'An update needs confirmation',
          style: const TextStyle(fontSize: 17, fontWeight: FontWeight.w700),
        ),
        const SizedBox(height: 8),
        Text(
          c.blocked
              ? c.error ?? 'Reopen Promotions to restore your update.'
              : 'Retry the saved update before making further changes.',
        ),
        if (!c.blocked) ...[
          const SizedBox(height: 16),
          FilledButton(
            onPressed: c.busy ? null : retry,
            child: PromotionBusyLabel('Retry original update', c.busy),
          ),
        ],
      ],
    ),
  );
  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => Scaffold(
      appBar: AppBar(
        title: Text(widget.id == null ? 'Promotions' : 'Offer details'),
        actions: [
          IconButton(
            tooltip: 'Refresh',
            onPressed: c.loading || c.busy ? null : c.load,
            icon: const Icon(Icons.refresh_rounded),
          ),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: c.load,
        child: ListView(
          padding: const EdgeInsets.fromLTRB(20, 12, 20, 30),
          children: [
            if (c.pending != null || c.blocked) ...[
              recovery(),
              const SizedBox(height: 16),
            ],
            if (c.loading)
              const PremiumSkeleton(detail: true)
            else if (c.error != null && c.pending == null && !c.blocked) ...[
              DashboardEmptyState(
                story: EmptyStory.unavailable,
                title: 'Promotions unavailable',
                message: c.error!,
                action: 'Try again',
                onAction: c.load,
              ),
            ] else if (widget.id == null)
              ...list()
            else if (c.record != null)
              ...detail(),
          ],
        ),
      ),
      bottomNavigationBar: widget.id == null
          ? SafeArea(
              top: false,
              minimum: const EdgeInsets.fromLTRB(20, 10, 20, 14),
              child: FilledButton.icon(
                onPressed: c.allowed('POST', promotionBase) ? edit : null,
                icon: const Icon(Icons.add_rounded),
                label: const Text('Create promotion'),
              ),
            )
          : null,
    ),
  );
  List<Widget> list() {
    final visible = c.rows
        .where((row) => filter == null || row['status'] == filter)
        .toList();
    return [
      Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  'Your offers',
                  style: TextStyle(
                    fontSize: 30,
                    fontWeight: FontWeight.w800,
                    letterSpacing: -1,
                  ),
                ),
                SizedBox(height: 6),
                Text('Discounts, bundles & delivery offers.'),
              ],
            ),
          ),
          Container(
            padding: const EdgeInsets.all(12),
            decoration: BoxDecoration(
              color: Theme.of(context).colorScheme.surface,
              borderRadius: BorderRadius.circular(18),
            ),
            child: Text(
              '${c.rows.length}',
              style: TextStyle(
                fontSize: 26,
                color: KarryamColors.tealInk.forInk(context),
                fontWeight: FontWeight.w700,
              ),
            ),
          ),
        ],
      ),
      const SizedBox(height: 24),
      SingleChildScrollView(
        scrollDirection: Axis.horizontal,
        child: Row(
          children: [
            for (final value in <String?>[null, ...promotionTransitions.keys])
              Padding(
                padding: const EdgeInsets.only(right: 8),
                child: ChoiceChip(
                  label: Text(value == null ? 'All' : dashboardLabel(value)),
                  selected: filter == value,
                  onSelected: (_) => setState(() => filter = value),
                ),
              ),
          ],
        ),
      ),
      const SizedBox(height: 20),
      if (visible.isEmpty)
        DashboardEmptyState(
          story: EmptyStory.filtered,
          iconOverride: Icons.confirmation_number_outlined,
          detailIconOverride: Icons.percent_rounded,
          title: filter == null
              ? 'Your next offer starts here'
              : 'No ${dashboardLabel(filter).toLowerCase()} offers',
          message: filter == null
              ? 'Create a vendor-funded offer for your customers.'
              : 'Choose another status to see your offers.',
        )
      else
        for (final row in visible)
          Padding(
            padding: const EdgeInsets.only(bottom: 16),
            child: PromotionTicket(row: row, onTap: () => open(row)),
          ),
    ];
  }

  List<Widget> detail() {
    final row = c.record!;
    final canEdit =
        row['funding_sponsor'] == 'VENDOR' &&
        c.allowed('PATCH', '$promotionBase/{promotionId}');
    return [
      PromotionTicket(row: row, hero: true),
      const SizedBox(height: 18),
      PromotionSurface(
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Icon(
              Icons.confirmation_number_outlined,
              color: KarryamColors.tealInk.forInk(context),
              size: 30,
            ),
            const SizedBox(width: 15),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    c.used?.toString() ?? 'Unavailable',
                    style: const TextStyle(
                      fontSize: 28,
                      fontWeight: FontWeight.w800,
                    ),
                  ),
                  const SizedBox(height: 3),
                  const Text('Reserved & used redemptions'),
                  if (c.usageError != null) ...[
                    const SizedBox(height: 8),
                    Text(
                      c.usageError!,
                      style: TextStyle(
                        color: Theme.of(context).colorScheme.error,
                      ),
                    ),
                  ],
                ],
              ),
            ),
          ],
        ),
      ),
      const SizedBox(height: 18),
      PromotionSurface(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Text(
              'Offer rules',
              style: TextStyle(fontSize: 18, fontWeight: FontWeight.w700),
            ),
            const SizedBox(height: 8),
            PromotionFact(
              'Type',
              promotionTypes[row['promotion_type']] ?? 'Unavailable',
            ),
            PromotionFact(
              'Starts',
              promotionDate(row['starts_at'], time: true),
            ),
            PromotionFact('Ends', promotionDate(row['ends_at'], time: true)),
            PromotionFact(
              'Minimum spend',
              dashboardMoney(row['minimum_order_minor'], row['currency']),
            ),
            PromotionFact(
              'Discount cap',
              row['max_discount_minor'] == null
                  ? 'No cap set'
                  : dashboardMoney(row['max_discount_minor'], row['currency']),
            ),
            PromotionFact(
              'Total usage limit',
              row['global_usage_limit']?.toString() ?? 'Not set',
            ),
            PromotionFact(
              'Per customer',
              row['per_customer_usage_limit']?.toString() ?? 'Not set',
            ),
            PromotionFact(
              'Coupon',
              row['coupon_required'] == true ||
                      row['promotion_type'] == 'COUPON'
                  ? 'Required'
                  : 'Not required',
            ),
            PromotionFact('Combination', dashboardLabel(row['combinability'])),
            PromotionFact('Priority', '${row['priority']}'),
            PromotionFact('Funding', dashboardLabel(row['funding_sponsor'])),
            if (row['funding_sponsor'] != 'VENDOR')
              const Text('This funding arrangement cannot be edited here.'),
          ],
        ),
      ),
      const SizedBox(height: 18),
      PromotionSurface(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Text(
              'Audience',
              style: TextStyle(fontSize: 18, fontWeight: FontWeight.w700),
            ),
            const SizedBox(height: 8),
            if ((row['scopes'] as List? ?? []).isEmpty)
              const Text('Business-wide')
            else
              for (final target in row['scopes'] as List)
                PromotionFact(
                  dashboardLabel(target['type']),
                  target['ref'].toString(),
                ),
            if (row['promotion_type'] == 'BUNDLE') ...[
              const SizedBox(height: 14),
              const Text(
                'Required bundle items',
                style: TextStyle(fontWeight: FontWeight.w700),
              ),
              for (final item in row['bundle_requirements'] as List? ?? [])
                PromotionFact(
                  '${item['minimum_quantity']} required',
                  '${item['item_ref']}${item['variant_ref'] == null || item['variant_ref'] == '' ? '' : ' · ${item['variant_ref']}'}',
                ),
            ],
            if (row['promotion_type'] == 'QUANTITY_BREAK') ...[
              const SizedBox(height: 14),
              const Text(
                'Quantity tiers',
                style: TextStyle(fontWeight: FontWeight.w700),
              ),
              for (final tier in row['quantity_tiers'] as List? ?? [])
                PromotionFact(
                  '${tier['minimum_quantity']}–${tier['maximum_quantity'] ?? 'up'} units',
                  promotionBenefit({
                    ...row,
                    ...jsonObject(tier),
                    'promotion_type': 'COUPON',
                  }),
                ),
            ],
          ],
        ),
      ),
      const SizedBox(height: 22),
      if (canEdit)
        FilledButton.icon(
          onPressed: edit,
          icon: const Icon(Icons.edit_outlined),
          label: const Text('Edit offer'),
        ),
      if ((promotionTransitions[row['status']] ?? []).isNotEmpty) ...[
        const SizedBox(height: 12),
        OutlinedButton.icon(
          onPressed: c.allowed('POST', '$promotionBase/{promotionId}/status')
              ? status
              : null,
          icon: const Icon(Icons.tune_rounded),
          label: const Text('Change status'),
        ),
      ],
      if (!['ENDED', 'ARCHIVED'].contains(row['status'])) ...[
        const SizedBox(height: 12),
        OutlinedButton.icon(
          onPressed: c.allowed('POST', '$promotionBase/{promotionId}/coupons')
              ? coupon
              : null,
          icon: const Icon(Icons.add_card_outlined),
          label: const Text('Create coupon'),
        ),
      ],
      if (c.busy)
        const Padding(
          padding: EdgeInsets.all(16),
          child: Center(child: CircularProgressIndicator(strokeWidth: 2)),
        ),
    ];
  }
}

class _CouponSheet extends StatefulWidget {
  const _CouponSheet({required this.mock});
  final bool mock;
  @override
  State<_CouponSheet> createState() => _CouponSheetState();
}

class _CouponSheetState extends State<_CouponSheet> {
  late final code = TextEditingController(
    text: widget.mock
        ? 'WELCOME${DateTime.now().millisecondsSinceEpoch % 100000}'
        : '',
  );
  final total = TextEditingController(), customer = TextEditingController();
  final form = GlobalKey<FormState>();
  @override
  void dispose() {
    code.dispose();
    total.dispose();
    customer.dispose();
    super.dispose();
  }

  Widget limit(String label, TextEditingController controller) => Padding(
    padding: const EdgeInsets.only(top: 14),
    child: TextFormField(
      controller: controller,
      keyboardType: TextInputType.number,
      decoration: InputDecoration(labelText: label, hintText: 'Optional'),
      validator: (value) =>
          value!.isNotEmpty &&
              (int.tryParse(value) == null || int.parse(value) < 1)
          ? 'Use a positive whole number'
          : null,
    ),
  );
  @override
  Widget build(BuildContext context) => Padding(
    padding: EdgeInsets.fromLTRB(
      20,
      0,
      20,
      MediaQuery.viewInsetsOf(context).bottom + 24,
    ),
    child: SingleChildScrollView(
      child: Form(
        key: form,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Text(
              'Create coupon',
              style: TextStyle(fontSize: 23, fontWeight: FontWeight.w700),
            ),
            const SizedBox(height: 8),
            const Text(
              'Save the code after creation. Existing codes cannot be retrieved here.',
            ),
            const SizedBox(height: 20),
            TextFormField(
              controller: code,
              textCapitalization: TextCapitalization.characters,
              decoration: const InputDecoration(labelText: 'Coupon code'),
              validator: (value) =>
                  value == null || value.trim().isEmpty ? 'Enter a code' : null,
            ),
            limit('Total usage limit', total),
            limit('Per-customer limit', customer),
            const SizedBox(height: 22),
            SizedBox(
              width: double.infinity,
              child: FilledButton(
                onPressed: () {
                  if (!form.currentState!.validate()) return;
                  Navigator.pop(context, <String, dynamic>{
                    'code': code.text.trim().toUpperCase(),
                    if (total.text.isNotEmpty)
                      'usage_limit': int.parse(total.text),
                    if (customer.text.isNotEmpty)
                      'per_customer_usage_limit': int.parse(customer.text),
                  });
                },
                child: const Text('Create coupon'),
              ),
            ),
          ],
        ),
      ),
    ),
  );
}
