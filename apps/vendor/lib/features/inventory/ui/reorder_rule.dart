import '../../../core/ui/premium_loading.dart';
import 'package:flutter/material.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../settings/ui/settings_design.dart';
import '../state/stock_controller.dart';
import 'stock_screen.dart';

const reorderPath =
    '${StockController.root}/stock-bases/{stockBasisId}/reorder-rule';

class ReorderRuleCard extends StatefulWidget {
  const ReorderRuleCard({
    super.key,
    required this.controller,
    required this.basis,
    this.mock = false,
  });
  final StockController controller;
  final String basis;
  final bool mock;
  @override
  State<ReorderRuleCard> createState() => _ReorderRuleCardState();
}

class _ReorderRuleCardState extends State<ReorderRuleCard> {
  JsonObject? rule;
  String? error;
  bool loading = true, editing = false;
  Future<void> load() async {
    setState(() {
      loading = true;
      error = null;
    });
    try {
      rule = await widget.controller.reorderRule(widget.basis);
    } catch (e) {
      rule = null;
      error = widget.controller.message(e);
    }
    if (mounted) setState(() => loading = false);
  }

  @override
  void initState() {
    super.initState();
    load();
  }

  Future<void> edit() async {
    if (editing || widget.controller.busy) return;
    setState(() => editing = true);
    final result = await showModalBottomSheet<JsonObject>(
      context: context,
      isScrollControlled: true,
      useSafeArea: true,
      isDismissible: false,
      enableDrag: false,
      showDragHandle: true,
      backgroundColor: KarryamColors.canvas.forSurface(context),
      builder: (_) => ReorderRuleEditor(
        controller: widget.controller,
        basis: widget.basis,
        rule: rule,
        mock: widget.mock,
      ),
    );
    if (!mounted) return;
    setState(() => editing = false);
    if (result != null) {
      await stockFeedback(
        context,
        widget.controller,
        result,
        'Reorder rule saved',
        'Your stock thresholds have been updated. Stock quantities are unchanged.',
      );
    }
    if (mounted) await load();
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: widget.controller,
    builder: (context, _) => SettingsCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Container(
                padding: const EdgeInsets.all(12),
                decoration: BoxDecoration(
                  color: KarryamColors.mint.forSurface(context),
                  borderRadius: BorderRadius.circular(16),
                ),
                child: Icon(
                  Icons.autorenew_rounded,
                  color: KarryamColors.tealInk.forInk(context),
                  size: 25,
                ),
              ),
              const SizedBox(width: 13),
              const Expanded(
                child: Text(
                  'Reorder rule',
                  style: TextStyle(fontSize: 19, fontWeight: FontWeight.w700),
                ),
              ),
            ],
          ),
          const SizedBox(height: 20),
          if (loading)
            const PremiumSkeleton(detail: true, rows: 2)
          else if (error != null) ...[
            SettingsNote(error!),
            SettingsAction(
              'Reload reorder rule',
              secondary: true,
              onPressed: widget.controller.busy ? null : load,
            ),
          ] else ...[
            if (rule == null)
              const Text(
                'Set stock thresholds and an optional expiry warning window.',
              )
            else ...[
              if (rule!['status'] != 'ACTIVE')
                SettingsNote(
                  'This rule is ${rule!['status'].toString().toLowerCase()}. Saving activates it.',
                  warning: true,
                ),
              Container(
                width: double.infinity,
                padding: const EdgeInsets.all(18),
                decoration: BoxDecoration(
                  color:
                      ((rule!['recommended_reorder_quantity'] > 0
                                  ? const Color(0xFF986013)
                                  : KarryamColors.tealInk)
                              .withValues(alpha: .07))
                          .forSurface(context),
                  borderRadius: BorderRadius.circular(19),
                ),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    const Text(
                      'Suggested reorder',
                      style: TextStyle(fontSize: 12),
                    ),
                    const SizedBox(height: 4),
                    Text(
                      '${rule!['recommended_reorder_quantity']} stock units',
                      style: TextStyle(
                        fontSize: 23,
                        fontWeight: FontWeight.w800,
                        color:
                            (rule!['recommended_reorder_quantity'] > 0
                                    ? const Color(0xFF986013)
                                    : KarryamColors.tealInk)
                                .forInk(context),
                      ),
                    ),
                    const SizedBox(height: 4),
                    const Text(
                      'Quantity needed to reach your target.',
                      style: TextStyle(fontSize: 12),
                    ),
                  ],
                ),
              ),
              const SizedBox(height: 14),
              SettingsFact('Minimum stock', '${rule!['minimum_quantity']}'),
              SettingsFact('Target stock', '${rule!['target_quantity']}'),
              SettingsFact(
                'Expiry warning',
                rule!['expiring_soon_hours'] == null
                    ? 'Not set'
                    : '${rule!['expiring_soon_hours']} hours',
              ),
            ],
            const SizedBox(height: 18),
            SettingsAction(
              rule == null ? 'Set reorder rule' : 'Edit reorder rule',
              secondary: true,
              icon: Icons.tune_rounded,
              onPressed:
                  editing ||
                      widget.controller.busy ||
                      widget.controller.denied.contains('PUT $reorderPath')
                  ? null
                  : edit,
            ),
          ],
        ],
      ),
    ),
  );
}

