import '../../media/ui/vendor_media_screen.dart';
import '../../taxonomy/ui/taxonomy_workspace_screen.dart';
import 'mock_catalogue_photo.dart';
import '../../../core/ui/premium_loading.dart';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../dashboard/ui/dashboard_empty_state.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../orders/ui/order_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../../orders/ui/order_form_sheet.dart';
import '../state/catalogue_controller.dart';
import '../state/catalogue_resource.dart';
import 'catalogue_editor.dart';
import 'advanced_screen.dart';
import 'catalogue_tools_screen.dart';
import '../state/advanced_resource.dart';
import '../../settings/ui/settings_design.dart';
import 'catalogue_art.dart';
import 'item_modifier_screen.dart';

IconData catalogueIcon(CatalogueKind kind) => switch (kind) {
  CatalogueKind.menus => Icons.menu_book_rounded,
  CatalogueKind.sections => Icons.view_agenda_outlined,
  CatalogueKind.items => Icons.ramen_dining_rounded,
  CatalogueKind.groups || CatalogueKind.modifiers => Icons.tune_rounded,
  CatalogueKind.categories => Icons.category_outlined,
  CatalogueKind.products => Icons.shopping_bag_outlined,
  CatalogueKind.listings => Icons.storefront_rounded,
  _ => Icons.layers_outlined,
};
Color catalogueColor(Object? status) => switch (status) {
  'ACTIVE' => KarryamColors.tealInk,
  'ARCHIVED' => KarryamColors.error,
  'PAUSED' || 'INACTIVE' => const Color(0xFF996114),
  _ => const Color(0xFF3974BC),
};

class CatalogueScreen extends StatefulWidget {
  const CatalogueScreen({
    super.key,
    required this.runtime,
    required this.resource,
    required this.currency,
    this.subtitle,
    this.createOnOpen = false,
  });
  final VendorRuntime runtime;
  final CatalogueResource resource;
  final String currency;
  final String? subtitle;
  final bool createOnOpen;
  @override
  State<CatalogueScreen> createState() => _CatalogueScreenState();
}

class _CatalogueScreenState extends State<CatalogueScreen> {
  late final c = CatalogueController(widget.runtime.backend, widget.resource);
  bool _presenting = false;
  @override
  void initState() {
    super.initState();
    c.load().then((_) {
      if (mounted && widget.createOnOpen && c.error == null) {
        WidgetsBinding.instance.addPostFrameCallback((_) {
          if (mounted) _edit();
        });
      }
    });
  }

  @override
  void dispose() {
    c.dispose();
    super.dispose();
  }

  Future<void> _edit([JsonObject? row]) async {
    if (_presenting ||
        !c.allowed(
          row == null ? 'POST' : 'PATCH',
          row == null ? widget.resource.collection : widget.resource.detail,
        )) {
      return;
    }
    _presenting = true;
    try {
      final record = row == null ? null : await c.read(row);
      final choices = await catalogueChoices(
        widget.runtime,
        widget.resource,
        record,
      );
      if (!mounted) return;
      final result = await openCatalogueEditor(
        context,
        runtime: widget.runtime,
        resource: widget.resource,
        currency: widget.currency,
        record: record,
        choices: choices,
        onSave: (body) {
          if (row == null && widget.resource.kind == CatalogueKind.menus) {
            body['branch_id'] = widget.resource.branchId;
          }
          if (record != null) body['expected_version'] = record['version'];
          return c.write(
            row == null ? 'POST' : 'PATCH',
            row == null ? widget.resource.collection : widget.resource.detail,
            row == null
                ? widget.resource.parameters
                : widget.resource.detailParameters(row['id'].toString()),
            body,
          );
        },
        errorMessage: () => c.error,
        canSave: () => c.allowed(
          row == null ? 'POST' : 'PATCH',
          row == null ? widget.resource.collection : widget.resource.detail,
        ),
      );
      if (result == true && mounted) {
        await showOrderFeedback(
          context,
          title: row == null
              ? '${dashboardLabel(widget.resource.singular)} created'
              : 'Changes saved',
          message: c.feedback!,
          success: !c.stale,
          warning: c.stale,
        );
      }
    } catch (e) {
      if (mounted) {
        await showOrderFeedback(
          context,
          title: 'Editor unavailable',
          message: CatalogueController.message(e),
        );
      }
    } finally {
      if (mounted && c.stale && !c.busy) await c.load();
      _presenting = false;
    }
  }

  Future<void> _open(JsonObject row) async {
    if (c.busy || c.loading) return;
    final resource = widget.resource;
    if (row['status'] == 'ARCHIVED' && resource.child(row) == null) {
      await showOrderFeedback(
        context,
        title: 'Archived entry',
        message: 'This entry is retired from the catalogue.',
        warning: true,
      );
      return;
    }
    if ([CatalogueKind.sections].contains(resource.kind)) {
      await orderPage<void>(
        context,
        CatalogueScreen(
          runtime: widget.runtime,
          resource: resource.child(row)!,
          currency: widget.currency,
          subtitle: row['name'].toString(),
        ),
      );
    } else if (resource.child(row) == null) {
      await _edit(row);
      return;
    } else {
      await orderPage<void>(
        context,
        CatalogueDetailScreen(
          runtime: widget.runtime,
          resource: resource,
          row: row,
          currency: widget.currency,
        ),
      );
    }
    if (mounted) c.load();
  }

