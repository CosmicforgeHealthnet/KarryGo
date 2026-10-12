import '../../../core/ui/premium_loading.dart';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../onboarding/ui/setup_design.dart';
import '../../settings/ui/settings_design.dart';
import '../state/stock_controller.dart';
import 'stock_screen.dart';
import 'transfer_editor.dart';

int transferRemaining(JsonObject line) =>
    (line['sent_quantity'] as int) -
    (line['received_quantity'] as int) -
    (line['damaged_quantity'] as int) -
    (line['missing_quantity'] as int);

class TransferScreen extends StatefulWidget {
  const TransferScreen({
    super.key,
    required this.runtime,
    required this.controller,
    required this.id,
  });
  final VendorRuntime runtime;
  final StockController controller;
  final String id;
  @override
  State<TransferScreen> createState() => _TransferScreenState();
}

class _TransferScreenState extends State<TransferScreen> {
  StockController get c => widget.controller;
  JsonObject? record;
  String? error;
  bool loading = true, presenting = false;
  List<JsonObject> get lines =>
      (record?['lines'] as List? ?? []).map(jsonObject).toList();
  bool get issues => lines.any(
    (line) => line['damaged_quantity'] > 0 || line['missing_quantity'] > 0,
  );
  @override
  void initState() {
    super.initState();
    load();
  }

  Future<void> load() async {
    if (c.busy) return;
    setState(() {
      loading = true;
      error = null;
    });
    try {
      final next = await c.transfer(widget.id);
      if (!mounted) return;
      setState(() => record = next);
      if (c.branches.isEmpty) {
        try {
          await c.loadBranches();
        } catch (_) {}
      }
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
      action == 'dispatch' ? 'Dispatch transfer?' : 'Cancel transfer?',
      action == 'dispatch'
          ? 'These goods will leave ${c.name(record!['source_branch_ref'])} stock and be recorded in transit. Confirm only when they have been sent.'
          : 'Cancel this transfer before dispatch. Stock quantities will stay unchanged.',
      danger: action == 'cancel',
    );
    if (!mounted) return;
    setState(() => presenting = false);
    if (!confirm) return;
    final result = await c.mutate(
      'POST',
      '${StockController.root}/transfers/{transferId}/$action',
      parameters: {'transferId': widget.id},
    );
    if (!mounted) return;
    await stockFeedback(
      context,
      c,
      result,
      action == 'dispatch' ? 'Transfer dispatched' : 'Transfer cancelled',
      action == 'dispatch'
          ? 'Goods are now in transit. Record arrivals at the destination branch.'
          : 'The draft transfer has been cancelled.',
    );
    if (mounted) await load();
  }

  Future<void> receive() async {
    if (presenting || c.busy || record == null) return;
    await Navigator.push<JsonObject>(
      context,
      setupRoute<JsonObject>(
        TransferEditor(runtime: widget.runtime, controller: c, record: record),
      ),
    );
    if (mounted) await load();
  }

