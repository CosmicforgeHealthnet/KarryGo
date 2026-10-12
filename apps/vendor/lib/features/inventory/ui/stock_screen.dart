import '../../../core/ui/premium_loading.dart';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../dashboard/ui/dashboard_empty_state.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../onboarding/ui/setup_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../../settings/ui/settings_design.dart';
import '../state/stock_controller.dart';
import 'stock_editor.dart';
import 'reorder_rule.dart';
import 'quarantine_screen.dart';
import 'transfer_screen.dart';
import 'transfer_editor.dart';

class StockScreen extends StatefulWidget {
  const StockScreen({
    super.key,
    required this.runtime,
    required this.branchId,
    required this.branchName,
  });
  final VendorRuntime runtime;
  final String branchId, branchName;
  @override
  State<StockScreen> createState() => _StockScreenState();
}

class _StockScreenState extends State<StockScreen> {
  late final c = StockController(widget.runtime.backend, widget.branchId);
  @override
  void initState() {
    super.initState();
    c.prepare();
  }

  @override
  void dispose() {
    c.dispose();
    super.dispose();
  }

  Future<void> edit(StockEdit kind, {JsonObject? record}) async {
    final result = await Navigator.push<JsonObject>(
      context,
      setupRoute<JsonObject>(
        StockEditor(
          runtime: widget.runtime,
          controller: c,
          kind: kind,
          record: record,
        ),
      ),
    );
    if (mounted &&
        result != null &&
        {StockEdit.receipt, StockEdit.stocktake}.contains(kind)) {
      await Navigator.push(
        context,
        setupRoute<void>(
          StockRecordScreen(
            controller: c,
            row: result,
            kind: kind == StockEdit.receipt ? 'receipts' : 'stocktakes',
            mock: widget.runtime.mode == VendorDataMode.mock,
          ),
        ),
      );
    }
    if (mounted) await c.load();
  }

  Future<void> newTransfer() async {
    final result = await Navigator.push<JsonObject>(
      context,
      setupRoute<JsonObject>(
        TransferEditor(runtime: widget.runtime, controller: c),
      ),
    );
    if (mounted && result != null) {
      await Navigator.push(
        context,
        setupRoute<void>(
          TransferScreen(
            runtime: widget.runtime,
            controller: c,
            id: result['id'].toString(),
          ),
        ),
      );
    }
    if (mounted) await c.load();
  }

  Future<void> newQuarantine() async {
    final saved = await Navigator.push<JsonObject>(
      context,
      setupRoute<JsonObject>(
        QuarantineEditor(runtime: widget.runtime, controller: c),
      ),
    );
    if (mounted && saved != null) await openQuarantine(saved['id'].toString());
    if (mounted) await c.load();
  }

