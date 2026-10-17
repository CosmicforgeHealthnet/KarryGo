import '../../support/ui/order_support_screen.dart' show supportDate;
import 'purchase_policy_screen.dart';
import 'dart:async';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/ui/premium_loading.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../onboarding/ui/setup_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../../settings/ui/settings_design.dart';
import '../../settings/ui/settings_verification_screen.dart';
import '../state/purchasing_workspace_controller.dart';
import 'purchasing_forms.dart';
import 'purchase_lines_screen.dart';
import 'purchase_receiving_screen.dart';

const purchaseTitles = <String, String>{
  'suppliers': 'Suppliers',
  'supplier-categories': 'Supplier categories',
  'contacts': 'Supplier contacts',
  'terms': 'Payment & delivery terms',
  'products': 'Supplier products',
  'purchase-orders': 'Purchase orders',
  'approvals': 'Approval history',
  'goods-receipts': 'Goods receipts',
  'supplier-discrepancies': 'Discrepancies',
  'documents': 'Receipt documents',
  'approval-policy': 'Approval policy',
  'receiving-policy': 'Receiving policy',
  'purchase-cost-history': 'Purchase costs',
  'outstanding-deliveries': 'Expected deliveries',
  'alerts': 'Purchasing alerts',
  'performance': 'Supplier performance',
};
IconData purchaseIcon(String kind) => switch (kind) {
  'suppliers' => Icons.local_shipping_outlined,
  'purchase-orders' => Icons.receipt_long_outlined,
  'goods-receipts' => Icons.inventory_2_outlined,
  'supplier-discrepancies' => Icons.fact_check_outlined,
  'contacts' => Icons.contact_phone_outlined,
  'products' => Icons.widgets_outlined,
  'documents' => Icons.attach_file_rounded,
  'alerts' => Icons.notifications_active_outlined,
  'approval-policy' || 'approvals' => Icons.approval_outlined,
  'performance' => Icons.insights_rounded,
  'purchase-cost-history' => Icons.price_change_outlined,
  _ => Icons.account_tree_outlined,
};
Color purchaseColor(String kind) => switch (kind) {
  'suppliers' => const Color(0xFF3775C5),
  'purchase-orders' => const Color(0xFF8768B8),
  'supplier-discrepancies' || 'alerts' => const Color(0xFFA66028),
  _ => KarryamColors.tealInk,
};
String purchaseTitle(JsonObject r) =>
    '${r['name'] ?? r['purchase_order_number'] ?? r['supplier_sku'] ?? r['display_name'] ?? (r['type'] != null
            ? dashboardLabel(r['type'])
            : r['alert_type'] != null
            ? dashboardLabel(r['alert_type'])
            : r['document_type'] != null
            ? dashboardLabel(r['document_type'])
            : r['decision'] != null
            ? dashboardLabel(r['decision'])
            : r['invoice_reference'] ?? 'Receipt')}';

class PurchasingHub extends StatefulWidget {
  const PurchasingHub({super.key, required this.runtime});
  final VendorRuntime runtime;
  @override
  State<PurchasingHub> createState() => _PurchasingHubState();
}

class _PurchasingHubState extends State<PurchasingHub> {
  List<JsonObject> branches = [];
  JsonObject? branch;
  JsonObject? dashboard;
  String? error, dashboardError;
  bool loading = true;
  @override
  void initState() {
    super.initState();
    unawaited(load());
  }

  Future<void> load() async {
    setState(() {
      loading = true;
      error = null;
      dashboardError = null;
    });
    try {
      final response = await widget.runtime.backend.businessApi.request(
        'GET',
        '/api/v1/vendor/businesses/{businessId}/branches',
        pathParameters: {'businessId': widget.runtime.backend.scope.vendorId!},
      );
      final raw = response.data is List
          ? response.data
          : response.object['items'];
      if (raw is! List) throw const FormatException('Missing branch list');
      final next = raw
          .map(jsonObject)
          .where((b) => b['status'] != 'ARCHIVED')
          .toList();
      final current =
          next.where((b) => b['id'] == branch?['id']).firstOrNull ??
          next.firstOrNull;
      JsonObject? stats;
      String? statsError;
      if (current != null) {
        try {
          final data = (await widget.runtime.backend.purchasingApi.request(
            'GET',
            '$purchasingBase/dashboard',
            query: {'branch_id': '${current['id']}'},
          )).object;
          stats = jsonObject(data['dashboard']);
          if (stats['business_id'] != widget.runtime.backend.scope.vendorId) {
            throw const FormatException('Invalid purchasing scope');
          }
        } catch (_) {
          statsError = 'Purchasing summary is unavailable for this branch.';
        }
      }
      if (mounted) {
        setState(() {
          branches = next;
          branch = current;
          dashboard = stats;
          dashboardError = statsError;
        });
      }
    } catch (_) {
      if (mounted) {
        setState(
          () => error = 'Branches could not be loaded. Refresh to try again.',
        );
      }
    } finally {
      if (mounted) setState(() => loading = false);
    }
  }

