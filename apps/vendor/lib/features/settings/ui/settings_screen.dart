import '../../access/ui/current_access_screen.dart';
import '../../media/ui/vendor_media_screen.dart';
import '../../pastry/ui/pastry_workspace_screen.dart';
import '../../cloud_kitchen/ui/cloud_kitchen_workspace_screen.dart';
import '../../crm/ui/crm_workspace_screen.dart';
import '../../operational_cost/ui/cost_workspace_screen.dart';
import '../../production/ui/production_operations_screen.dart';
import '../../production_batches/ui/production_batches_workspace_screen.dart';
import '../../recipes/ui/recipes_workspace_screen.dart';
import '../../production_master/ui/production_materials_screen.dart';
import '../../purchasing/ui/purchasing_workspace_screen.dart';
import '../../delivery/ui/rider_workspace_screen.dart';
import '../../promotions/ui/promotions_workspace_screen.dart';
import '../../delivery/ui/vendor_missions_screen.dart';
import '../../../core/theme/vendor_appearance.dart';
import '../../../core/ui/premium_loading.dart';
import 'dart:async';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../dashboard/ui/dashboard_empty_state.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../onboarding/ui/setup_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../state/settings_controller.dart';
import 'settings_design.dart';
import 'settings_editor.dart';
import 'branch_settings_screen.dart';
import 'team_settings_screen.dart';
import 'role_settings_screen.dart';

class SettingsHub extends StatelessWidget {
  const SettingsHub({
    super.key,
    required this.runtime,
    required this.business,
    required this.onSetup,
    required this.onBranches,
    required this.onSettings,
    required this.onSignOut,
    required this.onChanged,
    this.branchName,
  });
  final VendorRuntime runtime;
  final JsonObject business;
  final VoidCallback onSetup, onBranches, onSettings, onSignOut;
  final Future<void> Function() onChanged;
  final String? branchName;
  Future<void> _open(BuildContext context, SettingsKind kind) async {
    await Navigator.push(
      context,
      setupRoute<void>(SettingsCollectionScreen(runtime: runtime, kind: kind)),
    );
    await onChanged();
  }