  Future<void> openQuarantine(String id) => Navigator.push(
    context,
    setupRoute<void>(QuarantineScreen(controller: c, id: id)),
  );
  Future<void> open(JsonObject row) async {
    if (c.section == StockSection.quarantines) {
      await openQuarantine(row["id"].toString());
    } else if (c.section == StockSection.transfers) {
      await Navigator.push(
        context,
        setupRoute<void>(
          TransferScreen(
            runtime: widget.runtime,
            controller: c,
            id: row['id'].toString(),
          ),
        ),
      );
    } else if (c.section == StockSection.balances) {
      await Navigator.push(
        context,
        setupRoute<void>(
          StockBalanceScreen(
            controller: c,
            row: row,
            mock: widget.runtime.mode == VendorDataMode.mock,
            onAdjust: (record) => edit(StockEdit.adjustment, record: record),
          ),
        ),
      );
    } else if ({
      StockSection.receipts,
      StockSection.stocktakes,
    }.contains(c.section)) {
      await Navigator.push(
        context,
        setupRoute<void>(
          StockRecordScreen(
            controller: c,
            row: row,
            kind: c.section.name,
            mock: widget.runtime.mode == VendorDataMode.mock,
          ),
        ),
      );
    } else {
      await settingsSheet<String>(
        context,
        Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            if (row['status'] != null) SettingsStatus(row['status']),
            SettingsFact(
              'Item',
              c.name(row['variant_ref'] ?? row['stock_basis_ref']),
            ),
            for (final key
                in c.section == StockSection.alerts
                    ? ['alert_type', 'severity', 'created_at']
                    : [
                        'movement_type',
                        'quantity',
                        'on_hand_delta',
                        'reserved_delta',
                        'quarantined_delta',
                        'in_transit_delta',
                        'source_type',
                        'source_ref',
                        'reason_code',
                        'occurred_at',
                      ])
              if (row[key] != null)
                SettingsFact(dashboardLabel(key), row[key].toString()),
            if (row['details'] is Map)
              for (final entry in (row['details'] as Map).entries)
                if (entry.value is String ||
                    entry.value is num ||
                    entry.value is bool)
                  SettingsFact(
                    dashboardLabel(entry.key),
                    entry.value.toString(),
                  ),
            if (c.section == StockSection.alerts &&
                row['status'] != 'RESOLVED') ...[
              if (row['status'] == 'OPEN')
                SettingsAction(
                  'Acknowledge',
                  secondary: true,
                  icon: Icons.done_rounded,
                  onPressed: () => Navigator.pop(context, 'acknowledge'),
                ),
              const SizedBox(height: 12),
              SettingsAction(
                'Resolve alert',
                icon: Icons.task_alt_rounded,
                onPressed: () => Navigator.pop(context, 'resolve'),
              ),
            ],
          ],
        ),
      ).then((action) async {
        if (action is! String || !mounted) return;
        final confirmed = await confirmSettings(
          context,
          action == 'resolve'
              ? 'Resolve this alert?'
              : 'Acknowledge this alert?',
          action == 'resolve'
              ? 'This marks the alert as resolved. It does not change stock quantities.'
              : 'Mark this alert as seen by your team.',
        );
        if (confirmed != true || !mounted) return;
        final result = await c.mutate(
          'POST',
          '${StockController.root}/alerts/{alertId}/$action',
          parameters: {'alertId': row['id'].toString()},
        );
        if (mounted) {
          await stockFeedback(
            context,
            c,
            result,
            'Alert updated',
            'The alert status has been updated.',
          );
        }
      });
    }
    if (mounted) await c.load();
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => Scaffold(
      backgroundColor: KarryamColors.canvas.forSurface(context),
      appBar: AppBar(
        title: const Text('Inventory'),
        actions: [
          IconButton(
            tooltip: 'Refresh inventory',
            onPressed: c.loading || c.busy ? null : c.load,
            icon: const Icon(Icons.refresh_rounded),
          ),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: c.load,
        color: KarryamColors.tealInk.forInk(context),
        child: ListView(
          padding: const EdgeInsets.fromLTRB(20, 12, 20, 32),
          children: [
            SettingsCard(
              color: KarryamColors.mint.forInk(context),
              child: Row(
                children: [
                  const StockEmblem(),
                  const SizedBox(width: 20),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        const Text(
                          'Branch stock',
                          style: TextStyle(
                            fontSize: 26,
                            fontWeight: FontWeight.w700,
                            letterSpacing: -.8,
                          ),
                        ),
                        const SizedBox(height: 7),
                        Text(widget.branchName),
                      ],
                    ),
                  ),
                ],
              ),
            ),
            const SizedBox(height: 22),
            SingleChildScrollView(
              scrollDirection: Axis.horizontal,
              child: Row(
                children: [
                  for (final section in StockSection.values)
                    Padding(
                      padding: const EdgeInsets.only(right: 8),
                      child: ChoiceChip(
                        label: Text(stockTitle(section)),
                        avatar: Icon(
                          stockIcon(section),
                          size: 18,
                          color:
                              (c.section == section
                                      ? KarryamColors.tealInk
                                      : KarryamColors.indigo)
                                  .forInk(context),
                        ),
                        selected: c.section == section,
                        showCheckmark: false,
                        selectedColor: KarryamColors.mint.forSurface(context),
                        side: BorderSide(
                          color:
                              (c.section == section
                                      ? KarryamColors.teal
                                      : const Color(0xFFDEE8E5))
                                  .forOutline(context),
                        ),
                        onSelected: c.loading || c.busy
                            ? null
                            : (_) => c.load(selected: section),
                      ),
                    ),
                ],
              ),
            ),
            const SizedBox(height: 20),
            if (c.section == StockSection.balances)
              SettingsAction(
                'Set up stock tracking',
                secondary: true,
                icon: Icons.add_box_outlined,
                onPressed: c.loading || c.busy || c.setupError != null
                    ? null
                    : () => edit(StockEdit.basis),
              ),
            if (c.section == StockSection.receipts)
              SettingsAction(
                'Receive goods',
                icon: Icons.move_to_inbox_outlined,
                onPressed: c.loading || c.busy
                    ? null
                    : () => edit(StockEdit.receipt),
              ),
            if (c.section == StockSection.transfers)
              SettingsAction(
                'New transfer',
                icon: Icons.swap_horiz_rounded,
                onPressed: c.loading || c.busy || c.setupError != null
                    ? null
                    : newTransfer,
              ),
            if (c.section == StockSection.quarantines)
              SettingsAction(
                "Quarantine stock",
                icon: Icons.shield_outlined,
                onPressed:
                    c.loading ||
                        c.busy ||
                        c.setupError != null ||
                        c.denied.contains(
                          "POST ${StockController.root}/quarantines",
                        )
                    ? null
                    : newQuarantine,
              ),
            if (c.section == StockSection.stocktakes)
              SettingsAction(
                'Start stocktake',
                icon: Icons.fact_check_outlined,
                onPressed: c.loading || c.busy
                    ? null
                    : () => edit(StockEdit.stocktake),
              ),
            const SizedBox(height: 20),
            if (c.setupError != null) ...[
              SettingsNote(c.setupError!),
              SettingsAction(
                'Reload product choices',
                secondary: true,
                onPressed: c.loading || c.busy ? null : c.prepare,
              ),
            ],
            if (c.error != null) SettingsNote(c.error!),
            if (c.loading)
              const Padding(
                padding: EdgeInsets.all(40),
                child: PremiumSkeleton(detail: true, rows: 2),
              )
            else if (c.errors[c.section] != null)
              DashboardEmptyState(
                story: EmptyStory.unavailable,
                title: 'Inventory unavailable',
                message: c.errors[c.section]!,
                action: 'Try again',
                onAction: c.load,
              )
            else if (c.rows.isEmpty)
              DashboardEmptyState(
                story: EmptyStory.products,
                iconOverride: stockIcon(c.section),
                detailIconOverride: switch (c.section) {
                  StockSection.quarantines => Icons.lock_outline_rounded,
                  StockSection.transfers => Icons.local_shipping_outlined,
                  StockSection.balances => Icons.add_rounded,
                  StockSection.receipts => Icons.south_west_rounded,
                  StockSection.stocktakes => Icons.checklist_rounded,
                  StockSection.movements => Icons.schedule_rounded,
                  StockSection.alerts => Icons.done_rounded,
                },
                title: switch (c.section) {
                  StockSection.quarantines => 'No quarantined stock',
                  StockSection.transfers => 'Stock where it is needed',
                  StockSection.balances => 'Your stock starts here',
                  StockSection.receipts => 'No goods received yet',
                  StockSection.stocktakes => 'Ready for a fresh count',
                  StockSection.movements => 'No stock movements yet',
                  StockSection.alerts => 'No stock alerts',
                },
                message: switch (c.section) {
                  StockSection.quarantines =>
                    'Hold stock out of sale while its quality is checked.',
                  StockSection.transfers =>
                    'Move goods between branches and record each arrival.',
                  StockSection.balances =>
                    'Set up tracking for a branch listing, then receive your first delivery.',
                  StockSection.receipts =>
                    'Record a delivery, review it, then post it to update stock.',
                  StockSection.stocktakes =>
                    'Compare a physical count with your recorded stock.',
                  StockSection.movements =>
                    'Receipts and stock changes appear here.',
                  StockSection.alerts =>
                    'Inventory alerts for this branch will appear here.',
                },
              )
            else
              AnimatedSwitcher(
                duration: const Duration(milliseconds: 260),
                child: Column(
                  key: ValueKey(c.section),
                  children: [
                    for (final row in c.rows)
                      Padding(
                        padding: const EdgeInsets.only(bottom: 12),
                        child: StockTile(
                          controller: c,
                          row: row,
                          onTap: c.busy ? null : () => open(row),
                        ),
                      ),
                  ],
                ),
              ),
            if (c.hasMore && !c.loading)
              SettingsAction('Load more', secondary: true, onPressed: c.more),
          ],
        ),
      ),
    ),
  );
}