  Widget route() => SettingsCard(
    color: KarryamColors.mint.forInk(context),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          'BRANCH TRANSFER',
          style: TextStyle(
            fontSize: 11,
            letterSpacing: 1.5,
            color: KarryamColors.tealInk.forInk(context),
            fontWeight: FontWeight.w700,
          ),
        ),
        const SizedBox(height: 22),
        TransferBranch(
          'From',
          c.name(record!['source_branch_ref']),
          Icons.storefront_outlined,
        ),
        Padding(
          padding: const EdgeInsets.only(left: 18, top: 5, bottom: 5),
          child: Icon(
            Icons.south_rounded,
            size: 24,
            color: (KarryamColors.tealInk.withValues(
              alpha: .6,
            )).forInk(context),
          ),
        ),
        TransferBranch(
          'To',
          c.name(record!['destination_branch_ref']),
          Icons.move_to_inbox_outlined,
        ),
        const SizedBox(height: 22),
        Wrap(
          spacing: 10,
          runSpacing: 8,
          children: [
            StockBadge(record!['status']),
            if (issues) const StockBadge('Discrepancy recorded'),
          ],
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
        backgroundColor: KarryamColors.canvas.forSurface(context),
        appBar: AppBar(
          title: const Text('Transfer detail'),
          actions: [
            IconButton(
              tooltip: 'Refresh transfer',
              onPressed: loading || c.busy ? null : load,
              icon: const Icon(Icons.refresh_rounded),
            ),
          ],
        ),
        bottomNavigationBar: loading || error != null || record == null
            ? null
            : SafeArea(
                child: Padding(
                  padding: const EdgeInsets.fromLTRB(22, 12, 22, 18),
                  child: Column(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      if ({
                        'DRAFT',
                        'APPROVED',
                      }.contains(record!['status'])) ...[
                        SettingsAction(
                          'Dispatch transfer',
                          icon: Icons.local_shipping_outlined,
                          busy: c.busy,
                          onPressed:
                              presenting ||
                                  c.denied.contains(
                                    'POST ${StockController.root}/transfers/{transferId}/dispatch',
                                  )
                              ? null
                              : () => transition('dispatch'),
                        ),
                        const SizedBox(height: 10),
                        SettingsAction(
                          'Cancel transfer',
                          danger: true,
                          icon: Icons.close_rounded,
                          onPressed:
                              c.busy ||
                                  presenting ||
                                  c.denied.contains(
                                    'POST ${StockController.root}/transfers/{transferId}/cancel',
                                  )
                              ? null
                              : () => transition('cancel'),
                        ),
                      ] else if ({
                        'IN_TRANSIT',
                        'PARTIALLY_RECEIVED',
                      }.contains(record!['status']))
                        SettingsAction(
                          'Record arrival',
                          icon: Icons.move_to_inbox_outlined,
                          onPressed:
                              c.busy ||
                                  presenting ||
                                  c.denied.contains(
                                    'POST ${StockController.root}/transfers/{transferId}/receive',
                                  )
                              ? null
                              : receive,
                        ),
                    ],
                  ),
                ),
              ),
        body: RefreshIndicator(
          onRefresh: load,
          color: KarryamColors.tealInk.forInk(context),
          child: ListView(
            padding: const EdgeInsets.all(22),
            children: [
              if (loading)
                const Padding(
                  padding: EdgeInsets.all(40),
                  child: PremiumSkeleton(detail: true, rows: 2),
                )
              else if (error != null) ...[
                SettingsNote(error!),
                SettingsAction('Try again', onPressed: load),
              ] else if (record != null) ...[
                route(),
                const SizedBox(height: 24),
                TransferJourney(record: record!),
                const SizedBox(height: 24),
                if (issues)
                  const SettingsNote(
                    'Damaged or missing goods are recorded separately. Only goods received in usable condition are added to destination stock.',
                    warning: true,
                  ),
                const SettingsHeading('Goods in this transfer'),
                for (final line in lines)
                  Padding(
                    padding: const EdgeInsets.only(bottom: 14),
                    child: SettingsCard(
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
                                  Icons.inventory_2_outlined,
                                  color: KarryamColors.tealInk.forInk(context),
                                ),
                              ),
                              const SizedBox(width: 14),
                              Expanded(
                                child: Text(
                                  c.name(line['variant_ref']),
                                  style: const TextStyle(
                                    fontSize: 17,
                                    fontWeight: FontWeight.w700,
                                  ),
                                ),
                              ),
                            ],
                          ),
                          const SizedBox(height: 16),
                          Wrap(
                            spacing: 20,
                            runSpacing: 16,
                            children: [
                              TransferQuantity(
                                'Sent',
                                line['sent_quantity'],
                                const Color(0xFF326AA2),
                              ),
                              TransferQuantity(
                                'Received',
                                line['received_quantity'],
                                KarryamColors.tealInk,
                              ),
                              TransferQuantity(
                                'Damaged',
                                line['damaged_quantity'],
                                line['damaged_quantity'] == 0
                                    ? KarryamColors.indigo
                                    : const Color(0xFFA96112),
                              ),
                              TransferQuantity(
                                'Missing',
                                line['missing_quantity'],
                                line['missing_quantity'] == 0
                                    ? KarryamColors.indigo
                                    : KarryamColors.error,
                              ),
                            ],
                          ),
                          if ({
                            'IN_TRANSIT',
                            'PARTIALLY_RECEIVED',
                          }.contains(record!['status'])) ...[
                            const Divider(height: 32),
                            SettingsFact(
                              'Still to account for',
                              '${transferRemaining(line)}',
                            ),
                          ],
                        ],
                      ),
                    ),
                  ),
                if (record!['notes']?.toString().isNotEmpty == true)
                  SettingsCard(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        const Text(
                          'Transfer notes',
                          style: TextStyle(fontWeight: FontWeight.w700),
                        ),
                        const SizedBox(height: 10),
                        Text(record!['notes'].toString()),
                      ],
                    ),
                  ),
                const SizedBox(height: 18),
                SettingsFact('Created', '${record!['created_at']}'),
                if (record!['dispatched_at'] != null)
                  SettingsFact('Dispatched', '${record!['dispatched_at']}'),
                if (record!['received_at'] != null)
                  SettingsFact('Completed', '${record!['received_at']}'),
                if (record!['status'] == 'DRAFT')
                  const Padding(
                    padding: EdgeInsets.only(top: 12),
                    child: Text(
                      'Draft transfers do not reserve or move stock. Availability is checked when you dispatch.',
                    ),
                  ),
              ],
            ],
          ),
        ),
      ),
    ),
  );
}