  @override
  Widget build(BuildContext context) => Column(
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      SettingsLink(
        'My access',
        Icons.verified_user_outlined,
        color: KarryamColors.tealInk.forInk(context),
        onTap: () => Navigator.push(
          context,
          setupRoute<void>(CurrentAccessScreen(runtime: runtime)),
        ),
      ),
      const SizedBox(height: 16),
      const SettingsHeading('Business settings'),
      SettingsCard(
        color: Color(0xFFE8F4EF).forInk(context),
        child: Row(
          children: [
            const SettingsEmblem(SettingsKind.profile, size: 76),
            const SizedBox(width: 16),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    business['trading_name']?.toString().isNotEmpty == true
                        ? business['trading_name'].toString()
                        : business['legal_name']?.toString() ?? 'Your business',
                    style: const TextStyle(
                      fontSize: 20,
                      fontWeight: FontWeight.w700,
                      letterSpacing: -.5,
                    ),
                  ),
                  const SizedBox(height: 9),
                  if (business['status'] != null)
                    SettingsStatus(business['status']),
                ],
              ),
            ),
          ],
        ),
      ),
      const SizedBox(height: 24),
      for (final kind in [
        SettingsKind.profile,
        SettingsKind.brands,
        SettingsKind.branches,
        SettingsKind.capabilities,
        SettingsKind.staff,
        SettingsKind.roles,
      ])
        SettingsLink(
          switch (kind) {
            SettingsKind.profile => 'Business profile',
            SettingsKind.brands => 'Brands',
            SettingsKind.branches => 'Branches',
            SettingsKind.capabilities => 'Business types',
            SettingsKind.staff => 'Team',
            SettingsKind.roles => 'Roles & permissions',
          },
          settingsIcon(kind),
          color: (settingsAccent(kind)).forInk(context),
          onTap: () => _open(context, kind),
        ),
      const SizedBox(height: 12),
      SettingsLink(
        'Customers & relationships',
        Icons.people_outline_rounded,
        onTap: () =>
            Navigator.push(context, setupRoute<void>(CrmHub(runtime: runtime))),
      ),
      SettingsLink(
        'Pastry & bakery',
        Icons.bakery_dining_outlined,
        onTap: () => Navigator.push(
          context,
          setupRoute<void>(PastryHub(runtime: runtime)),
        ),
      ),
      SettingsLink(
        'Cloud kitchen',
        Icons.soup_kitchen_outlined,
        onTap: () => Navigator.push(
          context,
          setupRoute<void>(CloudKitchenHub(runtime: runtime)),
        ),
      ),
      SettingsLink(
        'Production materials',
        Icons.restaurant_outlined,
        onTap: () => Navigator.push(
          context,
          setupRoute<void>(ProductionMaterialsHub(runtime: runtime)),
        ),
      ),
      SettingsLink(
        'Production operations',
        Icons.precision_manufacturing_outlined,
        onTap: () => Navigator.push(
          context,
          setupRoute<void>(ProductionOperationsScreen(runtime: runtime)),
        ),
      ),
      SettingsLink(
        'Production batches',
        Icons.soup_kitchen_outlined,
        onTap: () => Navigator.push(
          context,
          setupRoute<void>(ProductionBatchesScreen(runtime: runtime)),
        ),
      ),
      SettingsLink(
        'Production costs',
        Icons.calculate_outlined,
        onTap: () => Navigator.push(
          context,
          setupRoute<void>(CostWorkspaceScreen(runtime: runtime)),
        ),
      ),
      SettingsLink(
        'Recipes',
        Icons.menu_book_outlined,
        onTap: () => Navigator.push(
          context,
          setupRoute<void>(RecipesWorkspaceScreen(runtime: runtime)),
        ),
      ),
      SettingsLink(
        'Purchasing',
        Icons.inventory_2_outlined,
        onTap: () => Navigator.push(
          context,
          setupRoute<void>(PurchasingHub(runtime: runtime)),
        ),
      ),
      SettingsLink(
        'Promotions',
        Icons.confirmation_number_outlined,
        onTap: () => Navigator.push(
          context,
          setupRoute<void>(PromotionsWorkspaceScreen(runtime: runtime)),
        ),
      ),
      SettingsLink(
        'Vendor deliveries',
        Icons.local_shipping_outlined,
        onTap: () => Navigator.push(
          context,
          setupRoute<void>(VendorMissionsScreen(runtime: runtime)),
        ),
      ),
      SettingsLink(
        'My rider workspace',
        Icons.delivery_dining_rounded,
        onTap: () => Navigator.push(
          context,
          setupRoute<void>(RiderWorkspaceScreen(runtime: runtime)),
        ),
      ),
      SettingsLink(
        'Business setup',
        Icons.verified_user_outlined,
        subtitle: 'Verification & bank details',
        onTap: onSetup,
      ),
      SettingsLink(
        'Switch branch',
        Icons.swap_horiz_rounded,
        subtitle: branchName,
        onTap: onBranches,
      ),
      if (AppearanceScope.maybeOf(context) != null)
        SettingsLink(
          'Appearance',
          Icons.palette_outlined,
          subtitle: AppearanceScope.maybeOf(context)!.mode.name,
          onTap: () => showAppearance(context),
        ),
      SettingsLink(
        'Data source',
        Icons.tune_rounded,
        subtitle: runtime.mode == VendorDataMode.mock ? 'Mock' : 'Live',
        onTap: onSettings,
      ),
      const SizedBox(height: 8),
      SettingsLink(
        'Sign out',
        Icons.logout_rounded,
        color: KarryamColors.error.forInk(context),
        onTap: onSignOut,
      ),
    ],
  );
}

class SettingsCollectionScreen extends StatefulWidget {
  const SettingsCollectionScreen({
    super.key,
    required this.runtime,
    required this.kind,
  });
  final VendorRuntime runtime;
  final SettingsKind kind;
  @override
  State<SettingsCollectionScreen> createState() =>
      _SettingsCollectionScreenState();
}

