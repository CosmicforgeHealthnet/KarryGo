import '../../../core/ui/premium_loading.dart';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../settings/ui/settings_design.dart';
import '../state/stock_controller.dart';
import 'stock_screen.dart';
import '../../catalogue/ui/catalogue_fields.dart' show catalogueMinor;

enum StockEdit { basis, receipt, adjustment, stocktake }

const stockAdjustmentReasons = [
  'FOUND_STOCK',
  'DATA_CORRECTION',
  'DAMAGE',
  'WASTE',
  'SPOILAGE',
  'THEFT_LOSS',
  'EXPIRY',
  'MANUAL_CORRECTION',
  'OTHER_CONTROLLED',
];

class StockEditor extends StatefulWidget {
  const StockEditor({
    super.key,
    required this.runtime,
    required this.controller,
    required this.kind,
    this.record,
  });
  final VendorRuntime runtime;
  final StockController controller;
  final StockEdit kind;
  final JsonObject? record;
  @override
  State<StockEditor> createState() => _StockEditorState();
}

class _StockEditorState extends State<StockEditor> {
  final form = GlobalKey<FormState>();
  late final source = TextEditingController(text: mock ? 'DELIVERY-001' : '');
  late final supplier = TextEditingController(text: mock ? 'SUPPLIER-001' : '');
  late final note = TextEditingController(
    text: mock ? 'Checked by the receiving team.' : '',
  );
  late final quantity = TextEditingController(text: mock ? '5' : '');
  final receiptLines = <ReceiptEntry>[];
  final selected = <String>{};
  List<JsonObject> balances = [];
  String? listingId, measureId;
  List<JsonObject> get eligibleListings => c.listings
      .where(
        (l) =>
            l['status'] == 'ACTIVE' &&
            !balances.any((b) => b['listing_ref'] == l['id']),
      )
      .toList();
  Future<void> receiveExisting() async {
    final saved = await Navigator.push<JsonObject>(
      context,
      MaterialPageRoute(
        builder: (_) => StockEditor(
          runtime: widget.runtime,
          controller: c,
          kind: StockEdit.receipt,
        ),
      ),
    );
    if (!mounted || saved == null) return;
    await Navigator.push(
      context,
      MaterialPageRoute(
        builder: (_) => StockRecordScreen(
          controller: c,
          row: saved,
          kind: 'receipts',
          mock: mock,
        ),
      ),
    );
    if (mounted) Navigator.pop(context);
  }

  String consumption = 'SAME_UNIT',
      allocation = 'FIFO',
      reason = 'MANUAL_CORRECTION',
      stocktakeType = 'FULL_STOCKTAKE';
  bool lotTracking = false,
      subtract = false,
      loading = true,
      presenting = false,
      submitted = false;
  String? error;
  DateTime receivedAt = DateTime.now();
  StockController get c => widget.controller;
  bool get locked => c.busy || c.commands.isNotEmpty;
  bool get mock => widget.runtime.mode == VendorDataMode.mock;
  String get title => switch (widget.kind) {
    StockEdit.basis => 'Stock tracking',
    StockEdit.receipt => 'Receive goods',
    StockEdit.adjustment => 'Adjust stock',
    StockEdit.stocktake => 'Start stocktake',
  };
  @override
  void initState() {
    super.initState();
    prepare();
  }

  Future<void> prepare() async {
    try {
      if (c.setupError != null) throw StateError(c.setupError!);
      balances = StockController.collection(
        (await c.app.shopCatalogueApi.request(
          'GET',
          '${StockController.branchPath}/balances',
          pathParameters: {'branchId': c.branchId},
        )).data,
      );
      if (mock) {
        listingId = c.listings
            .where(
              (l) =>
                  l['status'] == 'ACTIVE' &&
                  !balances.any((b) => b['listing_ref'] == l['id']),
            )
            .firstOrNull?['id']
            ?.toString();
        measureId = c.measures
            .where((m) => m['status'] == 'ACTIVE')
            .firstOrNull?['id']
            ?.toString();
      }
      if (widget.kind == StockEdit.receipt) addLine();
    } catch (e) {
      error = c.message(e);
    }
    if (mounted) setState(() => loading = false);
  }

