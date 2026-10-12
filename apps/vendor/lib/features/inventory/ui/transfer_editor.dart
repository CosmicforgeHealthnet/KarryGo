import '../../../core/ui/premium_loading.dart';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../settings/ui/settings_design.dart';
import '../state/stock_controller.dart';
import 'stock_screen.dart';
import 'transfer_screen.dart';

class TransferEditor extends StatefulWidget {
  const TransferEditor({
    super.key,
    required this.runtime,
    required this.controller,
    this.record,
  });
  final VendorRuntime runtime;
  final StockController controller;
  final JsonObject? record;
  @override
  State<TransferEditor> createState() => _TransferEditorState();
}

class _TransferEditorState extends State<TransferEditor> {
  StockController get c => widget.controller;
  bool get receiving => widget.record != null;
  bool get mock => widget.runtime.mode == VendorDataMode.mock;
  bool get locked => c.busy || uncertain || submitted;
  final form = GlobalKey<FormState>();
  late final notes = TextEditingController(
    text: mock ? 'Handle with care. Confirm goods on arrival.' : '',
  );
  final entries = <TransferEntry>[], removedEntries = <TransferEntry>[];
  final arrivals = <ArrivalEntry>[];
  List<JsonObject> source = [], destinationStock = [];
  String? destination, error;
  bool prepared = false,
      loading = true,
      destinationLoading = false,
      presenting = false,
      submitted = false,
      uncertain = false;
  int generation = 0;
  List<JsonObject> get eligibleBranches => c.branches
      .where((b) => b['id'] != c.branchId && b['status'] == 'ACTIVE')
      .toList();
  List<JsonObject> get pairs => source
      .where(
        (row) => destinationStock.any(
          (to) => to['variant_ref'] == row['variant_ref'],
        ),
      )
      .toList();
  @override
  void initState() {
    super.initState();
    prepare();
  }

  @override
  void dispose() {
    generation++;
    notes.dispose();
    for (final entry in [...entries, ...removedEntries]) {
      entry.dispose();
    }
    for (final arrival in arrivals) {
      arrival.dispose();
    }
    super.dispose();
  }

  Future<void> prepare() async {
    setState(() {
      loading = true;
      prepared = false;
      error = null;
    });
    try {
      if (receiving) {
        final fresh = await c.transfer(widget.record!['id'].toString());
        if (!{'IN_TRANSIT', 'PARTIALLY_RECEIVED'}.contains(fresh['status'])) {
          throw const FormatException('Transfer no longer accepts arrivals');
        }
        widget.record!.clear();
        widget.record!.addAll(fresh);
        for (final entry in arrivals) {
          entry.dispose();
        }
        arrivals.clear();
        for (final raw in fresh['lines']) {
          final line = jsonObject(raw);
          if (transferRemaining(line) > 0) {
            arrivals.add(ArrivalEntry(line, mock: mock));
          }
        }
      } else {
        if (c.setupError != null) throw StateError(c.setupError!);
        await c.loadBranches();
        source = await c.branchBalances(c.branchId);
        if (entries.isEmpty) addEntry();
        if (mock && eligibleBranches.isNotEmpty) {
          destination = eligibleBranches.first['id'].toString();
          await chooseDestination(destination);
        }
      }
      prepared = true;
    } catch (e) {
      error = c.message(e);
    }
    if (mounted) setState(() => loading = false);
  }

  Future<void> chooseDestination(String? value) async {
    final ticket = ++generation;
    setState(() {
      destination = value;
      destinationStock = [];
      error = null;
      destinationLoading = true;
      for (final entry in entries) {
        entry.sourceId = null;
      }
    });
    try {
      if (value == null) return;
      final rows = await c.branchBalances(value);
      if (!mounted || ticket != generation) return;
      setState(() {
        destinationStock = rows;
        if (mock) {
          for (var i = 0; i < entries.length && i < pairs.length; i++) {
            entries[i].sourceId = pairs[i]['stock_basis_ref'].toString();
          }
        }
      });
    } catch (e) {
      if (mounted && ticket == generation) setState(() => error = c.message(e));
    } finally {
      if (mounted && ticket == generation) {
        setState(() => destinationLoading = false);
      }
    }
  }

  void addEntry() {
    final entry = TransferEntry(mock: mock);
    if (mock) {
      entry.sourceId = pairs
          .where(
            (row) => !entries.any(
              (entry) => entry.sourceId == row['stock_basis_ref'],
            ),
          )
          .firstOrNull?['stock_basis_ref']
          ?.toString();
    }
    entries.add(entry);
  }

  String? number(String? text, {bool zero = false}) {
    final value = int.tryParse(text?.trim() ?? '');
    return value == null || value < (zero ? 0 : 1) || value > 1000000000
        ? 'Enter a whole quantity ${zero ? 'from 0' : 'from 1'} to 1,000,000,000.'
        : null;
  }

