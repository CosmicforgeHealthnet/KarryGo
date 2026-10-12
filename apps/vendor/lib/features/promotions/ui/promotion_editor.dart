import 'dart:async';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../../settings/state/settings_controller.dart';
import '../state/promotion_rules.dart';
import '../state/promotions_workspace_controller.dart';
import 'promotion_reference_picker.dart';
import 'promotions_design.dart';

class PromotionEditor extends StatefulWidget {
  const PromotionEditor({
    super.key,
    required this.runtime,
    required this.controller,
    this.record,
  });
  final VendorRuntime runtime;
  final PromotionsWorkspaceController controller;
  final JsonObject? record;
  @override
  State<PromotionEditor> createState() => _PromotionEditorState();
}

class _PromotionEditorState extends State<PromotionEditor> {
  final fields = <String, TextEditingController>{};
  final form = GlobalKey<FormState>();
  late String type, combination;
  late bool percentage, couponRequired;
  late DateTime start, end;
  List<JsonObject> branches = [], scopes = [], bundles = [], tiers = [];
  bool loadingBranches = true;
  String? branchError;
  bool get mock => widget.runtime.mode == VendorDataMode.mock;
  PromotionsWorkspaceController get c => widget.controller;
  @override
  void initState() {
    super.initState();
    final row = widget.record;
    final now = DateTime.now();
    type = row?['promotion_type'] ?? 'PERCENTAGE_DISCOUNT';
    combination = row?['combinability'] ?? 'EXCLUSIVE';
    percentage = row == null || (row['percentage_bps'] as int? ?? 0) > 0;
    couponRequired = row?['coupon_required'] == true;
    start =
        DateTime.tryParse(row?['starts_at']?.toString() ?? '')?.toLocal() ??
        now;
    end =
        DateTime.tryParse(row?['ends_at']?.toString() ?? '')?.toLocal() ??
        now.add(const Duration(days: 7));
    for (final key in [
      'name',
      'currency',
      'percentage_bps',
      'fixed_amount_minor',
      'minimum_order_minor',
      'max_discount_minor',
      'global_usage_limit',
      'per_customer_usage_limit',
      'priority',
    ]) {
      String value;
      if ([
        'percentage_bps',
        'fixed_amount_minor',
        'minimum_order_minor',
        'max_discount_minor',
      ].contains(key)) {
        value = row == null
            ? switch (key) {
                'percentage_bps' => mock ? '10' : '',
                'minimum_order_minor' => '0',
                _ => '',
              }
            : promotionAmount(row[key]);
      } else {
        value =
            row?[key]?.toString() ??
            switch (key) {
              'name' => mock ? 'Lunch offer' : '',
              'currency' => mock ? 'NGN' : '',
              'priority' => '100',
              _ => '',
            };
      }
      fields[key] = TextEditingController(text: value);
    }
    scopes = (row?['scopes'] as List? ?? []).map(jsonObject).toList();
    bundles = (row?['bundle_requirements'] as List? ?? [])
        .map(jsonObject)
        .toList();
    tiers = (row?['quantity_tiers'] as List? ?? []).map(jsonObject).toList()
      ..sort(
        (a, b) => (a['minimum_quantity'] as int).compareTo(
          b['minimum_quantity'] as int,
        ),
      );
    unawaited(loadBranches());
  }

  @override
  void dispose() {
    for (final field in fields.values) {
      field.dispose();
    }
    super.dispose();
  }

  Future<void> loadBranches() async {
    if (!loadingBranches && mounted) {
      setState(() {
        loadingBranches = true;
        branchError = null;
      });
    }
    final settings = SettingsController(
      widget.runtime.backend,
      SettingsKind.branches,
    );
    try {
      final rows = await settings.all(
        '${SettingsController.business}/branches',
        settings.scope,
      );
      if (!mounted) return;
      setState(() {
        branches = rows;
        if (widget.record == null && scopes.isEmpty && rows.isNotEmpty) {
          final selected =
              rows
                  .where(
                    (r) => r['id'] == widget.runtime.backend.scope.branchId,
                  )
                  .firstOrNull ??
              rows.first;
          scopes.add({'type': 'BRANCH', 'ref': selected['id']});
        }
      });
    } catch (_) {
      if (mounted) {
        setState(
          () => branchError =
              'Branches could not be loaded. Retry or use an existing branch reference.',
        );
      }
    } finally {
      settings.dispose();
      if (mounted) setState(() => loadingBranches = false);
    }
    if (widget.record == null && fields['currency']!.text.isEmpty) {
      try {
        final response = await widget.runtime.backend.businessApi.request(
          'GET',
          SettingsController.business,
          pathParameters: {
            'businessId': widget.runtime.backend.scope.vendorId!,
          },
        );
        if (mounted &&
            fields['currency']!.text.isEmpty &&
            response.object['currency'] is String) {
          fields['currency']!.text = response.object['currency'];
        }
      } catch (_) {}
    }
  }