  Future<void> _archive(JsonObject row) async {
    if (_presenting) return;
    final path = '${widget.resource.detail}/archive';
    if (!c.allowed('POST', path)) return;
    _presenting = true;
    try {
      final fresh = await c.read(row);
      if (!mounted) return;
      final confirmed = await showModalBottomSheet<JsonObject>(
        context: context,
        isScrollControlled: true,
        useSafeArea: true,
        showDragHandle: true,
        builder: (_) => OrderFormSheet(
          title: 'Archive ${widget.resource.singular}?',
          message:
              '${fresh['name'] ?? 'This entry'} will be retired from the catalogue. Existing orders keep their original details.',
          fields: const [],
          destructive: true,
          icon: Icons.archive_outlined,
        ),
      );
      if (confirmed == null) return;
      final saved = await c.write(
        'POST',
        path,
        widget.resource.detailParameters(row['id'].toString()),
        {'expected_version': fresh['version']},
      );
      if (mounted) {
        await showOrderFeedback(
          context,
          title: saved ? 'Entry archived' : 'Could not archive',
          message: saved ? c.feedback! : c.error!,
          success: saved && !c.stale,
          warning: c.stale,
          ending: saved,
        );
      }
    } catch (e) {
      if (mounted) {
        await showOrderFeedback(
          context,
          title: 'Could not archive',
          message: CatalogueController.message(e),
        );
      }
    } finally {
      _presenting = false;
    }
  }

  Future<void> _restore(JsonObject row) async {
    final path = '${widget.resource.detail}/restore';
    if (_presenting || !c.allowed('POST', path)) return;
    _presenting = true;
    try {
      final fresh = await c.read(row);
      if (!mounted) return;
      final confirmation = await showModalBottomSheet<JsonObject>(
        context: context,
        isScrollControlled: true,
        useSafeArea: true,
        showDragHandle: true,
        builder: (_) => OrderFormSheet(
          title: 'Restore ${fresh['name']}?',
          message:
              'The category will return to the active catalogue. The server will check its parent category.',
          fields: const [],
          icon: Icons.restore_rounded,
        ),
      );
      if (confirmation == null) return;
      final saved = await c.write(
        'POST',
        path,
        widget.resource.detailParameters(row['id'].toString()),
        {'expected_version': fresh['version']},
      );
      if (mounted) {
        await showOrderFeedback(
          context,
          title: saved ? 'Category restored' : 'Could not restore',
          message: saved ? c.feedback! : c.error!,
          success: saved && !c.stale,
          warning: c.stale,
        );
      }
    } catch (e) {
      if (mounted) {
        await showOrderFeedback(
          context,
          title: 'Could not restore',
          message: CatalogueController.message(e),
        );
      }
    } finally {
      _presenting = false;
    }
  }

