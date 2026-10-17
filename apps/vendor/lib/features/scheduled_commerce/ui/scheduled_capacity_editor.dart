import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../crm/ui/crm_editors.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../../production/ui/production_form_screen.dart';
import '../../purchasing/ui/purchasing_forms.dart';

Future<JsonObject?> scheduledCapacity(
  BuildContext context,
  VendorRuntime runtime,
  String branch,
) async {
  final response = await runtime.backend.productionApi.request(
    'GET',
    '/api/v1/vendor/production/pools',
  );
  final pools = (response.data as List)
      .map(jsonObject)
      .where((p) => p['status'] == 'ACTIVE' && p['branch_id'] == branch)
      .toList();
  if (!context.mounted) return null;
  if (pools.isEmpty) {
    await showOrderFeedback(
      context,
      title: 'Capacity unavailable',
      message: 'No active production pools are available for this branch.',
      warning: true,
    );
    return null;
  }
  final names = {
    for (final p in pools) '${p['id']}': '${p['name']} - ${p['capability']}',
  };
  final mock = runtime.mode == VendorDataMode.mock;
  final chosen = await crmChoose(
    context,
    'Production capacity',
    names,
    multiple: true,
    initial: mock ? [names.keys.first] : [],
  );
  if (chosen == null || chosen.isEmpty || !context.mounted) return null;
  final capabilities = pools
      .where((p) => chosen.contains(p['id']))
      .map((p) => p['capability'])
      .toSet();
  if (capabilities.length != 1) {
    await showOrderFeedback(
      context,
      title: 'Check capacity',
      message: 'Choose pools with the same production capability.',
      warning: true,
    );
    return null;
  }
  final allocations = <JsonObject>[];
  for (final id in chosen) {
    final quantity = await productionForm(context, names[id]!, [
      PurchaseField(
        'quantity',
        'Capacity units',
        kind: 'number',
        required: true,
        min: 1,
        max: 1000000,
        initial: mock ? 1 : null,
      ),
    ]);
    if (quantity == null || !context.mounted) return null;
    allocations.add({'capacity_pool_id': id, 'quantity': quantity['quantity']});
  }
  final window = await productionForm(
    context,
    'Production window',
    [
      PurchaseField(
        'window_start',
        'Production starts',
        kind: 'date',
        required: true,
        initial: mock
            ? DateTime.now()
                  .toUtc()
                  .add(const Duration(days: 1))
                  .toIso8601String()
            : null,
      ),
      PurchaseField(
        'window_end',
        'Production ends',
        kind: 'date',
        required: true,
        initial: mock
            ? DateTime.now()
                  .toUtc()
                  .add(const Duration(days: 1, hours: 2))
                  .toIso8601String()
            : null,
      ),
    ],
    validate: (v) {
      final a = DateTime.tryParse('${v['window_start']}'),
          b = DateTime.tryParse('${v['window_end']}');
      return a == null || b == null || !b.isAfter(a)
          ? 'Choose an end after the start.'
          : null;
    },
  );
  return window == null
      ? null
      : {
          'required': true,
          'branch_id': branch,
          'capability': capabilities.single,
          ...window,
          'allocations': allocations,
        };
}