  Widget field(
    String key,
    String label, {
    bool optional = false,
    bool integer = false,
    bool text = false,
  }) => Padding(
    padding: const EdgeInsets.only(top: 14),
    child: TextFormField(
      controller: fields[key],
      keyboardType: text
          ? TextInputType.text
          : integer
          ? TextInputType.number
          : const TextInputType.numberWithOptions(decimal: true),
      textCapitalization: key == 'currency'
          ? TextCapitalization.characters
          : TextCapitalization.none,
      decoration: InputDecoration(
        labelText: label,
        hintText: optional ? 'Optional' : null,
      ),
      validator: (value) {
        final input = value?.trim() ?? '';
        if (input.isEmpty) return optional ? null : 'Required';
        if (!text &&
            (integer
                ? int.tryParse(input) == null
                : promotionDecimal(input) == null)) {
          return integer
              ? 'Use a whole number'
              : 'Use up to two decimal places';
        }
        return null;
      },
    ),
  );
  Widget section(String title, IconData icon, List<Widget> children) => Padding(
    padding: const EdgeInsets.only(bottom: 18),
    child: PromotionSurface(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(
                icon,
                color: KarryamColors.tealInk.forInk(context),
                size: 22,
              ),
              const SizedBox(width: 10),
              Expanded(
                child: Text(
                  title,
                  style: const TextStyle(
                    fontSize: 18,
                    fontWeight: FontWeight.w700,
                  ),
                ),
              ),
            ],
          ),
          const SizedBox(height: 8),
          ...children,
        ],
      ),
    ),
  );
  Future<void> date(bool first) async {
    final current = first ? start : end;
    final selected = await showDatePicker(
      context: context,
      initialDate: current,
      firstDate: DateTime(current.year - 10),
      lastDate: DateTime(current.year + 20),
    );
    if (selected == null || !mounted) return;
    final time = await showTimePicker(
      context: context,
      initialTime: TimeOfDay.fromDateTime(current),
    );
    if (time == null || !mounted) return;
    final at = DateTime(
      selected.year,
      selected.month,
      selected.day,
      time.hour,
      time.minute,
    );
    setState(() {
      if (first) {
        start = at;
      } else {
        end = at;
      }
    });
  }

  String scopeLabel(JsonObject target) => target['type'] == 'BRANCH'
      ? (branches
                    .where((b) => b['id'] == target['ref'])
                    .firstOrNull?['display_name'] ??
                target['ref'])
            .toString()
      : '${dashboardLabel(target['type'])} · ${target['label'] ?? target['ref']}';
  Future<void> scope() async {
    final target = await showModalBottomSheet<JsonObject>(
      context: context,
      showDragHandle: true,
      useSafeArea: true,
      isScrollControlled: true,
      builder: (_) => _ScopeSheet(runtime: widget.runtime, branches: branches),
    );
    if (target == null || !mounted) return;
    if (scopes.any(
      (row) => row['type'] == target['type'] && row['ref'] == target['ref'],
    )) {
      return;
    }
    setState(() => scopes.add(target));
  }

  Future<void> bundle([int? index]) async {
    final value = await showModalBottomSheet<JsonObject>(
      context: context,
      showDragHandle: true,
      useSafeArea: true,
      isScrollControlled: true,
      builder: (_) => _BundleSheet(
        runtime: widget.runtime,
        record: index == null ? null : bundles[index],
      ),
    );
    if (value != null && mounted) {
      setState(() {
        if (index == null) {
          bundles.add(value);
        } else {
          bundles[index] = value;
        }
      });
    }
  }

  Future<void> tier([int? index]) async {
    final value = await showModalBottomSheet<JsonObject>(
      context: context,
      showDragHandle: true,
      useSafeArea: true,
      isScrollControlled: true,
      builder: (_) => _TierSheet(
        currency: fields['currency']!.text,
        record: index == null ? null : tiers[index],
        nextMinimum: tiers.isEmpty
            ? 1
            : (tiers.last['maximum_quantity'] as int? ??
                      tiers.last['minimum_quantity'] as int) +
                  1,
        mock: mock,
      ),
    );
    if (value != null && mounted) {
      setState(() {
        if (index == null) {
          tiers.add(value);
        } else {
          tiers[index] = value;
        }
        tiers.sort(
          (a, b) => (a['minimum_quantity'] as int).compareTo(
            b['minimum_quantity'] as int,
          ),
        );
      });
    }
  }

  JsonObject payload() {
    final percentType =
        [
          'PERCENTAGE_DISCOUNT',
          'DELIVERY_PERCENTAGE_DISCOUNT',
        ].contains(type) ||
        ['COUPON', 'BUNDLE'].contains(type) && percentage;
    final fixedType =
        ['FIXED_DISCOUNT', 'DELIVERY_FLAT_DISCOUNT'].contains(type) ||
        ['COUPON', 'BUNDLE'].contains(type) && !percentage;
    return {
      'name': fields['name']!.text.trim(),
      'promotion_type': type,
      'currency': fields['currency']!.text.trim().toUpperCase(),
      'percentage_bps': percentType
          ? promotionDecimal(fields['percentage_bps']!.text)
          : 0,
      'fixed_amount_minor': fixedType
          ? promotionDecimal(fields['fixed_amount_minor']!.text)
          : 0,
      'minimum_order_minor': promotionDecimal(
        fields['minimum_order_minor']!.text,
      ),
      'max_discount_minor': fields['max_discount_minor']!.text.trim().isEmpty
          ? null
          : promotionDecimal(fields['max_discount_minor']!.text),
      'starts_at': start.toUtc().toIso8601String(),
      'ends_at': end.toUtc().toIso8601String(),
      'funding_sponsor': 'VENDOR',
      'vendor_funding_bps': 10000,
      'coupon_required': type == 'COUPON' || couponRequired,
      'global_usage_limit': int.tryParse(
        fields['global_usage_limit']!.text.trim(),
      ),
      'per_customer_usage_limit': int.tryParse(
        fields['per_customer_usage_limit']!.text.trim(),
      ),
      'combinability': combination,
      'priority': int.tryParse(fields['priority']!.text.trim()),
      'scopes': [
        for (final target in scopes)
          {'type': target['type'], 'ref': target['ref']},
      ],
      'bundle_requirements': type == 'BUNDLE'
          ? [
              for (final row in bundles)
                {
                  'item_ref': row['item_ref'],
                  'variant_ref': row['variant_ref'] ?? '',
                  'minimum_quantity': row['minimum_quantity'],
                },
            ]
          : <JsonObject>[],
      'quantity_tiers': type == 'QUANTITY_BREAK' ? tiers : <JsonObject>[],
    };
  }

  Future<void> review() async {
    if (!form.currentState!.validate()) return;
    final body = payload();
    final error = validatePromotion(body);
    if (error != null) {
      await showOrderFeedback(
        context,
        title: 'Check your offer',
        message: error,
        warning: true,
      );
      return;
    }
    final confirm = await showModalBottomSheet<bool>(
      context: context,
      showDragHandle: true,
      useSafeArea: true,
      isScrollControlled: true,
      builder: (context) => SingleChildScrollView(
        padding: const EdgeInsets.fromLTRB(20, 0, 20, 24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Text(
              'Review your offer',
              style: TextStyle(fontSize: 23, fontWeight: FontWeight.w700),
            ),
            const SizedBox(height: 20),
            Text(
              promotionBenefit(body),
              style: const TextStyle(
                fontSize: 30,
                fontWeight: FontWeight.w800,
                letterSpacing: -1,
              ),
            ),
            const SizedBox(height: 6),
            Text(body['name']),
            const SizedBox(height: 18),
            PromotionFact(
              'Audience',
              scopes.isEmpty
                  ? 'Business-wide'
                  : scopes.map(scopeLabel).join('\n'),
            ),
            PromotionFact(
              'Starts',
              promotionDate(body['starts_at'], time: true),
            ),
            PromotionFact('Ends', promotionDate(body['ends_at'], time: true)),
            PromotionFact(
              'Minimum spend',
              dashboardMoney(body['minimum_order_minor'], body['currency']),
            ),
            PromotionFact(
              'Discount cap',
              body['max_discount_minor'] == null
                  ? 'Not set'
                  : dashboardMoney(
                      body['max_discount_minor'],
                      body['currency'],
                    ),
            ),
            PromotionFact('Funding', '100% your business'),
            const SizedBox(height: 12),
            Text(
              widget.record == null
                  ? 'This creates a draft. Activate or schedule it from Offer details.'
                  : 'This replaces the saved offer rules. Its current status stays unchanged.',
            ),
            const SizedBox(height: 20),
            SizedBox(
              width: double.infinity,
              child: FilledButton(
                onPressed: () => Navigator.pop(context, true),
                child: Text(
                  widget.record == null ? 'Save draft' : 'Save changes',
                ),
              ),
            ),
          ],
        ),
      ),
    );
    if (confirm != true || !mounted) return;
    final saved = await c.save(
      method: widget.record == null ? 'POST' : 'PATCH',
      path: widget.record == null
          ? promotionBase
          : '$promotionBase/{promotionId}',
      body: body,
      promotionId: widget.record?['id'],
      version: widget.record?['version'],
    );
    if (!mounted) return;
    await showOrderFeedback(
      context,
      title: saved != null ? 'Offer saved' : 'Offer not confirmed',
      message: saved != null
          ? c.error ??
                (widget.record == null
                    ? 'Your draft is ready. Activate or schedule it when you are ready.'
                    : 'Your changes have been saved.')
          : c.error ?? 'Try again.',
      success: saved != null && c.error == null,
      warning: c.pending != null || saved != null && c.error != null,
    );
    if (mounted && (saved != null || c.pending != null || c.stale)) {
      Navigator.pop(context, saved != null);
    }
  }

  @override
  Widget build(BuildContext context) {
    final flexible = ['COUPON', 'BUNDLE'].contains(type);
    final percentType =
        [
          'PERCENTAGE_DISCOUNT',
          'DELIVERY_PERCENTAGE_DISCOUNT',
        ].contains(type) ||
        flexible && percentage;
    final fixedType =
        ['FIXED_DISCOUNT', 'DELIVERY_FLAT_DISCOUNT'].contains(type) ||
        flexible && !percentage;
    return AnimatedBuilder(
      animation: c,
      builder: (context, _) => Scaffold(
        appBar: AppBar(
          title: Text(
            widget.record == null ? 'Create promotion' : 'Edit offer',
          ),
        ),
        body: Form(
          key: form,
          child: ListView(
            padding: const EdgeInsets.fromLTRB(20, 12, 20, 30),
            children: [
              section('The offer', Icons.sell_outlined, [
                field('name', 'Promotion name', text: true),
                const SizedBox(height: 16),
                DropdownButtonFormField<String>(
                  initialValue: type,
                  isExpanded: true,
                  decoration: const InputDecoration(labelText: 'Offer type'),
                  items: [
                    for (final entry in promotionTypes.entries)
                      DropdownMenuItem(
                        value: entry.key,
                        child: Text(
                          entry.value,
                          overflow: TextOverflow.ellipsis,
                        ),
                      ),
                  ],
                  onChanged: (value) => setState(() => type = value!),
                ),
                field('currency', 'Currency', text: true),
                if (flexible) ...[
                  const SizedBox(height: 14),
                  Wrap(
                    spacing: 8,
                    children: [
                      ChoiceChip(
                        label: const Text('Percentage'),
                        selected: percentage,
                        onSelected: (_) => setState(() => percentage = true),
                      ),
                      ChoiceChip(
                        label: const Text('Fixed amount'),
                        selected: !percentage,
                        onSelected: (_) => setState(() => percentage = false),
                      ),
                    ],
                  ),
                ],
                if (percentType) field('percentage_bps', 'Discount (%)'),
                if (fixedType) field('fixed_amount_minor', 'Discount amount'),
                if (type == 'FREE_DELIVERY')
                  const Padding(
                    padding: EdgeInsets.only(top: 14),
                    child: Text(
                      'Your business funds the eligible delivery discount.',
                    ),
                  ),
                field('minimum_order_minor', 'Minimum order amount'),
                field(
                  'max_discount_minor',
                  'Maximum discount amount',
                  optional: true,
                ),
                if (type != 'COUPON')
                  SwitchListTile.adaptive(
                    contentPadding: EdgeInsets.zero,
                    title: const Text('Require a coupon'),
                    value: couponRequired,
                    onChanged: (value) =>
                        setState(() => couponRequired = value),
                  ),
              ]),
              if (type == 'BUNDLE')
                section('Bundle items', Icons.widgets_outlined, [
                  const Text(
                    'Each selected item must meet its required quantity.',
                  ),
                  for (var i = 0; i < bundles.length; i++)
                    ListTile(
                      contentPadding: EdgeInsets.zero,
                      title: Text(
                        (bundles[i]['label'] ?? bundles[i]['item_ref'])
                            .toString(),
                      ),
                      subtitle: Text(
                        '${bundles[i]['minimum_quantity']} required${bundles[i]['variant_ref'] == '' ? '' : ' · ${bundles[i]['variant_ref']}'}',
                      ),
                      onTap: () => bundle(i),
                      trailing: IconButton(
                        tooltip: 'Remove item',
                        icon: const Icon(Icons.close_rounded),
                        onPressed: () => setState(() => bundles.removeAt(i)),
                      ),
                    ),
                  TextButton.icon(
                    onPressed: bundle,
                    icon: const Icon(Icons.add_rounded),
                    label: const Text('Add required item'),
                  ),
                ]),
              if (type == 'QUANTITY_BREAK')
                section('Quantity tiers', Icons.stacked_bar_chart_rounded, [
                  const Text(
                    'Ranges must be consecutive. Leave the final maximum empty for no upper limit.',
                  ),
                  for (var i = 0; i < tiers.length; i++)
                    ListTile(
                      contentPadding: EdgeInsets.zero,
                      title: Text(
                        '${tiers[i]['minimum_quantity']}–${tiers[i]['maximum_quantity'] ?? 'up'} units',
                      ),
                      subtitle: Text(
                        promotionBenefit({
                          ...tiers[i],
                          'promotion_type': 'COUPON',
                          'currency': fields['currency']!.text,
                        }),
                      ),
                      onTap: () => tier(i),
                      trailing: IconButton(
                        tooltip: 'Remove tier',
                        icon: const Icon(Icons.close_rounded),
                        onPressed: () => setState(() => tiers.removeAt(i)),
                      ),
                    ),
                  TextButton.icon(
                    onPressed: tier,
                    icon: const Icon(Icons.add_rounded),
                    label: const Text('Add quantity tier'),
                  ),
                ]),
              section('Audience', Icons.storefront_outlined, [
                if (loadingBranches)
                  const Padding(
                    padding: EdgeInsets.all(12),
                    child: Center(
                      child: CircularProgressIndicator(strokeWidth: 2),
                    ),
                  )
                else if (branchError != null) ...[
                  Text(branchError!),
                  TextButton(
                    onPressed: loadBranches,
                    child: const Text('Retry branches'),
                  ),
                ],
                for (final branch in branches)
                  CheckboxListTile(
                    contentPadding: EdgeInsets.zero,
                    controlAffinity: ListTileControlAffinity.leading,
                    title: Text(
                      (branch['display_name'] ?? branch['id']).toString(),
                    ),
                    value: scopes.any(
                      (s) => s['type'] == 'BRANCH' && s['ref'] == branch['id'],
                    ),
                    onChanged: (selected) => setState(() {
                      scopes.removeWhere(
                        (s) =>
                            s['type'] == 'BRANCH' && s['ref'] == branch['id'],
                      );
                      if (selected == true) {
                        scopes.add({'type': 'BRANCH', 'ref': branch['id']});
                      }
                    }),
                  ),
                for (final target in scopes.where(
                  (s) =>
                      s['type'] != 'BRANCH' ||
                      !branches.any((b) => b['id'] == s['ref']),
                ))
                  ListTile(
                    contentPadding: EdgeInsets.zero,
                    title: Text(scopeLabel(target)),
                    trailing: IconButton(
                      tooltip: 'Remove audience rule',
                      icon: const Icon(Icons.close_rounded),
                      onPressed: () => setState(() => scopes.remove(target)),
                    ),
                  ),
                TextButton.icon(
                  onPressed: scope,
                  icon: const Icon(Icons.add_rounded),
                  label: const Text('Add audience rule'),
                ),
                if (scopes.isEmpty)
                  const Text(
                    'No audience rules means business-wide. This requires business-wide promotion access.',
                  ),
              ]),
              section('Schedule', Icons.event_outlined, [
                for (final first in [true, false])
                  ListTile(
                    contentPadding: EdgeInsets.zero,
                    title: Text(first ? 'Starts' : 'Ends'),
                    subtitle: Text(
                      promotionDate(
                        (first ? start : end).toIso8601String(),
                        time: true,
                      ),
                    ),
                    trailing: const Icon(Icons.edit_calendar_outlined),
                    onTap: () => date(first),
                  ),
                Text(
                  'Times shown in your device timezone.',
                  style: TextStyle(
                    fontSize: 12,
                    color: Theme.of(context).colorScheme.onSurfaceVariant,
                  ),
                ),
              ]),
              section('Limits & combination', Icons.tune_rounded, [
                field(
                  'global_usage_limit',
                  'Total usage limit',
                  integer: true,
                  optional: true,
                ),
                field(
                  'per_customer_usage_limit',
                  'Per-customer usage limit',
                  integer: true,
                  optional: true,
                ),
                const SizedBox(height: 16),
                DropdownButtonFormField<String>(
                  initialValue: combination,
                  decoration: const InputDecoration(
                    labelText: 'Offer combination',
                  ),
                  items: const [
                    DropdownMenuItem(
                      value: 'EXCLUSIVE',
                      child: Text('Exclusive'),
                    ),
                    DropdownMenuItem(
                      value: 'STACKABLE',
                      child: Text('Stackable'),
                    ),
                  ],
                  onChanged: (value) => setState(() => combination = value!),
                ),
                const SizedBox(height: 10),
                Text(
                  combination == 'EXCLUSIVE'
                      ? 'The selected offer is applied on its own.'
                      : 'This offer can be combined with other eligible stackable offers.',
                ),
                field('priority', 'Priority', integer: true),
                const SizedBox(height: 10),
                const Text('Lower priority numbers are evaluated first.'),
              ]),
            ],
          ),
        ),
        bottomNavigationBar: SafeArea(
          top: false,
          minimum: const EdgeInsets.fromLTRB(20, 10, 20, 14),
          child: FilledButton(
            onPressed:
                c.writable &&
                    (widget.record == null ||
                        widget.record!['funding_sponsor'] == 'VENDOR')
                ? review
                : null,
            child: PromotionBusyLabel('Review offer', c.busy),
          ),
        ),
      ),
    );
  }
}