  void addLine() {
    final row = ReceiptEntry(mock: mock);
    if (mock) {
      row.stockBasisId = balances
          .where(
            (b) => !receiptLines.any(
              (l) => l.stockBasisId == b['stock_basis_ref'],
            ),
          )
          .firstOrNull?['stock_basis_ref']
          ?.toString();
    }
    receiptLines.add(row);
  }

  @override
  void dispose() {
    source.dispose();
    supplier.dispose();
    note.dispose();
    quantity.dispose();
    for (final line in receiptLines) {
      line.dispose();
    }
    super.dispose();
  }

  String? countValidator(String? value, {bool zero = false}) {
    final n = int.tryParse(value?.trim() ?? '');
    return n == null || n < (zero ? 0 : 1) || n > 1000000000
        ? 'Enter a whole quantity ${zero ? 'from 0' : 'from 1'} to 1,000,000,000.'
        : null;
  }

  Future<void> date({ReceiptEntry? line, String? field}) async {
    final initial = line == null ? receivedAt : line.dates[field] ?? receivedAt;
    final picked = await showDatePicker(
      context: context,
      initialDate: initial,
      firstDate: DateTime(2000),
      lastDate: DateTime(2100),
    );
    if (picked == null || !mounted) return;
    final time = await showTimePicker(
      context: context,
      initialTime: TimeOfDay.fromDateTime(initial),
    );
    if (time == null || !mounted) return;
    final value = DateTime(
      picked.year,
      picked.month,
      picked.day,
      time.hour,
      time.minute,
    );
    setState(() {
      if (line == null) {
        receivedAt = value;
      } else {
        line.dates[field!] = value;
      }
    });
  }