String stockTitle(StockSection section) => switch (section) {
  StockSection.quarantines => 'Quarantine',
  StockSection.transfers => 'Transfers',
  StockSection.balances => 'Stock',
  StockSection.receipts => 'Receiving',
  StockSection.stocktakes => 'Stocktakes',
  StockSection.movements => 'History',
  StockSection.alerts => 'Alerts',
};
IconData stockIcon(StockSection section) => switch (section) {
  StockSection.quarantines => Icons.shield_outlined,
  StockSection.transfers => Icons.swap_horiz_rounded,
  StockSection.balances => Icons.inventory_2_outlined,
  StockSection.receipts => Icons.move_to_inbox_outlined,
  StockSection.stocktakes => Icons.fact_check_outlined,
  StockSection.movements => Icons.swap_vert_rounded,
  StockSection.alerts => Icons.notifications_active_outlined,
};
Color stockColor(Object? state) => switch (state) {
  'POSTED' || 'RESOLVED' || 'RECEIVED' || 'RELEASED' => KarryamColors.tealInk,
  'CANCELLED' ||
  'STALE' ||
  'CRITICAL' ||
  'ERROR' ||
  'DISPOSED' => KarryamColors.error,
  'OPEN' ||
  'WARNING' ||
  'COUNTING' ||
  'READY' ||
  'Discrepancy recorded' ||
  'PARTIALLY_RECEIVED' => const Color(0xFF986013),
  _ => const Color(0xFF326AA2),
};

