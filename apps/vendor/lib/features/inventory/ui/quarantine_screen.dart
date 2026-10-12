import '../../../core/ui/premium_loading.dart';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../settings/ui/settings_design.dart';
import '../state/stock_controller.dart';
import 'stock_screen.dart';

const quarantineReasons = {
  'QUALITY_CHECK': ('Quality check', Icons.fact_check_outlined),
  'DAMAGED': ('Damaged', Icons.broken_image_outlined),
  'RECALL': ('Recall', Icons.assignment_return_outlined),
  'TEMPERATURE_EXCURSION': ('Temperature excursion', Icons.thermostat_rounded),
  'SUSPECTED_SPOILAGE': ('Suspected spoilage', Icons.science_outlined),
  'OTHER_CONTROLLED': ('Other controlled reason', Icons.rule_rounded),
};
Color quarantineColor(Object? status) => switch (status) {
  'ACTIVE' => const Color(0xFF986013),
  'RELEASED' => KarryamColors.tealInk,
  'DISPOSED' => KarryamColors.error,
  _ => const Color(0xFF64768A),
};
String quarantineStatus(Object? value) =>
    value == 'ACTIVE' ? 'On hold' : dashboardLabel(value);
String quarantineDate(BuildContext context, Object? value) {
  final date = DateTime.tryParse(value?.toString() ?? '')?.toLocal();
  if (date == null) return 'Unavailable';
  final local = MaterialLocalizations.of(context);
  return '${local.formatMediumDate(date)} · ${local.formatTimeOfDay(TimeOfDay.fromDateTime(date))}';
}

class QuarantineHero extends StatelessWidget {
  const QuarantineHero({
    super.key,
    required this.title,
    required this.subtitle,
    this.status = 'ACTIVE',
    this.quantity,
  });
  final String title, subtitle, status;
  final int? quantity;
  @override
  Widget build(BuildContext context) {
    final color = quarantineColor(status);
    final icon = status == 'RELEASED'
        ? Icons.verified_outlined
        : status == 'DISPOSED'
        ? Icons.delete_outline_rounded
        : Icons.shield_outlined;
    return SettingsCard(
      color: (color.withValues(alpha: .07)).forInk(context),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              SizedBox(
                width: 68,
                height: 68,
                child: Stack(
                  children: [
                    Positioned.fill(
                      child: Container(
                        decoration: BoxDecoration(
                          color: Colors.white.forSurface(context),
                          borderRadius: BorderRadius.circular(22),
                        ),
                        child: Icon(
                          Icons.inventory_2_outlined,
                          size: 32,
                          color: (color).forInk(context),
                        ),
                      ),
                    ),
                    Positioned(
                      right: 0,
                      bottom: 0,
                      child: Container(
                        padding: const EdgeInsets.all(5),
                        decoration: BoxDecoration(
                          color: (color).forSurface(context),
                          borderRadius: BorderRadius.circular(10),
                        ),
                        child: Icon(
                          icon,
                          color: Colors.white.forInk(context),
                          size: 18,
                        ),
                      ),
                    ),
                  ],
                ),
              ),
              const Spacer(),
              if (quantity != null)
                Text(
                  '$quantity',
                  style: TextStyle(
                    fontSize: 36,
                    fontWeight: FontWeight.w800,
                    color: (color).forInk(context),
                  ),
                ),
            ],
          ),
          const SizedBox(height: 20),
          Text(
            title,
            style: const TextStyle(
              fontSize: 23,
              fontWeight: FontWeight.w800,
              height: 1.15,
            ),
          ),
          const SizedBox(height: 8),
          Text(
            subtitle,
            style: TextStyle(color: (color).forInk(context), height: 1.45),
          ),
        ],
      ),
    );
  }
}

class QuarantineEditor extends StatefulWidget {
  const QuarantineEditor({
    super.key,
    required this.runtime,
    required this.controller,
  });
  final VendorRuntime runtime;
  final StockController controller;
  @override
  State<QuarantineEditor> createState() => _QuarantineEditorState();
}

