import '../../../core/ui/premium_loading.dart';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../dashboard/ui/dashboard_empty_state.dart';
import '../../orders/ui/order_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../../orders/ui/order_form_sheet.dart';
import '../state/catalogue_controller.dart';
import '../state/catalogue_resource.dart';
import 'catalogue_screen.dart';

class ItemModifierScreen extends StatefulWidget {
  const ItemModifierScreen({
    super.key,
    required this.runtime,
    required this.item,
    required this.currency,
  });
  final VendorRuntime runtime;
  final JsonObject item;
  final String currency;
  @override
  State<ItemModifierScreen> createState() => _ItemModifierScreenState();
}

class _ItemModifierScreenState extends State<ItemModifierScreen> {
  late final c = CatalogueController(
    widget.runtime.backend,
    CatalogueResource(
      CatalogueKind.foodVariants,
      parameters: {'itemId': widget.item['id'].toString()},
    ),
  );
  List<JsonObject> attached = [];
  bool loading = true, presenting = false;
  String? error;
  String get path => '/api/v1/vendor/food/items/{itemId}/modifier-groups';
  Map<String, String> get parameters => {
    'itemId': widget.item['id'].toString(),
  };
  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    c.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    if (c.busy) return;
    setState(() {
      loading = true;
      error = null;
    });
    await c.load();
    try {
      final response = await widget.runtime.backend.foodCatalogueApi.request(
        'GET',
        path,
        pathParameters: parameters,
      );
      attached = (response.object['modifier_groups'] as List)
          .map(jsonObject)
          .toList();
    } catch (e) {
      error = CatalogueController.message(e);
    }
    if (mounted) setState(() => loading = false);
  }

  Future<void> _add() async {
    if (presenting ||
        loading ||
        error != null ||
        !c.allowed('PUT', '$path/{groupId}')) {
      return;
    }
    presenting = true;
    final groups = CatalogueController(
      widget.runtime.backend,
      const CatalogueResource(CatalogueKind.groups),
    );
    try {
      await groups.load();
      while (groups.hasMore && groups.error == null) {
        await groups.load(more: true);
      }
      if (groups.error != null) throw StateError(groups.error!);
      final choices = groups.rows
          .where(
            (g) =>
                g['status'] == 'ACTIVE' &&
                !attached.any((a) => a['modifier_group_id'] == g['id']),
          )
          .toList();
      if (!mounted) return;
      final id = await showModalBottomSheet<String>(
        context: context,
        isScrollControlled: true,
        useSafeArea: true,
        showDragHandle: true,
        builder: (context) => ConstrainedBox(
          constraints: BoxConstraints(
            maxHeight: MediaQuery.sizeOf(context).height * .75,
          ),
          child: ListView(
            shrinkWrap: true,
            padding: const EdgeInsets.fromLTRB(22, 0, 22, 24),
            children: [
              const Text(
                'Add a modifier group',
                style: TextStyle(fontSize: 25, fontWeight: FontWeight.w700),
              ),
              const SizedBox(height: 12),
              if (choices.isEmpty)
                const Padding(
                  padding: EdgeInsets.symmetric(vertical: 24),
                  child: Text(
                    'Create a modifier group in Catalogue first. Groups already attached are not shown.',
                  ),
                ),
              for (final group in choices)
                ListTile(
                  contentPadding: EdgeInsets.zero,
                  leading: Icon(
                    Icons.tune_rounded,
                    color: KarryamColors.tealInk.forInk(context),
                  ),
                  title: Text(group['name'].toString()),
                  subtitle: Text(
                    'Choose ${group['min_select']}–${group['max_select']}',
                  ),
                  trailing: Icon(
                    Icons.add_circle_outline,
                    color: KarryamColors.tealInk.forInk(context),
                  ),
                  onTap: () => Navigator.pop(context, group['id'].toString()),
                ),
            ],
          ),
        ),
      );
      if (id == null) return;
      await _write(
        'PUT',
        '$path/{groupId}',
        {...parameters, 'groupId': id},
        null,
        'Modifier group added',
      );
    } catch (e) {
      if (mounted) {
        await showOrderFeedback(
          context,
          title: 'Groups unavailable',
          message: CatalogueController.message(e),
        );
      }
    } finally {
      groups.dispose();
      presenting = false;
    }
  }

  Future<void> _remove(JsonObject attachment) async {
    if (presenting || loading || !c.allowed('DELETE', '$path/{groupId}')) {
      return;
    }
    presenting = true;
    try {
      final group = jsonObject(attachment['group']);
      final confirm = await showModalBottomSheet<JsonObject>(
        context: context,
        isScrollControlled: true,
        useSafeArea: true,
        showDragHandle: true,
        builder: (_) => OrderFormSheet(
          title: 'Remove ${group['name']}?',
          message:
              'This removes the group from this food item. The shared group remains available for other items.',
          fields: const [],
          destructive: true,
          icon: Icons.link_off_rounded,
        ),
      );
      if (confirm != null) {
        await _write(
          'DELETE',
          '$path/{groupId}',
          {
            ...parameters,
            'groupId': attachment['modifier_group_id'].toString(),
          },
          null,
          'Modifier group removed',
        );
      }
    } finally {
      presenting = false;
    }
  }

  Future<void> _move(int index, int delta) async {
    if (!c.allowed('POST', '$path/reorder') || loading || error != null) return;
    final list = List<JsonObject>.from(attached),
        entry = List<JsonObject>.from(attached)[index];
    list.removeAt(index);
    list.insert(index + delta, entry);
    await _write('POST', '$path/reorder', parameters, {
      'ordered_ids': list.map((a) => a['modifier_group_id']).toList(),
    }, 'Modifier groups reordered');
  }

  Future<void> _write(
    String method,
    String route,
    Map<String, String> pp,
    JsonObject? body,
    String title,
  ) async {
    final saved = await c.write(method, route, pp, body);
    if (saved) await _load();
    if (mounted) {
      await showOrderFeedback(
        context,
        title: saved ? title : 'Change not saved',
        message: saved
            ? (error == null
                  ? 'Your item options are up to date.'
                  : 'Saved. Refresh to see the latest options.')
            : c.error!,
        success: saved && error == null && !c.stale,
        warning: error != null || c.stale,
      );
    }
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => PopScope(
      canPop: !c.busy,
      child: Scaffold(
        backgroundColor: KarryamColors.canvas.forSurface(context),
        appBar: AppBar(
          title: const Text('Item options'),
          actions: [
            IconButton(
              tooltip: 'Refresh item options',
              onPressed: loading || c.busy ? null : _load,
              icon: const Icon(Icons.refresh_rounded),
            ),
          ],
        ),
        body: ListView(
          padding: const EdgeInsets.all(22),
          children: [
            CatalogueBanner(
              kind: CatalogueKind.groups,
              title: widget.item['name'].toString(),
              message:
                  'Give customers the right choices. Attach shared groups, then arrange how they appear.',
            ),
            const SizedBox(height: 20),
            if (loading) const PremiumSkeleton(detail: true, rows: 2),
            if (!loading && error != null)
              DashboardEmptyState(
                story: EmptyStory.unavailable,
                title: 'Options unavailable',
                message: error!,
                action: 'Try again',
                onAction: _load,
              ),
            if (!loading && error == null && attached.isEmpty)
              DashboardEmptyState(
                story: EmptyStory.menu,
                title: 'A little choice goes a long way',
                message:
                    'Attach a group for sides, extras or other options your business offers.',
                action: 'Add modifier group',
                onAction: _add,
              ),
            if (!loading && error == null)
              for (var i = 0; i < attached.length; i++)
                CatalogueEntry(
                  row: jsonObject(attached[i]['group']),
                  kind: CatalogueKind.groups,
                  currency: widget.currency,
                  onTap: () async {
                    await orderPage<void>(
                      context,
                      CatalogueDetailScreen(
                        runtime: widget.runtime,
                        resource: const CatalogueResource(CatalogueKind.groups),
                        row: jsonObject(attached[i]['group']),
                        currency: widget.currency,
                      ),
                    );
                    if (mounted) _load();
                  },
                  actions: [
                    if (i > 0)
                      IconButton(
                        tooltip: 'Move up',
                        onPressed: c.busy ? null : () => _move(i, -1),
                        icon: const Icon(Icons.arrow_upward_rounded, size: 20),
                      ),
                    if (i < attached.length - 1)
                      IconButton(
                        tooltip: 'Move down',
                        onPressed: c.busy ? null : () => _move(i, 1),
                        icon: const Icon(
                          Icons.arrow_downward_rounded,
                          size: 20,
                        ),
                      ),
                    const Spacer(),
                    TextButton(
                      onPressed: c.busy ? null : () => _remove(attached[i]),
                      child: Text(
                        'Remove',
                        style: TextStyle(
                          color: KarryamColors.error.forInk(context),
                        ),
                      ),
                    ),
                  ],
                ),
          ],
        ),
        bottomNavigationBar: SafeArea(
          top: false,
          child: Padding(
            padding: const EdgeInsets.fromLTRB(22, 10, 22, 16),
            child: FilledButton.icon(
              onPressed:
                  loading ||
                      error != null ||
                      !c.allowed('PUT', '$path/{groupId}')
                  ? null
                  : _add,
              icon: const Icon(Icons.add_rounded),
              label: const Text('Add modifier group'),
            ),
          ),
        ),
      ),
    ),
  );
}