class StockBadge extends StatelessWidget {
  const StockBadge(this.value, {super.key});
  final Object? value;
  @override
  Widget build(BuildContext context) => Container(
    padding: const EdgeInsets.symmetric(horizontal: 11, vertical: 6),
    decoration: BoxDecoration(
      color: (stockColor(value).withValues(alpha: .09)).forSurface(context),
      borderRadius: BorderRadius.circular(10),
    ),
    child: Text(
      dashboardLabel(value),
      style: TextStyle(
        color: (stockColor(value)).forInk(context),
        fontSize: 12,
        fontWeight: FontWeight.w700,
      ),
    ),
  );
}

class StockEmblem extends StatelessWidget {
  const StockEmblem({super.key});
  @override
  Widget build(BuildContext context) => SizedBox(
    width: 76,
    height: 84,
    child: Stack(
      children: [
        Positioned(
          left: 0,
          top: 12,
          child: Transform.rotate(
            angle: -.12,
            child: Container(
              width: 66,
              height: 64,
              decoration: BoxDecoration(
                color: (Colors.white.withValues(alpha: .8)).forSurface(context),
                borderRadius: BorderRadius.circular(21),
              ),
              child: Icon(
                Icons.inventory_2_outlined,
                color: KarryamColors.tealInk.forInk(context),
                size: 34,
              ),
            ),
          ),
        ),
        Positioned(
          right: 0,
          bottom: 0,
          child: Container(
            padding: const EdgeInsets.all(7),
            decoration: BoxDecoration(
              color: KarryamColors.teal.forSurface(context),
              borderRadius: BorderRadius.circular(12),
            ),
            child: Icon(
              Icons.qr_code_2_rounded,
              size: 19,
              color: KarryamColors.navy.forInk(context),
            ),
          ),
        ),
      ],
    ),
  );
}