class _ScopeSheet extends StatefulWidget {
  const _ScopeSheet({required this.runtime, required this.branches});
  final VendorRuntime runtime;
  final List<JsonObject> branches;
  @override
  State<_ScopeSheet> createState() => _ScopeSheetState();
}

class _ScopeSheetState extends State<_ScopeSheet> {
  String type = 'ITEM';
  final ref = TextEditingController();
  String? label, error;
  @override
  void dispose() {
    ref.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => _EditSheet(
    title: 'Audience rule',
    children: [
      DropdownButtonFormField<String>(
        initialValue: type,
        decoration: const InputDecoration(labelText: 'Applies to'),
        items: [
          for (final value in promotionScopeTypes)
            DropdownMenuItem(value: value, child: Text(dashboardLabel(value))),
        ],
        onChanged: (value) => setState(() {
          type = value!;
          ref.clear();
          label = null;
          error = null;
        }),
      ),
      const SizedBox(height: 16),
      if (type == 'BRANCH' && widget.branches.isNotEmpty)
        DropdownButtonFormField<String>(
          key: const ValueKey('branch'),
          isExpanded: true,
          decoration: const InputDecoration(labelText: 'Branch'),
          items: [
            for (final row in widget.branches)
              DropdownMenuItem(
                value: row['id'] as String,
                child: Text(
                  (row['display_name'] ?? row['id']).toString(),
                  overflow: TextOverflow.ellipsis,
                ),
              ),
          ],
          onChanged: (value) => ref.text = value!,
        ),
      if (type != 'BRANCH' || widget.branches.isEmpty)
        TextField(
          controller: ref,
          decoration: InputDecoration(
            labelText:
                'Existing ${dashboardLabel(type).toLowerCase()} reference',
          ),
        ),
      if (['ITEM', 'VARIANT'].contains(type))
        TextButton.icon(
          onPressed: () async {
            final picked = await pickPromotionItem(
              context,
              widget.runtime,
              variant: type == 'VARIANT',
            );
            if (picked == null || !mounted) return;
            setState(() {
              ref.text = picked[type == 'VARIANT' ? 'variant_ref' : 'item_ref'];
              label = picked['label'];
            });
          },
          icon: const Icon(Icons.search_rounded),
          label: Text(label ?? 'Choose from catalogue'),
        ),
      if (!['ITEM', 'VARIANT', 'BRANCH'].contains(type))
        const Padding(
          padding: EdgeInsets.only(top: 12),
          child: Text(
            'Use an existing backend reference. This does not create a category, capability or customer segment.',
          ),
        ),
      if (error != null)
        Text(
          error!,
          style: TextStyle(color: Theme.of(context).colorScheme.error),
        ),
      const SizedBox(height: 20),
      FilledButton(
        onPressed: () {
          if (ref.text.trim().isEmpty) {
            setState(() => error = 'Select or enter an existing reference.');
            return;
          }
          Navigator.pop(context, <String, dynamic>{
            'type': type,
            'ref': ref.text.trim(),
            if (label != null) 'label': label,
          });
        },
        child: const Text('Add rule'),
      ),
    ],
  );
}

class _BundleSheet extends StatefulWidget {
  const _BundleSheet({required this.runtime, this.record});
  final VendorRuntime runtime;
  final JsonObject? record;
  @override
  State<_BundleSheet> createState() => _BundleSheetState();
}

class _BundleSheetState extends State<_BundleSheet> {
  late final item = TextEditingController(text: widget.record?['item_ref']);
  late final variant = TextEditingController(
    text: widget.record?['variant_ref'],
  );
  late final quantity = TextEditingController(
    text: widget.record?['minimum_quantity']?.toString() ?? '1',
  );
  late String? label = widget.record?['label'];
  String? error;
  @override
  void dispose() {
    item.dispose();
    variant.dispose();
    quantity.dispose();
    super.dispose();
  }