class ReorderRuleEditor extends StatefulWidget {
  const ReorderRuleEditor({
    super.key,
    required this.controller,
    required this.basis,
    this.rule,
    this.mock = false,
  });
  final StockController controller;
  final String basis;
  final JsonObject? rule;
  final bool mock;
  @override
  State<ReorderRuleEditor> createState() => _ReorderRuleEditorState();
}

class _ReorderRuleEditorState extends State<ReorderRuleEditor> {
  StockController get c => widget.controller;
  final form = GlobalKey<FormState>();
  late final minimum = TextEditingController(
    text:
        widget.rule?['minimum_quantity']?.toString() ??
        (widget.mock ? '10' : ''),
  );
  late final target = TextEditingController(
    text:
        widget.rule?['target_quantity']?.toString() ??
        (widget.mock ? '30' : ''),
  );
  late final hours = TextEditingController(
    text: widget.rule == null
        ? (widget.mock ? '72' : '')
        : widget.rule!['expiring_soon_hours']?.toString() ?? '',
  );
  bool uncertain = false;
  String? error;
  JsonObject? payload;
  @override
  void dispose() {
    minimum.dispose();
    target.dispose();
    hours.dispose();
    super.dispose();
  }

  Future<void> save() async {
    if (c.busy) return;
    if (!uncertain) {
      if (!form.currentState!.validate()) return;
      payload = {
        'minimum_quantity': int.parse(minimum.text.trim()),
        'target_quantity': int.parse(target.text.trim()),
        'expiring_soon_hours': hours.text.trim().isEmpty
            ? null
            : int.parse(hours.text.trim()),
      };
      FocusManager.instance.primaryFocus?.unfocus();
    }
    final result = await c.mutate(
      'PUT',
      reorderPath,
      parameters: {'stockBasisId': widget.basis},
      body: payload,
    );
    if (!mounted) return;
    if (result != null) {
      Navigator.pop(context, result);
      return;
    }
    setState(() {
      uncertain = c.uncertainOutcome;
      error = c.error;
    });
    await stockFeedback(context, c, null, '', '');
  }