class StockTile extends StatelessWidget {
  const StockTile({
    super.key,
    required this.controller,
    required this.row,
    this.onTap,
  });
  final StockController controller;
  final JsonObject row;
  final VoidCallback? onTap;
  @override
  Widget build(BuildContext context) {
    final section = controller.section;
    final color = section == StockSection.quarantines
        ? quarantineColor(row["status"])
        : section == StockSection.alerts
        ? stockColor(row['severity'])
        : KarryamColors.tealInk;
    return Material(
      color: Colors.white.forSurface(context),
      borderRadius: BorderRadius.circular(24),
      child: InkWell(
        borderRadius: BorderRadius.circular(24),
        onTap: onTap,
        child: Padding(
          padding: const EdgeInsets.all(20),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Container(
                padding: const EdgeInsets.all(13),
                decoration: BoxDecoration(
                  color: (color.withValues(alpha: .08)).forSurface(context),
                  borderRadius: BorderRadius.circular(17),
                ),
                child: Icon(
                  stockIcon(section),
                  color: (color).forInk(context),
                  size: 26,
                ),
              ),
              const SizedBox(width: 14),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      switch (section) {
                        StockSection.quarantines => controller.name(
                          row["variant_ref"],
                        ),
                        StockSection.transfers =>
                          '${controller.name(row['source_branch_ref'])} to ${controller.name(row['destination_branch_ref'])}',
                        StockSection.balances => controller.name(
                          row['variant_ref'],
                        ),
                        StockSection.receipts =>
                          row['source_ref']?.toString().isNotEmpty == true
                              ? row['source_ref'].toString()
                              : 'Goods receipt',
                        StockSection.stocktakes => dashboardLabel(
                          row['stocktake_type'],
                        ),
                        StockSection.movements => dashboardLabel(
                          row['movement_type'],
                        ),
                        StockSection.alerts => dashboardLabel(
                          row['alert_type'],
                        ),
                      },
                      style: const TextStyle(
                        fontSize: 16,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                    const SizedBox(height: 8),
                    if (section == StockSection.balances) ...[
                      Text(
                        '${row['physical_available']} available',
                        style: TextStyle(
                          fontSize: 23,
                          fontWeight: FontWeight.w700,
                          color: KarryamColors.tealInk.forInk(context),
                        ),
                      ),
                      const SizedBox(height: 5),
                      Text('${row['physical_on_hand']} on hand'),
                    ] else if (section == StockSection.movements)
                      Text(
                        '${row['on_hand_delta']} on hand · ${controller.name(row['variant_ref'])}',
                      )
                    else if (section == StockSection.quarantines) ...[
                      Text(
                        '${row['quantity']} stock units ? ${quarantineReasons[row['reason_code']]?.$1 ?? dashboardLabel(row['reason_code'])}',
                        style: const TextStyle(fontSize: 12),
                      ),
                      const SizedBox(height: 8),
                      Container(
                        padding: const EdgeInsets.symmetric(
                          horizontal: 12,
                          vertical: 7,
                        ),
                        decoration: BoxDecoration(
                          color: (color.withValues(
                            alpha: .09,
                          )).forSurface(context),
                          borderRadius: BorderRadius.circular(12),
                        ),
                        child: Text(
                          quarantineStatus(row['status']),
                          style: TextStyle(
                            color: (color).forInk(context),
                            fontWeight: FontWeight.w700,
                            fontSize: 12,
                          ),
                        ),
                      ),
                    ] else
                      StockBadge(row['status']),
                  ],
                ),
              ),
              Icon(
                Icons.chevron_right_rounded,
                color: KarryamColors.indigo.forInk(context),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

Future<void> stockFeedback(
  BuildContext context,
  StockController c,
  JsonObject? result,
  String title,
  String message,
) => showOrderFeedback(
  context,
  title: result == null ? 'Could not complete action' : title,
  message: result == null ? c.error ?? 'Please try again.' : message,
  success: result != null,
).then((_) {});

class StockBalanceScreen extends StatefulWidget {
  const StockBalanceScreen({
    super.key,
    required this.controller,
    required this.row,
    required this.onAdjust,
    this.mock = false,
  });
  final StockController controller;
  final JsonObject row;
  final Future<void> Function(JsonObject) onAdjust;
  final bool mock;
  @override
  State<StockBalanceScreen> createState() => _StockBalanceScreenState();
}

class _StockBalanceScreenState extends State<StockBalanceScreen> {
  JsonObject? record;
  String? error;
  Future<void> load() async {
    setState(() {
      record = null;
      error = null;
    });
    try {
      final r = await widget.controller.balance(
        widget.row['stock_basis_ref'].toString(),
      );
      if (mounted) setState(() => record = r);
    } catch (e) {
      if (mounted) setState(() => error = widget.controller.message(e));
    }
  }

  @override
  void initState() {
    super.initState();
    load();
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    backgroundColor: KarryamColors.canvas.forSurface(context),
    appBar: AppBar(title: const Text('Stock detail')),
    bottomNavigationBar: record == null || error != null
        ? null
        : SafeArea(
            child: Padding(
              padding: const EdgeInsets.fromLTRB(22, 12, 22, 18),
              child: SettingsAction(
                'Adjust stock',
                icon: Icons.tune_rounded,
                onPressed: () async {
                  await widget.onAdjust(record!);
                  if (mounted) await load();
                },
              ),
            ),
          ),
    body: ListView(
      padding: const EdgeInsets.all(22),
      children: [
        SettingsHeading(widget.controller.name(widget.row['variant_ref'])),
        if (error != null) ...[
          SettingsNote(error!),
          SettingsAction('Try again', onPressed: load),
        ] else if (record == null)
          const PremiumSkeleton(detail: true, rows: 2)
        else ...[
          SettingsCard(
            color: KarryamColors.mint.forInk(context),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Text('Available'),
                const SizedBox(height: 7),
                Text(
                  '${record!['available_quantity']}',
                  style: TextStyle(
                    fontSize: 46,
                    fontWeight: FontWeight.w700,
                    color: KarryamColors.tealInk.forInk(context),
                  ),
                ),
              ],
            ),
          ),
          const SizedBox(height: 18),
          StockFigures(record: record!),
          const SizedBox(height: 18),
          ReorderRuleCard(
            controller: widget.controller,
            basis: widget.row['stock_basis_ref'].toString(),
            mock: widget.mock,
          ),
        ],
      ],
    ),
  );
}

class StockFigures extends StatelessWidget {
  const StockFigures({super.key, required this.record});
  final JsonObject record;
  static const figures = [
    (
      'on_hand_quantity',
      'On hand',
      Icons.inventory_2_outlined,
      KarryamColors.tealInk,
    ),
    (
      'reserved_quantity',
      'Reserved',
      Icons.bookmark_border_rounded,
      Color(0xFF326AA2),
    ),
    (
      'quarantined_quantity',
      'Quarantined',
      Icons.shield_outlined,
      Color(0xFF986013),
    ),
    (
      'in_transit_quantity',
      'In transit',
      Icons.local_shipping_outlined,
      Color(0xFF7660A6),
    ),
    (
      'backorder_quantity',
      'Backorder',
      Icons.schedule_rounded,
      Color(0xFF64768A),
    ),
  ];
  @override
  Widget build(BuildContext context) => LayoutBuilder(
    builder: (context, constraints) {
      final columns =
          constraints.maxWidth >= 300 &&
              MediaQuery.textScalerOf(context).scale(16) <= 21
          ? 2
          : 1;
      final width = (constraints.maxWidth - (columns - 1) * 12) / columns;
      return Wrap(
        spacing: 12,
        runSpacing: 12,
        children: [
          for (final item in figures)
            SizedBox(
              width: width,
              child: Container(
                padding: const EdgeInsets.all(18),
                decoration: BoxDecoration(
                  color: Colors.white.forSurface(context),
                  borderRadius: BorderRadius.circular(22),
                  border: Border.all(
                    color: Color(0xFFEAF0F1).forOutline(context),
                  ),
                ),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Container(
                      padding: const EdgeInsets.all(9),
                      decoration: BoxDecoration(
                        color: (item.$4.withValues(
                          alpha: .08,
                        )).forSurface(context),
                        borderRadius: BorderRadius.circular(12),
                      ),
                      child: Icon(
                        item.$3,
                        color: (item.$4).forInk(context),
                        size: 22,
                      ),
                    ),
                    const SizedBox(height: 16),
                    Text(
                      item.$2,
                      style: TextStyle(
                        fontSize: 12,
                        color: Color(0xFF64768A).forInk(context),
                      ),
                    ),
                    const SizedBox(height: 4),
                    FittedBox(
                      fit: BoxFit.scaleDown,
                      alignment: Alignment.centerLeft,
                      child: Text(
                        '${record[item.$1]}',
                        style: const TextStyle(
                          fontSize: 25,
                          fontWeight: FontWeight.w800,
                          letterSpacing: -.6,
                        ),
                      ),
                    ),
                  ],
                ),
              ),
            ),
        ],
      );
    },
  );
}

