import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/ui/premium_loading.dart';
import '../../settings/ui/settings_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../state/fulfilment_workspace_controller.dart';

class ParcelManifestScreen extends StatefulWidget {
  const ParcelManifestScreen({
    super.key,
    required this.controller,
    required this.unit,
    this.parcel,
    required this.mock,
  });
  final FulfilmentWorkspaceController controller;
  final JsonObject unit;
  final JsonObject? parcel;
  final bool mock;
  @override
  State<ParcelManifestScreen> createState() => _ParcelManifestScreenState();
}

class _ParcelManifestScreenState extends State<ParcelManifestScreen> {
  List<JsonObject> lines = [];
  final selected = <String>{};
  final quantities = <String, TextEditingController>{};
  final count = TextEditingController(), notes = TextEditingController();
  final dimensions = <String, TextEditingController>{
    for (final name in ['weight_grams', 'length_mm', 'width_mm', 'height_mm'])
      name: TextEditingController(),
  };
  bool loading = true, fragile = false;
  String temperature = 'NONE';
  String? error;
  @override
  void initState() {
    super.initState();
    load();
  }

  Future<void> load() async {
    try {
      final order = await widget.controller.order(
        widget.unit['vendor_order_id'] as String,
      );
      final raw = order['lines'];
      if (raw is! List) throw const FormatException('Order items unavailable');
      final existing = widget.parcel?['manifest'] as Map? ?? {};
      final manifest = (existing['lines'] as List? ?? [])
          .map(jsonObject)
          .toList();
      lines = raw.map(jsonObject).toList();
      for (final saved in manifest) {
        if (!lines.any((line) => line['id'] == saved['order_line_ref'])) {
          lines.add({
            'id': saved['order_line_ref'],
            'item_name': saved['description'] ?? saved['order_line_ref'],
            'quantity': saved['quantity'],
          });
        }
      }
      for (final line in lines) {
        final id = line['id'] as String;
        final saved = manifest
            .where((l) => l['order_line_ref'] == id)
            .firstOrNull;
        if (saved != null || widget.mock) selected.add(id);
        quantities[id] = TextEditingController(
          text:
              saved?['quantity']?.toString() ??
              (widget.mock ? line['quantity']?.toString() ?? '1' : ''),
        );
      }
      count.text =
          existing['package_count']?.toString() ?? (widget.mock ? '1' : '');
      notes.text = existing['notes']?.toString() ?? '';
      fragile = widget.parcel?['fragile'] == true;
      temperature = widget.parcel?['temperature_control']?.toString() ?? 'NONE';
      for (final e in dimensions.entries) {
        e.value.text =
            widget.parcel?[e.key]?.toString() ?? (widget.mock ? '0' : '');
      }
    } catch (_) {
      error = 'Could not load the order items for this package.';
    } finally {
      if (mounted) setState(() => loading = false);
    }
  }

  @override
  void dispose() {
    count.dispose();
    notes.dispose();
    for (final t in quantities.values) {
      t.dispose();
    }
    for (final t in dimensions.values) {
      t.dispose();
    }
    super.dispose();
  }