  Future<void> _reorder(int index, int delta) async {
    if (!c.allowed('POST', '${widget.resource.collection}/reorder') ||
        c.hasMore ||
        c.archived) {
      return;
    }
    final ordered = List<JsonObject>.from(c.rows);
    final row = ordered.removeAt(index);
    ordered.insert(index + delta, row);
    final saved = await c.write(
      'POST',
      '${widget.resource.collection}/reorder',
      widget.resource.parameters,
      {'ordered_ids': ordered.map((r) => r['id']).toList()},
    );
    if (!saved && mounted) {
      await showOrderFeedback(
        context,
        title: 'Order unchanged',
        message: c.error!,
        warning: c.stale,
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
          title: Text(widget.resource.title),
          actions: [
            IconButton(
              tooltip: 'Refresh catalogue',
              onPressed: c.loading || c.busy ? null : () => c.load(),
              icon: const Icon(Icons.refresh_rounded),
            ),
          ],
        ),
        body: RefreshIndicator(
          onRefresh: () => c.load(),
          color: KarryamColors.tealInk.forInk(context),
          child: ListView(
            padding: const EdgeInsets.fromLTRB(22, 10, 22, 28),
            children: [
              CatalogueBanner(
                kind: widget.resource.kind,
                title: widget.subtitle ?? widget.resource.title,
                message: widget.resource.kind == CatalogueKind.listings
                    ? 'Choose which product variants this branch sells.'
                    : widget.resource.kind == CatalogueKind.menus
                    ? 'A menu worth coming back for.'
                    : widget.resource.kind == CatalogueKind.products
                    ? 'Build a catalogue your customers will love.'
                    : 'Thoughtful details. A better customer experience.',
              ),
              if (widget.resource.kind == CatalogueKind.products)
                SettingsLink(
                  'Find a product',
                  Icons.manage_search_rounded,
                  subtitle: 'Search by SKU or barcode.',
                  onTap: () => orderPage<void>(
                    context,
                    CatalogueLookupScreen(
                      runtime: widget.runtime,
                      currency: widget.currency,
                      writable: widget.resource.writable,
                    ),
                  ),
                ),
              if (widget.resource.kind == CatalogueKind.products)
                SettingsLink(
                  'Trade measures',
                  Icons.straighten_rounded,
                  subtitle: 'Define exact units and local measures.',
                  onTap: () => orderPage<void>(
                    context,
                    AdvancedCatalogueScreen(
                      runtime: widget.runtime,
                      resource: AdvancedResource(
                        AdvancedKind.measures,
                        writable: widget.resource.writable,
                      ),
                      currency: widget.currency,
                    ),
                  ),
                ),
              if (widget.resource.kind == CatalogueKind.listings &&
                  widget.resource.branchId != null)
                SettingsLink(
                  'Selling models',
                  Icons.storefront_outlined,
                  subtitle: 'Choose the models supported by this branch.',
                  onTap: () => orderPage<void>(
                    context,
                    AdvancedCatalogueScreen(
                      runtime: widget.runtime,
                      resource: AdvancedResource(
                        AdvancedKind.models,
                        parameters: {'branchId': widget.resource.branchId!},
                        writable: widget.resource.writable,
                      ),
                      currency: widget.currency,
                    ),
                  ),
                ),
              const SizedBox(height: 22),
              if (widget.resource.kind != CatalogueKind.menus)
                Row(
                  children: [
                    Expanded(
                      child: Text(
                        '${c.rows.length}${c.hasMore ? '+' : ''} ${widget.resource.title.toLowerCase()}',
                        style: const TextStyle(fontWeight: FontWeight.w700),
                      ),
                    ),
                    ArchiveFilter(
                      selected: c.archived,
                      onChanged: c.loading || c.busy
                          ? null
                          : (v) {
                              c.archived = v;
                              c.load();
                            },
                    ),
                  ],
                ),
              if (c.stale && !c.loading && c.rows.isNotEmpty)
                _catalogueNotice(
                  context,
                  c.error ?? 'Refresh before making another change.',
                  () => c.load(),
                ),
              if (c.loading && c.rows.isEmpty) const PremiumSkeleton(),
              if (c.error != null && c.rows.isEmpty)
                DashboardEmptyState(
                  story: c.restricted
                      ? EmptyStory.restricted
                      : EmptyStory.unavailable,
                  title: c.restricted
                      ? 'Catalogue access required'
                      : 'Catalogue unavailable',
                  message: c.error!,
                  action: 'Try again',
                  onAction: () => c.load(),
                ),
              if (!c.loading && c.error == null && c.rows.isEmpty)
                DashboardEmptyState(
                  story: widget.resource.food
                      ? EmptyStory.menu
                      : EmptyStory.products,
                  title: 'Your next ${widget.resource.singular} starts here',
                  message: 'Add the details that make your business yours.',
                  action: 'Create ${widget.resource.singular}',
                  onAction: () => _edit(),
                ),
              for (var i = 0; i < c.rows.length; i++)
                CatalogueEntry(
                  row: c.rows[i],
                  mockPhotos: widget.runtime.mode == VendorDataMode.mock,
                  displayName: c.labels[c.rows[i]['id'].toString()],
                  kind: widget.resource.kind,
                  currency: widget.currency,
                  onTap: () => _open(c.rows[i]),
                  actions: c.rows[i]['status'] == 'ARCHIVED'
                      ? [
                          if (widget.resource.kind == CatalogueKind.categories)
                            TextButton.icon(
                              onPressed:
                                  c.allowed(
                                    'POST',
                                    '${widget.resource.detail}/restore',
                                  )
                                  ? () => _restore(c.rows[i])
                                  : null,
                              icon: const Icon(Icons.restore_rounded),
                              label: const Text('Restore category'),
                            ),
                        ]
                      : [
                          IconButton(
                            tooltip: 'Edit ${widget.resource.singular}',
                            onPressed:
                                c.allowed('PATCH', widget.resource.detail)
                                ? () => _edit(c.rows[i])
                                : null,
                            icon: Icon(
                              Icons.edit_outlined,
                              size: 20,
                              color: KarryamColors.tealInk.forInk(context),
                            ),
                          ),
                          if (widget.resource.reorderable &&
                              !c.archived &&
                              !c.hasMore) ...[
                            if (i > 0)
                              IconButton(
                                tooltip: 'Move up',
                                onPressed: c.busy
                                    ? null
                                    : () => _reorder(i, -1),
                                icon: const Icon(
                                  Icons.arrow_upward_rounded,
                                  size: 19,
                                ),
                              ),
                            if (i < c.rows.length - 1)
                              IconButton(
                                tooltip: 'Move down',
                                onPressed: c.busy ? null : () => _reorder(i, 1),
                                icon: const Icon(
                                  Icons.arrow_downward_rounded,
                                  size: 19,
                                ),
                              ),
                          ],
                          TextButton(
                            onPressed:
                                c.allowed(
                                  'POST',
                                  '${widget.resource.detail}/archive',
                                )
                                ? () => _archive(c.rows[i])
                                : null,
                            child: Text(
                              'Archive',
                              style: TextStyle(
                                color: KarryamColors.error.forInk(context),
                                fontSize: 12,
                              ),
                            ),
                          ),
                        ],
                ),
              if (c.hasMore)
                TextButton(
                  onPressed: c.loading ? null : () => c.load(more: true),
                  child: Text(c.loading ? 'Loading…' : 'Load more'),
                ),
            ],
          ),
        ),
        bottomNavigationBar: SafeArea(
          top: false,
          child: Padding(
            padding: const EdgeInsets.fromLTRB(22, 10, 22, 16),
            child: FilledButton.icon(
              onPressed: c.allowed('POST', widget.resource.collection)
                  ? () => _edit()
                  : null,
              icon: c.busy
                  ? const SizedBox(
                      width: 20,
                      height: 20,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : const Icon(Icons.add_rounded),
              label: Text('Create ${widget.resource.singular}'),
            ),
          ),
        ),
      ),
    ),
  );
}

Future<Map<String, Map<String, String>>> catalogueChoices(
  VendorRuntime runtime,
  CatalogueResource resource,
  JsonObject? record,
) async {
  final choices = <String, Map<String, String>>{};
  final app = runtime.backend;
  if (resource.kind == CatalogueKind.menus && record == null) {
    final response = await app.businessApi.request(
      'GET',
      '/api/v1/vendor/businesses/{businessId}/branches/{branchId}/capabilities',
      pathParameters: {
        'businessId': app.scope.vendorId!,
        'branchId': resource.branchId!,
      },
    );
    final data = response.data;
    final values = data is List ? data : (data as Map)['capabilities'];
    choices['capability_type'] = {
      for (final c in (values as List).map(jsonObject))
        if (['RESTAURANT', 'FAST_FOOD'].contains(c['capability_type']) &&
            c['status'] != 'ARCHIVED')
          c['capability_type'].toString(): dashboardLabel(c['capability_type']),
    };
  }
  if ([
    CatalogueKind.products,
    CatalogueKind.categories,
  ].contains(resource.kind)) {
    final categories = CatalogueController(
      app,
      const CatalogueResource(CatalogueKind.categories),
    );
    try {
      await categories.load();
      while (categories.hasMore && categories.error == null) {
        await categories.load(more: true);
      }
      if (categories.error != null) throw StateError(categories.error!);
      choices[resource.kind == CatalogueKind.products
          ? 'category_id'
          : 'parent_id'] = {
        for (final row in categories.rows)
          if (row['status'] != 'ARCHIVED' && row['id'] != record?['id'])
            row['id'].toString(): row['name'].toString(),
      };
    } finally {
      categories.dispose();
    }
  }
  if (resource.kind == CatalogueKind.listings && record == null) {
    final products = CatalogueController(
      app,
      const CatalogueResource(CatalogueKind.products),
    );
    try {
      await products.load();
      while (products.hasMore && products.error == null) {
        await products.load(more: true);
      }
      if (products.error != null) throw StateError(products.error!);
      final options = <String, String>{};
      for (final product in products.rows.where(
        (p) => p['status'] != 'ARCHIVED',
      )) {
        final response = await app.shopCatalogueApi
            .getShopProductsProductIdVariants(
              productId: product['id'].toString(),
            );
        for (final variant
            in (response.data as List)
                .map(jsonObject)
                .where((v) => v['status'] != 'ARCHIVED')) {
          options[variant['id'].toString()] =
              '${product['name']} · ${variant['name']}';
        }
      }
      choices['variant_id'] = options;
    } finally {
      products.dispose();
    }
  }
  return choices;
}