  Future<void> open(String kind) async {
    await Navigator.push<void>(
      context,
      setupRoute<void>(
        PurchasingWorkspaceScreen(
          runtime: widget.runtime,
          kind: kind,
          branchId: branch?['id'],
        ),
      ),
    );
    if (mounted) await load();
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(
      title: const Text('Purchasing'),
      actions: [
        IconButton(
          tooltip: 'Refresh purchasing',
          onPressed: loading ? null : load,
          icon: const Icon(Icons.refresh_rounded),
        ),
      ],
    ),
    body: SafeArea(
      child: ListView(
        padding: const EdgeInsets.all(20),
        children: [
          if (loading)
            const PremiumSkeleton(detail: true)
          else ...[
            if (error != null) SettingsNote(error!),
            if (branches.isEmpty && error == null)
              const SettingsCard(
                child: Text('Set up a branch before creating purchase orders.'),
              ),
            if (branches.isNotEmpty) ...[
              DropdownButtonFormField<String>(
                initialValue: branch?['id'],
                isExpanded: true,
                decoration: const InputDecoration(
                  labelText: 'Purchasing branch',
                  prefixIcon: Icon(Icons.storefront_outlined),
                ),
                items: [
                  for (final b in branches)
                    DropdownMenuItem(
                      value: '${b['id']}',
                      child: Text('${b['display_name'] ?? b['name']}'),
                    ),
                ],
                onChanged: (id) {
                  branch = branches.firstWhere((b) => b['id'] == id);
                  unawaited(load());
                },
              ),
              const SizedBox(height: 20),
              SettingsCard(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    const Text(
                      'Supply overview',
                      style: TextStyle(
                        fontSize: 24,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                    const SizedBox(height: 18),
                    if (dashboard != null)
                      Wrap(
                        spacing: 24,
                        runSpacing: 20,
                        children: [
                          for (final metric in [
                            (
                              'purchase_orders',
                              'submitted',
                              'Awaiting approval',
                            ),
                            ('receiving', 'awaiting_inspection', 'To inspect'),
                            ('discrepancies', 'unresolved', 'Open issues'),
                          ])
                            SizedBox(
                              width: 125,
                              child: Column(
                                crossAxisAlignment: CrossAxisAlignment.start,
                                children: [
                                  Text(
                                    '${dashboard![metric.$1][metric.$2]}',
                                    style: TextStyle(
                                      fontSize: 32,
                                      fontWeight: FontWeight.w700,
                                      color: purchaseColor(
                                        metric.$1,
                                      ).forInk(context),
                                    ),
                                  ),
                                  Text(metric.$3),
                                ],
                              ),
                            ),
                        ],
                      )
                    else
                      Text(dashboardError ?? 'No summary available.'),
                  ],
                ),
              ),
              const SizedBox(height: 24),
              for (final kind in [
                'purchase-orders',
                'goods-receipts',
                'supplier-discrepancies',
                'suppliers',
                'outstanding-deliveries',
                'alerts',
                'supplier-categories',
                'purchase-cost-history',
                'approval-policy',
                'receiving-policy',
              ]) ...[
                SettingsLink(
                  purchaseTitles[kind]!,
                  purchaseIcon(kind),
                  onTap: () => open(kind),
                ),
                const SizedBox(height: 12),
              ],
            ],
          ],
        ],
      ),
    ),
  );
}

class PurchasingWorkspaceScreen extends StatefulWidget {
  const PurchasingWorkspaceScreen({
    super.key,
    required this.runtime,
    required this.kind,
    this.branchId,
    this.id,
    this.supplierId,
    this.orderId,
    this.receiptId,
  });
  final VendorRuntime runtime;
  final String kind;
  final String? branchId, id, supplierId, orderId, receiptId;
  @override
  State<PurchasingWorkspaceScreen> createState() =>
      _PurchasingWorkspaceScreenState();
}

class _PurchasingWorkspaceScreenState extends State<PurchasingWorkspaceScreen> {
  String get kind => widget.kind;
  bool get singleton => [
    'terms',
    'approval-policy',
    'receiving-policy',
    'performance',
  ].contains(kind);
  String get base => kind == 'receiving-policy'
      ? '$purchasingBase/receiving-policies/branches/{branchId}'
      : ['contacts', 'terms', 'products', 'performance'].contains(kind)
      ? '$purchasingBase/suppliers/{supplierId}/$kind'
      : kind == 'approvals'
      ? '$purchasingBase/purchase-orders/{purchaseOrderId}/approvals'
      : kind == 'documents'
      ? '$purchasingBase/goods-receipts/{receiptId}/documents'
      : '$purchasingBase/$kind';
  String get idKey => switch (kind) {
    'suppliers' => 'supplierId',
    'supplier-categories' => 'categoryId',
    'products' => 'supplierProductId',
    'purchase-orders' => 'purchaseOrderId',
    'goods-receipts' => 'receiptId',
    'supplier-discrepancies' => 'discrepancyId',
    _ => 'contactId',
  };
  String get path => widget.id != null && !singleton ? '$base/{$idKey}' : base;
  Map<String, String> get params => {
    'supplierId': ?widget.supplierId,
    'purchaseOrderId': ?widget.orderId,
    'receiptId': ?widget.receiptId,
    'branchId': ?widget.branchId,
    if (widget.id != null) idKey: widget.id!,
  };
  late final c = PurchasingWorkspaceController(
    widget.runtime,
    path,
    parameters: params,
    branchId: widget.branchId,
    wrapper: ['alerts', 'outstanding-deliveries', 'performance'].contains(kind)
        ? {
            'alerts': 'alerts',
            'outstanding-deliveries': 'deliveries',
            'performance': 'performance',
          }[kind]
        : null,
  );
  String search = '', filter = '';
  @override
  void initState() {
    super.initState();
    unawaited(c.load());
  }

  @override
  void dispose() {
    c.dispose();
    super.dispose();
  }

  Future<void> open(String type, {JsonObject? row}) async {
    await Navigator.push<void>(
      context,
      setupRoute<void>(
        PurchasingWorkspaceScreen(
          runtime: widget.runtime,
          kind: type,
          branchId: widget.branchId,
          id: row?['id'],
          supplierId:
              type == 'products' ||
                  type == 'contacts' ||
                  type == 'terms' ||
                  type == 'performance'
              ? (kind == 'suppliers' ? c.record!['id'] : widget.supplierId)
              : null,
          orderId: type == 'approvals' ? c.record!['id'] : null,
          receiptId: type == 'documents' ? c.record!['id'] : null,
        ),
      ),
    );
    if (mounted) await c.load(status: filter);
  }

  Future<List<JsonObject>> all(
    String route, {
    Map<String, String> parameters = const {},
    Map<String, String> query = const {},
  }) async {
    final rows = <JsonObject>[];
    for (var offset = 0; ; offset += 200) {
      final response = await widget.runtime.backend.purchasingApi.request(
        'GET',
        route,
        pathParameters: parameters,
        query: {...query, 'limit': '200', 'offset': '$offset'},
      );
      final next = c.collection(response.data);
      for (final r in next) {
        c.validate(r);
      }
      rows.addAll(next);
      if (next.length < 200) return rows;
    }
  }

  String? activeRoute;