  Future<void> pick(bool withVariant) async {
    final result = await pickPromotionItem(
      context,
      widget.runtime,
      variant: withVariant,
    );
    if (result == null || !mounted) return;
    setState(() {
      item.text = result['item_ref'];
      variant.text = result['variant_ref'];
      label = result['label'];
    });
  }

  @override
  Widget build(BuildContext context) => _EditSheet(
    title: 'Required bundle item',
    children: [
      if (label != null)
        Padding(
          padding: const EdgeInsets.only(bottom: 12),
          child: Text(
            label!,
            style: const TextStyle(fontWeight: FontWeight.w700),
          ),
        ),
      Wrap(
        spacing: 8,
        children: [
          TextButton.icon(
            onPressed: () => pick(false),
            icon: const Icon(Icons.restaurant_outlined),
            label: const Text('Choose item'),
          ),
          TextButton.icon(
            onPressed: () => pick(true),
            icon: const Icon(Icons.style_outlined),
            label: const Text('Choose variant'),
          ),
        ],
      ),
      const SizedBox(height: 10),
      TextField(
        controller: item,
        decoration: const InputDecoration(labelText: 'Existing item reference'),
      ),
      const SizedBox(height: 14),
      TextField(
        controller: variant,
        decoration: const InputDecoration(
          labelText: 'Variant reference (optional)',
        ),
      ),
      const SizedBox(height: 14),
      TextField(
        controller: quantity,
        keyboardType: TextInputType.number,
        decoration: const InputDecoration(labelText: 'Required quantity'),
      ),
      if (error != null)
        Padding(
          padding: const EdgeInsets.only(top: 10),
          child: Text(
            error!,
            style: TextStyle(color: Theme.of(context).colorScheme.error),
          ),
        ),
      const SizedBox(height: 20),
      FilledButton(
        onPressed: () {
          final min = int.tryParse(quantity.text.trim());
          if (item.text.trim().isEmpty || min == null || min < 1) {
            setState(
              () => error = 'Choose an item and enter a positive quantity.',
            );
            return;
          }
          Navigator.pop(context, <String, dynamic>{
            'item_ref': item.text.trim(),
            'variant_ref': variant.text.trim(),
            'minimum_quantity': min,
            if (label != null) 'label': label,
          });
        },
        child: const Text('Save item'),
      ),
    ],
  );
}

class _TierSheet extends StatefulWidget {
  const _TierSheet({
    required this.currency,
    required this.nextMinimum,
    required this.mock,
    this.record,
  });
  final String currency;
  final int nextMinimum;
  final bool mock;
  final JsonObject? record;
  @override
  State<_TierSheet> createState() => _TierSheetState();
}

class _TierSheetState extends State<_TierSheet> {
  late final min = TextEditingController(
    text: '${widget.record?['minimum_quantity'] ?? widget.nextMinimum}',
  );
  late final max = TextEditingController(
    text: widget.record?['maximum_quantity']?.toString(),
  );
  late bool percent =
      widget.record == null || (widget.record!['percentage_bps'] as int) > 0;
  late final discount = TextEditingController(
    text: widget.record == null
        ? widget.mock
              ? '10'
              : ''
        : promotionAmount(
            widget.record![percent ? 'percentage_bps' : 'fixed_amount_minor'],
          ),
  );
  String? error;
  @override
  void dispose() {
    min.dispose();
    max.dispose();
    discount.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => _EditSheet(
    title: 'Quantity tier',
    children: [
      TextField(
        controller: min,
        keyboardType: TextInputType.number,
        decoration: const InputDecoration(labelText: 'Minimum quantity'),
      ),
      const SizedBox(height: 14),
      TextField(
        controller: max,
        keyboardType: TextInputType.number,
        decoration: const InputDecoration(
          labelText: 'Maximum quantity (optional)',
        ),
      ),
      const SizedBox(height: 14),
      Wrap(
        spacing: 8,
        children: [
          ChoiceChip(
            label: const Text('Percentage'),
            selected: percent,
            onSelected: (_) => setState(() {
              percent = true;
              discount.clear();
            }),
          ),
          ChoiceChip(
            label: const Text('Fixed amount'),
            selected: !percent,
            onSelected: (_) => setState(() {
              percent = false;
              discount.clear();
            }),
          ),
        ],
      ),
      const SizedBox(height: 14),
      TextField(
        controller: discount,
        keyboardType: const TextInputType.numberWithOptions(decimal: true),
        decoration: InputDecoration(
          labelText: percent ? 'Discount (%)' : 'Discount (${widget.currency})',
        ),
      ),
      if (error != null)
        Padding(
          padding: const EdgeInsets.only(top: 10),
          child: Text(
            error!,
            style: TextStyle(color: Theme.of(context).colorScheme.error),
          ),
        ),
      const SizedBox(height: 20),
      FilledButton(
        onPressed: () {
          final lower = int.tryParse(min.text.trim()),
              upper = int.tryParse(max.text.trim()),
              amount = promotionDecimal(discount.text);
          if (lower == null ||
              lower < 1 ||
              max.text.trim().isNotEmpty && (upper == null || upper < lower) ||
              amount == null ||
              amount <= 0 ||
              percent && amount > 10000) {
            setState(() => error = 'Check the quantity range and discount.');
            return;
          }
          Navigator.pop(context, <String, dynamic>{
            'minimum_quantity': lower,
            'maximum_quantity': upper,
            'percentage_bps': percent ? amount : 0,
            'fixed_amount_minor': percent ? 0 : amount,
          });
        },
        child: const Text('Save tier'),
      ),
    ],
  );
}

class _EditSheet extends StatelessWidget {
  const _EditSheet({required this.title, required this.children});
  final String title;
  final List<Widget> children;
  @override
  Widget build(BuildContext context) => SingleChildScrollView(
    padding: EdgeInsets.fromLTRB(
      20,
      0,
      20,
      MediaQuery.viewInsetsOf(context).bottom + 24,
    ),
    child: Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text(
          title,
          style: const TextStyle(fontSize: 23, fontWeight: FontWeight.w700),
        ),
        const SizedBox(height: 20),
        ...children,
      ],
    ),
  );
}
