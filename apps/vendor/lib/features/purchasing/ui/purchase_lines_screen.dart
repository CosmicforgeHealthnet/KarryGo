import 'dart:async';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/ui/premium_loading.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../settings/ui/settings_design.dart';
import '../state/purchasing_workspace_controller.dart';
import 'purchasing_forms.dart';

class PurchaseLinesScreen extends StatefulWidget {
  const PurchaseLinesScreen({
    super.key,
    required this.runtime,
    required this.branchId,
    required this.supplierId,
    required this.currency,
    this.existing = const [],
  });
  final VendorRuntime runtime;
  final String branchId, supplierId, currency;
  final List<JsonObject> existing;
  @override
  State<PurchaseLinesScreen> createState() => _PurchaseLinesScreenState();
}

class _PurchaseLinesScreenState extends State<PurchaseLinesScreen> {
  List<JsonObject> products = [], lines = [];
  bool loading = true;
  String? error;
  @override
  void initState() {
    super.initState();
    lines = [
      for (final r in widget.existing)
        {
          'supplier_product_id': r['supplier_product_id'],
          'purchase_quantity': r['purchase_quantity'],
          'unit_cost_minor': r['unit_cost_minor'],
        },
    ];
    unawaited(load());
  }

  Future<void> load() async {
    try {
      final all = <JsonObject>[];
      for (var offset = 0; ; offset += 200) {
        final response = await widget.runtime.backend.purchasingApi.request(
          'GET',
          '$purchasingBase/suppliers/{supplierId}/products',
          pathParameters: {'supplierId': widget.supplierId},
          query: {
            'branch_id': widget.branchId,
            'limit': '200',
            'offset': '$offset',
          },
        );
        final rows = response.list.map(jsonObject).toList();
        if (rows.any(
          (r) =>
              r['supplier_id'] != widget.supplierId ||
              r['branch_id'] != widget.branchId ||
              r['business_id'] != widget.runtime.backend.scope.vendorId,
        )) {
          throw const FormatException('Out-of-scope products');
        }
        all.addAll(rows);
        if (rows.length < 200) break;
      }
      if (mounted) {
        setState(
          () => products = all
              .where(
                (p) =>
                    p['status'] == 'ACTIVE' &&
                    p['currency_code'] == widget.currency,
              )
              .toList(),
        );
      }
    } catch (_) {
      if (mounted) {
        setState(
          () => error =
              'Supplier products could not be loaded. Return and try again.',
        );
      }
    } finally {
      if (mounted) setState(() => loading = false);
    }
  }

  Future<void> edit([int? index]) async {
    final current = index == null ? null : lines[index];
    final available = products
        .where(
          (p) => !lines.any(
            (l) => l['supplier_product_id'] == p['id'] && l != current,
          ),
        )
        .toList();
    final result = await purchaseForm(
      context,
      index == null ? 'Add order line' : 'Edit order line',
      [
        PurchaseField(
          'supplier_product_id',
          'Supplier product',
          kind: 'select',
          required: true,
          options: purchaseChoices(available, label: 'supplier_sku'),
          initial:
              current?['supplier_product_id'] ??
              (widget.runtime.mode == VendorDataMode.mock
                  ? (available.firstOrNull?['id'])
                  : null),
        ),
        PurchaseField(
          'purchase_quantity',
          'Purchase quantity',
          kind: 'number',
          required: true,
          min: 1,
          initial: current?['purchase_quantity'] ?? 1,
        ),
        PurchaseField(
          'unit_cost_minor',
          'Unit cost (${widget.currency})',
          kind: 'money',
          initial: current?['unit_cost_minor'],
        ),
      ],
      validate: (body) {
        final product = products
            .where((p) => p['id'] == body['supplier_product_id'])
            .firstOrNull;
        if (product == null) return 'Choose an available product.';
        final q = body['purchase_quantity'] as int;
        final multiple = product['purchase_multiple'] as int;
        if (q < (product['minimum_order_quantity'] as int) ||
            multiple < 1 ||
            q % multiple != 0) {
          return 'Quantity must meet the minimum and purchase multiple for this product.';
        }
        if (body['unit_cost_minor'] == null &&
            product['typical_cost_minor'] == null) {
          return 'Enter a unit cost; this product has no typical cost.';
        }
        return null;
      },
    );
    if (result != null && mounted) {
      setState(() {
        result.removeWhere((k, v) => v == null);
        if (index == null) {
          lines.add(result);
        } else {
          lines[index] = result;
        }
      });
    }
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Order lines')),
    bottomNavigationBar: SafeArea(
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: SettingsAction(
          'Review order',
          onPressed:
              loading ||
                  error != null ||
                  lines.isEmpty ||
                  lines.any(
                    (l) => !products.any(
                      (p) => p['id'] == l['supplier_product_id'],
                    ),
                  )
              ? null
              : () => Navigator.pop(context, lines),
        ),
      ),
    ),
    body: SafeArea(
      child: ListView(
        padding: const EdgeInsets.all(20),
        children: [
          if (loading)
            const PremiumSkeleton()
          else ...[
            if (error != null) SettingsNote(error!),
            if (products.isEmpty && error == null)
              const SettingsCard(
                child: Text(
                  'No active supplier products match this branch and currency. Add a supplier product first.',
                ),
              ),
            if (products.isNotEmpty) ...[
              SettingsAction(
                'Add order line',
                icon: Icons.add_rounded,
                onPressed: () => edit(),
              ),
              const SizedBox(height: 18),
            ],
            for (var i = 0; i < lines.length; i++) ...[
              SettingsCard(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      '${products.where((p) => p['id'] == lines[i]['supplier_product_id']).firstOrNull?['supplier_sku'] ?? 'Unavailable product'}',
                      style: const TextStyle(
                        fontSize: 20,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                    const SizedBox(height: 12),
                    Text('Quantity ${lines[i]['purchase_quantity']}'),
                    Text(
                      lines[i]['unit_cost_minor'] == null
                          ? 'Use current supplier cost'
                          : dashboardMoney(
                              lines[i]['unit_cost_minor'],
                              widget.currency,
                            ),
                    ),
                    Row(
                      children: [
                        TextButton.icon(
                          onPressed: () => edit(i),
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
            if (lines.isEmpty && products.isNotEmpty)
              const Text(
                'Choose the products and quantities to order. Prices are confirmed by the server.',
              ),
          ],
        ],
      ),
    ),
  );
}