  Future<JsonObject?> command(
    String method,
    String route,
    JsonObject body, {
    Map<String, String>? parameters,
    int? version,
    String title = 'Save changes',
    bool danger = false,
  }) async {
    if (!await confirmSettings(
          context,
          title,
          danger
              ? 'This action changes the saved record and is audited.'
              : 'Review your details before continuing.',
          danger: danger,
        ) ||
        !mounted) {
      return null;
    }
    activeRoute = route;
    final result = await c.save(
      method: method,
      route: route,
      body: body,
      params: parameters ?? params,
      headerVersion: version,
    );
    if (!mounted) return null;
    setState(() => activeRoute = null);
    // Refresh before the confirmation, so the list behind it already shows
    // the new record instead of "No suppliers yet".
    final saveError = c.error;
    if (result != null) await c.load(status: filter);
    if (!mounted) return null;
    await showOrderFeedback(
      context,
      title: result == null ? 'Could not confirm change' : 'Changes saved',
      message: saveError ?? 'The latest purchasing record is ready.',
      success: result != null,
      warning: result == null,
    );
    if (result != null && mounted) {
      if (method == 'POST' &&
          route == base &&
          ['suppliers', 'products', 'purchase-orders'].contains(kind) &&
          widget.id == null &&
          mounted) {
        await open(kind, row: result);
      }
    }
    return result;
  }

  Future<void> retry() async {
    final result = await c.save();
    if (!mounted) return;
    await showOrderFeedback(
      context,
      title: result == null
          ? 'Change needs attention'
          : 'Saved change confirmed',
      message: c.error ?? 'The original update was confirmed.',
      success: result != null,
      warning: result == null,
    );
    if (result != null && mounted) await c.load(status: filter);
  }