  bool get valid =>
      !loading &&
      error == null &&
      selected.isNotEmpty &&
      int.tryParse(count.text) != null &&
      int.parse(count.text) >= 0 &&
      selected.every((id) => (int.tryParse(quantities[id]!.text) ?? -1) >= 0) &&
      dimensions.values.every(
        (t) => t.text.isEmpty || (int.tryParse(t.text) ?? -1) >= 0,
      );
  Future<void> save() async {
    if (!valid) return;
    final old = widget.parcel?['manifest'] as Map? ?? {};
    final savedLines = (old['lines'] as List? ?? []).map(jsonObject).toList();
    final manifest = <String, dynamic>{
      ...old,
      'package_count': int.parse(count.text),
      'notes': notes.text.trim(),
      'lines': [
        for (final line in lines)
          if (selected.contains(line['id']))
            {
              ...(savedLines
                      .where((v) => v['order_line_ref'] == line['id'])
                      .firstOrNull ??
                  {}),
              'order_line_ref': line['id'],
              'description':
                  line['item_name']?.toString() ?? line['id'].toString(),
              'quantity': int.parse(quantities[line['id']]!.text),
            },
      ],
    }..remove('sealed_at');
    final confirmed = await confirmSettings(
      context,
      widget.parcel == null ? 'Pack parcel' : 'Save manifest',
      '${selected.length} order items ? ${count.text} packages inside this parcel. Confirm these are the quantities actually packed.',
    );
    if (!confirmed || !mounted) return;
    final edit = widget.parcel != null;
    final ok = await widget.controller.command(
      method: edit ? 'PUT' : 'POST',
      path: edit
          ? '/api/v1/vendor/fulfilment/parcels/{parcelId}/manifest'
          : '/api/v1/vendor/fulfilment/units/{unitId}/parcels',
      params: edit
          ? {'parcelId': widget.parcel!['parcel_id'] as String}
          : {'unitId': widget.unit['fulfilment_unit_id'] as String},
      version: edit ? widget.parcel!['version'] as int : 0,
      body: edit
          ? manifest
          : {
              'manifest': manifest,
              'fragile': fragile,
              'temperature_control': temperature,
              for (final e in dimensions.entries)
                if (e.value.text.isNotEmpty) e.key: int.parse(e.value.text),
            },
    );
    if (!mounted) return;
    await showOrderFeedback(
      context,
      title: ok ? 'Package saved' : 'Package needs attention',
      message: ok
          ? 'The parcel and its packed contents have been saved.'
          : widget.controller.error ?? 'Refresh and try again.',
      success: ok,
    );
    if (ok && mounted) Navigator.pop(context, true);
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: widget.controller,
    builder: (context, _) => PopScope(
      canPop: !widget.controller.busy,
      child: Scaffold(
        appBar: AppBar(
          title: Text(widget.parcel == null ? 'Pack parcel' : 'Edit manifest'),
        ),
        body: ListView(
          padding: const EdgeInsets.all(20),
          children: [
            if (loading)
              const PremiumSkeleton(detail: true)
            else if (error != null)
              Text(error!, style: TextStyle(color: Theme.of(context).colorScheme.error))
            else ...[
              const SettingsHeading(
                'Packed contents',
                subtitle: 'Select the actual items in this parcel.',
              ),
              const SizedBox(height: 16),
              for (final line in lines)
                Padding(
                  padding: const EdgeInsets.only(bottom: 12),
                  child: SettingsCard(
                    child: Column(
                      children: [
                        CheckboxListTile(
                          contentPadding: EdgeInsets.zero,
                          title: Text(
                            line['item_name']?.toString() ??
                                line['id'].toString(),
                          ),
                          value: selected.contains(line['id']),
                          onChanged: widget.controller.busy
                              ? null
                              : (v) => setState(
                                  () => v == true
                                      ? selected.add(line['id'] as String)
                                      : selected.remove(line['id']),
                                ),
                        ),
                        if (selected.contains(line['id']))
                          TextField(
                            controller: quantities[line['id']],
                            keyboardType: TextInputType.number,
                            inputFormatters: [
                              FilteringTextInputFormatter.digitsOnly,
                            ],
                            decoration: const InputDecoration(
                              labelText: 'Packed quantity',
                            ),
                            onChanged: (_) => setState(() {}),
                          ),
                      ],
                    ),
                  ),
                ),
              TextField(
                controller: count,
                keyboardType: TextInputType.number,
                inputFormatters: [FilteringTextInputFormatter.digitsOnly],
                decoration: const InputDecoration(
                  labelText: 'Packages inside this parcel',
                ),
                onChanged: (_) => setState(() {}),
              ),
              const SizedBox(height: 16),
              TextField(
                controller: notes,
                maxLines: 3,
                decoration: const InputDecoration(
                  labelText: 'Packing notes (optional)',
                ),
              ),
              if (widget.parcel == null) ...[
                const SizedBox(height: 24),
                const SettingsHeading('Handling'),
                SwitchListTile(
                  contentPadding: EdgeInsets.zero,
                  title: const Text('Fragile'),
                  value: fragile,
                  onChanged: (v) => setState(() => fragile = v),
                ),
                DropdownButtonFormField<String>(
                  initialValue: temperature,
                  decoration: const InputDecoration(
                    labelText: 'Temperature control',
                  ),
                  items: [
                    for (final t in ['NONE', 'CHILLED', 'FROZEN', 'HOT'])
                      DropdownMenuItem(
                        value: t,
                        child: Text(t[0] + t.substring(1).toLowerCase()),
                      ),
                  ],
                  onChanged: (v) => setState(() => temperature = v!),
                ),
                const SizedBox(height: 16),
                for (final e in dimensions.entries)
                  Padding(
                    padding: const EdgeInsets.only(bottom: 16),
                    child: TextField(
                      controller: e.value,
                      keyboardType: TextInputType.number,
                      inputFormatters: [FilteringTextInputFormatter.digitsOnly],
                      decoration: InputDecoration(
                        labelText: switch (e.key) {
                          'weight_grams' => 'Weight (grams, optional)',
                          'length_mm' => 'Length (mm, optional)',
                          'width_mm' => 'Width (mm, optional)',
                          _ => 'Height (mm, optional)',
                        },
                      ),
                      onChanged: (_) => setState(() {}),
                    ),
                  ),
              ],
              const SizedBox(height: 20),
              SettingsAction(
                'Review package',
                icon: Icons.inventory_2_outlined,
                busy: widget.controller.busy,
                onPressed: valid && widget.controller.canWrite ? save : null,
              ),
            ],
          ],
        ),
      ),
    ),
  );
}