Future<bool?> openCatalogueEditor(
  BuildContext context, {
  required VendorRuntime runtime,
  required CatalogueResource resource,
  required String currency,
  JsonObject? record,
  Map<String, Map<String, String>> choices = const {},
  required Future<bool> Function(JsonObject) onSave,
  String? Function()? errorMessage,
  bool Function()? canSave,
}) {
  final sheet = [
    CatalogueKind.sections,
    CatalogueKind.foodVariants,
    CatalogueKind.modifiers,
  ].contains(resource.kind);
  final editor = CatalogueEditor(
    runtime: runtime,
    resource: resource,
    currency: currency,
    record: record,
    choices: choices,
    onSave: onSave,
    errorMessage: errorMessage,
    canSave: canSave,
    sheet: sheet,
  );
  return sheet
      ? showModalBottomSheet<bool>(
          context: context,
          isScrollControlled: true,
          useSafeArea: true,
          showDragHandle: true,
          isDismissible: false,
          enableDrag: false,
          backgroundColor: KarryamColors.canvas.forSurface(context),
          builder: (_) => editor,
        )
      : orderPage<bool>(context, editor);
}

class CatalogueDetailScreen extends StatefulWidget {
  const CatalogueDetailScreen({
    super.key,
    required this.runtime,
    required this.resource,
    required this.row,
    required this.currency,
  });
  final VendorRuntime runtime;
  final CatalogueResource resource;
  final JsonObject row;
  final String currency;
  @override
  State<CatalogueDetailScreen> createState() => _CatalogueDetailScreenState();
}