class _QuarantineEditorState extends State<QuarantineEditor> {
  StockController get c => widget.controller;
  bool get mock => widget.runtime.mode == VendorDataMode.mock;
  bool get locked => c.busy || uncertain || submitted;
  final form = GlobalKey<FormState>();
  late final quantity = TextEditingController(text: mock ? '3' : '');
  late final note = TextEditingController(
    text: mock ? 'Set aside for the quality team.' : '',
  );
  List<JsonObject> choices = [];
  String? basis, reason, error;
  bool loading = true,
      prepared = false,
      presenting = false,
      uncertain = false,
      submitted = false;
  JsonObject? payload;
  @override
  void initState() {
    super.initState();
    prepare();
  }

  @override
  void dispose() {
    quantity.dispose();
    note.dispose();
    super.dispose();
  }

  Future<void> prepare() async {
    setState(() {
      loading = true;
      prepared = false;
      error = null;
    });
    try {
      choices = await c.branchBalances(c.branchId);
      if (mock && basis == null) {
        basis = choices
            .where(
              (r) =>
                  r['physical_available'] is int && r['physical_available'] > 0,
            )
            .firstOrNull?['stock_basis_ref'];
        reason = 'QUALITY_CHECK';
      }
      if (basis != null && !choices.any((r) => r['stock_basis_ref'] == basis)) {
        basis = null;
      }
      prepared = true;
    } catch (e) {
      error = c.message(e);
    }
    if (mounted) setState(() => loading = false);
  }

