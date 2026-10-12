import 'dart:async';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/ui/premium_loading.dart';
import '../../catalogue/state/catalogue_controller.dart';
import '../../catalogue/state/catalogue_resource.dart';
import '../../dashboard/ui/dashboard_empty_state.dart';
import '../../onboarding/ui/setup_design.dart';

Future<JsonObject?> pickPromotionItem(
  BuildContext context,
  VendorRuntime runtime, {
  bool variant = false,
}) async {
  final food = await showModalBottomSheet<bool>(
    context: context,
    showDragHandle: true,
    useSafeArea: true,
    builder: (context) => Padding(
      padding: const EdgeInsets.fromLTRB(20, 0, 20, 24),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          const Text(
            'Choose a catalogue',
            style: TextStyle(fontSize: 21, fontWeight: FontWeight.w700),
          ),
          const SizedBox(height: 18),
          ListTile(
            leading: const Icon(Icons.restaurant_outlined),
            title: const Text('Food menus'),
            trailing: const Icon(Icons.chevron_right_rounded),
            onTap: () => Navigator.pop(context, true),
          ),
          ListTile(
            leading: const Icon(Icons.shopping_bag_outlined),
            title: const Text('Shop products'),
            trailing: const Icon(Icons.chevron_right_rounded),
            onTap: () => Navigator.pop(context, false),
          ),
        ],
      ),
    ),
  );
  if (food == null || !context.mounted) return null;
  return Navigator.push<JsonObject>(
    context,
    setupRoute<JsonObject>(
      PromotionReferencePicker(
        runtime: runtime,
        variant: variant,
        resource: CatalogueResource(
          food ? CatalogueKind.menus : CatalogueKind.products,
          branchId: runtime.backend.scope.branchId,
          writable: false,
        ),
      ),
    ),
  );
}

class PromotionReferencePicker extends StatefulWidget {
  const PromotionReferencePicker({
    super.key,
    required this.runtime,
    required this.resource,
    this.variant = false,
    this.item,
  });
  final VendorRuntime runtime;
  final CatalogueResource resource;
  final bool variant;
  final JsonObject? item;
  @override
  State<PromotionReferencePicker> createState() =>
      _PromotionReferencePickerState();
}

class _PromotionReferencePickerState extends State<PromotionReferencePicker> {
  late final c = CatalogueController(widget.runtime.backend, widget.resource);
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

  Future<void> choose(JsonObject row) async {
    final kind = widget.resource.kind;
    final isItem = [CatalogueKind.items, CatalogueKind.products].contains(kind);
    final isVariant = [
      CatalogueKind.foodVariants,
      CatalogueKind.shopVariants,
    ].contains(kind);
    if (isVariant || isItem && !widget.variant) {
      Navigator.pop(context, <String, dynamic>{
        'item_ref': isVariant ? widget.item!['id'] : row['id'],
        'variant_ref': isVariant ? row['id'] : '',
        'label': isVariant
            ? '${widget.item!['name']} · ${row['name']}'
            : row['name'],
      });
      return;
    }
    final child = widget.resource.child(row);
    if (child == null) return;
    final selected = await Navigator.push<JsonObject>(
      context,
      setupRoute<JsonObject>(
        PromotionReferencePicker(
          runtime: widget.runtime,
          resource: child,
          variant: widget.variant,
          item: isItem ? row : widget.item,
        ),
      ),
    );
    if (selected != null && mounted) Navigator.pop(context, selected);
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: Text(widget.resource.title)),
    body: AnimatedBuilder(
      animation: c,
      builder: (context, _) => ListView(
        padding: const EdgeInsets.all(20),
        children: [
          if (c.loading && c.rows.isEmpty)
            const PremiumSkeleton()
          else if (c.error != null)
            DashboardEmptyState(
              story: EmptyStory.unavailable,
              title: 'Catalogue unavailable',
              message: c.error!,
              action: 'Try again',
              onAction: c.load,
            )
          else if (c.rows.isEmpty)
            const DashboardEmptyState(
              story: EmptyStory.products,
              title: 'Nothing to select yet',
              message:
                  'Add catalogue entries before including them in an offer.',
            )
          else ...[
            for (final row in c.rows)
              ListTile(
                contentPadding: const EdgeInsets.symmetric(
                  horizontal: 8,
                  vertical: 6,
                ),
                leading: Icon(
                  widget.resource.food
                      ? Icons.restaurant_outlined
                      : Icons.inventory_2_outlined,
                ),
                title: Text(
                  (row['name'] ?? row['display_name'] ?? 'Catalogue entry')
                      .toString(),
                ),
                trailing: const Icon(Icons.chevron_right_rounded),
                onTap: () => choose(row),
              ),
            if (c.hasMore)
              TextButton(
                onPressed: c.loading ? null : () => c.load(more: true),
                child: const Text('Load more'),
              ),
          ],
        ],
      ),
    ),
  );
}