class _SettingsCollectionScreenState extends State<SettingsCollectionScreen> {
  late final c = SettingsController(widget.runtime.backend, widget.kind);
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) unawaited(c.load());
    });
  }

  @override
  void dispose() {
    c.dispose();
    super.dispose();
  }

  Future<void> _open(JsonObject row) async {
    final screen = switch (widget.kind) {
      SettingsKind.branches => BranchSettingsScreen(
        runtime: widget.runtime,
        record: row,
      ),
      SettingsKind.staff => StaffSettingsScreen(
        runtime: widget.runtime,
        record: row,
      ),
      SettingsKind.roles => RoleSettingsScreen(
        runtime: widget.runtime,
        record: row,
      ),
      _ => SettingsRecordScreen(
        runtime: widget.runtime,
        kind: widget.kind,
        record: row,
      ),
    };
    await Navigator.push(context, setupRoute<void>(screen));
    await c.load();
  }

  Future<void> _create() async {
    if (widget.kind == SettingsKind.roles) {
      await Navigator.push(
        context,
        setupRoute<bool>(RolePermissionEditor(runtime: widget.runtime)),
      );
      await c.load();
      return;
    }
    if (widget.kind == SettingsKind.capabilities) {
      await _capability();
      return;
    }
    try {
      final brands = widget.kind == SettingsKind.branches
          ? await c.all('${SettingsController.business}/brands', c.scope)
          : <JsonObject>[];
      final branches = widget.kind == SettingsKind.staff
          ? await c.all('${SettingsController.business}/branches', c.scope)
          : <JsonObject>[];
      final roles = widget.kind == SettingsKind.staff
          ? SettingsController.objects(
              (await c.read('/api/v1/vendor/roles')).data,
              'roles',
            )
          : <JsonObject>[];
      if (!mounted) return;
      final saved = await Navigator.push<bool>(
        context,
        setupRoute<bool>(
          SettingsEditor(
            runtime: widget.runtime,
            kind: widget.kind,
            brands: brands,
            branches: branches,
            roles: roles,
            onSave: (body) =>
                c.mutate('POST', c.collection, body, parameters: c.params()),
            onReload: () async {
              await c.load();
              return null;
            },
            canSave: () => c.allowed('POST', c.collection),
            errorMessage: () => c.feedback ?? c.error,
          ),
        ),
      );
      if (saved == true && mounted) {
        await showOrderFeedback(
          context,
          title: widget.kind == SettingsKind.staff
              ? 'Staff member added'
              : '${widget.kind == SettingsKind.branches ? 'Branch' : 'Brand'} created',
          message:
              c.feedback ??
              (widget.kind == SettingsKind.staff
                  ? 'They can sign in with the contact you entered.'
                  : widget.kind == SettingsKind.branches
                  ? 'Your branch starts as Draft.'
                  : 'Your brand details are saved.'),
          success: true,
        );
      }
    } catch (e) {
      if (mounted) {
        await showOrderFeedback(
          context,
          title: 'Could not open this form',
          message: SettingsController.message(e),
        );
      }
    }
  }

  Future<void> _capability() async {
    try {
      final catalogue = SettingsController.objects(
        (await c.read('/api/v1/vendor/capability-types')).data,
      );
      final existing = await c.all(c.collection, c.params());
      final options = catalogue
          .where((t) => !existing.any((e) => e['capability_type'] == t['type']))
          .toList();
      if (!mounted) return;
      if (options.isEmpty) {
        await showOrderFeedback(
          context,
          title: 'All business types are added',
          message: 'No additional type is available.',
          warning: true,
        );
        return;
      }
      final type = await settingsSheet<String>(
        context,
        Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const SettingsHeading('Add business type'),
            for (final value in options)
              SettingsLink(
                dashboardLabel(value['type']),
                Icons.widgets_outlined,
                subtitle: dashboardLabel(value['family']),
                onTap: () => Navigator.pop(context, value['type']),
              ),
          ],
        ),
      );
      if (type == null || !mounted) return;
      final saved = await c.mutate('POST', c.collection, {
        'capability_type': type,
      }, parameters: c.params());
      if (mounted) {
        await showOrderFeedback(
          context,
          title: saved ? 'Business type added' : 'Could not add this type',
          message: saved
              ? 'The new type starts as Draft.'
              : c.feedback ?? c.error ?? 'Try again.',
          success: saved,
        );
      }
    } catch (e) {
      if (mounted) {
        await showOrderFeedback(
          context,
          title: 'Business types could not load',
          message: SettingsController.message(e),
        );
      }
    }
  }

  String _name(JsonObject row) => switch (widget.kind) {
    SettingsKind.profile =>
      row['trading_name']?.toString().isNotEmpty == true
          ? row['trading_name'].toString()
          : row['legal_name'].toString(),
    SettingsKind.capabilities => dashboardLabel(row['capability_type']),
    SettingsKind.roles => row['name'].toString(),
    SettingsKind.staff =>
      row['id'] == widget.runtime.auth.session?.staffId
          ? 'Your staff access'
          : 'Staff member',
    _ => row['display_name'].toString(),
  };
  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(
      title: Text(c.title),
      backgroundColor: KarryamColors.canvas.forSurface(context),
    ),
    bottomNavigationBar: widget.kind == SettingsKind.profile
        ? null
        : AnimatedBuilder(
            animation: c,
            builder: (context, _) => SafeArea(
              child: Padding(
                padding: const EdgeInsets.fromLTRB(24, 12, 24, 16),
                child: SettingsAction(
                  widget.kind == SettingsKind.staff
                      ? 'Add staff member'
                      : widget.kind == SettingsKind.capabilities
                      ? 'Add business type'
                      : widget.kind == SettingsKind.roles
                      ? 'Create role'
                      : widget.kind == SettingsKind.brands
                      ? 'Create brand'
                      : 'Create branch',
                  icon: Icons.add_rounded,
                  onPressed: c.allowed('POST', c.collection) ? _create : null,
                ),
              ),
            ),
          ),
    body: AnimatedBuilder(
      animation: c,
      builder: (context, _) => RefreshIndicator(
        onRefresh: c.load,
        child: ListView(
          physics: const AlwaysScrollableScrollPhysics(),
          padding: const EdgeInsets.all(24),
          children: [
            Row(
              children: [
                SettingsEmblem(widget.kind, size: 76),
                const SizedBox(width: 18),
                Expanded(
                  child: Text(
                    c.title,
                    style: const TextStyle(
                      fontSize: 29,
                      fontWeight: FontWeight.w700,
                      letterSpacing: -1,
                    ),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 26),
            if (c.loading)
              c.rows.isEmpty
                  ? const PremiumSkeleton(rows: 2)
                  : const ContentRefresh(),
            if (c.error != null) SettingsNote(c.error!, warning: c.restricted),
            if (c.stale && !c.loading)
              SettingsAction(
                'Refresh',
                secondary: true,
                icon: Icons.refresh_rounded,
                onPressed: c.load,
              ),
            if (!c.loading && c.error == null && c.rows.isEmpty)
              DashboardEmptyState(
                story: EmptyStory.setup,
                title: widget.kind == SettingsKind.staff
                    ? 'No staff records yet.'
                    : widget.kind == SettingsKind.brands
                    ? 'No brands yet.'
                    : widget.kind == SettingsKind.branches
                    ? 'No branches yet.'
                    : widget.kind == SettingsKind.roles
                    ? 'No roles available.'
                    : 'No business types yet.',
                message: widget.kind == SettingsKind.roles
                    ? 'Roles will appear when the server returns them.'
                    : 'Use the action below to add your first record.',
              ),
            for (final row in c.rows)
              Padding(
                padding: const EdgeInsets.only(bottom: 14),
                child: Material(
                  color: Colors.white.forSurface(context),
                  borderRadius: BorderRadius.circular(26),
                  clipBehavior: Clip.antiAlias,
                  child: InkWell(
                    onTap: c.loading ? null : () => _open(row),
                    child: Padding(
                      padding: const EdgeInsets.all(22),
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Row(
                            children: [
                              Container(
                                padding: const EdgeInsets.all(12),
                                decoration: BoxDecoration(
                                  color: (settingsAccent(
                                    widget.kind,
                                  ).withValues(alpha: .08)).forSurface(context),
                                  borderRadius: BorderRadius.circular(16),
                                ),
                                child: Icon(
                                  settingsIcon(widget.kind),
                                  color: (settingsAccent(
                                    widget.kind,
                                  )).forInk(context),
                                  size: 25,
                                ),
                              ),
                              const Spacer(),
                              Flexible(
                                child: widget.kind == SettingsKind.roles
                                    ? Text(
                                        row['is_system'] == true
                                            ? 'System role'
                                            : 'Custom role',
                                        style: TextStyle(
                                          fontSize: 11,
                                          color: (settingsAccent(
                                            widget.kind,
                                          )).forInk(context),
                                          fontWeight: FontWeight.w700,
                                        ),
                                      )
                                    : SettingsStatus(
                                        row['status'] ??
                                            row['verification_state'],
                                      ),
                              ),
                            ],
                          ),
                          const SizedBox(height: 18),
                          Text(
                            _name(row),
                            style: const TextStyle(
                              fontSize: 22,
                              fontWeight: FontWeight.w700,
                              letterSpacing: -.6,
                            ),
                          ),
                          const SizedBox(height: 8),
                          Row(
                            children: [
                              Expanded(
                                child: Text(
                                  widget.kind == SettingsKind.staff
                                      ? row['id'].toString()
                                      : widget.kind == SettingsKind.roles
                                      ? row['code'].toString()
                                      : widget.kind == SettingsKind.branches
                                      ? jsonObject(
                                              row['address'] ?? {},
                                            )['city']?.toString() ??
                                            ''
                                      : widget.kind == SettingsKind.brands
                                      ? row['description']?.toString() ?? ''
                                      : 'View details',
                                  style: const TextStyle(fontSize: 12),
                                ),
                              ),
                              Icon(
                                Icons.arrow_forward_rounded,
                                size: 19,
                                color: KarryamColors.tealInk.forInk(context),
                              ),
                            ],
                          ),
                        ],
                      ),
                    ),
                  ),
                ),
              ),
            if (c.hasMore)
              SettingsAction(
                'Load more',
                secondary: true,
                busy: c.loading,
                onPressed: () => c.load(more: true),
              ),
          ],
        ),
      ),
    ),
  );
}