  Future<void> submit() async {
    if (locked && !uncertain || presenting || !prepared) return;
    if (!uncertain) {
      if (!form.currentState!.validate()) return;
      final units = int.tryParse(quantity.text.trim());
      final stock = choices
          .where((r) => r['stock_basis_ref'] == basis)
          .firstOrNull;
      if (stock == null ||
          !quarantineReasons.containsKey(reason) ||
          units == null ||
          units <= 0 ||
          units > 1000000000 ||
          stock['physical_available'] is! int ||
          units > stock['physical_available']) {
        setState(
          () => error =
              'Choose a stock item, a controlled reason and a valid quantity within available stock.',
        );
        return;
      }
      FocusManager.instance.primaryFocus?.unfocus();
      setState(() {
        presenting = true;
        error = null;
      });
      final confirmed = await settingsSheet<bool>(
        context,
        Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const SettingsHeading('Review quarantine'),
            QuarantineHero(
              title: c.name(
                choices.firstWhere(
                  (r) => r['stock_basis_ref'] == basis,
                )['variant_ref'],
              ),
              subtitle:
                  '${quarantineReasons[reason]!.$1} · ${quantity.text.trim()} stock units',
              quantity: int.parse(quantity.text.trim()),
            ),
            const SizedBox(height: 18),
            const Text(
              'These units will remain on hand but will be excluded from sellable stock until the hold is resolved.',
            ),
            if (note.text.trim().isNotEmpty) ...[
              const SizedBox(height: 12),
              Text(note.text.trim()),
            ],
          ],
        ),
        footer: SettingsAction(
          'Confirm quarantine',
          icon: Icons.shield_outlined,
          onPressed: () => Navigator.pop(context, true),
        ),
      );
      if (!mounted) return;
      setState(() => presenting = false);
      if (confirmed != true) return;
      payload = {
        'stock_basis_id': basis,
        'quantity': int.parse(quantity.text.trim()),
        'reason_code': reason,
        'note': note.text.trim(),
      };
    }
    final result = await c.mutate(
      'POST',
      '${StockController.root}/quarantines',
      body: payload,
    );
    if (!mounted) return;
    setState(() {
      submitted = result != null;
      uncertain = result == null && c.uncertainOutcome;
      error = result == null ? c.error : null;
    });
    await stockFeedback(
      context,
      c,
      result,
      'Stock quarantined',
      'The selected units are on hold and excluded from sale.',
    );
    if (!mounted) return;
    if (result != null) {
      Navigator.pop(context, result);
      return;
    }
    if (!uncertain) await prepare();
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => PopScope(
      canPop: !c.busy && !uncertain,
      child: Scaffold(
        backgroundColor: KarryamColors.canvas.forSurface(context),
        appBar: AppBar(title: const Text('Quarantine stock')),
        bottomNavigationBar: loading
            ? null
            : SafeArea(
                child: Padding(
                  padding: const EdgeInsets.fromLTRB(22, 12, 22, 18),
                  child: SettingsAction(
                    uncertain ? 'Retry same submission' : 'Review quarantine',
                    icon: Icons.arrow_forward_rounded,
                    busy: c.busy,
                    onPressed:
                        !prepared ||
                            presenting ||
                            submitted ||
                            choices.isEmpty ||
                            c.denied.contains(
                              'POST ${StockController.root}/quarantines',
                            )
                        ? null
                        : submit,
                  ),
                ),
              ),
        body: loading
            ? const SingleChildScrollView(
                padding: EdgeInsets.all(22),
                child: PremiumSkeleton(detail: true, rows: 2),
              )
            : Form(
                key: form,
                child: ListView(
                  padding: const EdgeInsets.all(22),
                  children: [
                    const QuarantineHero(
                      title: 'Keep questionable stock out of sale',
                      subtitle:
                          'Set it aside while your team checks its quality.',
                    ),
                    const SizedBox(height: 24),
                    if (error != null) SettingsNote(error!),
                    if (!prepared)
                      SettingsAction(
                        'Try again',
                        secondary: true,
                        onPressed: locked ? null : prepare,
                      ),
                    if (uncertain)
                      const SettingsNote(
                        'The response could not be confirmed. Retry these exact details to avoid a duplicate hold.',
                        warning: true,
                      ),
                    if (prepared && choices.isEmpty)
                      const SettingsNote(
                        'Set up stock tracking and receive goods before creating a quarantine.',
                        warning: true,
                      ),
                    DropdownButtonFormField<String>(
                      key: ValueKey(basis),
                      initialValue: basis,
                      isExpanded: true,
                      decoration: const InputDecoration(
                        labelText: 'Stock item',
                        prefixIcon: Icon(Icons.inventory_2_outlined),
                      ),
                      items: [
                        for (final row in choices)
                          DropdownMenuItem(
                            value: row['stock_basis_ref'].toString(),
                            child: Text(
                              c.name(row['variant_ref']),
                              overflow: TextOverflow.ellipsis,
                            ),
                          ),
                      ],
                      onChanged: locked || presenting
                          ? null
                          : (value) => setState(() => basis = value),
                      validator: (v) =>
                          v == null ? 'Choose a stock item.' : null,
                    ),
                    if (basis != null)
                      Padding(
                        padding: const EdgeInsets.only(top: 12),
                        child: Text(
                          '${choices.firstWhere((r) => r['stock_basis_ref'] == basis)['physical_available']} currently available stock units',
                          style: TextStyle(
                            color: KarryamColors.tealInk.forInk(context),
                          ),
                        ),
                      ),
                    const SizedBox(height: 22),
                    TextFormField(
                      controller: quantity,
                      enabled: !locked && !presenting,
                      keyboardType: TextInputType.number,
                      decoration: const InputDecoration(
                        labelText: 'Quantity to hold',
                        prefixIcon: Icon(Icons.lock_outline_rounded),
                      ),
                      validator: (v) {
                        final n = int.tryParse(v?.trim() ?? '');
                        if (n == null || n <= 0 || n > 1000000000) {
                          return 'Enter a whole quantity from 1 to 1,000,000,000.';
                        }
                        final available = choices
                            .where((r) => r['stock_basis_ref'] == basis)
                            .firstOrNull?['physical_available'];
                        return available is int && n > available
                            ? 'Quantity exceeds currently available stock.'
                            : null;
                      },
                    ),
                    const SizedBox(height: 24),
                    const Text(
                      'Reason for the hold',
                      style: TextStyle(
                        fontSize: 17,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                    const SizedBox(height: 12),
                    FormField<String>(
                      key: ValueKey(reason),
                      initialValue: reason,
                      validator: (_) =>
                          reason == null ? 'Choose a quarantine reason.' : null,
                      builder: (field) => Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          for (final entry in quarantineReasons.entries)
                            Padding(
                              padding: const EdgeInsets.only(bottom: 8),
                              child: Material(
                                color:
                                    (reason == entry.key
                                            ? const Color(0xFFFFF4E3)
                                            : Colors.white)
                                        .forSurface(context),
                                borderRadius: BorderRadius.circular(18),
                                child: InkWell(
                                  borderRadius: BorderRadius.circular(18),
                                  onTap: locked || presenting
                                      ? null
                                      : () =>
                                            setState(() => reason = entry.key),
                                  child: Padding(
                                    padding: const EdgeInsets.all(16),
                                    child: Row(
                                      children: [
                                        Icon(
                                          entry.value.$2,
                                          color: (Color(
                                            0xFF986013,
                                          )).forInk(context),
                                          size: 23,
                                        ),
                                        const SizedBox(width: 14),
                                        Expanded(
                                          child: Text(
                                            entry.value.$1,
                                            style: const TextStyle(
                                              fontWeight: FontWeight.w600,
                                            ),
                                          ),
                                        ),
                                        Icon(
                                          reason == entry.key
                                              ? Icons.check_circle_rounded
                                              : Icons
                                                    .radio_button_unchecked_rounded,
                                          color:
                                              (reason == entry.key
                                                      ? const Color(0xFF986013)
                                                      : const Color(0xFF91A0AA))
                                                  .forInk(context),
                                          size: 21,
                                        ),
                                      ],
                                    ),
                                  ),
                                ),
                              ),
                            ),
                          if (field.errorText != null)
                            Text(
                              field.errorText!,
                              style: TextStyle(
                                color: KarryamColors.error.forInk(context),
                              ),
                            ),
                        ],
                      ),
                    ),
                    const SizedBox(height: 16),
                    TextFormField(
                      controller: note,
                      enabled: !locked && !presenting,
                      maxLines: 3,
                      decoration: const InputDecoration(
                        labelText: 'Note (optional)',
                        alignLabelWithHint: true,
                      ),
                    ),
                  ],
                ),
              ),
      ),
    ),
  );
}