  Future<void> edit([JsonObject? row]) async {
    try {
      final mock = widget.runtime.mode == VendorDataMode.mock;
      final current = row ?? c.record;
      final updating = current != null;
      if (kind == 'approval-policy') {
        if (current != null && current['rules_complete'] != true) {
          throw StateError(
            'The complete policy is not visible with your branch scope. A business administrator must edit these rules.',
          );
        }
        final response = await widget.runtime.backend.businessApi.request(
          'GET',
          '/api/v1/vendor/businesses/{businessId}/branches',
          pathParameters: {
            'businessId': widget.runtime.backend.scope.vendorId!,
          },
        );
        final branches = c.collection(
          response.data is List ? response.data : response.object['items'],
        );
        if (!mounted) return;
        final draft = await Navigator.push<JsonObject>(
          context,
          setupRoute<JsonObject>(
            PurchasePolicyScreen(policy: current, branches: branches),
          ),
        );
        if (draft != null && mounted) {
          await command('PUT', base, draft, title: 'Replace approval policy');
        }
        return;
      }
      final fields = <PurchaseField>[];
      final enums = purchaseEnums;
      PurchaseField f(
        String key,
        String label, {
        String type = 'text',
        bool required = false,
        Object? initial,
        int min = 0,
        int? max,
        Map<String, String> options = const {},
      }) => PurchaseField(
        key,
        label,
        kind: type,
        required: required,
        initial: current?[key] ?? initial,
        min: min,
        max: max,
        options: options,
      );
      if (kind == 'suppliers' || kind == 'supplier-categories') {
        fields.addAll([
          f(
            'code',
            'Code',
            required: true,
            initial: mock
                ? 'SUP-${DateTime.now().millisecondsSinceEpoch}'
                : null,
          ),
          f(
            'name',
            'Name',
            required: true,
            initial: mock ? 'Fresh Harvest Supplies' : null,
          ),
        ]);
        if (kind == 'supplier-categories') {
          fields.add(f('description', 'Description', type: 'notes'));
        } else {
          final categories = await all('$purchasingBase/supplier-categories');
          fields.addAll([
            f(
              'category_id',
              'Category',
              type: 'select',
              options: purchaseChoices(
                categories.where((r) => r['status'] == 'ACTIVE').toList(),
              ),
            ),
            f(
              'legal_name',
              'Legal name',
              initial: mock ? 'Fresh Harvest Limited' : null,
            ),
            f(
              'default_lead_time_hours',
              'Default lead time (hours)',
              type: 'number',
              required: true,
              initial: 24,
            ),
            f(
              'status',
              'Status',
              type: 'select',
              required: true,
              initial: 'ACTIVE',
              options: enums(['ACTIVE', 'INACTIVE', 'SUSPENDED']),
            ),
            f('notes', 'Notes', type: 'notes'),
          ]);
        }
      } else if (kind == 'contacts') {
        fields.addAll([
          f(
            'contact_role',
            'Role',
            type: 'select',
            required: true,
            initial: 'PRIMARY',
            options: enums([
              'PRIMARY',
              'SALES',
              'OPERATIONS',
              'FINANCE',
              'OTHER',
            ]),
          ),
          f(
            'name',
            'Name',
            required: true,
            initial: mock ? 'Ada Okafor' : null,
          ),
          f('email', 'Email', initial: mock ? 'ada@example.com' : null),
          f('phone', 'Phone', initial: mock ? '+2348012345678' : null),
          f('is_primary', 'Primary contact', type: 'bool'),
        ]);
      } else if (kind == 'terms') {
        fields.addAll([
          f(
            'payment_terms_days',
            'Payment terms (days)',
            type: 'number',
            required: true,
            initial: 7,
          ),
          f('currency_code', 'Currency', required: true, initial: 'NGN'),
          f(
            'delivery_terms',
            'Delivery terms',
            type: 'notes',
            initial: mock ? 'Delivery to branch receiving area.' : null,
          ),
        ]);
      } else if (kind == 'products') {
        if (!updating) {
          final balanceResponse = await widget.runtime.backend.inventoryApi
              .request(
                'GET',
                '/api/v1/vendor/inventory/branches/{branchId}/balances',
                pathParameters: {'branchId': widget.branchId!},
              );
          final balances = c.collection(balanceResponse.object['items']);
          for (final b in balances) {
            c.validate(b);
          }
          if (balances.isEmpty) {
            throw StateError(
              'This branch has no stock items yet. In Production materials, mark an ingredient or packaging as stock-tracked for this branch, then link it here.',
            );
          }
          // Newer servers include the item name on each balance; older ones
          // need a lookup through the production materials.
          final names = balances.every(
                (b) => '${b['name'] ?? ''}'.isNotEmpty,
              )
              ? <String, String>{}
              : await _inventoryItemNamesFor(
                  widget.runtime.backend.productionMasterApi,
                  widget.branchId!,
                );
          var unnamed = 0;
          fields.addAll([
            PurchaseField(
              'inventory_item_id',
              'Inventory item',
              kind: 'select',
              required: true,
              options: {
                for (final b in balances)
                  '${b['inventory_item_id']}':
                      '${b['name'] ?? ''}'.isNotEmpty
                      ? '${b['name']}'
                      : names['${b['inventory_item_id']}'] ??
                            'Stock item ${++unnamed}',
              },
            ),
            f(
              'purchase_unit_code',
              'Purchase unit code',
              required: true,
              initial: mock ? 'BAG' : null,
            ),
            f(
              'purchase_unit_name',
              'Purchase unit name',
              required: true,
              initial: mock ? 'Bag' : null,
            ),
            f(
              'conversion_mode',
              'Inventory conversion',
              type: 'select',
              required: true,
              initial: 'SAME_AS_INVENTORY',
              options: enums(['SAME_AS_INVENTORY', 'EXACT_INTEGER_MULTIPLE']),
            ),
            f(
              'inventory_units_per_purchase_unit',
              'Inventory units per purchase unit',
              type: 'number',
              required: true,
              initial: 1,
              min: 1,
            ),
          ]);
        }
        fields.addAll([
          f('supplier_sku', 'Supplier SKU', initial: mock ? 'RICE-25KG' : null),
          f(
            'minimum_order_quantity',
            'Minimum order quantity',
            type: 'number',
            required: true,
            initial: 1,
            min: 1,
          ),
          f(
            'purchase_multiple',
            'Purchase multiple',
            type: 'number',
            required: true,
            initial: 1,
            min: 1,
          ),
          f(
            'typical_cost_minor',
            'Typical cost',
            type: 'money',
            initial: mock ? 250000 : null,
          ),
          f('currency_code', 'Currency', required: true, initial: 'NGN'),
          f(
            'lead_time_hours',
            'Lead time (hours)',
            type: 'number',
            initial: 24,
          ),
        ]);
      } else if (kind == 'purchase-orders') {
        if (!updating) {
          final suppliers = (await all(
            '$purchasingBase/suppliers',
            query: {'branch_id': widget.branchId!},
          )).where((r) => r['status'] == 'ACTIVE').toList();
          if (suppliers.isEmpty) {
            throw StateError(
              'No supplier sells to this branch yet. Open Suppliers, choose a supplier and add the products they supply to this branch, then create the order.',
            );
          }
          fields.add(
            PurchaseField(
              'supplier_id',
              'Supplier',
              kind: 'select',
              required: true,
              options: purchaseChoices(suppliers),
              initial: mock ? (suppliers.firstOrNull?['id']) : null,
            ),
          );
          fields.add(
            f('currency_code', 'Currency', required: true, initial: 'NGN'),
          );
        }
        fields.addAll([
          f(
            'expected_delivery_date',
            'Expected delivery date',
            type: 'date',
            initial: mock
                ? DateTime.now()
                      .add(const Duration(days: 2))
                      .toUtc()
                      .toIso8601String()
                : null,
          ),
          f('notes', 'Notes', type: 'notes'),
        ]);
      } else if (kind == 'documents') {
        fields.addAll([
          f(
            'document_type',
            'Document type',
            type: 'select',
            required: true,
            options: enums([
              'DELIVERY_NOTE',
              'INVOICE',
              'PACKING_LIST',
              'QUALITY_CERTIFICATE',
              'OTHER',
            ]),
            initial: 'INVOICE',
          ),
          f(
            'media_ref',
            'Document number (as printed)',
            required: true,
            initial: mock ? 'mock-receipt-document' : null,
          ),
          f('external_reference', 'External reference'),
        ]);
      } else if (kind == 'receiving-policy') {
        fields.addAll([
          f(
            'over_delivery_mode',
            'Over-delivery policy',
            type: 'select',
            required: true,
            options: enums(['DENY', 'ALLOW_WITH_TOLERANCE']),
            initial: 'DENY',
          ),
          f(
            'over_delivery_tolerance_bps',
            'Over-delivery tolerance (%)',
            type: 'percent',
            required: true,
            max: 10000,
            initial: 0,
          ),
          f(
            'cost_variance_approval_bps',
            'Cost variance threshold (%)',
            type: 'percent',
            required: true,
            initial: 0,
          ),
          f(
            'cost_variance_approval_minor',
            'Cost variance amount threshold',
            type: 'money',
            required: true,
            initial: 0,
          ),
          f(
            'require_cost_variance_approval',
            'Require cost variance approval',
            type: 'bool',
            initial: true,
          ),
        ]);
      } else {
        return;
      }
      if (!mounted) return;
      final body = await purchaseForm(
        context,
        updating
            ? 'Edit ${purchaseTitles[kind]!.toLowerCase()}'
            : 'Add ${kind == 'products'
                  ? 'supplier product'
                  : kind == 'suppliers'
                  ? 'supplier'
                  : kind == 'contacts'
                  ? 'contact'
                  : 'purchase order'}',
        fields,
      );
      if (body == null || !mounted) return;
      if (kind == 'purchase-orders' && !updating) {
        final lines = await Navigator.push<List<JsonObject>>(
          context,
          setupRoute<List<JsonObject>>(
            PurchaseLinesScreen(
              runtime: widget.runtime,
              branchId: widget.branchId!,
              supplierId: body['supplier_id'],
              currency: body['currency_code'],
            ),
          ),
        );
        if (lines == null || !mounted) return;
        body['branch_id'] = widget.branchId;
        body['lines'] = lines;
      }
      if (kind == 'products' && !updating) body['branch_id'] = widget.branchId;
      if (updating && kind != 'receiving-policy') {
        body['expected_version'] = current['version'];
      }
      await command(
        singleton
            ? 'PUT'
            : updating
            ? 'PATCH'
            : 'POST',
        updating && !singleton ? '$base/{$idKey}' : base,
        body,
        parameters: {
          ...params,
          if (updating && !singleton) idKey: current['id'],
        },
        title: 'Save details',
      );
    } catch (e) {
      if (mounted) {
        await showOrderFeedback(
          context,
          title: "This can't be opened yet",
          message: e is StateError
              ? e.message
              : 'The required purchasing references could not be loaded.',
          warning: true,
        );
      }
    }
  }

  Future<void> orderAction(String action) async {
    final record = c.record!;
    if (action == 'cancel' &&
        (record['lines'] as List? ?? []).any(
          (l) => (l['received_purchase_quantity'] ?? 0) > 0,
        )) {
      return;
    }
    final note = await purchaseForm(
      context,
      '${dashboardLabel(action)} purchase order',
      [const PurchaseField('note', 'Note', kind: 'notes')],
    );
    if (note == null || !mounted) return;
    if (action == 'approve' || action == 'reject') {
      if (record['self_approval_allowed'] != true &&
          record['created_by_staff_id'] ==
              widget.runtime.auth.session?.staffId) {
        await showOrderFeedback(
          context,
          title: 'Another approver is required',
          message:
              'This order does not permit approval decisions by its creator.',
          warning: true,
        );
        return;
      }
      if (record['approval_step_up_required'] == true) {
        final grant = await Navigator.push<String>(
          context,
          setupRoute<String>(
            SettingsVerificationScreen(
              runtime: widget.runtime,
              purpose: 'purchasing.purchase_order.approve',
              title: 'Verify purchase approval',
              actionLabel: 'Verify approval',
            ),
          ),
        );
        if (grant == null || !mounted) return;
        note['step_up_grant_id'] = grant;
      }
    }
    await command(
      'POST',
      '$base/{purchaseOrderId}/$action',
      {'expected_version': record['version'], ...note},
      title: '${dashboardLabel(action)} purchase order',
      danger: ['cancel', 'reject'].contains(action),
    );
  }

  Future<void> replaceLines() async {
    final r = c.record!;
    final result = await Navigator.push<List<JsonObject>>(
      context,
      setupRoute<List<JsonObject>>(
        PurchaseLinesScreen(
          runtime: widget.runtime,
          branchId: r['branch_id'],
          supplierId: r['supplier_id'],
          currency: r['currency_code'],
          existing: c.collection(r['lines']),
        ),
      ),
    );
    if (result != null && mounted) {
      await command('PUT', '$base/{purchaseOrderId}/lines', {
        'expected_version': r['version'],
        'lines': result,
      }, title: 'Replace order lines');
    }
  }

  Future<void> receive() async {
    final r = c.record!;
    final body = await purchaseForm(context, 'Record delivery', [
      PurchaseField(
        'received_at',
        'Received at',
        kind: 'date',
        required: true,
        initial: widget.runtime.mode == VendorDataMode.mock
            ? DateTime.now().toUtc().toIso8601String()
            : null,
      ),
      const PurchaseField('supplier_delivery_reference', 'Delivery reference'),
      const PurchaseField('invoice_reference', 'Invoice reference'),
      const PurchaseField(
        'invoice_total_minor',
        'Invoice total',
        kind: 'money',
      ),
      const PurchaseField('notes', 'Notes', kind: 'notes'),
    ]);
    if (body == null || !mounted) return;
    final receipt = await command(
      'POST',
      '$purchasingBase/goods-receipts',
      {'purchase_order_id': r['id'], ...body},
      parameters: {},
      title: 'Create receipt',
    );
    if (receipt != null && mounted) await open('goods-receipts', row: receipt);
  }

  Future<void> receiptAction(String action) async {
    final r = c.record!;
    if (action == 'inspection') {
      try {
        final order = jsonObject(
          await c.fetch(
            '$purchasingBase/purchase-orders/{purchaseOrderId}',
            params: {'purchaseOrderId': r['purchase_order_id']},
          ),
        );
        c.validate(order);
        if (!mounted) return;
        final lines = await Navigator.push<List<JsonObject>>(
          context,
          setupRoute<List<JsonObject>>(
            PurchaseReceivingScreen(
              order: order,
              receipt: r,
              mock: widget.runtime.mode == VendorDataMode.mock,
            ),
          ),
        );
        if (lines != null && mounted) {
          await command(
            'PUT',
            '$base/{receiptId}/inspection',
            {'lines': lines},
            version: r['version'],
            title: 'Save inspection',
          );
        }
      } catch (_) {
        if (mounted) {
          await showOrderFeedback(
            context,
            title: 'Inspection unavailable',
            message: 'The purchase order could not be loaded.',
            warning: true,
          );
        }
      }
    } else {
      JsonObject body = {};
      if (action == 'reverse') {
        final result = await purchaseForm(context, 'Reverse receipt', [
          const PurchaseField(
            'reason',
            'Reason',
            kind: 'notes',
            required: true,
          ),
        ]);
        if (result == null || !mounted) return;
        body = result;
      }
      await command(
        'POST',
        '$base/{receiptId}/$action',
        body,
        version: r['version'],
        title: action == 'confirm' ? 'Post accepted stock' : 'Reverse receipt',
        danger: action == 'reverse',
      );
    }
  }

  Future<void> discrepancyAction(String action) async {
    final r = c.record!;
    final body = await purchaseForm(
      context,
      '${dashboardLabel(action)} discrepancy',
      [const PurchaseField('notes', 'Decision notes', kind: 'notes')],
    );
    if (body != null && mounted) {
      await command(
        'POST',
        '$base/{discrepancyId}/$action',
        body,
        version: r['version'],
        title: '${dashboardLabel(action)} discrepancy',
        danger: action == 'reject',
      );
    }
  }

  bool get canCreate =>
      [
        'suppliers',
        'supplier-categories',
        'contacts',
        'products',
        'documents',
        'purchase-orders',
      ].contains(kind) &&
      widget.id == null;
  bool get canEdit =>
      [
        'suppliers',
        'supplier-categories',
        'contacts',
        'terms',
        'products',
        'receiving-policy',
      ].contains(kind) ||
      kind == 'approval-policy' && c.record?['rules_complete'] == true ||
      kind == 'purchase-orders' && c.record?['status'] == 'DRAFT';
  String formatFact(String key, Object value, JsonObject r) {
    if (key.endsWith('_at') || key.endsWith('_date')) return supportDate(value);
    if (key.endsWith('_minor')) {
      return dashboardMoney(
        value,
        r['currency_code'] ?? r['currency'] ?? 'NGN',
      );
    }
    if (key.endsWith('_bps')) return '${(value as num) / 100}%';
    if (value is bool) return value ? 'Yes' : 'No';
    return value is String &&
            value == value.toUpperCase() &&
            value.contains('_')
        ? dashboardLabel(value)
        : '$value';
  }

  Widget fact(String title, String value) => Padding(
    padding: const EdgeInsets.only(top: 14),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          title,
          style: TextStyle(
            fontSize: 12,
            color: Theme.of(context).colorScheme.onSurfaceVariant,
          ),
        ),
        const SizedBox(height: 4),
        Text(
          value,
          style: const TextStyle(fontSize: 15, fontWeight: FontWeight.w600),
        ),
      ],
    ),
  );
  Widget summary(JsonObject r) => SettingsCard(
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            Container(
              padding: const EdgeInsets.all(15),
              decoration: BoxDecoration(
                color: Theme.of(context).colorScheme.surfaceContainerHighest,
                borderRadius: BorderRadius.circular(20),
              ),
              child: Icon(
                purchaseIcon(kind),
                size: 30,
                color: purchaseColor(kind).forInk(context),
              ),
            ),
            const SizedBox(width: 14),
            Expanded(
              child: Text(
                singleton ? purchaseTitles[kind]! : purchaseTitle(r),
                style: const TextStyle(
                  fontSize: 23,
                  fontWeight: FontWeight.w700,
                ),
              ),
            ),
          ],
        ),
        const SizedBox(height: 16),
        if (r['status'] != null) SettingsStatus(r['status']),
        if (r['total_amount_minor'] != null) ...[
          const SizedBox(height: 16),
          Text(
            dashboardMoney(r['total_amount_minor'], r['currency_code']),
            style: const TextStyle(fontSize: 30, fontWeight: FontWeight.w700),
          ),
        ],
        for (final key in [
          'supplier_name_snapshot',
          'supplier_delivery_reference',
          'expected_delivery_date',
          'received_at',
          'notes',
          'reversal_reason',
          'inventory_receipt_id',
          'type',
          'quantity',
          'variance_minor',
          'approval_count',
          'required_approval_count',
          'approval_permission_code',
          'approval_scope',
          'approval_step_up_required',
          'payment_terms_days',
          'currency_code',
          'delivery_terms',
          'purchase_unit_name',
          'conversion_mode',
          'inventory_units_per_purchase_unit',
          'minimum_order_quantity',
          'purchase_multiple',
          'typical_cost_minor',
          'lead_time_hours',
          'over_delivery_mode',
          'over_delivery_tolerance_bps',
          'cost_variance_approval_bps',
          'cost_variance_approval_minor',
          'require_cost_variance_approval',
        ])
          if (r[key] != null && '${r[key]}'.isNotEmpty)
            Padding(
              padding: const EdgeInsets.only(top: 12),
              child: fact(dashboardLabel(key), formatFact(key, r[key], r)),
            ),
      ],
    ),
  );
  List<Widget> details(JsonObject r) => [
    summary(r),
    const SizedBox(height: 18),
    if (kind == 'approval-policy') ...[
      SettingsCard(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Text(
              'Approval rules',
              style: TextStyle(fontSize: 20, fontWeight: FontWeight.w700),
            ),
            fact(
              'Self-approval',
              r['allow_self_approval'] == true ? 'Allowed' : 'Not allowed',
            ),
            if (r['rules_complete'] != true)
              const Text(
                'Your access shows a partial rule list. A business administrator must edit the full policy.',
              ),
            for (final rule in c.collection(r['rules'])) ...[
              const Divider(height: 32),
              fact('Required approvals', '${rule['required_approval_count']}'),
              fact(
                'Amount range',
                '${dashboardMoney(rule['minimum_amount_minor'], 'NGN')} ? ${rule['maximum_amount_minor'] == null ? 'No upper limit' : dashboardMoney(rule['maximum_amount_minor'], 'NGN')}',
              ),
              fact('Approver scope', dashboardLabel(rule['approval_scope'])),
              fact(
                'Approval permission',
                '${rule['required_permission_code']}',
              ),
              fact(
                'OTP verification',
                rule['step_up_required'] == true ? 'Required' : 'Not required',
              ),
            ],
          ],
        ),
      ),
      const SizedBox(height: 18),
    ],
    if (kind == 'performance') ...[
      SettingsCard(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Text(
              'Purchasing evidence',
              style: TextStyle(fontSize: 20, fontWeight: FontWeight.w700),
            ),
            for (final key in [
              'total_purchase_orders',
              'orders_outstanding',
              'orders_fully_received',
              'total_goods_receipts',
              'open_discrepancies',
              'total_discrepancies',
            ])
              if (r[key] != null) fact(dashboardLabel(key), '${r[key]}'),
            for (final key in [
              'on_time_rate',
              'acceptance_rate',
              'damage_rate',
              'rejection_rate',
              'short_delivery_rate',
              'cost_variance_rate',
            ])
              if (r[key] is Map)
                fact(
                  dashboardLabel(key),
                  r[key]['metric_status'] == 'OK' && r[key]['value'] is num
                      ? '${((r[key]['value'] as num) * 100).toStringAsFixed(1)}% - ${r[key]['sample_count']} samples'
                      : 'Insufficient history - ${r[key]['sample_count']} samples',
                ),
          ],
        ),
      ),
      const SizedBox(height: 18),
    ],
    if (c.writable && canEdit && r['status'] != 'ARCHIVED') ...[
      SettingsAction(
        'Edit details',
        icon: Icons.edit_outlined,
        onPressed: () => edit(),
      ),
      const SizedBox(height: 12),
    ],
    if (kind == 'suppliers')
      for (final child in ['products', 'contacts', 'terms', 'performance']) ...[
        SettingsLink(
          purchaseTitles[child]!,
          purchaseIcon(child),
          onTap: () => open(child),
        ),
        const SizedBox(height: 12),
      ],
    if (kind == 'purchase-orders') ...[
      for (final line in c.collection(r['lines'])) ...[
        SettingsCard(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                '${line['supplier_sku_snapshot']}',
                style: const TextStyle(
                  fontSize: 18,
                  fontWeight: FontWeight.w700,
                ),
              ),
              const SizedBox(height: 10),
              Text(
                '${line['purchase_quantity']} ${line['purchase_unit_name_snapshot']} - ${dashboardMoney(line['line_total_minor'], r['currency_code'])}',
              ),
              Text(
                'Received ${line['received_purchase_quantity']} · Stock units ${line['converted_inventory_quantity']}',
              ),
            ],
          ),
        ),
        const SizedBox(height: 12),
      ],
      if (r['status'] == 'DRAFT') ...[
        SettingsAction(
          'Edit order lines',
          secondary: true,
          onPressed: c.writable ? replaceLines : null,
        ),
        const SizedBox(height: 12),
      ],
      SettingsLink(
        'Approval history',
        Icons.approval_outlined,
        onTap: () => open('approvals'),
      ),
      const SizedBox(height: 12),
      for (final action in purchaseOrderActions[r['status']] ?? []) ...[
        SettingsAction(
          '${dashboardLabel(action)} purchase order',
          danger: ['cancel', 'reject'].contains(action),
          secondary: action == 'cancel',
          busy: c.busy && activeRoute == '$base/{purchaseOrderId}/$action',
          onPressed: c.allowed('POST', '$base/{purchaseOrderId}/$action')
              ? () => orderAction(action)
              : null,
        ),
        const SizedBox(height: 12),
      ],
      if (['ORDERED', 'PARTIALLY_RECEIVED'].contains(r['status']))
        SettingsAction(
          'Record delivery',
          icon: Icons.inventory_2_outlined,
          onPressed: c.writable ? receive : null,
        ),
    ],
    if (kind == 'goods-receipts') ...[
      for (final line in c.collection(r['lines'])) ...[
        SettingsCard(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                line['wrong_item'] == true
                    ? 'Wrong item'
                    : '${line['purchase_unit_code']}',
                style: const TextStyle(
                  fontSize: 18,
                  fontWeight: FontWeight.w700,
                ),
              ),
              const SizedBox(height: 10),
              Wrap(
                spacing: 20,
                runSpacing: 12,
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
                            '${line[key]}',
                            style: const TextStyle(
                              fontSize: 24,
                              fontWeight: FontWeight.w700,
                            ),
                          ),
                          Text(dashboardLabel(key)),
                        ],
                      ),
                    ),
                ],
              ),
            ],
          ),
        ),
        const SizedBox(height: 12),
      ],
      SettingsLink(
        'Receipt documents',
        Icons.attach_file_outlined,
        onTap: () => open('documents'),
      ),
      const SizedBox(height: 12),
      for (final action in receiptActions[r['status']] ?? []) ...[
        SettingsAction(
          action == 'inspection'
              ? 'Inspect delivery'
              : action == 'confirm'
              ? 'Post accepted stock'
              : 'Reverse receipt',
          danger: action == 'reverse',
          busy: c.busy && activeRoute == '$base/{goodsReceiptId}/$action',
          onPressed: c.writable ? () => receiptAction(action) : null,
        ),
        const SizedBox(height: 12),
      ],
    ],
    if (kind == 'supplier-discrepancies')
      for (final action in discrepancyActions[r['status']] ?? []) ...[
        SettingsAction(
          '${dashboardLabel(action)} discrepancy',
          danger: action == 'reject',
          busy: c.busy && activeRoute == '$base/{discrepancyId}/$action',
          onPressed: c.writable ? () => discrepancyAction(action) : null,
        ),
        const SizedBox(height: 12),
      ],
    if (['suppliers', 'products'].contains(kind) && r['status'] != 'ARCHIVED')
      SettingsAction(
        'Archive ${kind == 'suppliers' ? 'supplier' : 'product'}',
        danger: true,
        onPressed: c.writable
            ? () => command(
                'POST',
                '$base/{$idKey}/archive',
                {'expected_version': r['version']},
                title: 'Archive record',
                danger: true,
              )
            : null,
      ),
  ];
  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => Scaffold(
      appBar: AppBar(
        title: Text(purchaseTitles[kind]!),
        actions: [
          IconButton(
            tooltip: 'Refresh',
            onPressed: c.loading || c.busy
                ? null
                : () => c.load(status: filter),
            icon: const Icon(Icons.refresh_rounded),
          ),
        ],
      ),
      body: SafeArea(
        child: ListView(
          padding: const EdgeInsets.all(20),
          children: [
            if (c.loading)
              const PremiumSkeleton(detail: true)
            else ...[
              if (c.error != null) ...[
                SettingsNote(c.error!),
                const SizedBox(height: 12),
              ],
              if (c.pending != null) ...[
                SettingsCard(
                  child: Column(
                    children: [
                      const Text(
                        'A previous purchasing change needs confirmation. New changes stay paused until it is resolved.',
                      ),
                      const SizedBox(height: 16),
                      SettingsAction(
                        c.pending?['alert_target'] != null
                            ? 'Check saved action'
                            : 'Retry saved change',
                        busy: c.busy,
                        onPressed: c.blocked ? null : retry,
                      ),
                    ],
                  ),
                ),
                const SizedBox(height: 18),
              ],
              if (c.blocked)
                const SettingsNote(
                  'Reopen Purchasing to recover your saved change.',
                ),
              if (c.record != null && !c.stale) ...details(c.record!),
              if (c.record == null && !c.stale) ...[
                if ([
                  'purchase-orders',
                  'goods-receipts',
                  'supplier-discrepancies',
                ].contains(kind)) ...[
                  SingleChildScrollView(
                    scrollDirection: Axis.horizontal,
                    child: Row(
                      children: [
                        for (final state in [
                          '',
                          ...(kind == 'purchase-orders'
                              ? [
                                  'DRAFT',
                                  'SUBMITTED',
                                  'APPROVED',
                                  'ORDERED',
                                  'PARTIALLY_RECEIVED',
                                  'RECEIVED',
                                  'CLOSED',
                                  'REJECTED',
                                  'CANCELLED',
                                ]
                              : kind == 'goods-receipts'
                              ? [
                                  'DRAFT',
                                  'INSPECTED',
                                  'POSTED',
                                  'REVERSED',
                                  'CANCELLED',
                                ]
                              : [
                                  'OPEN',
                                  'APPROVAL_REQUIRED',
                                  'APPROVED',
                                  'REJECTED',
                                  'RESOLVED',
                                  'VOIDED',
                                ]),
                        ])
                          Padding(
                            padding: const EdgeInsets.only(right: 8),
                            child: ChoiceChip(
                              label: Text(
                                state.isEmpty ? 'All' : dashboardLabel(state),
                              ),
                              selected: filter == state,
                              showCheckmark: false,
                              onSelected: (_) {
                                setState(() => filter = state);
                                unawaited(c.load(status: state));
                              },
                            ),
                          ),
                      ],
                    ),
                  ),
                  const SizedBox(height: 18),
                ],
                if (canCreate) ...[
                  SettingsAction(
                    'Add ${kind == 'purchase-orders'
                        ? 'purchase order'
                        : kind == 'products'
                        ? 'supplier product'
                        : kind == 'documents'
                        ? 'document'
                        : kind == 'contacts'
                        ? 'contact'
                        : kind == 'suppliers'
                        ? 'supplier'
                        : 'category'}',
                    icon: Icons.add_rounded,
                    onPressed: c.writable ? () => edit() : null,
                  ),
                  const SizedBox(height: 20),
                ],
                if (singleton && c.data == null)
                  SettingsAction(
                    'Set up ${purchaseTitles[kind]!.toLowerCase()}',
                    onPressed: c.writable ? () => edit() : null,
                  ),
                TextField(
                  decoration: const InputDecoration(
                    hintText: 'Search this list',
                    prefixIcon: Icon(Icons.search_rounded),
                  ),
                  onChanged: (s) => setState(() => search = s.toLowerCase()),
                ),
                const SizedBox(height: 18),
                if (c.rows.isEmpty)
                  SettingsCard(
                    child: Column(
                      children: [
                        Icon(
                          purchaseIcon(kind),
                          size: 56,
                          color: purchaseColor(kind).forInk(context),
                        ),
                        const SizedBox(height: 18),
                        Text(
                          'No ${purchaseTitles[kind]!.toLowerCase()} yet.',
                          textAlign: TextAlign.center,
                        ),
                      ],
                    ),
                  ),
                for (final r in c.rows.where(
                  (r) => purchaseTitle(r).toLowerCase().contains(search),
                )) ...[
                  SettingsCard(
                    child: ListTile(
                      contentPadding: EdgeInsets.zero,
                      leading: Icon(
                        purchaseIcon(kind),
                        color: purchaseColor(kind).forInk(context),
                      ),
                      title: Text(
                        purchaseTitle(r),
                        style: const TextStyle(fontWeight: FontWeight.w700),
                      ),
                      subtitle: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          if (r['status'] != null)
                            Text(dashboardLabel(r['status'])),
                          if (r['supplier_name_snapshot'] != null)
                            Text('${r['supplier_name_snapshot']}'),
                          if (r['total_amount_minor'] != null)
                            Text(
                              dashboardMoney(
                                r['total_amount_minor'],
                                r['currency_code'],
                              ),
                            ),
                          if (r['purchase_unit_name'] != null)
                            Text(
                              '${r['purchase_unit_name']} - ${dashboardMoney(r['typical_cost_minor'], r['currency_code'])}',
                            ),
                          if (r['note'] != null) Text('${r['note']}'),
                          if (r['supplier_name'] != null)
                            Text('${r['supplier_name']}'),
                          if (r['expected_delivery_date'] != null)
                            Text(
                              'Expected ${supportDate(r['expected_delivery_date'])}',
                            ),
                          if (r['evidence_type'] != null)
                            Text(dashboardLabel(r['evidence_type'])),
                          if (r['total_cost_minor'] != null)
                            Text(
                              dashboardMoney(
                                r['total_cost_minor'],
                                r['currency'],
                              ),
                            ),
                          if (r['observed_at'] != null)
                            Text(supportDate(r['observed_at'])),
                          if (r['external_reference'] != null)
                            Text('${r['external_reference']}'),
                          if (kind == 'alerts')
                            Wrap(
                              spacing: 8,
                              children: [
                                for (final action
                                    in (r['status'] == 'OPEN'
                                        ? ['acknowledge', 'resolve']
                                        : r['status'] == 'ACKNOWLEDGED'
                                        ? ['resolve']
                                        : <String>[]))
                                  TextButton(
                                    onPressed: c.writable
                                        ? () => command(
                                            'POST',
                                            '$purchasingBase/alerts/{alertId}/$action',
                                            {},
                                            parameters: {'alertId': r['id']},
                                            title:
                                                '${dashboardLabel(action)} alert',
                                          )
                                        : null,
                                    child: Text(dashboardLabel(action)),
                                  ),
                              ],
                            ),
                        ],
                      ),
                      trailing:
                          ['contacts', 'supplier-categories'].contains(kind)
                          ? PopupMenuButton<String>(
                              onSelected: (action) {
                                if (action == 'edit') {
                                  unawaited(edit(r));
                                } else {
                                  unawaited(
                                    command(
                                      'POST',
                                      '$base/{$idKey}/archive',
                                      {'expected_version': r['version']},
                                      parameters: {...params, idKey: r['id']},
                                      title: 'Archive record',
                                      danger: true,
                                    ),
                                  );
                                }
                              },
                              itemBuilder: (_) => [
                                const PopupMenuItem(
                                  value: 'edit',
                                  child: Text('Edit'),
                                ),
                                if (r['status'] != 'ARCHIVED')
                                  const PopupMenuItem(
                                    value: 'archive',
                                    child: Text('Archive'),
                                  ),
                              ],
                            )
                          : [
                              'suppliers',
                              'products',
                              'purchase-orders',
                              'goods-receipts',
                              'supplier-discrepancies',
                            ].contains(kind)
                          ? const Icon(Icons.chevron_right_rounded)
                          : null,
                      onTap:
                          [
                            'suppliers',
                            'products',
                            'purchase-orders',
                            'goods-receipts',
                            'supplier-discrepancies',
                          ].contains(kind)
                          ? () => open(kind, row: r)
                          : kind == 'outstanding-deliveries'
                          ? () => open(
                              'purchase-orders',
                              row: {'id': r['purchase_order_id']},
                            )
                          : ['contacts', 'supplier-categories'].contains(kind)
                          ? () => edit(r)
                          : null,
                    ),
                  ),
                  const SizedBox(height: 12),
                ],
                if (c.hasMore)
                  SettingsAction(
                    'Load more',
                    secondary: true,
                    onPressed: () => c.load(append: true, status: filter),
                  ),
              ],
            ],
          ],
        ),
      ),
    ),
  );
}


/// Maps inventory item ids to the merchant's ingredient and packaging names,
/// so pickers never show raw ids. Missing names are tolerated.
Future<Map<String, String>> _inventoryItemNamesFor(
  dynamic api,
  String branchId,
) async {
  final names = <String, String>{};
  for (final (kind, param) in [
    ('ingredients', 'ingredientId'),
    ('packaging-materials', 'packagingId'),
  ]) {
    try {
      final list = await api.request('GET', '/api/v1/vendor/$kind');
      final rows = list.data;
      if (rows is! List) continue;
      await Future.wait(rows.whereType<Map>().map((r) async {
        try {
          final cfg = await api.request(
            'GET',
            '/api/v1/vendor/$kind/{$param}/branches/{branchId}',
            pathParameters: {param: '${r['id']}', 'branchId': branchId},
          );
          final d = cfg.data;
          if (d is Map && d['inventory_item_id'] != null) {
            names['${d['inventory_item_id']}'] = '${r['name']}';
          }
        } catch (_) {}
      }));
    } catch (_) {}
  }
  return names;
}
