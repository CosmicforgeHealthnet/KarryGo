import 'dart:async';
import 'dart:math';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import '../../../app/vendor_app.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/ui/premium_loading.dart';
import '../../settings/ui/settings_design.dart';
import '../state/branch_stock_controller.dart';

String _signed(int v) => v > 0 ? '+$v' : '$v';

Widget _branchPicker(
  BuildContext context, {
  required String label,
  required List<JsonObject> branches,
  required String? value,
  required ValueChanged<String?>? onChanged,
  String? exclude,
}) => DropdownButtonFormField<String>(
  key: ValueKey('$label-$exclude'),
  initialValue: value,
  isExpanded: true,
  decoration: InputDecoration(labelText: label),
  items: [
    for (final b in branches)
      if (b['id'] != exclude)
        DropdownMenuItem(
          value: '${b['id']}',
          child: Text(BranchStockController.branchName(b)),
        ),
  ],
  onChanged: onChanged,
);

Widget _quantityField({
  Key? key,
  required TextEditingController controller,
  required String label,
  bool enabled = true,
  ValueChanged<String>? onChanged,
}) => TextField(
  key: key,
  controller: controller,
  enabled: enabled,
  keyboardType: TextInputType.number,
  inputFormatters: [FilteringTextInputFormatter.digitsOnly],
  decoration: InputDecoration(labelText: label),
  onChanged: onChanged,
);

Widget _emptyCard(IconData icon, String title, String body) => SettingsCard(
  child: Column(
    children: [
      Icon(icon, size: 56),
      const SizedBox(height: 16),
      Text(title, textAlign: TextAlign.center),
      const SizedBox(height: 8),
      Text(body, textAlign: TextAlign.center),
    ],
  ),
);

/// Count the ingredients and packaging held at one food branch.
class BranchStocktakeScreen extends StatefulWidget {
  const BranchStocktakeScreen({super.key, required this.app});
  final VendorApp app;
  @override
  State<BranchStocktakeScreen> createState() => _BranchStocktakeScreenState();
}

class _BranchStocktakeScreenState extends State<BranchStocktakeScreen> {
  late final c = BranchStockController(widget.app);
  List<JsonObject> branches = [];
  List<BranchStockItem> items = [];
  String? branchId, error;
  bool loading = true, itemsLoading = false, busy = false;
  JsonObject? stocktake, result;
  bool stale = false, cancelled = false;
  final counts = <String, TextEditingController>{};
  final saved = <String, int>{};

  Map<String, String> get names => {for (final i in items) i.id: i.name};
  BranchStockItem? stockItem(String id) =>
      items.where((i) => i.id == id).firstOrNull;
  String amount(String id, Object? v) =>
      stockItem(id)?.amount((v as num?)?.toInt() ?? 0) ?? '${v ?? 0}';
  List<JsonObject> get lines =>
      (stocktake?['lines'] as List? ?? []).map(jsonObject).toList();

  @override
  void initState() {
    super.initState();
    unawaited(loadBranches());
  }

  @override
  void dispose() {
    for (final t in counts.values) {
      t.dispose();
    }
    super.dispose();
  }

  Future<void> loadBranches() async {
    setState(() {
      loading = true;
      error = null;
    });
    try {
      final next = await c.branches();
      if (!mounted) return;
      setState(() {
        branches = next;
        if (next.length == 1) branchId = '${next.first['id']}';
      });
      if (branchId != null) await loadItems();
    } catch (e) {
      if (mounted) {
        setState(
          () => error = e is FormatException
              ? 'Branches could not be loaded. Refresh to try again.'
              : BranchStockController.message(e),
        );
      }
    } finally {
      if (mounted) setState(() => loading = false);
    }
  }

  Future<void> loadItems() async {
    if (branchId == null) return;
    setState(() {
      itemsLoading = true;
      error = null;
    });
    try {
      final next = await c.items(branchId!);
      if (mounted) setState(() => items = next);
    } catch (e) {
      if (mounted) {
        setState(
          () => error = e is FormatException
              ? 'Stock for this branch could not be loaded. Refresh to try again.'
              : BranchStockController.message(e),
        );
      }
    } finally {
      if (mounted) setState(() => itemsLoading = false);
    }
  }