  Future<void> save() async {
    if (c.busy || presenting || submitted || !form.currentState!.validate()) {
      return;
    }
    if (widget.kind == StockEdit.stocktake &&
        stocktakeType == 'CYCLE_COUNT' &&
        selected.isEmpty) {
      setState(() => error = 'Choose the stock items to count.');
      return;
    }
    FocusManager.instance.primaryFocus?.unfocus();
    setState(() {
      presenting = true;
      error = null;
    });
    if (widget.kind == StockEdit.adjustment) {
      final confirmed = await confirmSettings(
        context,
        'Apply stock adjustment?',
        '${subtract ? 'Remove' : 'Add'} ${quantity.text.trim()} stock units for ${c.name(widget.record!['variant_ref'])}. This updates physical stock immediately.',
      );
      if (!mounted) return;
      if (!confirmed) {
        setState(() => presenting = false);
        return;
      }
    }
    setState(() => presenting = false);
    final kind = widget.kind;
    final path = switch (kind) {
      StockEdit.basis => '${StockController.branchPath}/stock-bases',
      StockEdit.receipt => '${StockController.root}/receipts',
      StockEdit.adjustment => '${StockController.root}/adjustments',
      StockEdit.stocktake => '${StockController.branchPath}/stocktakes',
    };
    final body = switch (kind) {
      StockEdit.basis => {
        'listing_id': listingId,
        'measure_id': measureId,
        'consumption_mode': consumption,
        'allocation_policy': allocation,
        'lot_tracking': lotTracking,
        'expected_version': 0,
      },
      StockEdit.receipt => {
        'branch_id': c.branchId,
        'received_at': receivedAt.toUtc().toIso8601String(),
        if (source.text.trim().isNotEmpty) 'source_ref': source.text.trim(),
        if (supplier.text.trim().isNotEmpty)
          'supplier_ref': supplier.text.trim(),
        if (note.text.trim().isNotEmpty) 'notes': note.text.trim(),
        'lines': [for (final line in receiptLines) line.payload()],
      },
      StockEdit.adjustment => {
        'stock_basis_id': widget.record!['stock_basis_ref'],
        'expected_revision': widget.record!['revision'],
        'quantity_delta': int.parse(quantity.text.trim()) * (subtract ? -1 : 1),
        'reason_code': reason,
        if (note.text.trim().isNotEmpty) 'note': note.text.trim(),
      },
      StockEdit.stocktake => {
        'stocktake_type': stocktakeType,
        if (stocktakeType == 'CYCLE_COUNT')
          'stock_basis_ids': selected.toList(),
      },
    };
    final result = await c.mutate(
      kind == StockEdit.basis ? 'PUT' : 'POST',
      path,
      body: body,
      parameters: {
        if ({StockEdit.basis, StockEdit.stocktake}.contains(kind))
          'branchId': c.branchId,
      },
    );
    if (!mounted) return;
    if (result != null) setState(() => submitted = true);
    await stockFeedback(
      context,
      c,
      result,
      switch (kind) {
        StockEdit.basis => 'Stock tracking ready',
        StockEdit.receipt => 'Receipt saved',
        StockEdit.adjustment => 'Stock adjusted',
        StockEdit.stocktake => 'Ready to count',
      },
      switch (kind) {
        StockEdit.basis => 'You can now receive goods for this listing.',
        StockEdit.receipt =>
          'Your draft is ready to review. Post it to add goods to stock.',
        StockEdit.adjustment => 'Your adjustment has been applied.',
        StockEdit.stocktake =>
          'The stock snapshot is ready. Record each physical count, then post it.',
      },
    );
    if (!mounted) return;
    if (result != null) {
      Navigator.pop(context, result);
    } else {
      setState(() => error = c.error);
      if (kind == StockEdit.adjustment &&
          c.errorCode == 'SHOP_INVENTORY_STALE_REVISION') {
        try {
          final latest = await c.balance(
            widget.record!['stock_basis_ref'].toString(),
          );
          if (mounted) {
            setState(() {
              widget.record!.addAll(latest);
              error =
                  'Stock changed. The latest quantities are shown above. Review your adjustment before submitting again.';
            });
          }
        } catch (e) {
          if (mounted) {
            setState(() {
              submitted = true;
              error =
                  'Refresh the stock detail before making another adjustment. ${c.message(e)}';
            });
          }
        }
      }
    }
  }