class _CatalogueDetailScreenState extends State<CatalogueDetailScreen> {
  late final c = CatalogueController(widget.runtime.backend, widget.resource);
  JsonObject? record;
  String? error;
  bool loading = true, presenting = false;
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
      final row = await c.read(widget.row);
      if (mounted) setState(() => record = row);
    } catch (e) {
      error = CatalogueController.message(e);
    }
    if (mounted) setState(() => loading = false);
  }

  Future<void> _edit() async {
    if (presenting || loading || c.stale) return;
    presenting = true;
    try {
      final fresh = await c.read(record!);
      final choices = await catalogueChoices(
        widget.runtime,
        widget.resource,
        fresh,
      );
      if (!mounted) return;
      final saved = await openCatalogueEditor(
        context,
        runtime: widget.runtime,
        resource: widget.resource,
        currency: widget.currency,
        record: fresh,
        choices: choices,
        errorMessage: () => c.error,
        canSave: () => c.allowed('PATCH', widget.resource.detail),
        onSave: (body) => c.write(
          'PATCH',
          widget.resource.detail,
          widget.resource.detailParameters(fresh['id'].toString()),
          {...body, 'expected_version': fresh['version']},
        ),
      );
      if (saved == true && mounted) {
        await _load();
        if (mounted) {
          await showOrderFeedback(
            context,
            title: 'Changes saved',
            message: c.feedback ?? 'Your catalogue is up to date.',
            success: error == null,
            warning: error != null,
          );
        }
      }
    } catch (e) {
      if (mounted) {
        await showOrderFeedback(
          context,
          title: 'Editor unavailable',
          message: CatalogueController.message(e),
        );
      }
    } finally {
      if (mounted && c.stale && !c.busy) await _load();
      presenting = false;
    }
  }

  Widget? _primaryPhoto() {
    if (record == null) return null;
    String? ref;
    final food = widget.resource.kind == CatalogueKind.items;
    if (food) {
      final media = (record!['media'] as List? ?? []).map(jsonObject).toList();
      ref =
          (media.where((e) => e['is_primary'] == true).firstOrNull ??
                  media.firstOrNull)?['media_ref']
              ?.toString();
    } else if (widget.resource.kind == CatalogueKind.products) {
      ref = (record!['media_refs'] as List? ?? []).firstOrNull?.toString();
    }
    return ref == null
        ? null
        : MediaPreview(
            runtime: widget.runtime,
            targetType: food ? 'FOOD_ITEM' : 'SHOP_PRODUCT',
            targetRef: record!['id'].toString(),
            ref: ref,
          );
  }

  Future<void> _media() async {
    if (presenting || loading || record == null) return;
    presenting = true;
    try {
      final food = widget.resource.kind == CatalogueKind.items;
      final fresh = await c.read(record!);
      final id = fresh['id'].toString();
      final parameters = widget.resource.detailParameters(id);
      final initial = food
          ? (await c.api.request(
                  'GET',
                  '${widget.resource.detail}/media',
                  pathParameters: parameters,
                )).object['media']
                as List
          : fresh['media_refs'] as List? ?? [];
      if (!mounted) return;
      final result = await vendorMedia(
        context,
        runtime: widget.runtime,
        targetType: food ? 'FOOD_ITEM' : 'SHOP_PRODUCT',
        targetRef: id,
        entries: food
            ? initial.map(jsonObject).toList()
            : [
                for (final ref in initial) {'media_ref': ref},
              ],
        save: (media) async {
          final latest = await c.read(fresh);
          if (latest['version'] != fresh['version']) {
            throw const VendorContractException(
              'This product changed. Reopen photos before saving.',
            );
          }
          return MediaSave(
            c.api,
            food ? 'PUT' : 'PATCH',
            food ? '${widget.resource.detail}/media' : widget.resource.detail,
            parameters,
            {
              'expected_version': latest['version'],
              if (food)
                'media': [
                  for (final e in media)
                    {
                      'media_ref': e['media_ref'],
                      'is_primary': e['is_primary'] == true,
                    },
                ],
              if (!food) 'media_refs': [for (final e in media) e['media_ref']],
            },
          );
        },
      );
      if (result != null && mounted) await _load();
    } catch (e) {
      if (mounted) {
        await showOrderFeedback(
          context,
          title: 'Photos unavailable',
          message: CatalogueController.message(e),
        );
      }
    } finally {
      presenting = false;
    }
  }

  Future<void> _status(String action) async {
    if (presenting || loading || record == null) return;
    presenting = true;
    try {
      final fresh = await c.read(record!);
      if (!mounted) return;
      if (action == 'activate' && widget.resource.kind == CatalogueKind.menus) {
        final blocker = await c.publicationBlocker(fresh);
        if (!mounted) return;
        if (blocker != null) {
          await showOrderFeedback(
            context,
            title: 'Menu not ready to publish',
            message: blocker,
            warning: true,
          );
          return;
        }
      }
      final confirmation = await showModalBottomSheet<JsonObject>(
        context: context,
        isScrollControlled: true,
        useSafeArea: true,
        showDragHandle: true,
        builder: (_) => OrderFormSheet(
          title: '${dashboardLabel(action)} ${widget.resource.singular}?',
          message: action == 'activate'
              ? 'The server will check whether this entry can be activated.'
              : action == 'pause'
              ? 'Customers will no longer see this as an active catalogue entry.'
              : 'This entry will be retired. Existing orders keep their original details.',
          fields: const [],
          destructive: action == 'archive',
          icon: action == 'archive'
              ? Icons.archive_outlined
              : action == 'pause'
              ? Icons.pause_circle_outline
              : Icons.publish_rounded,
        ),
      );
      if (confirmation == null) return;
      final menu = widget.resource.kind == CatalogueKind.menus;
      final saved = await c.write(
        menu || action == 'archive' ? 'POST' : 'PATCH',
        menu || action == 'archive'
            ? '${widget.resource.detail}/$action'
            : widget.resource.detail,
        widget.resource.detailParameters(fresh['id'].toString()),
        {
          'expected_version': fresh['version'],
          if (!menu && action != 'archive')
            'status': action == 'activate' ? 'ACTIVE' : 'PAUSED',
        },
      );
      if (saved) await _load();
      if (mounted) {
        await showOrderFeedback(
          context,
          title: saved
              ? '${dashboardLabel(widget.resource.singular)} ${action == 'activate'
                    ? 'activated'
                    : action == 'pause'
                    ? 'paused'
                    : 'archived'}'
              : 'Change not saved',
          message: saved
              ? c.feedback ?? 'Your catalogue is up to date.'
              : c.error!,
          success: saved && !c.stale,
          warning: c.stale,
          ending: saved && action == 'archive',
        );
      }
    } catch (e) {
      if (mounted) {
        await showOrderFeedback(
          context,
          title: 'Change not saved',
          message: CatalogueController.message(e),
        );
      }
    } finally {
      presenting = false;
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
          title: Text(dashboardLabel(widget.resource.singular)),
          actions: [
            IconButton(
              tooltip: 'Refresh details',
              onPressed: loading || c.busy ? null : _load,
              icon: const Icon(Icons.refresh_rounded),
            ),
          ],
        ),
        body: loading
            ? const SingleChildScrollView(
                padding: EdgeInsets.all(22),
                child: PremiumSkeleton(detail: true, rows: 2),
              )
            : record == null
            ? Padding(
                padding: const EdgeInsets.all(22),
                child: DashboardEmptyState(
                  story: EmptyStory.unavailable,
                  title: 'Details unavailable',
                  message: error ?? c.error ?? 'Please try again.',
                  action: 'Try again',
                  onAction: _load,
                ),
              )
            : ListView(
                padding: const EdgeInsets.fromLTRB(22, 12, 22, 28),
                children: [
                  CatalogueBanner(
                    kind: widget.resource.kind,
                    title: record!['name'].toString(),
                    message: record!['description']?.toString() ?? '',
                    status: record!['status'],
                    photo: _primaryPhoto(),
                    mockPhotoId:
                        widget.runtime.mode == VendorDataMode.mock &&
                            [
                              CatalogueKind.items,
                              CatalogueKind.products,
                              CatalogueKind.shopVariants,
                            ].contains(widget.resource.kind)
                        ? record!['id']?.toString()
                        : null,
                  ),
                  if (error != null || c.stale)
                    _catalogueNotice(
                      context,
                      error ?? c.error ?? 'Refresh before making changes.',
                      _load,
                    ),
                  const SizedBox(height: 22),
                  if ({
                    CatalogueKind.items,
                    CatalogueKind.products,
                    CatalogueKind.shopVariants,
                  }.contains(widget.resource.kind))
                    Padding(
                      padding: const EdgeInsets.only(bottom: 18),
                      child: SettingsLink(
                        'Classification',
                        Icons.account_tree_outlined,
                        subtitle:
                            'Categories, assigned tags and dietary claims.',
                        onTap: () => orderPage<void>(
                          context,
                          TaxonomyWorkspaceScreen(
                            runtime: widget.runtime,
                            sourceType:
                                widget.resource.kind == CatalogueKind.items
                                ? 'FOOD_ITEM'
                                : widget.resource.kind == CatalogueKind.products
                                ? 'SHOP_PRODUCT'
                                : 'SHOP_VARIANT',
                            sourceRef: record!['id'].toString(),
                            name: (record!['name'] ?? 'Variant').toString(),
                          ),
                        ),
                      ),
                    ),
                  if (widget.resource.kind == CatalogueKind.items)
                    Container(
                      padding: const EdgeInsets.all(20),
                      decoration: BoxDecoration(
                        color: Colors.white.forSurface(context),
                        borderRadius: BorderRadius.circular(24),
                      ),
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          const Text(
                            'Base price',
                            style: TextStyle(fontSize: 12),
                          ),
                          const SizedBox(height: 8),
                          Text(
                            dashboardMoney(
                              record!['base_price_minor'],
                              record!['currency'] ?? widget.currency,
                            ),
                            style: const TextStyle(
                              fontSize: 28,
                              fontWeight: FontWeight.w700,
                            ),
                          ),
                          for (final key in ['dietary_codes', 'allergen_codes'])
                            if (record![key] is List &&
                                (record![key] as List).isNotEmpty)
                              Padding(
                                padding: const EdgeInsets.only(top: 12),
                                child: Text(
                                  '${dashboardLabel(key)}: ${(record![key] as List).join(', ')}',
                                ),
                              ),
                        ],
                      ),
                    ),
                  if ({
                    CatalogueKind.items,
                    CatalogueKind.listings,
                  }.contains(widget.resource.kind))
                    Padding(
                      padding: const EdgeInsets.only(top: 18),
                      child: SettingsLink(
                        'Price preview',
                        Icons.price_check_rounded,
                        subtitle: widget.resource.kind == CatalogueKind.items
                            ? 'Check the price with a variant and extras.'
                            : 'Check quantity and channel pricing.',
                        onTap: () => orderPage<void>(
                          context,
                          CataloguePriceScreen(
                            runtime: widget.runtime,
                            record: record!,
                            food: widget.resource.kind == CatalogueKind.items,
                          ),
                        ),
                      ),
                    ),
                  if ({
                    CatalogueKind.menus,
                    CatalogueKind.items,
                    CatalogueKind.products,
                    CatalogueKind.shopVariants,
                    CatalogueKind.listings,
                  }.contains(widget.resource.kind))
                    Padding(
                      padding: const EdgeInsets.only(top: 18),
                      child: SettingsLink(
                        widget.resource.kind == CatalogueKind.menus
                            ? 'Menu schedules'
                            : 'Selling settings',
                        Icons.tune_rounded,
                        subtitle: widget.resource.kind == CatalogueKind.menus
                            ? 'Set the days and hours this menu is available.'
                            : 'Configure availability, units and product details.',
                        onTap: () async {
                          final hub = AdvancedCatalogueHub(
                            runtime: widget.runtime,
                            kind: widget.resource.kind,
                            record: record!,
                            currency: widget.currency,
                            branchId: widget.resource.branchId,
                            writable: widget.resource.writable,
                          );
                          await orderPage<void>(
                            context,
                            widget.resource.kind == CatalogueKind.menus
                                ? AdvancedCatalogueScreen(
                                    runtime: widget.runtime,
                                    resource: hub.resources.first,
                                    currency: widget.currency,
                                  )
                                : hub,
                          );
                          if (mounted) _load();
                        },
                      ),
                    ),
                  if (widget.resource.kind == CatalogueKind.items)
                    Padding(
                      padding: const EdgeInsets.only(top: 18),
                      child: ListTile(
                        tileColor: Colors.white.forSurface(context),
                        shape: RoundedRectangleBorder(
                          borderRadius: BorderRadius.circular(24),
                        ),
                        leading: Icon(
                          Icons.tune_rounded,
                          color: KarryamColors.tealInk.forInk(context),
                        ),
                        title: const Text(
                          'Manage item options',
                          style: TextStyle(fontWeight: FontWeight.w700),
                        ),
                        subtitle: const Text(
                          'Attach and arrange modifier groups.',
                        ),
                        trailing: Icon(
                          Icons.arrow_forward_rounded,
                          color: KarryamColors.tealInk.forInk(context),
                        ),
                        onTap: () async {
                          await orderPage<void>(
                            context,
                            ItemModifierScreen(
                              runtime: widget.runtime,
                              item: record!,
                              currency: widget.currency,
                            ),
                          );
                          if (mounted) _load();
                        },
                      ),
                    ),
                  if (widget.resource.kind == CatalogueKind.products)
                    _detailFact(
                      'Product class',
                      dashboardLabel(record!['product_class']),
                      Icons.category_outlined,
                    ),
                  if (widget.resource.kind == CatalogueKind.groups)
                    _detailFact(
                      'Selection rules',
                      'Choose ${record!['min_select']}–${record!['max_select']}',
                      Icons.checklist_rounded,
                    ),
                  if (widget.resource.child(record!) case final child?)
                    Padding(
                      padding: const EdgeInsets.only(top: 18),
                      child: Material(
                        color: Colors.white.forSurface(context),
                        borderRadius: BorderRadius.circular(24),
                        child: ListTile(
                          contentPadding: const EdgeInsets.all(20),
                          leading: Icon(
                            catalogueIcon(child.kind),
                            color: KarryamColors.tealInk.forInk(context),
                            size: 30,
                          ),
                          title: Text(
                            'Manage ${child.title.toLowerCase()}',
                            style: const TextStyle(fontWeight: FontWeight.w700),
                          ),
                          subtitle: Text(switch (child.kind) {
                            CatalogueKind.sections =>
                              'Organise your menu, then add food items.',
                            CatalogueKind.foodVariants =>
                              'Sizes, portions and price overrides.',
                            CatalogueKind.shopVariants =>
                              'Packs, prices and handling requirements.',
                            _ => 'Add and organise the available options.',
                          }),
                          trailing: Icon(
                            Icons.arrow_forward_rounded,
                            color: KarryamColors.tealInk.forInk(context),
                          ),
                          onTap: () async {
                            await orderPage<void>(
                              context,
                              CatalogueScreen(
                                runtime: widget.runtime,
                                resource: child,
                                currency: widget.currency,
                                subtitle: record!['name'].toString(),
                              ),
                            );
                            if (mounted) _load();
                          },
                        ),
                      ),
                    ),
                  const SizedBox(height: 24),
                  if ([
                        CatalogueKind.items,
                        CatalogueKind.products,
                      ].contains(widget.resource.kind) &&
                      record!['status'] != 'ARCHIVED') ...[
                    OutlinedButton.icon(
                      onPressed:
                          error == null &&
                              c.allowed(
                                widget.resource.kind == CatalogueKind.items
                                    ? 'PUT'
                                    : 'PATCH',
                                widget.resource.kind == CatalogueKind.items
                                    ? '${widget.resource.detail}/media'
                                    : widget.resource.detail,
                              )
                          ? _media
                          : null,
                      icon: const Icon(Icons.photo_library_outlined),
                      label: const Text('Manage photos'),
                    ),
                    const SizedBox(height: 12),
                  ],
                  if (record!['status'] != 'ARCHIVED') ...[
                    OutlinedButton.icon(
                      onPressed:
                          c.allowed('PATCH', widget.resource.detail) &&
                              error == null
                          ? _edit
                          : null,
                      icon: const Icon(Icons.edit_outlined),
                      style: OutlinedButton.styleFrom(
                        minimumSize: const Size.fromHeight(58),
                        padding: const EdgeInsets.symmetric(
                          horizontal: 20,
                          vertical: 16,
                        ),
                        shape: RoundedRectangleBorder(
                          borderRadius: BorderRadius.circular(20),
                        ),
                      ),
                      label: const Text('Edit details'),
                    ),
                    if ([
                      CatalogueKind.menus,
                      CatalogueKind.items,
                      CatalogueKind.products,
                    ].contains(widget.resource.kind)) ...[
                      const SizedBox(height: 12),
                      if (['DRAFT', 'PAUSED'].contains(record!['status']))
                        FilledButton.icon(
                          onPressed:
                              !c.allowed(
                                    widget.resource.kind == CatalogueKind.menus
                                        ? 'POST'
                                        : 'PATCH',
                                    widget.resource.kind == CatalogueKind.menus
                                        ? '${widget.resource.detail}/activate'
                                        : widget.resource.detail,
                                  ) ||
                                  error != null
                              ? null
                              : () => _status('activate'),
                          icon: const Icon(Icons.publish_rounded),
                          label: const Text('Activate'),
                        ),
                      if (record!['status'] == 'ACTIVE')
                        OutlinedButton.icon(
                          onPressed:
                              !c.allowed(
                                    widget.resource.kind == CatalogueKind.menus
                                        ? 'POST'
                                        : 'PATCH',
                                    widget.resource.kind == CatalogueKind.menus
                                        ? '${widget.resource.detail}/pause'
                                        : widget.resource.detail,
                                  ) ||
                                  error != null
                              ? null
                              : () => _status('pause'),
                          icon: const Icon(Icons.pause_circle_outline),
                          style: OutlinedButton.styleFrom(
                            minimumSize: const Size.fromHeight(58),
                            padding: const EdgeInsets.symmetric(
                              horizontal: 20,
                              vertical: 16,
                            ),
                            shape: RoundedRectangleBorder(
                              borderRadius: BorderRadius.circular(20),
                            ),
                          ),
                          label: const Text('Pause'),
                        ),
                    ],
                    const SizedBox(height: 12),
                    TextButton.icon(
                      onPressed:
                          c.allowed(
                                'POST',
                                '${widget.resource.detail}/archive',
                              ) &&
                              error == null
                          ? () => _status('archive')
                          : null,
                      icon: Icon(
                        Icons.archive_outlined,
                        color: KarryamColors.error.forInk(context),
                      ),
                      label: Text(
                        'Archive ${widget.resource.singular}',
                        style: TextStyle(
                          color: KarryamColors.error.forInk(context),
                        ),
                      ),
                    ),
                  ],
                ],
              ),
      ),
    ),
  );
  Widget _detailFact(String title, String value, IconData icon) => Padding(
    padding: const EdgeInsets.only(top: 18),
    child: ListTile(
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(22)),
      tileColor: Colors.white.forSurface(context),
      leading: Icon(icon, color: KarryamColors.tealInk.forInk(context)),
      title: Text(title),
      subtitle: Text(value),
    ),
  );
}