class SettingsRecordScreen extends StatefulWidget {
  const SettingsRecordScreen({
    super.key,
    required this.runtime,
    required this.kind,
    required this.record,
  });
  final VendorRuntime runtime;
  final SettingsKind kind;
  final JsonObject record;
  @override
  State<SettingsRecordScreen> createState() => _SettingsRecordScreenState();
}

class _SettingsRecordScreenState extends State<SettingsRecordScreen> {
  late final c = SettingsController(widget.runtime.backend, widget.kind);
  JsonObject? _record;
  String? _error;
  bool _loading = false;
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) unawaited(_load());
    });
  }

  @override
  void dispose() {
    c.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    await c.load();
    try {
      final record = await c.readRecord(widget.record);
      if (mounted) setState(() => _record = record);
    } catch (e) {
      if (mounted) setState(() => _error = SettingsController.message(e));
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _edit() async {
    final row = _record!;
    final saved = await Navigator.push<bool>(
      context,
      setupRoute<bool>(
        SettingsEditor(
          runtime: widget.runtime,
          kind: widget.kind,
          record: row,
          onSave: (body) =>
              c.mutate('PATCH', c.detail, body, parameters: c.params(row)),
          onReload: () async {
            await c.load();
            return c.readRecord(row);
          },
          canSave: () => c.allowed('PATCH', c.detail),
          errorMessage: () => c.feedback ?? c.error,
        ),
      ),
    );
    if (saved == true) {
      await _load();
      if (mounted) {
        await showOrderFeedback(
          context,
          title: 'Details saved',
          message: widget.kind == SettingsKind.profile
              ? 'Review any updated verification requirements in Business setup.'
              : 'Your brand details are updated.',
          success: true,
        );
      }
    }
  }

  Future<void> _media() async {
    if (_loading || _record == null) return;
    try {
      final fresh = await c.readRecord(_record!);
      if (!mounted) return;
      final result = await vendorMedia(
        context,
        runtime: widget.runtime,
        targetType: 'BRAND',
        targetRef: fresh['id'].toString(),
        entries: (fresh['media_refs'] as List? ?? []).map(jsonObject).toList(),
        save: (media) async {
          final latest = await c.readRecord(fresh);
          if (latest['version'] != fresh['version']) {
            throw const VendorContractException(
              'This brand changed. Reopen images before saving.',
            );
          }
          return MediaSave(
            c.api(c.detail),
            'PATCH',
            c.detail,
            c.params(latest),
            {
              'display_name': latest['display_name'],
              'description': latest['description'] ?? '',
              'media_refs': media,
              'expected_version': latest['version'],
            },
          );
        },
      );
      if (result != null && mounted) await _load();
    } catch (e) {
      if (mounted) {
        await showOrderFeedback(
          context,
          title: 'Images unavailable',
          message: SettingsController.message(e),
        );
      }
    }
  }

  Future<void> _status() async {
    final row = _record!,
        pause = row['status'] == 'ACTIVE',
        action = pause ? 'pause' : 'resume',
        path = '${c.detail}/$action';
    if (!await confirmSettings(
          context,
          pause
              ? 'Pause ${widget.kind == SettingsKind.profile ? 'business' : 'business type'}'
              : 'Resume ${widget.kind == SettingsKind.profile ? 'business' : 'business type'}',
          pause
              ? 'This changes availability. Existing records remain available.'
              : 'Resume this paused record?',
          danger: pause,
        ) ||
        !mounted) {
      return;
    }
    final saved = await c.mutate('POST', path, {
      'expected_version': row['version'],
    }, parameters: c.params(row));
    if (saved) await _load();
    if (mounted) {
      await showOrderFeedback(
        context,
        title: saved
            ? '${pause ? 'Paused' : 'Resumed'} successfully'
            : 'Change not accepted',
        message: saved
            ? 'The status has been updated.'
            : c.feedback ?? c.error ?? 'Try again.',
        success: saved,
        warning: pause && saved,
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    final row = _record;
    final status = row?['status'];
    return Scaffold(
      appBar: AppBar(
        title: Text(c.title),
        backgroundColor: KarryamColors.canvas.forSurface(context),
      ),
      body: AnimatedBuilder(
        animation: c,
        builder: (context, _) => RefreshIndicator(
          onRefresh: _load,
          child: ListView(
            padding: const EdgeInsets.all(24),
            physics: const AlwaysScrollableScrollPhysics(),
            children: [
              if (_loading) const PremiumSkeleton(rows: 2),
              if (_error != null) SettingsNote(_error!),
              if (row != null) ...[
                SettingsCard(
                  color: (settingsAccent(
                    widget.kind,
                  ).withValues(alpha: .07)).forInk(context),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Row(
                        children: [
                          if (widget.kind == SettingsKind.brands &&
                              (row['media_refs'] as List? ?? []).isNotEmpty)
                            SizedBox(
                              width: 76,
                              height: 76,
                              child: ClipRRect(
                                borderRadius: BorderRadius.circular(22),
                                child: MediaPreview(
                                  runtime: widget.runtime,
                                  targetType: 'BRAND',
                                  targetRef: row['id'].toString(),
                                  ref:
                                      ((row['media_refs'] as List)
                                                  .map(jsonObject)
                                                  .where(
                                                    (e) => e['kind'] == 'LOGO',
                                                  )
                                                  .firstOrNull ??
                                              jsonObject(
                                                (row['media_refs'] as List)
                                                    .first,
                                              ))['media_ref']
                                          .toString(),
                                ),
                              ),
                            )
                          else
                            SettingsEmblem(widget.kind, size: 76),
                          const Spacer(),
                          if (status != null ||
                              row['verification_state'] != null)
                            Flexible(
                              child: SettingsStatus(
                                status ?? row['verification_state'],
                              ),
                            ),
                        ],
                      ),
                      const SizedBox(height: 22),
                      Text(
                        widget.kind == SettingsKind.profile
                            ? row['legal_name'].toString()
                            : widget.kind == SettingsKind.brands
                            ? row['display_name'].toString()
                            : dashboardLabel(row['capability_type']),
                        style: const TextStyle(
                          fontSize: 28,
                          fontWeight: FontWeight.w700,
                          letterSpacing: -1,
                        ),
                      ),
                    ],
                  ),
                ),
                const SizedBox(height: 18),
                SettingsCard(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: widget.kind == SettingsKind.profile
                        ? [
                            SettingsFact('Trading name', row['trading_name']),
                            SettingsFact('Support email', row['support_email']),
                            SettingsFact('Support phone', row['support_phone']),
                            SettingsFact('Currency', row['currency']),
                            SettingsFact('Timezone', row['timezone']),
                          ]
                        : widget.kind == SettingsKind.brands
                        ? [
                            SettingsFact('Description', row['description']),
                            for (final media
                                in (row['media_refs'] as List? ?? []).map(
                                  jsonObject,
                                ))
                              SettingsFact(
                                dashboardLabel(media['kind']),
                                media['media_ref'],
                              ),
                          ]
                        : [
                            SettingsFact('Status', dashboardLabel(status)),
                            if (row['status_reason']?.toString().isNotEmpty ==
                                true)
                              SettingsFact('Reason', row['status_reason']),
                          ],
                  ),
                ),
                const SizedBox(height: 24),
                if (widget.kind == SettingsKind.brands) ...[
                  SettingsAction(
                    'Manage brand images',
                    icon: Icons.photo_library_outlined,
                    secondary: true,
                    onPressed:
                        !_loading &&
                            _error == null &&
                            c.allowed('PATCH', c.detail)
                        ? _media
                        : null,
                  ),
                  const SizedBox(height: 12),
                ],
                if (widget.kind != SettingsKind.capabilities)
                  SettingsAction(
                    'Edit details',
                    icon: Icons.edit_outlined,
                    onPressed:
                        !_loading &&
                            _error == null &&
                            c.allowed('PATCH', c.detail)
                        ? _edit
                        : null,
                  ),
                const SizedBox(height: 12),
                if ({'ACTIVE', 'PAUSED'}.contains(status))
                  SettingsAction(
                    status == 'ACTIVE' ? 'Pause' : 'Resume',
                    secondary: status == 'PAUSED',
                    danger: status == 'ACTIVE',
                    icon: status == 'ACTIVE'
                        ? Icons.pause_rounded
                        : Icons.play_arrow_rounded,
                    busy: c.busy,
                    onPressed:
                        !_loading &&
                            _error == null &&
                            c.allowed(
                              'POST',
                              '${c.detail}/${status == 'ACTIVE' ? 'pause' : 'resume'}',
                            )
                        ? _status
                        : null,
                  ),
                if (widget.kind == SettingsKind.capabilities &&
                    status == 'DRAFT')
                  const SettingsNote(
                    'This business type is Draft. Public settings cannot activate it.',
                    warning: true,
                  ),
              ],
              if (_error != null)
                SettingsAction(
                  'Refresh',
                  secondary: true,
                  icon: Icons.refresh_rounded,
                  onPressed: _load,
                ),
            ],
          ),
        ),
      ),
    );
  }
}