class StockRecordScreen extends StatefulWidget {
  const StockRecordScreen({
    super.key,
    required this.controller,
    required this.row,
    required this.kind,
    this.mock = false,
  });
  final bool mock;
  final StockController controller;
  final JsonObject row;
  final String kind;
  @override
  State<StockRecordScreen> createState() => _StockRecordScreenState();
}

class _StockRecordScreenState extends State<StockRecordScreen> {
  JsonObject? record;
  String? error;
  bool loading = true, presenting = false, recountRequired = false;
  final countControllers = <TextEditingController>[];
  @override
  void dispose() {
    for (final controller in countControllers) {
      controller.dispose();
    }
    super.dispose();
  }

  StockController get c => widget.controller;
  bool get receipt => widget.kind == 'receipts';
  List<JsonObject> get lines =>
      (record?['lines'] as List? ?? []).map(jsonObject).toList();
  @override
  void initState() {
    super.initState();
    load();
  }

  Future<void> load() async {
    setState(() {
      loading = true;
      error = null;
    });
    try {
      final next = await c.detail(widget.kind, widget.row['id'].toString());
      if (mounted) setState(() => record = next);
    } catch (e) {
      if (mounted) setState(() => error = c.message(e));
    } finally {
      if (mounted) setState(() => loading = false);
    }
  }