  void reset() {
    for (final t in counts.values) {
      t.dispose();
    }
    counts.clear();
    saved.clear();
    stocktake = null;
    result = null;
    stale = false;
    cancelled = false;
    error = null;
  }

  Future<void> start() async {
    if (busy || branchId == null) return;
    setState(() {
      busy = true;
      error = null;
    });
    try {
      final next = await c.send(
        'POST',
        '${BranchStockController.root}/stocktakes',
        body: {'branch_id': branchId, 'stocktake_type': 'FULL_STOCKTAKE'},
      );
      if (!mounted) return;
      setState(() {
        reset();
        stocktake = next;
        for (final line in lines) {
          counts['${line['inventory_item_id']}'] = TextEditingController();
        }
      });
    } catch (e) {
      if (mounted) {
        setState(
          () => error = BranchStockController.message(
            e,
            conflict:
                'A count is already in progress at this branch. Finish or cancel it first.',
          ),
        );
      }
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  int? counted(String id) => int.tryParse(counts[id]?.text ?? '');
  bool get complete =>
      lines.isNotEmpty &&
      lines.every((l) => counted('${l['inventory_item_id']}') != null);

  Future<void> finish() async {
    if (busy || stocktake == null || !complete) return;
    final confirm = await confirmSettings(
      context,
      'Update stock to your count?',
      'Each item’s stock will be set to the quantity you counted. This cannot be undone.',
    );
    if (!confirm || !mounted) return;
    setState(() {
      busy = true;
      error = null;
    });
    final id = '${stocktake!['id']}';
    try {
      for (final line in lines) {
        final item = '${line['inventory_item_id']}';
        final value = counted(item)!;
        if (saved[item] == value) continue;
        await c.send(
          'PUT',
          '${BranchStockController.root}/stocktakes/{stocktakeId}/items/{inventoryItemId}/count',
          parameters: {'stocktakeId': id, 'inventoryItemId': item},
          body: {'counted_quantity': value},
        );
        saved[item] = value;
      }
      final posted = await c.send(
        'POST',
        '${BranchStockController.root}/stocktakes/{stocktakeId}/post',
        parameters: {'stocktakeId': id},
      );
      if (mounted) setState(() => result = posted);
      unawaited(loadItems());
    } catch (e) {
      if (!mounted) return;
      final message = BranchStockController.message(
        e,
        conflict:
            'Stock changed at this branch while you were counting, so this count can’t be used. Start a new count.',
      );
      setState(() {
        error = message;
        if (message.startsWith('Stock changed at this branch')) stale = true;
      });
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  Future<void> cancel() async {
    if (busy || stocktake == null) return;
    final confirm = await confirmSettings(
      context,
      'Cancel this count?',
      'Nothing you entered will be saved. Stock quantities stay as they are.',
      danger: true,
    );
    if (!confirm || !mounted) return;
    setState(() {
      busy = true;
      error = null;
    });
    try {
      await c.send(
        'POST',
        '${BranchStockController.root}/stocktakes/{stocktakeId}/cancel',
        parameters: {'stocktakeId': '${stocktake!['id']}'},
      );
      if (mounted) {
        setState(() {
          reset();
          cancelled = true;
        });
      }
    } catch (e) {
      if (mounted) setState(() => error = BranchStockController.message(e));
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  Widget countRow(JsonObject line) {
    final item = '${line['inventory_item_id']}';
    final expected = (line['expected_quantity'] as num?)?.toInt() ?? 0;
    final value = counted(item);
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: SettingsCard(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              names[item] ?? 'Stock item',
              style: Theme.of(context).textTheme.titleMedium,
            ),
            const SizedBox(height: 6),
            Text('Recorded in stock: ${amount(item, expected)}'),
            const SizedBox(height: 12),
            _quantityField(
              key: ValueKey('count-$item'),
              controller: counts[item]!,
              label: stockItem(item)?.unit == null
                  ? 'Counted quantity'
                  : 'Counted quantity (${stockItem(item)!.unit})',
              enabled: !busy && result == null && !stale,
              onChanged: (_) => setState(() {}),
            ),
            if (value != null) ...[
              const SizedBox(height: 8),
              Text(
                value == expected
                    ? 'Matches stock'
                    : 'Difference: ${_signed(value - expected)}',
                style: TextStyle(
                  color: value == expected
                      ? KarryamColors.tealInk.forInk(context)
                      : const Color(0xFF986013),
                  fontWeight: FontWeight.w600,
                ),
              ),
            ],
          ],
        ),
      ),
    );
  }

  Widget summary() {
    final posted = (result!['lines'] as List? ?? []).map(jsonObject).toList();
    final changed = posted
        .where((l) => ((l['variance_quantity'] as num?)?.toInt() ?? 0) != 0)
        .toList();
    return SettingsCard(
      color: KarryamColors.mint.forInk(context),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            'Stock count saved',
            style: Theme.of(context).textTheme.titleLarge,
          ),
          const SizedBox(height: 8),
          Text(
            changed.isEmpty
                ? 'Everything you counted matched the recorded stock.'
                : '${changed.length} of ${posted.length} items were updated to match your count.',
          ),
          for (final l in changed) ...[
            const SizedBox(height: 8),
            Text(
              '${names['${l['inventory_item_id']}'] ?? 'Stock item'}: ${_signed((l['variance_quantity'] as num).toInt())} '
              '(now ${l['counted_quantity']})',
            ),
          ],
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(
      title: const Text('Count stock'),
      actions: [
        IconButton(
          tooltip: 'Refresh',
          onPressed: loading || busy ? null : loadBranches,
          icon: const Icon(Icons.refresh_rounded),
        ),
      ],
    ),
    body: SafeArea(
      child: ListView(
        padding: const EdgeInsets.all(20),
        children: [
          const SettingsHeading(
            'Count stock',
            subtitle:
                'Count the ingredients and packaging on your shelves and update the stock to match.',
          ),
          if (loading)
            const PremiumSkeleton(detail: true)
          else ...[
            if (error != null) SettingsNote(error!),
            if (branches.isEmpty && error == null)
              _emptyCard(
                Icons.storefront_outlined,
                'No branches available',
                'Add a branch before counting stock.',
              ),
            if (branches.isNotEmpty) ...[
              _branchPicker(
                context,
                label: 'Branch',
                branches: branches,
                value: branchId,
                onChanged: stocktake != null || busy
                    ? null
                    : (v) {
                        setState(() {
                          branchId = v;
                          items = [];
                          reset();
                        });
                        unawaited(loadItems());
                      },
              ),
              const SizedBox(height: 20),
            ],
            if (cancelled)
              const SettingsNote(
                'The count was cancelled. Stock is unchanged.',
                warning: true,
              ),
            if (itemsLoading)
              const PremiumSkeleton()
            else if (branchId != null && stocktake == null) ...[
              if (items.isEmpty && error == null)
                _emptyCard(
                  Icons.inventory_2_outlined,
                  'No stock to count yet',
                  'Ingredients and packaging tracked at this branch will appear here.',
                )
              else if (items.isNotEmpty) ...[
                for (final i in items)
                  ListTile(
                    contentPadding: EdgeInsets.zero,
                    title: Text(i.name),
                    trailing: Text('${i.amount(i.onHand)} in stock'),
                  ),
                const SizedBox(height: 16),
                SettingsAction(
                  'Start counting',
                  icon: Icons.fact_check_outlined,
                  busy: busy,
                  onPressed: busy ? null : start,
                ),
              ],
            ] else if (stocktake != null) ...[
              if (result != null) ...[
                summary(),
                const SizedBox(height: 16),
                SettingsAction(
                  'Done',
                  icon: Icons.check_rounded,
                  onPressed: () => Navigator.maybePop(context),
                ),
                const SizedBox(height: 16),
              ],
              if (lines.isEmpty)
                _emptyCard(
                  Icons.inventory_2_outlined,
                  'Nothing to count',
                  'This branch has no tracked stock.',
                ),
              for (final line in lines) countRow(line),
              if (result == null) ...[
                const SizedBox(height: 8),
                if (stale)
                  SettingsAction(
                    'Start a new count',
                    busy: busy,
                    onPressed: busy
                        ? null
                        : () {
                            setState(reset);
                            unawaited(loadItems());
                          },
                  )
                else ...[
                  SettingsAction(
                    'Save count',
                    icon: Icons.check_rounded,
                    busy: busy,
                    onPressed: busy || !complete ? null : finish,
                  ),
                  if (!complete)
                    const Padding(
                      padding: EdgeInsets.only(top: 8),
                      child: Text('Enter a count for every item to save.'),
                    ),
                  const SizedBox(height: 12),
                  SettingsAction(
                    'Cancel count',
                    secondary: true,
                    danger: true,
                    onPressed: busy ? null : cancel,
                  ),
                ],
              ],
            ],
          ],
        ],
      ),
    ),
  );
}

class _TransferLine {
  _TransferLine(this.itemId);
  String? itemId;
  final quantity = TextEditingController();
}

/// Move ingredients or packaging from one branch to another.
class BranchTransferScreen extends StatefulWidget {
  const BranchTransferScreen({super.key, required this.app});
  final VendorApp app;
  @override
  State<BranchTransferScreen> createState() => _BranchTransferScreenState();
}

class _BranchTransferScreenState extends State<BranchTransferScreen> {
  late final c = BranchStockController(widget.app);
  List<JsonObject> branches = [];
  List<BranchStockItem> source = [], destination = [];
  String? fromId, toId, error;
  bool loading = true, itemsLoading = false, busy = false;
  final lines = <_TransferLine>[];
  final notes = TextEditingController();
  JsonObject? transfer;
  bool received = false, cancelled = false;
  final receivedQty = <String, TextEditingController>{};
  late final receiptRef =
      'app-${List.generate(12, (_) => Random.secure().nextInt(16).toRadixString(16)).join()}';

  @override
  void initState() {
    super.initState();
    unawaited(loadBranches());
  }

  @override
  void dispose() {
    for (final l in lines) {
      l.quantity.dispose();
    }
    for (final t in receivedQty.values) {
      t.dispose();
    }
    notes.dispose();
    super.dispose();
  }

  String branchName(String? id) => BranchStockController.branchName(
    branches.firstWhere((b) => b['id'] == id, orElse: () => {}),
  );
  BranchStockItem? sourceItem(String? id) =>
      source.where((i) => i.id == id).firstOrNull;
  BranchStockItem? match(BranchStockItem item) => destination
      .where((d) => d.name.toLowerCase() == item.name.toLowerCase())
      .firstOrNull;
  String itemName(String? id) =>
      sourceItem(id)?.name ??
      destination.where((i) => i.id == id).firstOrNull?.name ??
      'Stock item';

  Future<void> loadBranches() async {
    setState(() {
      loading = true;
      error = null;
    });
    try {
      final next = await c.branches();
      if (mounted) setState(() => branches = next);
    } catch (e) {
      if (mounted) {
        setState(
          () => error = e is FormatException
              ? 'Branches could not be loaded. Refresh to try again.'
              : BranchStockController.message(e),
        );
      }
    } finally {
      if (mounted) setState(() => loading = false);
    }
  }

  Future<void> loadItems() async {
    if (fromId == null || toId == null) return;
    setState(() {
      itemsLoading = true;
      error = null;
    });
    try {
      final results = await Future.wait([c.items(fromId!), c.items(toId!)]);
      if (!mounted) return;
      setState(() {
        source = results[0];
        destination = results[1];
        for (final l in lines) {
          l.quantity.dispose();
        }
        lines
          ..clear()
          ..add(_TransferLine(null));
      });
    } catch (e) {
      if (mounted) {
        setState(
          () => error = e is FormatException
              ? 'Stock could not be loaded. Refresh to try again.'
              : BranchStockController.message(e),
        );
      }
    } finally {
      if (mounted) setState(() => itemsLoading = false);
    }
  }

  String? get problem {
    final chosen = lines.where((l) => l.itemId != null).toList();
    if (chosen.isEmpty) return 'Choose at least one item to move.';
    final ids = <String>{};
    for (final l in chosen) {
      final item = sourceItem(l.itemId)!;
      if (!ids.add(item.id)) return '${item.name} is listed more than once.';
      final q = int.tryParse(l.quantity.text) ?? 0;
      if (q <= 0) return 'Enter how much ${item.name} to move.';
      if (q > item.onHand) {
        return 'Only ${item.amount(item.onHand)} of ${item.name} is in stock at ${branchName(fromId)}.';
      }
      if (match(item) == null) {
        return '${item.name} isn’t set up at ${branchName(toId)} yet. Set it up there first.';
      }
    }
    return null;
  }

  Future<void> send() async {
    if (busy || problem != null) return;
    final confirm = await confirmSettings(
      context,
      'Send stock to ${branchName(toId)}?',
      'The items will leave ${branchName(fromId)} now and arrive when ${branchName(toId)} confirms they were received.',
    );
    if (!confirm || !mounted) return;
    setState(() {
      busy = true;
      error = null;
    });
    try {
      var record = transfer;
      record ??= await c.send(
        'POST',
        '${BranchStockController.root}/transfers',
        body: {
          'source_branch_id': fromId,
          'destination_branch_id': toId,
          'notes': notes.text.trim(),
          'lines': [
            for (final l in lines.where((l) => l.itemId != null))
              {
                'source_inventory_item_id': l.itemId,
                'destination_inventory_item_id': match(
                  sourceItem(l.itemId)!,
                )!.id,
                'quantity': int.parse(l.quantity.text),
              },
          ],
        },
      );
      if (mounted) setState(() => transfer = record);
      final sent = await c.send(
        'POST',
        '${BranchStockController.root}/transfers/{transferId}/dispatch',
        parameters: {'transferId': '${record['id']}'},
      );
      if (!mounted) return;
      setState(() {
        transfer = sent;
        for (final line in (sent['lines'] as List? ?? []).map(jsonObject)) {
          receivedQty['${line['id']}'] = TextEditingController(
            text: '${line['sent_quantity']}',
          );
        }
      });
    } catch (e) {
      if (mounted) {
        setState(
          () => error = BranchStockController.message(
            e,
            conflict:
                'There isn’t enough stock to send, or the items don’t match at both branches. Refresh and try again.',
          ),
        );
      }
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  bool get sent =>
      transfer != null &&
      {'IN_TRANSIT', 'PARTIALLY_RECEIVED'}.contains(transfer!['status']);

  Future<void> receive() async {
    if (busy || !sent) return;
    final body = <JsonObject>[];
    for (final line in (transfer!['lines'] as List).map(jsonObject)) {
      final remaining =
          (line['sent_quantity'] as int) -
          (line['received_quantity'] as int) -
          (line['damaged_quantity'] as int) -
          (line['missing_quantity'] as int);
      if (remaining <= 0) continue;
      final got = int.tryParse(receivedQty['${line['id']}']?.text ?? '') ?? 0;
      if (got > remaining) {
        setState(
          () => error =
              'You can’t receive more ${itemName('${line['source_inventory_item_id']}')} than was sent ($remaining).',
        );
        return;
      }
      body.add({
        'line_id': line['id'],
        'received_quantity': got,
        'damaged_quantity': 0,
        'missing_quantity': remaining - got,
      });
    }
    if (body.isEmpty) return;
    setState(() {
      busy = true;
      error = null;
    });
    try {
      final next = await c.send(
        'POST',
        '${BranchStockController.root}/transfers/{transferId}/receive',
        parameters: {'transferId': '${transfer!['id']}'},
        body: {'source_ref': receiptRef, 'lines': body},
      );
      if (mounted) {
        setState(() {
          transfer = next;
          received = true;
        });
      }
    } catch (e) {
      if (mounted) setState(() => error = BranchStockController.message(e));
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  Future<void> cancelDraft() async {
    if (busy || transfer == null || sent) return;
    setState(() => busy = true);
    try {
      await c.send(
        'POST',
        '${BranchStockController.root}/transfers/{transferId}/cancel',
        parameters: {'transferId': '${transfer!['id']}'},
      );
      if (mounted) {
        setState(() {
          transfer = null;
          cancelled = true;
          error = null;
        });
      }
    } catch (e) {
      if (mounted) setState(() => error = BranchStockController.message(e));
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  Widget lineEditor(_TransferLine line, int index) => Padding(
    padding: const EdgeInsets.only(bottom: 12),
    child: SettingsCard(
      child: Column(
        children: [
          DropdownButtonFormField<String>(
            key: ValueKey('item-$index-${source.length}'),
            initialValue: line.itemId,
            isExpanded: true,
            decoration: const InputDecoration(labelText: 'Item'),
            items: [
              for (final i in source)
                DropdownMenuItem(
                  value: i.id,
                  child: Text('${i.name} (${i.amount(i.onHand)} in stock)'),
                ),
            ],
            onChanged: busy || transfer != null
                ? null
                : (v) => setState(() => line.itemId = v),
          ),
          const SizedBox(height: 12),
          _quantityField(
            key: ValueKey('quantity-$index'),
            controller: line.quantity,
            label: sourceItem(line.itemId)?.unit == null
                ? 'Quantity to move'
                : 'Quantity to move (${sourceItem(line.itemId)!.unit})',
            enabled: !busy && transfer == null,
            onChanged: (_) => setState(() {}),
          ),
          if (lines.length > 1 && transfer == null)
            Align(
              alignment: Alignment.centerRight,
              child: TextButton(
                onPressed: busy
                    ? null
                    : () => setState(() {
                        lines.removeAt(index).quantity.dispose();
                      }),
                child: const Text('Remove'),
              ),
            ),
        ],
      ),
    ),
  );

  Widget arrival() {
    final rows = (transfer!['lines'] as List? ?? []).map(jsonObject).toList();
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        SettingsCard(
          color: KarryamColors.mint.forInk(context),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                received ? 'Stock received' : 'Stock is on its way',
                style: Theme.of(context).textTheme.titleLarge,
              ),
              const SizedBox(height: 8),
              Text(
                received
                    ? 'The items have been added to ${branchName(toId)}.'
                    : 'The items have left ${branchName(fromId)}. When they arrive at ${branchName(toId)}, enter what was received.',
              ),
              for (final l in rows) ...[
                const SizedBox(height: 8),
                Text(
                  received
                      ? '${itemName('${l['source_inventory_item_id']}')}: ${l['received_quantity']} received'
                            '${(l['missing_quantity'] as int) > 0 ? ', ${l['missing_quantity']} missing' : ''}'
                      : '${itemName('${l['source_inventory_item_id']}')}: ${l['sent_quantity']} sent',
                ),
              ],
            ],
          ),
        ),
        const SizedBox(height: 16),
        if (!received) ...[
          for (final l in rows)
            if (receivedQty['${l['id']}'] != null)
              Padding(
                padding: const EdgeInsets.only(bottom: 12),
                child: _quantityField(
                  key: ValueKey('received-${l['id']}'),
                  controller: receivedQty['${l['id']}']!,
                  label:
                      '${itemName('${l['source_inventory_item_id']}')} received',
                  enabled: !busy,
                ),
              ),
          const Text('Anything not received will be recorded as missing.'),
          const SizedBox(height: 12),
          SettingsAction(
            'Confirm received',
            icon: Icons.check_rounded,
            busy: busy,
            onPressed: busy ? null : receive,
          ),
        ] else
          SettingsAction(
            'Done',
            icon: Icons.check_rounded,
            onPressed: () => Navigator.maybePop(context),
          ),
      ],
    );
  }

  @override
  Widget build(BuildContext context) {
    final issue = source.isEmpty ? null : problem;
    return Scaffold(
      appBar: AppBar(
        title: const Text('Move stock'),
        actions: [
          IconButton(
            tooltip: 'Refresh',
            onPressed: loading || busy || transfer != null
                ? null
                : loadBranches,
            icon: const Icon(Icons.refresh_rounded),
          ),
        ],
      ),
      body: SafeArea(
        child: ListView(
          padding: const EdgeInsets.all(20),
          children: [
            const SettingsHeading(
              'Move stock to another branch',
              subtitle:
                  'Send ingredients or packaging from one of your branches to another.',
            ),
            if (loading)
              const PremiumSkeleton(detail: true)
            else ...[
              if (error != null) SettingsNote(error!),
              if (cancelled)
                const SettingsNote(
                  'The move was cancelled. Stock is unchanged.',
                  warning: true,
                ),
              if (branches.length < 2 && error == null)
                _emptyCard(
                  Icons.storefront_outlined,
                  'You need two branches',
                  'Add another branch before moving stock between branches.',
                )
              else if (branches.length >= 2) ...[
                _branchPicker(
                  context,
                  label: 'From branch',
                  branches: branches,
                  value: fromId,
                  onChanged: busy || transfer != null
                      ? null
                      : (v) {
                          setState(() {
                            fromId = v;
                            if (toId == v) toId = null;
                            source = [];
                          });
                          unawaited(loadItems());
                        },
                ),
                const SizedBox(height: 12),
                _branchPicker(
                  context,
                  label: 'To branch',
                  branches: branches,
                  value: toId,
                  exclude: fromId,
                  onChanged: busy || transfer != null
                      ? null
                      : (v) {
                          setState(() {
                            toId = v;
                            source = [];
                          });
                          unawaited(loadItems());
                        },
                ),
                const SizedBox(height: 20),
                if (itemsLoading)
                  const PremiumSkeleton()
                else if (transfer != null && (sent || received))
                  arrival()
                else if (fromId != null && toId != null) ...[
                  if (source.isEmpty && error == null)
                    _emptyCard(
                      Icons.inventory_2_outlined,
                      'No stock to move',
                      '${branchName(fromId)} has no tracked ingredients or packaging.',
                    )
                  else if (source.isNotEmpty) ...[
                    for (var i = 0; i < lines.length; i++)
                      lineEditor(lines[i], i),
                    if (transfer == null)
                      TextButton.icon(
                        onPressed: busy
                            ? null
                            : () => setState(
                                () => lines.add(_TransferLine(null)),
                              ),
                        icon: const Icon(Icons.add_rounded),
                        label: const Text('Add another item'),
                      ),
                    const SizedBox(height: 12),
                    TextField(
                      controller: notes,
                      enabled: !busy && transfer == null,
                      decoration: const InputDecoration(
                        labelText: 'Note (optional)',
                      ),
                    ),
                    const SizedBox(height: 16),
                    if (issue != null)
                      Padding(
                        padding: const EdgeInsets.only(bottom: 12),
                        child: Text(issue),
                      ),
                    SettingsAction(
                      transfer == null ? 'Send stock' : 'Retry sending',
                      icon: Icons.local_shipping_outlined,
                      busy: busy,
                      onPressed: busy || issue != null ? null : send,
                    ),
                    if (transfer != null) ...[
                      const SizedBox(height: 12),
                      SettingsAction(
                        'Cancel move',
                        secondary: true,
                        danger: true,
                        onPressed: busy ? null : cancelDraft,
                      ),
                    ],
                  ],
                ],
              ],
            ],
          ],
        ),
      ),
    );
  }
}