class QuarantineScreen extends StatefulWidget {
  const QuarantineScreen({
    super.key,
    required this.controller,
    required this.id,
  });
  final StockController controller;
  final String id;
  @override
  State<QuarantineScreen> createState() => _QuarantineScreenState();
}

class _QuarantineScreenState extends State<QuarantineScreen> {
  StockController get c => widget.controller;
  JsonObject? row;
  String? error, pendingAction, workingAction;
  bool loading = true, presenting = false;
  @override
  void initState() {
    super.initState();
    refresh();
  }

  Future<void> refresh() async {
    if (c.busy || pendingAction != null) return;
    setState(() {
      loading = true;
      error = null;
    });
    try {
      row = await c.quarantine(widget.id);
    } catch (e) {
      row = null;
      error = c.message(e);
    }
    if (mounted) setState(() => loading = false);
  }

  Future<void> resolve(String action) async {
    if (c.busy || presenting || row == null || row!['status'] != 'ACTIVE') {
      return;
    }
    final dispose = action == 'DISPOSE';
    if (pendingAction == null) {
      setState(() => presenting = true);
      final confirmed = await confirmSettings(
        context,
        dispose ? 'Dispose stock' : 'Release stock',
        dispose
            ? 'Permanently remove all ${row!['quantity']} quarantined units from on-hand stock. This case cannot be reopened.'
            : 'Remove the hold on all ${row!['quantity']} units. Expired stock remains unavailable for sale.',
        danger: dispose,
      );
      if (!mounted) return;
      setState(() => presenting = false);
      if (!confirmed) return;
    } else if (pendingAction != action) {
      return;
    }
    setState(() => workingAction = action);
    final result = await c.mutate(
      'POST',
      '${StockController.root}/quarantines/{quarantineId}/resolve',
      parameters: {'quarantineId': widget.id},
      body: {'action': action},
    );
    if (!mounted) return;
    setState(() {
      pendingAction = result == null && c.uncertainOutcome ? action : null;
      error = result == null ? c.error : null;
      if (result != null) row = result;
    });
    await stockFeedback(
      context,
      c,
      result,
      dispose ? 'Stock disposed' : 'Stock released',
      dispose
          ? 'The quarantined units have been removed from on-hand stock.'
          : 'The quarantine hold has been removed. Expiry and other stock restrictions still apply.',
    );
    if (mounted && pendingAction == null) await refresh();
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => PopScope(
      canPop: !c.busy && pendingAction == null,
      child: Scaffold(
        backgroundColor: KarryamColors.canvas.forSurface(context),
        appBar: AppBar(
          title: const Text('Quarantine details'),
          actions: [
            IconButton(
              tooltip: 'Refresh quarantine',
              onPressed:
                  c.busy || loading || pendingAction != null || presenting
                  ? null
                  : refresh,
              icon: const Icon(Icons.refresh_rounded),
            ),
          ],
        ),
        bottomNavigationBar:
            row == null || loading || row!['status'] != 'ACTIVE'
            ? null
            : SafeArea(
                child: Padding(
                  padding: const EdgeInsets.fromLTRB(22, 12, 22, 18),
                  child: Column(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      if (pendingAction != null)
                        SettingsAction(
                          'Retry same submission',
                          busy: c.busy,
                          icon: Icons.refresh_rounded,
                          onPressed: () => resolve(pendingAction!),
                        )
                      else ...[
                        SettingsAction(
                          'Release stock',
                          busy: c.busy && workingAction == 'RELEASE',
                          icon: Icons.lock_open_rounded,
                          onPressed:
                              c.busy ||
                                  presenting ||
                                  c.denied.contains(
                                    'POST ${StockController.root}/quarantines/{quarantineId}/resolve',
                                  )
                              ? null
                              : () => resolve('RELEASE'),
                        ),
                        const SizedBox(height: 10),
                        SettingsAction(
                          'Dispose stock',
                          busy: c.busy && workingAction == 'DISPOSE',
                          danger: true,
                          icon: Icons.delete_outline_rounded,
                          onPressed:
                              c.busy ||
                                  presenting ||
                                  c.denied.contains(
                                    'POST ${StockController.root}/quarantines/{quarantineId}/resolve',
                                  )
                              ? null
                              : () => resolve('DISPOSE'),
                        ),
                      ],
                    ],
                  ),
                ),
              ),
        body: loading
            ? const SingleChildScrollView(
                padding: EdgeInsets.all(22),
                child: PremiumSkeleton(detail: true, rows: 2),
              )
            : ListView(
                padding: const EdgeInsets.all(22),
                children: [
                  if (error != null) SettingsNote(error!),
                  if (row == null)
                    SettingsAction(
                      'Try again',
                      secondary: true,
                      onPressed: refresh,
                    ),
                  if (pendingAction != null)
                    const SettingsNote(
                      'The response could not be confirmed. Retry the same action before continuing.',
                      warning: true,
                    ),
                  if (row != null) ...[
                    QuarantineHero(
                      title: c.name(row!['variant_ref']),
                      status: row!['status'],
                      quantity: row!['quantity'],
                      subtitle:
                          '${quarantineStatus(row!['status'])} · ${quarantineReasons[row!['reason_code']]?.$1 ?? dashboardLabel(row!['reason_code'])}',
                    ),
                    const SizedBox(height: 20),
                    SettingsCard(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(
                            switch (row!['status']) {
                              'ACTIVE' => 'Excluded from sale',
                              'RELEASED' => 'Hold removed',
                              _ => 'Removed from stock',
                            },
                            style: const TextStyle(
                              fontSize: 19,
                              fontWeight: FontWeight.w700,
                            ),
                          ),
                          const SizedBox(height: 10),
                          Text(switch (row!['status']) {
                            'ACTIVE' =>
                              'These units remain on hand while quarantined. Release the complete hold after checks, or dispose of the stock.',
                            'RELEASED' =>
                              'The hold is resolved. Expiry and other stock restrictions still determine whether these units can be sold.',
                            _ =>
                              'These units have been permanently removed from on-hand stock.',
                          }),
                        ],
                      ),
                    ),
                    const SizedBox(height: 20),
                    SettingsCard(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          const Text(
                            'Record',
                            style: TextStyle(
                              fontSize: 17,
                              fontWeight: FontWeight.w700,
                            ),
                          ),
                          const SizedBox(height: 16),
                          Text(
                            'Created ${quarantineDate(context, row!['created_at'])}',
                          ),
                          if (row!['resolved_at'] != null) ...[
                            const SizedBox(height: 10),
                            Text(
                              'Resolved ${quarantineDate(context, row!['resolved_at'])}',
                            ),
                          ],
                          const SizedBox(height: 14),
                          SelectableText(
                            row!['id'].toString(),
                            style: TextStyle(
                              fontSize: 12,
                              color: Color(0xFF64768A).forInk(context),
                            ),
                          ),
                        ],
                      ),
                    ),
                  ],
                ],
              ),
      ),
    ),
  );
}