Widget _catalogueNotice(
  BuildContext context,
  String message,
  VoidCallback refresh,
) => Container(
  margin: const EdgeInsets.symmetric(vertical: 14),
  padding: const EdgeInsets.all(16),
  decoration: BoxDecoration(
    color: const Color(0xFFFFF2D9).forSurface(context),
    borderRadius: BorderRadius.circular(20),
  ),
  child: Column(
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      Text(
        message,
        style: TextStyle(color: const Color(0xFF996114).forInk(context)),
      ),
      TextButton(onPressed: refresh, child: const Text('Refresh')),
    ],
  ),
);

class CatalogueBanner extends StatelessWidget {
  const CatalogueBanner({
    super.key,
    required this.kind,
    required this.title,
    required this.message,
    this.status,
    this.mockPhotoId,
    this.photo,
  });
  final String? mockPhotoId;
  final Widget? photo;
  final CatalogueKind kind;
  final String title, message;
  final Object? status;
  @override
  Widget build(BuildContext context) => TweenAnimationBuilder<double>(
    tween: Tween(
      begin: MediaQuery.disableAnimationsOf(context) ? 1 : 0,
      end: 1,
    ),
    duration: MediaQuery.disableAnimationsOf(context)
        ? Duration.zero
        : const Duration(milliseconds: 380),
    curve: Curves.easeOutCubic,
    builder: (_, value, child) => Opacity(
      opacity: value,
      child: Transform.translate(
        offset: Offset(0, 12 * (1 - value)),
        child: child,
      ),
    ),
    child: Container(
      padding: const EdgeInsets.all(24),
      decoration: BoxDecoration(
        gradient: LinearGradient(
          colors:
              (kind.index <= CatalogueKind.modifiers.index
                      ? const [Color(0xFFFFF2DA), Color(0xFFFFFBF3)]
                      : const [Color(0xFFE0F7F0), Color(0xFFF1FCF9)])
                  .map((color) => color.forSurface(context))
                  .toList(),
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
        ),
        borderRadius: BorderRadius.circular(30),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              if (photo != null)
                SizedBox(
                  width: 104,
                  height: 104,
                  child: ClipRRect(
                    borderRadius: BorderRadius.circular(24),
                    child: photo,
                  ),
                )
              else if (mockPhotoId != null)
                MockCataloguePhoto(name: title, id: mockPhotoId!, size: 104)
              else
                CatalogueArt(food: kind.index <= CatalogueKind.modifiers.index),
              const Spacer(),
              if (status != null)
                Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 12,
                    vertical: 7,
                  ),
                  decoration: BoxDecoration(
                    color: (catalogueColor(
                      status,
                    ).withValues(alpha: .09)).forSurface(context),
                    borderRadius: BorderRadius.circular(20),
                  ),
                  child: Text(
                    dashboardLabel(status),
                    style: TextStyle(
                      color: (catalogueColor(status)).forInk(context),
                      fontWeight: FontWeight.w700,
                      fontSize: 12,
                    ),
                  ),
                ),
            ],
          ),
          const SizedBox(height: 26),
          Text(
            title,
            style: const TextStyle(
              fontSize: 29,
              height: 1.15,
              letterSpacing: -1,
              fontWeight: FontWeight.w700,
            ),
          ),
          if (message.isNotEmpty) ...[
            const SizedBox(height: 12),
            Text(
              message,
              style: TextStyle(
                height: 1.6,
                color: KarryamColors.indigo.forInk(context),
              ),
            ),
          ],
        ],
      ),
    ),
  );
}