  List<JsonObject> receiptPayload() => [
    for (final entry in arrivals)
      if (entry.included) entry.payload(),
  ];
  Future<void> submit() async {
    if (c.busy || presenting || submitted || !form.currentState!.validate()) {
      return;
    }
    final receipt = receiving ? receiptPayload() : <JsonObject>[];
    if (receiving && receipt.isEmpty) {
      setState(() => error = 'Include at least one item in this arrival.');
      return;
    }
    if (receiving) {
      for (final entry in arrivals.where((entry) => entry.included)) {
        if (entry.total == 0 || entry.total > transferRemaining(entry.line)) {
          setState(
            () => error =
                'For ${c.name(entry.line['variant_ref'])}, account for at least one unit and no more than the ${transferRemaining(entry.line)} remaining.',
          );
          return;
        }
      }
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
        mainAxisSize: MainAxisSize.min,
        children: [
          SettingsHeading(
            receiving ? 'Review this arrival' : 'Review transfer',
          ),
          TransferBranch(
            'From',
            c.name(
              receiving ? widget.record!['source_branch_ref'] : c.branchId,
            ),
            Icons.storefront_outlined,
          ),
          const SizedBox(height: 20),
          TransferBranch(
            'To',
            c.name(
              receiving
                  ? widget.record!['destination_branch_ref']
                  : destination,
            ),
            Icons.move_to_inbox_outlined,
          ),
          const SizedBox(height: 22),
          if (receiving) ...[
            for (final entry in arrivals.where((entry) => entry.included))
              Padding(
                padding: const EdgeInsets.only(bottom: 16),
                child: SettingsCard(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        c.name(entry.line['variant_ref']),
                        style: const TextStyle(fontWeight: FontWeight.w700),
                      ),
                      SettingsFact(
                        'Received in usable condition',
                        entry.received.text,
                      ),
                      SettingsFact('Damaged', entry.damaged.text),
                      SettingsFact('Missing', entry.missing.text),
                    ],
                  ),
                ),
              ),
            const Text(
              'Only usable received goods enter destination stock. Any remainder stays in transit.',
            ),
          ] else ...[
            for (final entry in entries)
              SettingsFact(
                c.name(
                  source.firstWhere(
                    (row) => row['stock_basis_ref'] == entry.sourceId,
                  )['variant_ref'],
                ),
                '${entry.quantity.text.trim()} stock units',
              ),
            const SizedBox(height: 16),
            const Text(
              'This saves a draft. Dispatch is a separate action that moves stock.',
            ),
          ],
        ],
      ),
      footer: SettingsAction(
        receiving ? 'Confirm arrival' : 'Create draft transfer',
        icon: receiving ? Icons.task_alt_rounded : Icons.swap_horiz_rounded,
        onPressed: () => Navigator.pop(context, true),
      ),
    );
    if (!mounted) return;
    setState(() => presenting = false);
    if (confirmed != true) return;
    final JsonObject body = receiving
        ? {'lines': receipt}
        : {
            'source_branch_id': c.branchId,
            'destination_branch_id': destination,
            if (notes.text.trim().isNotEmpty) 'notes': notes.text.trim(),
            'lines': [
              for (final entry in entries)
                {
                  'source_stock_basis_id': entry.sourceId,
                  'destination_stock_basis_id': destinationStock.firstWhere(
                    (to) =>
                        to['variant_ref'] ==
                        source.firstWhere(
                          (from) => from['stock_basis_ref'] == entry.sourceId,
                        )['variant_ref'],
                  )['stock_basis_ref'],
                  'quantity': int.parse(entry.quantity.text.trim()),
                },
            ],
          };
    final path = receiving
        ? '${StockController.root}/transfers/{transferId}/receive'
        : '${StockController.root}/transfers';
    final result = await c.mutate(
      'POST',
      path,
      body: body,
      parameters: {
        if (receiving) 'transferId': widget.record!['id'].toString(),
      },
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
      receiving ? 'Arrival recorded' : 'Transfer draft created',
      receiving
          ? (result != null && result['status'] == 'RECEIVED'
                ? 'All sent goods have been accounted for. Review any damaged or missing quantities in the transfer.'
                : 'This arrival is recorded. Unresolved quantities remain in transit for a later arrival.')
          : 'Review your transfer and dispatch when the goods have been sent.',
    );
    if (mounted && result != null) Navigator.pop(context, result);
  }

  Widget quantityField(
    String label,
    TextEditingController text, {
    bool zero = false,
  }) => Padding(
    padding: const EdgeInsets.only(bottom: 16),
    child: TextFormField(
      controller: text,
      enabled: !locked && !presenting,
      keyboardType: TextInputType.number,
      decoration: InputDecoration(labelText: label),
      validator: (value) => number(value, zero: zero),
      onChanged: (_) => setState(() {}),
    ),
  );
  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => PopScope(
      canPop: !c.busy && !uncertain,
      child: Scaffold(
        backgroundColor: KarryamColors.canvas.forSurface(context),
        appBar: AppBar(
          title: Text(receiving ? 'Record arrival' : 'New transfer'),
        ),
        bottomNavigationBar: loading
            ? null
            : SafeArea(
                child: Padding(
                  padding: const EdgeInsets.fromLTRB(22, 12, 22, 18),
                  child: SettingsAction(
                    uncertain
                        ? 'Retry same submission'
                        : receiving
                        ? 'Review arrival'
                        : 'Review transfer',
                    busy: c.busy,
                    icon: Icons.arrow_forward_rounded,
                    onPressed:
                        !prepared ||
                            presenting ||
                            submitted ||
                            destinationLoading ||
                            receiving && arrivals.isEmpty ||
                            !receiving &&
                                (destination == null || pairs.isEmpty) ||
                            c.denied.contains(
                              'POST ${StockController.root}/transfers${receiving ? '/{transferId}/receive' : ''}',
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
                    SettingsHeading(
                      receiving
                          ? 'What arrived?'
                          : 'Move stock between branches',
                      subtitle: receiving
                          ? 'Record this arrival at ${c.name(widget.record!['destination_branch_ref'])}. Quantities are for this arrival only.'
                          : 'Choose the destination and goods to send. Matching stock tracking must exist at both branches.',
                    ),
                    if (uncertain)
                      const SettingsNote(
                        'The response could not be confirmed. Keep these details and retry the same submission.',
                        warning: true,
                      ),
                    if (error != null) SettingsNote(error!),
                    if (!prepared)
                      TextButton.icon(
                        onPressed: locked ? null : prepare,
                        icon: const Icon(Icons.refresh_rounded),
                        label: const Text("Try again"),
                      ),
                    if (receiving) ...[
                      for (final entry in arrivals) arrivalCard(entry),
                      if (arrivals.isEmpty)
                        const SettingsNote(
                          'There are no remaining quantities to receive. Return to the transfer and refresh.',
                          warning: true,
                        ),
                    ] else ...[
                      SettingsCard(
                        color: KarryamColors.mint.forInk(context),
                        child: TransferBranch(
                          'From',
                          c.name(c.branchId),
                          Icons.storefront_outlined,
                        ),
                      ),
                      const SizedBox(height: 22),
                      DropdownButtonFormField<String>(
                        initialValue: destination,
                        isExpanded: true,
                        decoration: const InputDecoration(
                          labelText: 'Destination branch',
                        ),
                        items: [
                          for (final branch in eligibleBranches)
                            DropdownMenuItem(
                              value: branch['id'].toString(),
                              child: Text(branch['display_name'].toString()),
                            ),
                        ],
                        onChanged: locked || presenting || destinationLoading
                            ? null
                            : chooseDestination,
                        validator: (value) => value == null
                            ? 'Choose a destination branch.'
                            : null,
                      ),
                      if (eligibleBranches.isEmpty)
                        const Padding(
                          padding: EdgeInsets.only(top: 18),
                          child: SettingsNote(
                            'Another active branch is needed to create a transfer.',
                            warning: true,
                          ),
                        ),
                      const SizedBox(height: 24),
                      if (destinationLoading)
                        const PremiumSkeleton(detail: true, rows: 2)
                      else if (destination != null && pairs.isEmpty)
                        SettingsCard(
                          child: Column(
                            children: [
                              Icon(
                                Icons.link_off_rounded,
                                size: 44,
                                color: Color(0xFF986013).forInk(context),
                              ),
                              const SizedBox(height: 14),
                              const Text(
                                'No matching tracked stock',
                                style: TextStyle(
                                  fontSize: 18,
                                  fontWeight: FontWeight.w700,
                                ),
                              ),
                              const SizedBox(height: 10),
                              const Text(
                                'Set up stock tracking for the same product variant at both branches, then return here.',
                                textAlign: TextAlign.center,
                              ),
                              const SizedBox(height: 20),
                              SettingsAction(
                                'Reload destination stock',
                                secondary: true,
                                onPressed: locked
                                    ? null
                                    : () => chooseDestination(destination),
                              ),
                            ],
                          ),
                        )
                      else if (destination != null) ...[
                        for (var index = 0; index < entries.length; index++)
                          createCard(entries[index], index),
                        SettingsAction(
                          'Add another item',
                          secondary: true,
                          icon: Icons.add_rounded,
                          onPressed:
                              locked ||
                                  presenting ||
                                  entries.length >= pairs.length ||
                                  entries.length >= 200
                              ? null
                              : () => setState(addEntry),
                        ),
                      ],
                      const SizedBox(height: 24),
                      TextFormField(
                        controller: notes,
                        enabled: !locked && !presenting,
                        maxLines: 3,
                        maxLength: 2000,
                        decoration: const InputDecoration(
                          labelText: 'Transfer notes (optional)',
                          counterText: '',
                        ),
                      ),
                    ],
                  ],
                ),
              ),
      ),
    ),
  );
  Widget createCard(TransferEntry entry, int index) {
    final row = source
        .where((row) => row['stock_basis_ref'] == entry.sourceId)
        .firstOrNull;
    return Padding(
      padding: const EdgeInsets.only(bottom: 18),
      child: SettingsCard(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Expanded(
                  child: Text(
                    'Transfer item ${index + 1}',
                    style: const TextStyle(
                      fontSize: 18,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                ),
                if (entries.length > 1)
                  IconButton(
                    tooltip: 'Remove item',
                    onPressed: locked
                        ? null
                        : () => setState(() {
                            entries.remove(entry);
                            removedEntries.add(entry);
                          }),
                    icon: Icon(
                      Icons.delete_outline_rounded,
                      color: KarryamColors.error.forInk(context),
                    ),
                  ),
              ],
            ),
            const SizedBox(height: 18),
            DropdownButtonFormField<String>(
              key: ValueKey('$destination-${entry.sourceId}-$index'),
              initialValue: entry.sourceId,
              isExpanded: true,
              decoration: const InputDecoration(labelText: 'Stock item'),
              items: [
                for (final row in pairs)
                  if (!entries.any(
                    (other) =>
                        other != entry &&
                        other.sourceId == row['stock_basis_ref'],
                  ))
                    DropdownMenuItem(
                      value: row['stock_basis_ref'].toString(),
                      child: Text(c.name(row['variant_ref'])),
                    ),
              ],
              onChanged: locked || presenting
                  ? null
                  : (id) => setState(() => entry.sourceId = id),
              validator: (id) => id == null ? 'Choose a stock item.' : null,
            ),
            const SizedBox(height: 18),
            if (row != null) ...[
              SettingsFact(
                'Currently available at source',
                '${row['physical_available']}',
              ),
              const SizedBox(height: 16),
            ],
            quantityField('Quantity to send', entry.quantity),
          ],
        ),
      ),
    );
  }

  Widget arrivalCard(ArrivalEntry entry) => Padding(
    padding: const EdgeInsets.only(bottom: 18),
    child: SettingsCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          CheckboxListTile(
            contentPadding: EdgeInsets.zero,
            title: Text(
              c.name(entry.line['variant_ref']),
              style: const TextStyle(fontWeight: FontWeight.w700),
            ),
            subtitle: Text(
              '${transferRemaining(entry.line)} still to account for',
            ),
            activeColor: KarryamColors.tealInk.forInk(context),
            value: entry.included,
            onChanged: locked || presenting
                ? null
                : (value) => setState(() => entry.included = value!),
          ),
          if (entry.included) ...[
            const SizedBox(height: 18),
            quantityField(
              'Received in usable condition',
              entry.received,
              zero: true,
            ),
            quantityField('Damaged', entry.damaged, zero: true),
            quantityField('Missing', entry.missing, zero: true),
            if (entry.total >= 0)
              Text(
                '${entry.total} accounted for in this arrival',
                style: TextStyle(
                  fontWeight: FontWeight.w600,
                  color:
                      (entry.total > transferRemaining(entry.line)
                              ? KarryamColors.error
                              : KarryamColors.indigo)
                          .forInk(context),
                ),
              ),
          ],
        ],
      ),
    ),
  );
}

class TransferEntry {
  TransferEntry({required bool mock})
    : quantity = TextEditingController(text: mock ? '5' : '');
  String? sourceId;
  final TextEditingController quantity;
  void dispose() => quantity.dispose();
}

class ArrivalEntry {
  ArrivalEntry(this.line, {required bool mock})
    : received = TextEditingController(
        text: mock ? '${transferRemaining(line)}' : '',
      );
  final JsonObject line;
  final TextEditingController received;
  final damaged = TextEditingController(text: '0'),
      missing = TextEditingController(text: '0');
  bool included = true;
  int get total {
    final r = int.tryParse(received.text.trim()),
        d = int.tryParse(damaged.text.trim()),
        m = int.tryParse(missing.text.trim());
    return r == null || d == null || m == null ? -1 : r + d + m;
  }

  JsonObject payload() => {
    'line_id': line['id'],
    'received_quantity': int.parse(received.text.trim()),
    'damaged_quantity': int.parse(damaged.text.trim()),
    'missing_quantity': int.parse(missing.text.trim()),
  };
  void dispose() {
    received.dispose();
    damaged.dispose();
    missing.dispose();
  }
}
