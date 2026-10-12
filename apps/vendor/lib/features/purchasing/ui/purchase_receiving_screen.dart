import 'package:flutter/material.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../settings/ui/settings_design.dart';
import 'purchasing_forms.dart';

String? validatePurchaseInspection(JsonObject body) {
  final delivered = body['delivered_quantity'] as int,
      accepted = body['accepted_quantity'] as int,
      damaged = body['damaged_quantity'] as int,
      rejected = body['rejected_quantity'] as int;
  if (delivered != accepted + damaged + rejected) {
    return 'Delivered must equal accepted, damaged and rejected quantities combined.';
  }
  if (body['wrong_item'] == true &&
      (accepted != 0 ||
          (body['wrong_item_reference'] as String).trim().isEmpty ||
          delivered <= 0 ||
          (body['purchase_unit_code'] as String).trim().isEmpty)) {
    return 'Wrong items need a reference and unit, and cannot have accepted stock.';
  }
  return null;
}

class PurchaseReceivingScreen extends StatefulWidget {
  const PurchaseReceivingScreen({
    super.key,
    required this.order,
    required this.receipt,
    required this.mock,
  });
  final JsonObject order, receipt;
  final bool mock;
  @override
  State<PurchaseReceivingScreen> createState() =>
      _PurchaseReceivingScreenState();
}

class _PurchaseReceivingScreenState extends State<PurchaseReceivingScreen> {
  late final poLines = (widget.order['lines'] as List).map(jsonObject).toList();
  late final lines = <JsonObject>[
    for (final line in (widget.receipt['lines'] as List? ?? []))
      {
        for (final key in [
          'purchase_order_line_id',
          'delivered_quantity',
          'accepted_quantity',
          'damaged_quantity',
          'rejected_quantity',
          'short_quantity',
          'wrong_item',
          'wrong_item_reference',
          'purchase_unit_code',
          'quality_issue',
          'quality_notes',
          'actual_unit_cost_minor',
          'lot_number',
          'batch_number',
          'manufactured_at',
          'best_before_at',
          'expires_at',
        ])
          if (line[key] != null) key: line[key],
      },
  ];
  Future<void> edit({String? poId, int? index, bool wrong = false}) async {
    final current = index == null ? null : lines[index];
    wrong = current?['wrong_item'] == true || wrong;
    final po = poLines
        .where((p) => p['id'] == (current?['purchase_order_line_id'] ?? poId))
        .firstOrNull;
    final remaining = po == null
        ? 0
        : (po['purchase_quantity'] as int) -
              (po['received_purchase_quantity'] as int);
    PurchaseField f(
      String key,
      String label, {
      String kind = 'text',
      bool required = false,
      Object? initial,
    }) => PurchaseField(
      key,
      label,
      kind: kind,
      required: required,
      initial: current?[key] ?? initial,
    );
    final body = await purchaseForm(
      context,
      wrong
          ? 'Inspect wrong item'
          : 'Inspect ${po?['supplier_sku_snapshot'] ?? 'delivery'}',
      [
        if (wrong) ...[
          f('wrong_item_reference', 'Wrong item reference', required: true),
          f('purchase_unit_code', 'Purchase unit', required: true),
        ],
        for (final key in [
          'delivered_quantity',
          'accepted_quantity',
          'damaged_quantity',
          'rejected_quantity',
          'short_quantity',
        ])
          f(
            key,
            key.replaceAll('_', ' '),
            kind: 'number',
            required: true,
            initial:
                widget.mock &&
                    ['delivered_quantity', 'accepted_quantity'].contains(key) &&
                    !wrong
                ? remaining
                : 0,
          ),
        f('quality_issue', 'Quality issue', kind: 'bool', initial: false),
        f('quality_notes', 'Quality notes', kind: 'notes'),
        f(
          'actual_unit_cost_minor',
          'Actual unit cost (${widget.order['currency_code']})',
          kind: 'money',
          initial: po?['unit_cost_minor'],
        ),
        f('lot_number', 'Lot number'),
        f('batch_number', 'Batch number'),
        f('manufactured_at', 'Manufactured date', kind: 'date'),
        f('best_before_at', 'Best before', kind: 'date'),
        f('expires_at', 'Expiry date', kind: 'date'),
      ],
      validate: (body) {
        body['wrong_item'] = wrong;
        return validatePurchaseInspection(body);
      },
    );
    if (body == null || !mounted) return;
    body['wrong_item'] = wrong;
    if (!wrong) body['purchase_order_line_id'] = po!['id'];
    body.removeWhere((k, v) => v == null);
    setState(() {
      if (index == null) {
        lines.add(body);
      } else {
        lines[index] = body;
      }
    });
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Inspect delivery')),
    bottomNavigationBar: SafeArea(
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: SettingsAction(
          'Review inspection',
          onPressed: lines.isEmpty ? null : () => Navigator.pop(context, lines),
        ),
      ),
    ),
    body: SafeArea(
      child: ListView(
        padding: const EdgeInsets.all(20),
        children: [
          const SettingsCard(
            child: Text(
              'Record delivered, accepted, damaged and rejected purchase units. Only accepted quantities are posted to inventory.',
            ),
          ),
          const SizedBox(height: 20),
          for (final po in poLines.where(
            (p) => !lines.any((l) => l['purchase_order_line_id'] == p['id']),
          )) ...[
            SettingsLink(
              'Inspect ${po['supplier_sku_snapshot']}',
              Icons.fact_check_outlined,
              subtitle:
                  '${po['purchase_quantity']} ordered ? ${po['received_purchase_quantity']} previously accepted',
              onTap: () => edit(poId: po['id']),
            ),
            const SizedBox(height: 12),
          ],
          for (var i = 0; i < lines.length; i++) ...[
            SettingsCard(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    lines[i]['wrong_item'] == true
                        ? 'Wrong item: ${lines[i]['wrong_item_reference']}'
                        : '${poLines.where((p) => p['id'] == lines[i]['purchase_order_line_id']).firstOrNull?['supplier_sku_snapshot']}',
                    style: const TextStyle(
                      fontSize: 19,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  const SizedBox(height: 14),
                  Wrap(
                    spacing: 20,
                    runSpacing: 14,
                    children: [
                      for (final key in [
                        'delivered_quantity',
                        'accepted_quantity',
                        'damaged_quantity',
                        'rejected_quantity',
                        'short_quantity',
                      ])
                        SizedBox(
                          width: 120,
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              Text(
                                '${lines[i][key]}',
                                style: const TextStyle(
                                  fontSize: 25,
                                  fontWeight: FontWeight.w700,
                                ),
                              ),
                              Text(key.replaceAll('_', ' ')),
                            ],
                          ),
                        ),
                    ],
                  ),
                  Row(
                    children: [
                      TextButton.icon(
                        onPressed: () => edit(index: i),
                        icon: const Icon(Icons.edit_outlined),
                        label: const Text('Edit'),
                      ),
                      TextButton.icon(
                        onPressed: () => setState(() => lines.removeAt(i)),
                        icon: const Icon(Icons.delete_outline_rounded),
                        label: const Text('Remove'),
                      ),
                    ],
                  ),
                ],
              ),
            ),
            const SizedBox(height: 12),
          ],
          SettingsAction(
            'Record wrong item',
            secondary: true,
            icon: Icons.report_gmailerrorred_rounded,
            onPressed: () => edit(wrong: true),
          ),
        ],
      ),
    ),
  );
}