class CatalogueEntry extends StatelessWidget {
  const CatalogueEntry({
    super.key,
    required this.row,
    required this.kind,
    required this.currency,
    required this.onTap,
    this.actions = const [],
    this.displayName,
    this.mockPhotos = false,
  });
  final JsonObject row;
  final CatalogueKind kind;
  final String currency;
  final VoidCallback onTap;
  final List<Widget> actions;
  final String? displayName;
  final bool mockPhotos;
  @override
  Widget build(BuildContext context) {
    final price =
        row['base_price_minor'] ??
        row['retail_price_minor'] ??
        row['price_delta_minor'] ??
        row['price_override_minor'];
    return Padding(
      padding: const EdgeInsets.only(top: 12),
      child: Material(
        color: Colors.white.forSurface(context),
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(24),
          side: BorderSide(color: Color(0xFFE4EDE9).forOutline(context)),
        ),
        clipBehavior: Clip.antiAlias,
        child: Column(
          children: [
            InkWell(
              onTap: onTap,
              child: Padding(
                padding: const EdgeInsets.all(18),
                child: Row(
                  children: [
                    if (mockPhotos &&
                        [
                          CatalogueKind.items,
                          CatalogueKind.products,
                          CatalogueKind.shopVariants,
                          CatalogueKind.listings,
                          CatalogueKind.menus,
                        ].contains(kind))
                      MockCataloguePhoto(
                        name: displayName ?? row['name']?.toString() ?? '',
                        id: row['id'].toString(),
                      )
                    else
                      Container(
                        width: 52,
                        height: 58,
                        decoration: BoxDecoration(
                          color:
                              (kind.index <= CatalogueKind.modifiers.index
                                      ? const Color(0xFFFFF3DD)
                                      : KarryamColors.mint)
                                  .forSurface(context),
                          borderRadius: BorderRadius.circular(18),
                        ),
                        child: Icon(
                          catalogueIcon(kind),
                          color:
                              (kind.index <= CatalogueKind.modifiers.index
                                      ? const Color(0xFFAC6F1C)
                                      : KarryamColors.tealInk)
                                  .forInk(context),
                          size: 28,
                        ),
                      ),
                    const SizedBox(width: 14),
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(
                            displayName ??
                                row['name']?.toString() ??
                                'Product listing',
                            style: const TextStyle(
                              fontSize: 16,
                              fontWeight: FontWeight.w700,
                              letterSpacing: -.3,
                            ),
                          ),
                          const SizedBox(height: 6),
                          Text(
                            dashboardLabel(row['status']),
                            style: TextStyle(
                              fontSize: 12,
                              color: (catalogueColor(
                                row['status'],
                              )).forInk(context),
                              fontWeight: FontWeight.w600,
                            ),
                          ),
                          if (price is int)
                            Padding(
                              padding: const EdgeInsets.only(top: 6),
                              child: Text(
                                dashboardMoney(
                                  price,
                                  row['currency'] ?? currency,
                                ),
                                style: const TextStyle(
                                  fontWeight: FontWeight.w600,
                                ),
                              ),
                            ),
                        ],
                      ),
                    ),
                    const SizedBox(width: 8),
                    Icon(
                      Icons.arrow_forward_rounded,
                      size: 20,
                      color: KarryamColors.tealInk.forInk(context),
                    ),
                  ],
                ),
              ),
            ),
            if (actions.isNotEmpty)
              Container(
                decoration: BoxDecoration(
                  border: Border(
                    top: BorderSide(
                      color: Color(0xFFF0F4F2).forOutline(context),
                    ),
                  ),
                ),
                padding: const EdgeInsets.symmetric(
                  horizontal: 12,
                  vertical: 2,
                ),
                child: Row(children: [...actions]),
              ),
          ],
        ),
      ),
    );
  }
}