  String? quantityError(String? value, {bool isTarget = false}) {
    final n = int.tryParse(value?.trim() ?? '');
    if (n == null || n < 0 || n > 1000000000) {
      return 'Enter a whole quantity from 0 to 1,000,000,000.';
    }
    if (isTarget && n <= (int.tryParse(minimum.text.trim()) ?? -1)) {
      return 'Target must be greater than minimum stock.';
    }
    return null;
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => PopScope(
      canPop: !c.busy && !uncertain,
      child: Padding(
        padding: EdgeInsets.only(
          bottom: MediaQuery.viewInsetsOf(context).bottom,
        ),
        child: ConstrainedBox(
          constraints: BoxConstraints(
            maxHeight:
                (MediaQuery.sizeOf(context).height * .9 -
                        MediaQuery.viewInsetsOf(context).bottom)
                    .clamp(150, double.infinity),
          ),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Flexible(
                child: SingleChildScrollView(
                  padding: const EdgeInsets.fromLTRB(24, 0, 24, 12),
                  child: Form(
                    key: form,
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Row(
                          children: [
                            const Expanded(
                              child: Text(
                                'Reorder rule',
                                style: TextStyle(
                                  fontSize: 23,
                                  fontWeight: FontWeight.w800,
                                ),
                              ),
                            ),
                            IconButton(
                              tooltip: 'Close reorder rule',
                              onPressed: c.busy || uncertain
                                  ? null
                                  : () => Navigator.pop(context),
                              icon: const Icon(Icons.close_rounded),
                            ),
                          ],
                        ),
                        const SizedBox(height: 10),
                        if (widget.rule != null &&
                            widget.rule!['status'] != 'ACTIVE')
                          SettingsNote(
                            'Saving activates this ${widget.rule!['status'].toString().toLowerCase()} rule.',
                            warning: true,
                          ),
                        const Text(
                          'Stock alerts use the minimum. The target sets the suggested replenishment quantity.',
                        ),
                        const SizedBox(height: 22),
                        if (error != null) SettingsNote(error!),
                        if (uncertain)
                          const SettingsNote(
                            'The response could not be confirmed. Retry the same values before continuing.',
                            warning: true,
                          ),
                        TextFormField(
                          controller: minimum,
                          enabled: !c.busy && !uncertain,
                          keyboardType: TextInputType.number,
                          decoration: const InputDecoration(
                            labelText: 'Minimum stock',
                            errorMaxLines: 3,
                            prefixIcon: Icon(Icons.low_priority_rounded),
                          ),
                          validator: quantityError,
                        ),
                        const SizedBox(height: 18),
                        TextFormField(
                          controller: target,
                          enabled: !c.busy && !uncertain,
                          keyboardType: TextInputType.number,
                          decoration: const InputDecoration(
                            labelText: 'Target stock',
                            errorMaxLines: 3,
                            prefixIcon: Icon(Icons.flag_outlined),
                          ),
                          validator: (v) => quantityError(v, isTarget: true),
                        ),
                        const SizedBox(height: 18),
                        TextFormField(
                          controller: hours,
                          enabled: !c.busy && !uncertain,
                          keyboardType: TextInputType.number,
                          decoration: const InputDecoration(
                            labelText: 'Expiry warning hours (optional)',
                            errorMaxLines: 3,
                            helperMaxLines: 2,
                            helperText:
                                'Leave empty to clear the expiry window.',
                            prefixIcon: Icon(Icons.hourglass_bottom_rounded),
                          ),
                          validator: (v) {
                            if (v == null || v.trim().isEmpty) return null;
                            final n = int.tryParse(v.trim());
                            return n == null || n < 1 || n > 8760
                                ? 'Enter whole hours from 1 to 8,760.'
                                : null;
                          },
                        ),
                        const SizedBox(height: 18),
                        Text(
                          'Saving does not change stock or place an order.',
                          style: TextStyle(
                            fontSize: 12,
                            color: Color(0xFF64768A).forInk(context),
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
              ),
              Padding(
                padding: const EdgeInsets.fromLTRB(24, 8, 24, 24),
                child: SettingsAction(
                  uncertain ? 'Retry same submission' : 'Save reorder rule',
                  busy: c.busy,
                  icon: Icons.check_rounded,
                  onPressed: c.denied.contains('PUT $reorderPath')
                      ? null
                      : save,
                ),
              ),
            ],
          ),
        ),
      ),
    ),
  );
}