  Future<void> transition(String action) async {
    if (presenting || c.busy) return;
    setState(() => presenting = true);
    final confirm = await confirmSettings(
      context,
      action == 'cancel'
          ? 'Cancel this receipt?'
          : receipt
          ? 'Post this receipt?'
          : 'Post this stocktake?',
      action == 'cancel'
          ? (record!['status'] == 'POSTED'
                ? 'This reverses the original receipt stock only if its lots are still intact. The server will refuse stock already used or reserved.'
                : 'The draft will be cancelled without changing stock.')
          : receipt
          ? 'Received quantities will be added to branch stock.'
          : 'Your physical counts will update branch stock. Stock must not have moved since the snapshot.',
      danger: action == 'cancel',
    );
    if (!mounted) return;
    setState(() => presenting = false);
    if (confirm != true) return;
    final idKey = receipt ? 'receiptId' : 'stocktakeId';
    final result = await c.mutate(
      'POST',
      '${StockController.root}/${widget.kind}/{$idKey}/$action',
      parameters: {idKey: widget.row['id'].toString()},
    );
    if (!receipt && c.errorCode == 'SHOP_INVENTORY_STALE_REVISION') {
      recountRequired = true;
    }
    if (!mounted) return;
    await stockFeedback(
      context,
      c,
      result,
      action == 'cancel' ? 'Receipt cancelled' : 'Stock updated',
      action == 'cancel'
          ? 'This receipt has been cancelled.'
          : 'The posted record is now reflected in inventory.',
    );
    if (mounted) await load();
  }