  Widget select(
    String label,
    String? value,
    List<DropdownMenuItem<String>> items,
    ValueChanged<String?> onChanged,
  ) => Padding(
    padding: const EdgeInsets.only(bottom: 18),
    child: DropdownButtonFormField<String>(
      initialValue: value,
      isExpanded: true,
      decoration: InputDecoration(labelText: label),
      items: items,
      onChanged: locked ? null : onChanged,
      validator: (v) => v == null ? 'Choose ${label.toLowerCase()}.' : null,
    ),
  );
  List<DropdownMenuItem<String>> choices(List<String> values) => [
    for (final v in values)
      DropdownMenuItem(value: v, child: Text(dashboardLabel(v))),
  ];
  Widget field(
    String label,
    TextEditingController controller, {
    bool numeric = false,
    bool money = false,
    String? Function(String?)? validator,
    int maxLength = 200,
  }) => Padding(
    padding: const EdgeInsets.only(bottom: 18),
    child: TextFormField(
      controller: controller,
      enabled: !locked,
      maxLength: maxLength,
      keyboardType: money
          ? const TextInputType.numberWithOptions(decimal: true)
          : numeric
          ? TextInputType.number
          : TextInputType.text,
      decoration: InputDecoration(labelText: label, counterText: ''),
      validator: validator,
    ),
  );
  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => PopScope(
      canPop: !locked,
      child: Scaffold(
        backgroundColor: KarryamColors.canvas.forSurface(context),
        appBar: AppBar(title: Text(title)),
        bottomNavigationBar: loading
            ? null
            : SafeArea(
                child: Padding(
                  padding: const EdgeInsets.fromLTRB(22, 12, 22, 18),
                  child: SettingsAction(
                    switch (widget.kind) {
                      StockEdit.basis => 'Enable stock tracking',
                      StockEdit.receipt => 'Save draft receipt',
                      StockEdit.adjustment => 'Apply adjustment',
                      StockEdit.stocktake => 'Start counting',
                    },
                    busy: c.busy,
                    onPressed:
                        presenting ||
                            submitted ||
                            (widget.kind == StockEdit.basis &&
                                (eligibleListings.isEmpty ||
                                    c.setupError != null)) ||
                            (widget.kind == StockEdit.receipt &&
                                balances.isEmpty) ||
                            (widget.kind == StockEdit.stocktake &&
                                balances.isEmpty)
                        ? null
                        : save,
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
                      title,
                      subtitle: switch (widget.kind) {
                        StockEdit.basis =>
                          'Choose the measure in which this listing holds physical stock.',
                        StockEdit.receipt =>
                          'Record what arrived. Stock changes only after you post the receipt.',
                        StockEdit.adjustment =>
                          'Record a controlled correction to physical stock.',
                        StockEdit.stocktake =>
                          'Take a snapshot, count your stock, then reconcile.',
                      },
                    ),
                    if (c.commands.isNotEmpty)
                      const SettingsNote(
                        'The last request could not be confirmed. Keep these details and retry the same submission.',
                        warning: true,
                      ),
                    if (error != null) SettingsNote(error!),
                    if (widget.kind == StockEdit.basis) ...[
                      if (eligibleListings.isEmpty) ...[
                        SettingsNote(
                          balances.isNotEmpty
                              ? 'No active untracked listing is available. Receive goods to add quantities to existing tracked stock.'
                              : 'No active branch listing is available. Add or activate a Shop listing in Catalogue first.',
                          warning: true,
                        ),
                        if (balances.isNotEmpty)
                          SettingsAction(
                            'Receive goods',
                            icon: Icons.move_to_inbox_outlined,
                            onPressed: locked ? null : receiveExisting,
                          ),
                        const SizedBox(height: 18),
                      ] else
                        select(
                          'Branch listing',
                          listingId,
                          [
                            for (final l in eligibleListings)
                              DropdownMenuItem(
                                value: l['id'].toString(),
                                child: Text(c.name(l['variant_ref'])),
                              ),
                          ],
                          (v) => setState(() => listingId = v),
                        ),
                      if (eligibleListings.isNotEmpty) ...[
                        select(
                          'Stock measure',
                          measureId,
                          [
                            for (final m in c.measures)
                              if (m['status'] == 'ACTIVE')
                                DropdownMenuItem(
                                  value: m['id'].toString(),
                                  child: Text(m['display_name'].toString()),
                                ),
                          ],
                          (v) => setState(() => measureId = v),
                        ),
                        select(
                          'Consumption mode',
                          consumption,
                          choices([
                            'SAME_UNIT',
                            'EXACT_CONVERSION',
                            'VARIABLE_FULFILMENT',
                          ]),
                          (v) => setState(() => consumption = v!),
                        ),
                        select(
                          'Lot allocation',
                          allocation,
                          choices(['FIFO', 'FEFO']),
                          (v) => setState(() => allocation = v!),
                        ),
                        const Text(
                          'FIFO uses the oldest received goods first. FEFO uses goods with the earliest expiry first.',
                        ),
                        const SizedBox(height: 14),
                        SwitchListTile.adaptive(
                          contentPadding: EdgeInsets.zero,
                          title: const Text('Track lots'),
                          value: lotTracking,
                          activeTrackColor: KarryamColors.teal.forInk(context),
                          onChanged: locked
                              ? null
                              : (v) => setState(() => lotTracking = v),
                        ),
                      ],
                    ],
                    if (widget.kind == StockEdit.receipt) ...[
                      SettingsLink(
                        'Received date & time',
                        Icons.calendar_today_outlined,
                        subtitle: receivedAt.toString().substring(0, 16),
                        onTap: locked ? null : () => date(),
                      ),
                      const SizedBox(height: 18),
                      field('Delivery reference (optional)', source),
                      field('Supplier reference (optional)', supplier),
                      for (var i = 0; i < receiptLines.length; i++)
                        receiptLine(receiptLines[i], i),
                      SettingsAction(
                        'Add another item',
                        secondary: true,
                        icon: Icons.add_rounded,
                        onPressed:
                            locked ||
                                receiptLines.length >= balances.length ||
                                receiptLines.length >= 500
                            ? null
                            : () => setState(addLine),
                      ),
                      const SizedBox(height: 22),
                      field(
                        'Receiving notes (optional)',
                        note,
                        maxLength: 2000,
                      ),
                    ],
                    if (widget.kind == StockEdit.adjustment) ...[
                      SettingsCard(
                        color: KarryamColors.mint.forInk(context),
                        child: Column(
                          children: [
                            Text(
                              c.name(widget.record!['variant_ref']),
                              style: const TextStyle(
                                fontSize: 18,
                                fontWeight: FontWeight.w700,
                              ),
                            ),
                            const SizedBox(height: 12),
                            SettingsFact(
                              'On hand',
                              '${widget.record!['on_hand_quantity']}',
                            ),
                            SettingsFact(
                              'Available',
                              '${widget.record!['available_quantity']}',
                            ),
                          ],
                        ),
                      ),
                      const SizedBox(height: 22),
                      Row(
                        children: [
                          for (final remove in [false, true])
                            Expanded(
                              child: Padding(
                                padding: const EdgeInsets.only(right: 8),
                                child: ChoiceChip(
                                  label: Text(
                                    remove ? 'Remove stock' : 'Add stock',
                                  ),
                                  selected: subtract == remove,
                                  selectedColor: KarryamColors.mint.forSurface(
                                    context,
                                  ),
                                  showCheckmark: false,
                                  onSelected: locked
                                      ? null
                                      : (_) =>
                                            setState(() => subtract = remove),
                                ),
                              ),
                            ),
                        ],
                      ),
                      const SizedBox(height: 18),
                      field(
                        'Stock quantity',
                        quantity,
                        numeric: true,
                        validator: (v) => countValidator(v),
                      ),
                      select(
                        'Reason',
                        reason,
                        choices(stockAdjustmentReasons),
                        (v) => setState(() => reason = v!),
                      ),
                      field('Note (optional)', note, maxLength: 1000),
                    ],
                    if (widget.kind == StockEdit.stocktake) ...[
                      select(
                        'Count type',
                        stocktakeType,
                        choices(['FULL_STOCKTAKE', 'CYCLE_COUNT']),
                        (v) => setState(() => stocktakeType = v!),
                      ),
                      if (stocktakeType == 'FULL_STOCKTAKE')
                        const SettingsCard(
                          child: Text(
                            'All tracked stock items in this branch will be included.',
                          ),
                        )
                      else
                        for (final row in balances)
                          CheckboxListTile(
                            contentPadding: EdgeInsets.zero,
                            title: Text(c.name(row['variant_ref'])),
                            value: selected.contains(row['stock_basis_ref']),
                            activeColor: KarryamColors.tealInk.forInk(context),
                            onChanged: locked
                                ? null
                                : (v) => setState(() {
                                    if (v == true) {
                                      selected.add(
                                        row['stock_basis_ref'].toString(),
                                      );
                                    } else {
                                      selected.remove(row['stock_basis_ref']);
                                    }
                                  }),
                          ),
                    ],
                    const SizedBox(height: 26),
                  ],
                ),
              ),
      ),
    ),
  );
  Widget receiptLine(ReceiptEntry line, int index) => Padding(
    padding: const EdgeInsets.only(bottom: 18),
    child: SettingsCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Expanded(
                child: Text(
                  'Received item ${index + 1}',
                  style: const TextStyle(
                    fontSize: 18,
                    fontWeight: FontWeight.w700,
                  ),
                ),
              ),
              if (receiptLines.length > 1)
                IconButton(
                  tooltip: 'Remove item',
                  onPressed: locked
                      ? null
                      : () => setState(() => receiptLines.remove(line)),
                  icon: Icon(
                    Icons.delete_outline_rounded,
                    color: KarryamColors.error.forInk(context),
                  ),
                ),
            ],
          ),
          const SizedBox(height: 18),
          select(
            'Stock item',
            line.stockBasisId,
            [
              for (final row in balances)
                if (!receiptLines.any(
                  (other) =>
                      other != line &&
                      other.stockBasisId == row['stock_basis_ref'],
                ))
                  DropdownMenuItem(
                    value: row['stock_basis_ref'].toString(),
                    child: Text(c.name(row['variant_ref'])),
                  ),
            ],
            (v) => setState(() => line.stockBasisId = v),
          ),
          field(
            'Stock quantity',
            line.quantity,
            numeric: true,
            validator: (v) => countValidator(v),
          ),
          ExpansionTile(
            tilePadding: EdgeInsets.zero,
            title: const Text('Lot, dates & cost (optional)'),
            children: [
              field('Lot number', line.lot),
              field('Batch number', line.batch),
              field(
                'Unit cost (optional)',
                line.cost,
                money: true,
                validator: (v) {
                  if (v == null || v.trim().isEmpty) return null;
                  final n = catalogueMinor(v.trim());
                  return n == null || n < 0 || n > 99999999999
                      ? 'Enter a valid amount with up to two decimal places.'
                      : null;
                },
              ),
              for (final key in [
                'manufactured_at',
                'best_before_at',
                'expires_at',
              ])
                Row(
                  children: [
                    Expanded(
                      child: SettingsLink(
                        dashboardLabel(key),
                        Icons.event_outlined,
                        subtitle:
                            line.dates[key]?.toString().substring(0, 16) ??
                            'Not set',
                        onTap: locked
                            ? null
                            : () => date(line: line, field: key),
                      ),
                    ),
                    if (line.dates[key] != null)
                      IconButton(
                        tooltip: 'Clear ${dashboardLabel(key)}',
                        onPressed: locked
                            ? null
                            : () => setState(() => line.dates.remove(key)),
                        icon: const Icon(Icons.close_rounded),
                      ),
                  ],
                ),
            ],
          ),
        ],
      ),
    ),
  );
}

class ReceiptEntry {
  ReceiptEntry({required bool mock})
    : quantity = TextEditingController(text: mock ? '10' : ''),
      lot = TextEditingController(text: mock ? 'LOT-001' : ''),
      batch = TextEditingController(text: mock ? 'BATCH-001' : '');
  String? stockBasisId;
  final TextEditingController quantity, lot, batch;
  final cost = TextEditingController();
  final dates = <String, DateTime>{};
  JsonObject payload() => {
    'stock_basis_id': stockBasisId,
    'quantity': int.parse(quantity.text.trim()),
    if (lot.text.trim().isNotEmpty) 'lot_number': lot.text.trim(),
    if (batch.text.trim().isNotEmpty) 'batch_number': batch.text.trim(),
    if (cost.text.trim().isNotEmpty)
      'unit_cost_minor': catalogueMinor(cost.text.trim()),
    for (final entry in dates.entries)
      entry.key: entry.value.toUtc().toIso8601String(),
  };
  void dispose() {
    quantity.dispose();
    lot.dispose();
    batch.dispose();
    cost.dispose();
  }
}