class TransferBranch extends StatelessWidget {
  const TransferBranch(this.label, this.name, this.icon, {super.key});
  final String label, name;
  final IconData icon;
  @override
  Widget build(BuildContext context) => Row(
    children: [
      Icon(icon, color: KarryamColors.tealInk.forInk(context), size: 30),
      const SizedBox(width: 18),
      Expanded(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(label, style: const TextStyle(fontSize: 12)),
            const SizedBox(height: 3),
            Text(
              name,
              style: const TextStyle(
                fontSize: 20,
                fontWeight: FontWeight.w700,
                letterSpacing: -.4,
              ),
            ),
          ],
        ),
      ),
    ],
  );
}

class TransferQuantity extends StatelessWidget {
  const TransferQuantity(this.label, this.quantity, this.color, {super.key});
  final String label;
  final Object? quantity;
  final Color color;
  @override
  Widget build(BuildContext context) => SizedBox(
    width: 112,
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          '$quantity',
          style: TextStyle(
            fontSize: 26,
            fontWeight: FontWeight.w700,
            color: (color).forInk(context),
          ),
        ),
        const SizedBox(height: 4),
        Text(label, style: const TextStyle(fontSize: 12)),
      ],
    ),
  );
}

class TransferJourney extends StatelessWidget {
  const TransferJourney({super.key, required this.record});
  final JsonObject record;
  @override
  Widget build(BuildContext context) {
    final cancelled = record['status'] == 'CANCELLED';
    return SettingsCard(
      child: Column(
        children: [
          for (var index = 0; index < 3; index++)
            Padding(
              padding: const EdgeInsets.symmetric(vertical: 6),
              child: Row(
                children: [
                  Icon(
                    index == 0 ||
                            index == 1 && record['dispatched_at'] != null ||
                            index == 2 && record['status'] == 'RECEIVED'
                        ? Icons.check_circle_rounded
                        : Icons.radio_button_unchecked_rounded,
                    color:
                        (cancelled
                                ? KarryamColors.indigo
                                : index == 0 ||
                                      index == 1 &&
                                          record['dispatched_at'] != null ||
                                      index == 2 &&
                                          record['status'] == 'RECEIVED'
                                ? KarryamColors.tealInk
                                : const Color(0xFFACBAB8))
                            .forInk(context),
                    size: 22,
                  ),
                  const SizedBox(width: 14),
                  Expanded(
                    child: Text(
                      ['Prepared', 'Dispatched', 'Received'][index],
                      style: const TextStyle(fontWeight: FontWeight.w600),
                    ),
                  ),
                  if (index == 2 && record['status'] == 'PARTIALLY_RECEIVED')
                    const StockBadge('PARTIALLY_RECEIVED'),
                ],
              ),
            ),
        ],
      ),
    );
  }
}