  Future<void> count(JsonObject line) async {
    if (presenting || c.busy) return;
    setState(() => presenting = true);
    final value = TextEditingController(
      text:
          line['counted_quantity']?.toString() ??
          (widget.mock ? line['expected_quantity'].toString() : ''),
    );
    countControllers.add(value);
    final form = GlobalKey<FormState>();
    final result = await settingsSheet<int>(
      context,
      Form(
        key: form,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(
              c.name(line['variant_ref']),
              style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w700),
            ),
            const SizedBox(height: 20),
            TextFormField(
              controller: value,
              autofocus: true,
              keyboardType: TextInputType.number,
              decoration: const InputDecoration(labelText: 'Physical count'),
              validator: (v) {
                final n = int.tryParse(v ?? '');
                return n == null || n < 0 || n > 1000000000
                    ? 'Enter a whole count from 0 to 1,000,000,000.'
                    : null;
              },
            ),
            const SizedBox(height: 22),
            SettingsAction(
              'Save count',
              onPressed: () {
                if (form.currentState!.validate()) {
                  Navigator.pop(context, int.parse(value.text));
                }
              },
            ),
          ],
        ),
      ),
    );
    if (!mounted) return;
    setState(() => presenting = false);
    if (result == null) return;
    final saved = await c.mutate(
      'PUT',
      '${StockController.root}/stocktakes/{stocktakeId}/lines/{stockBasisId}',
      parameters: {
        'stocktakeId': widget.row['id'].toString(),
        'stockBasisId': line['stock_basis_ref'].toString(),
      },
      body: {'counted_quantity': result},
    );
    if (mounted && saved == null) {
      await stockFeedback(context, c, saved, '', '');
    }
    if (mounted) await load();
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => Scaffold(
      backgroundColor: KarryamColors.canvas.forSurface(context),
      appBar: AppBar(
        title: Text(receipt ? 'Goods receipt' : 'Stocktake'),
        actions: [
          IconButton(
            tooltip: 'Refresh',
            onPressed: loading || c.busy ? null : load,
            icon: const Icon(Icons.refresh_rounded),
          ),
        ],
      ),
      body: ListView(
        padding: const EdgeInsets.all(22),
        children: [
          if (loading)
            const PremiumSkeleton(detail: true, rows: 2)
          else if (error != null) ...[
            SettingsNote(error!),
            SettingsAction('Try again', onPressed: load),
          ] else if (record != null) ...[
            SettingsCard(
              color: KarryamColors.mint.forInk(context),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    receipt
                        ? record!['source_ref']?.toString().isNotEmpty == true
                              ? record!['source_ref'].toString()
                              : 'Goods receipt'
                        : dashboardLabel(record!['stocktake_type']),
                    style: const TextStyle(
                      fontSize: 25,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  const SizedBox(height: 12),
                  StockBadge(record!['status']),
                  const SizedBox(height: 12),
                  SettingsFact(
                    receipt ? 'Received' : 'Started',
                    '${record![receipt ? 'received_at' : 'started_at']}',
                  ),
                ],
              ),
            ),
            const SizedBox(height: 24),
            SettingsHeading(
              receipt ? 'Received items' : 'Physical counts',
              subtitle: receipt
                  ? 'Review these quantities before posting.'
                  : 'Tap each item to record its physical count.',
            ),
            for (final line in lines)
              Padding(
                padding: const EdgeInsets.only(bottom: 12),
                child: SettingsCard(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        c.name(line['variant_ref']),
                        style: const TextStyle(
                          fontSize: 17,
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                      const SizedBox(height: 12),
                      if (receipt) ...[
                        SettingsFact('Quantity', '${line['quantity']}'),
                        for (final key in [
                          'lot_number',
                          'batch_number',
                          'manufactured_at',
                          'best_before_at',
                          'expires_at',
                        ])
                          if (line[key] != null &&
                              line[key].toString().isNotEmpty)
                            SettingsFact(
                              dashboardLabel(key),
                              line[key].toString(),
                            ),
                      ] else ...[
                        SettingsFact(
                          'Expected',
                          '${line['expected_quantity']}',
                        ),
                        SettingsFact(
                          'Counted',
                          line['counted_quantity']?.toString() ?? 'Not counted',
                        ),
                        if (line['counted_quantity'] != null)
                          SettingsFact(
                            'Variance',
                            '${line['variance_quantity']}',
                          ),
                        if (record!['status'] == 'COUNTING')
                          SettingsAction(
                            line['counted_quantity'] == null
                                ? 'Enter count'
                                : 'Edit count',
                            secondary: true,
                            icon: Icons.edit_outlined,
                            onPressed: c.busy || presenting
                                ? null
                                : () => count(line),
                          ),
                      ],
                    ],
                  ),
                ),
              ),
            const SizedBox(height: 14),
            if (receipt && record!['status'] == 'DRAFT' ||
                !receipt &&
                    {'COUNTING', 'READY'}.contains(record!['status'])) ...[
              SettingsAction(
                receipt ? 'Post receipt' : 'Post stocktake',
                icon: Icons.task_alt_rounded,
                busy: c.busy,
                onPressed:
                    presenting ||
                        recountRequired ||
                        lines.isEmpty ||
                        !receipt &&
                            lines.any((l) => l['counted_quantity'] == null)
                    ? null
                    : () => transition('post'),
              ),
              if (receipt) ...[
                const SizedBox(height: 12),
                SettingsAction(
                  'Cancel receipt',
                  danger: true,
                  icon: Icons.close_rounded,
                  onPressed: c.busy || presenting
                      ? null
                      : () => transition('cancel'),
                ),
              ],
            ],
            if (receipt && record!['status'] == 'POSTED') ...[
              const SizedBox(height: 12),
              SettingsAction(
                'Reverse receipt',
                danger: true,
                icon: Icons.undo_rounded,
                onPressed: c.busy || presenting
                    ? null
                    : () => transition('cancel'),
              ),
            ],
            if (record!['status'] == 'STALE' || recountRequired) ...[
              const SettingsNote(
                'Stock moved after this snapshot. Start a new stocktake and recount.',
                warning: true,
              ),
              SettingsAction(
                'Back to stocktakes',
                secondary: true,
                onPressed: () => Navigator.pop(context),
              ),
            ],
          ],
        ],
      ),
    ),
  );
}
