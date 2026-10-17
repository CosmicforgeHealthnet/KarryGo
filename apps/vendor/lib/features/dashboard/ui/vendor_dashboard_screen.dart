import '../../scheduled_commerce/ui/scheduling_workspace_screen.dart';
import '../../catalogue/state/catalogue_controller.dart';
import '../../delivery/ui/vendor_missions_screen.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../../catalogue/ui/mock_catalogue_photo.dart';
import '../../../core/ui/premium_loading.dart';
import '../../settings/state/settings_controller.dart';
import '../../settings/ui/settings_design.dart';
import '../../settings/ui/settings_screen.dart';
import 'dart:async';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/widgets/vendor_widgets.dart';
import '../../onboarding/ui/business_onboarding_screen.dart';
import '../../onboarding/ui/setup_design.dart';
import '../state/dashboard_controller.dart';
import 'dashboard_design.dart';
import 'dashboard_catalogue_banner.dart';
import '../../orders/ui/order_detail_screen.dart';
import '../../orders/ui/order_design.dart';
import 'dashboard_empty_state.dart';
import 'dashboard_format.dart';
export 'dashboard_format.dart';
import '../../catalogue/ui/catalogue_screen.dart';
import '../../catalogue/state/catalogue_resource.dart';
import '../../finance/state/finance_workspace_controller.dart';
import '../../finance/state/withdrawal_attempt_store.dart';
import '../../finance/ui/finance_panel.dart';
import '../../inventory/ui/stock_screen.dart';

class VendorDashboardScreen extends StatefulWidget {
  const VendorDashboardScreen({
    super.key,
    required this.runtime,
    required this.onSettings,
  });
  final VendorRuntime runtime;
  final VoidCallback onSettings;
  @override
  State<VendorDashboardScreen> createState() => _VendorDashboardScreenState();
}

class _VendorDashboardScreenState extends State<VendorDashboardScreen>
    with SingleTickerProviderStateMixin {
  late final c = DashboardController(widget.runtime.backend);
  late final finance = FinanceWorkspaceController(
    widget.runtime.backend,
    attempts: widget.runtime.mode == VendorDataMode.mock
        ? MemoryWithdrawalAttemptStore()
        : SecureWithdrawalAttemptStore(
            Uri.parse(widget.runtime.liveOrigin).origin,
            widget.runtime.backend.scope.vendorId!,
          ),
  );
  late final _tabMotion = AnimationController(
    vsync: this,
    value: 1,
    duration: const Duration(milliseconds: 220),
  );
  int _tab = 0;
  String _filter = 'All';
  String _catalogueType = 'Food';
  @override
  void initState() {
    super.initState();
    unawaited(c.load());
  }

  @override
  void dispose() {
    _tabMotion.dispose();
    c.dispose();
    finance.dispose();
    super.dispose();
  }

  void _select(int tab) {
    if (_tab == tab) return;
    setState(() => _tab = tab);
    if (MediaQuery.disableAnimationsOf(context)) {
      _tabMotion.value = 1;
    } else {
      _tabMotion.forward(from: 0);
    }
    if (tab == 3 && !finance.initialized) unawaited(finance.initialize());
    if (tab == 2 && !c.catalogueLoaded) unawaited(c.loadCatalogue());
  }

  Future<void> _refresh() async {
    await c.load();
    if (_tab == 2) await c.loadCatalogue();
  }

  Future<void> _branches() async {
    final id = await showModalBottomSheet<String>(
      context: context,
      useSafeArea: true,
      showDragHandle: true,
      backgroundColor: KarryamColors.canvas.forSurface(context),
      isScrollControlled: true,
      builder: (_) => ConstrainedBox(
        constraints: BoxConstraints(
          maxHeight: MediaQuery.sizeOf(context).height * .7,
        ),
        child: ListView(
          shrinkWrap: true,
          padding: const EdgeInsets.fromLTRB(24, 0, 24, 24),
          children: [
            const Text(
              'Choose your branch.',
              style: TextStyle(
                fontSize: 28,
                letterSpacing: -1,
                fontWeight: FontWeight.w700,
              ),
            ),
            const SizedBox(height: 8),
            const Text('Your order queue follows the branch you select.'),
            const SizedBox(height: 18),
            for (final branch in c.branches)
              ListTile(
                contentPadding: EdgeInsets.zero,
                leading: const Icon(Icons.storefront_outlined),
                title: Text(branch['display_name'].toString()),
                subtitle: Text(dashboardLabel(branch['status'])),
                trailing: branch['id'] == c.branchId
                    ? Icon(
                        Icons.check_circle_rounded,
                        color: KarryamColors.tealInk.forInk(context),
                      )
                    : null,
                onTap: () => Navigator.pop(context, branch['id']),
              ),
          ],
        ),
      ),
    );
    if (id != null && mounted) {
      await c.selectBranch(id);
      if (_tab == 2) await c.loadCatalogue();
    }
  }

  Future<void> _setup() async {
    await Navigator.push(
      context,
      setupRoute<void>(
        BusinessOnboardingScreen(
          runtime: widget.runtime,
          onSettings: widget.onSettings,
        ),
      ),
    );
    if (mounted) await _refresh();
  }

  Future<void> _signOut() async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('Sign out of Karryam?'),
        content: const Text('You can sign back in whenever you need to.'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('Stay here'),
          ),
          TextButton(
            onPressed: () => Navigator.pop(context, true),
            child: Text(
              'Sign out',
              style: TextStyle(color: KarryamColors.error.forInk(context)),
            ),
          ),
        ],
      ),
    );
    if (confirmed == true) await widget.runtime.signOut();
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: Listenable.merge([c, widget.runtime]),
    builder: (context, _) => Scaffold(
      backgroundColor: KarryamColors.canvas.forSurface(context),
      body: SafeArea(
        child: Center(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 600),
            child: Column(
              children: [
                Padding(
                  padding: const EdgeInsets.fromLTRB(22, 14, 14, 8),
                  child: Row(
                    children: [
                      const KarryamMark(size: 32),
                      const SizedBox(width: 9),
                      const Text(
                        'Karryam',
                        style: TextStyle(
                          fontSize: 23,
                          fontWeight: FontWeight.w700,
                          letterSpacing: -.8,
                        ),
                      ),
                      const Spacer(),
                      IconButton(
                        onPressed: _activity,
                        tooltip: 'Order activity',
                        icon: Icon(
                          Icons.notifications_none_rounded,
                          color: KarryamColors.tealInk.forInk(context),
                          size: 24,
                        ),
                      ),
                    ],
                  ),
                ),
                Expanded(
                  child: FadeTransition(
                    opacity: CurvedAnimation(
                      parent: _tabMotion,
                      curve: Curves.easeOut,
                    ),
                    child: IndexedStack(
                      index: _tab,
                      children: [
                        _page('home', _home()),
                        _page('orders', _orders()),
                        _page('catalogue', _catalogue()),
                        FinancePanel(
                          runtime: widget.runtime,
                          controller: finance,
                          onSetup: _setup,
                          onChanged: c.load,
                        ),
                        _page('more', _more()),
                      ],
                    ),
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
      bottomNavigationBar: Container(
        decoration: BoxDecoration(
          border: Border(
            top: BorderSide(color: Color(0xFFE0E6E1).forOutline(context)),
          ),
        ),
        child: NavigationBar(
          height: 76,
          elevation: 0,
          backgroundColor: Colors.white.forSurface(context),
          indicatorColor: Color(0xFFE0F8F1).forSurface(context),
          selectedIndex: _tab,
          onDestinationSelected: _select,
          labelTextStyle: WidgetStateProperty.resolveWith(
            (states) => TextStyle(
              fontSize: 11,
              fontWeight: states.contains(WidgetState.selected)
                  ? FontWeight.w700
                  : FontWeight.w400,
              color:
                  (states.contains(WidgetState.selected)
                          ? KarryamColors.tealInk
                          : const Color(0xFF76858A))
                      .forInk(context),
            ),
          ),
          destinations: [
            NavigationDestination(
              icon: Icon(
                Icons.home_outlined,
                color: Color(0xFF76858A).forInk(context),
              ),
              selectedIcon: Icon(
                Icons.home_rounded,
                color: KarryamColors.tealInk.forInk(context),
              ),
              label: 'Home',
            ),
            NavigationDestination(
              icon: Icon(
                Icons.receipt_long_outlined,
                color: Color(0xFF76858A).forInk(context),
              ),
              selectedIcon: Icon(
                Icons.receipt_long_rounded,
                color: KarryamColors.tealInk.forInk(context),
              ),
              label: 'Orders',
            ),
            NavigationDestination(
              icon: Icon(
                Icons.grid_view_outlined,
                color: Color(0xFF76858A).forInk(context),
              ),
              selectedIcon: Icon(
                Icons.grid_view_rounded,
                color: KarryamColors.tealInk.forInk(context),
              ),
              label: 'Catalogue',
            ),
            NavigationDestination(
              icon: Icon(
                Icons.account_balance_wallet_outlined,
                color: Color(0xFF76858A).forInk(context),
              ),
              selectedIcon: Icon(
                Icons.account_balance_wallet_rounded,
                color: KarryamColors.tealInk.forInk(context),
              ),
              label: 'Finance',
            ),
            NavigationDestination(
              icon: Icon(
                Icons.more_horiz_rounded,
                color: Color(0xFF76858A).forInk(context),
              ),
              selectedIcon: Icon(
                Icons.more_horiz_rounded,
                color: KarryamColors.tealInk.forInk(context),
              ),
              label: 'More',
            ),
          ],
        ),
      ),
    ),
  );
  Widget _page(String key, List<Widget> children) => RefreshIndicator(
    onRefresh: _refresh,
    color: KarryamColors.tealInk.forInk(context),
    child: ListView(
      key: PageStorageKey(key),
      physics: const AlwaysScrollableScrollPhysics(),
      padding: const EdgeInsets.fromLTRB(24, 14, 24, 24),
      children: children,
    ),
  );
  Widget _heading(String title, String caption) => Padding(
    padding: const EdgeInsets.only(bottom: 24),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          title,
          style: const TextStyle(
            fontSize: 30,
            height: 1.08,
            letterSpacing: -1.7,
            fontWeight: FontWeight.w700,
          ),
        ),
        const SizedBox(height: 12),
        Text(
          caption,
          style: TextStyle(
            color: KarryamColors.indigo.forInk(context),
            height: 1.6,
          ),
        ),
      ],
    ),
  );
  Widget _section(String title, {VoidCallback? onTap}) => Padding(
    padding: const EdgeInsets.only(top: 24, bottom: 14),
    child: Row(
      children: [
        Expanded(
          child: Text(
            title,
            style: const TextStyle(
              fontSize: 20,
              letterSpacing: -.5,
              fontWeight: FontWeight.w700,
            ),
          ),
        ),
        if (onTap != null)
          TextButton(onPressed: onTap, child: const Text('View all')),
      ],
    ),
  );
  Widget _notice(
    String title,
    String text,
    IconData icon, {
    bool error = false,
    VoidCallback? retry,
    String? action,
    EmptyStory? story,
  }) => DashboardEmptyState(
    title: title,
    message: text,
    story: story ?? (error ? EmptyStory.unavailable : EmptyStory.orders),
    action: action ?? 'Try again',
    onAction: retry,
  );
  Widget _resource(String key, Widget content) {
    if (c.loading) return const _LoadingBlock();
    if (c.errors[key] != null) {
      return _notice(
        c.restricted.contains(key)
            ? 'Access required'
            : 'Unable to load ${key == 'wallet' ? 'balance' : key}',
        c.errors[key]!,
        Icons.info_outline_rounded,
        error: true,
        retry: _refresh,
      );
    }
    return content;
  }

  List<Widget> _home() => [
    _heading(
      'Business overview',
      c.business['trading_name']?.toString().isNotEmpty == true
          ? c.business['trading_name'].toString()
          : c.business['legal_name']?.toString() ?? 'Your Karryam workspace',
    ),
    if ({
      'PENDING_VERIFICATION',
      'PENDING_REVIEW',
    }.contains(c.business['status']))
      Padding(
        padding: const EdgeInsets.only(bottom: 22),
        child: Material(
          color: Colors.white.forSurface(context),
          borderRadius: BorderRadius.circular(24),
          clipBehavior: Clip.antiAlias,
          child: InkWell(
            onTap: _setup,
            child: Container(
              padding: const EdgeInsets.all(20),
              decoration: BoxDecoration(
                borderRadius: BorderRadius.circular(24),
                border: Border.all(
                  color: const Color(
                    0xFFAE781E,
                  ).forOutline(context).withValues(alpha: .25),
                ),
              ),
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Container(
                    width: 46,
                    height: 46,
                    decoration: BoxDecoration(
                      color: const Color(0xFFAE781E).withValues(alpha: .12),
                      borderRadius: BorderRadius.circular(15),
                    ),
                    child: Icon(
                      Icons.fact_check_outlined,
                      color: const Color(0xFFAE781E).forInk(context),
                    ),
                  ),
                  const SizedBox(width: 14),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        const Text(
                          'Business under review',
                          style: TextStyle(
                            fontSize: 17,
                            fontWeight: FontWeight.w700,
                          ),
                        ),
                        const SizedBox(height: 7),
                        Text(
                          'Prepare your catalogue while we review your business. Customer ordering becomes available after activation.',
                          style: TextStyle(
                            color: KarryamColors.gray.forInk(context),
                            height: 1.5,
                            fontSize: 13,
                          ),
                        ),
                        const SizedBox(height: 12),
                        Row(
                          children: [
                            Text(
                              'Track verification',
                              style: TextStyle(
                                color: KarryamColors.tealInk.forInk(context),
                                fontWeight: FontWeight.w700,
                              ),
                            ),
                            const SizedBox(width: 7),
                            Icon(
                              Icons.arrow_forward_rounded,
                              size: 17,
                              color: KarryamColors.tealInk.forInk(context),
                            ),
                          ],
                        ),
                      ],
                    ),
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    if (c.business['status'] == 'PAUSED')
      Padding(
        padding: const EdgeInsets.only(bottom: 18),
        child: SettingsLink(
          'Business paused',
          Icons.pause_circle_outline,
          color: Color(0xFF986013).forInk(context),
          subtitle: 'Open your profile to resume.',
          onTap: () async {
            await Navigator.push(
              context,
              setupRoute<void>(
                SettingsCollectionScreen(
                  runtime: widget.runtime,
                  kind: SettingsKind.profile,
                ),
              ),
            );
            await c.load();
          },
        ),
      ),
    if (c.errors['business'] != null)
      _notice(
        'Business unavailable',
        c.errors['business']!,
        story: c.restricted.contains('business')
            ? EmptyStory.restricted
            : EmptyStory.unavailable,
        Icons.info_outline,
        error: true,
        retry: _refresh,
      ),
    if (c.errors['branches'] != null)
      _notice(
        'Branches unavailable',
        c.errors['branches']!,
        story: c.restricted.contains('branches')
            ? EmptyStory.restricted
            : EmptyStory.unavailable,
        Icons.info_outline,
        error: true,
        retry: _refresh,
      ),
    _resource('wallet', _wallet()),
    _section('Order queue', onTap: () => _select(1)),
    Padding(
      padding: const EdgeInsets.only(bottom: 14),
      child: Text(
        '${c.branch?['display_name'] ?? 'Selected branch'} / Latest 50 orders',
        style: TextStyle(
          fontSize: 12,
          color: KarryamColors.indigo.forInk(context),
        ),
      ),
    ),
    _resource(
      'orders',
      DashboardQueue(
        counts: [
          c.count(DashboardController.incoming),
          c.count(DashboardController.working),
          c.count(DashboardController.ready),
        ],
        onSelect: (index) {
          _filter = ['Incoming', 'In progress', 'Ready'][index];
          _select(1);
        },
      ),
    ),
    if (!c.loading && c.errors['capabilities'] == null && (c.food || c.shop))
      DashboardCatalogueBanner(
        food: c.food,
        onTap: () => _catalogueWorkspace(
          c.food ? CatalogueKind.menus : CatalogueKind.products,
        ),
      ),
    _section('Quick access'),
    _quickAccess(),
    _section(
      'Recent orders',
      onTap: () {
        _filter = 'All';
        _select(1);
      },
    ),
    _resource(
      'orders',
      c.orders.isEmpty
          ? _notice(
              'No recent orders',
              'New orders will appear here when they reach your business.',
              Icons.receipt_long_outlined,
            )
          : Column(
              children: [for (final order in c.orders.take(3)) _order(order)],
            ),
    ),
  ];
  bool _quickOpening = false;

  Widget _quickAccess() => Column(
    children: [
      DashboardAction(
        title: 'Browse catalogue',
        subtitle: 'Food menus and shop products',
        icon: Icons.widgets_outlined,
        iconColor: const Color(0xFF3775C5),
        onTap: () => _select(2),
      ),
      if (c.shop && c.branch != null) ...[
        const SizedBox(height: 10),
        DashboardAction(
          title: 'Update stock',
          subtitle: 'Manage quantities for this branch',
          icon: Icons.inventory_2_outlined,
          iconColor: const Color(0xFFB27318),
          onTap: () => orderPage<void>(
            context,
            StockScreen(
              runtime: widget.runtime,
              branchId: c.branchId!,
              branchName:
                  c.branch!['display_name']?.toString() ?? 'Your branch',
            ),
          ),
        ),
      ] else if (c.food && !c.restricted.contains('capabilities')) ...[
        const SizedBox(height: 10),
        DashboardAction(
          title: 'Add item',
          subtitle: 'Add something new to your menu',
          icon: Icons.add_box_outlined,
          iconColor: const Color(0xFFB27318),
          onTap: c.loading || _quickOpening ? null : _quickAdd,
        ),
      ],
      if (c.food || c.shop) ...[
        const SizedBox(height: 10),
        DashboardAction(
          title: 'Deliveries',
          subtitle: 'Assign riders and track fulfilment',
          icon: Icons.local_shipping_outlined,
          iconColor: const Color(0xFF7860C3),
          onTap: () => orderPage<void>(
            context,
            VendorMissionsScreen(runtime: widget.runtime),
          ),
        ),
      ],
      if (!c.restricted.contains('wallet')) ...[
        const SizedBox(height: 10),
        DashboardAction(
          title: 'View wallet activity',
          subtitle: 'Balances and recent transactions',
          icon: Icons.account_balance_wallet_outlined,
          iconColor: KarryamColors.tealInk,
          onTap: () => _select(3),
        ),
      ],
    ],
  );

  Future<JsonObject?> _quickPick(String title, List<JsonObject> rows) async {
    if (rows.length == 1) return rows.single;
    if (rows.isEmpty) {
      await showOrderFeedback(
        context,
        title: 'Set up your menu first',
        message: 'Create a menu and section before adding food items.',
        warning: true,
      );
      return null;
    }
    return showModalBottomSheet<JsonObject>(
      context: context,
      isScrollControlled: true,
      useSafeArea: true,
      showDragHandle: true,
      backgroundColor: KarryamColors.canvas.forSurface(context),
      builder: (sheetContext) => ConstrainedBox(
        constraints: BoxConstraints(
          maxHeight: MediaQuery.sizeOf(sheetContext).height * .65,
        ),
        child: ListView(
          shrinkWrap: true,
          padding: const EdgeInsets.fromLTRB(22, 8, 22, 24),
          children: [
            Text(
              title,
              style: const TextStyle(fontSize: 23, fontWeight: FontWeight.w700),
            ),
            const SizedBox(height: 16),
            for (final row in rows)
              ListTile(
                title: Text(row['name']?.toString() ?? row['id'].toString()),
                trailing: const Icon(Icons.chevron_right_rounded),
                onTap: () => Navigator.pop(sheetContext, row),
              ),
          ],
        ),
      ),
    );
  }

  Future<void> _quickAdd() async {
    if (_quickOpening) return;
    setState(() => _quickOpening = true);
    try {
      var food = c.food && !c.shop;
      if (c.food && c.shop) {
        final selection = await _quickPick('What are you adding?', [
          {'id': 'food', 'name': 'Food item'},
          {'id': 'shop', 'name': 'Shop product'},
        ]);
        if (selection == null || !mounted) return;
        food = selection['id'] == 'food';
      }
      var resource = const CatalogueResource(CatalogueKind.products);
      if (food) {
        final menus = CatalogueController(
          widget.runtime.backend,
          CatalogueResource(CatalogueKind.menus, branchId: c.branchId),
        );
        JsonObject? menu;
        try {
          await menus.load();
          if (!mounted) return;
          if (menus.error != null) throw StateError(menus.error!);
          menu = await _quickPick(
            'Choose a menu',
            menus.rows.where((r) => r['status'] == 'ACTIVE').toList(),
          );
        } finally {
          menus.dispose();
        }
        if (menu == null || !mounted) return;
        final sections = CatalogueController(
          widget.runtime.backend,
          CatalogueResource(
            CatalogueKind.sections,
            parameters: {'menuId': menu['id'].toString()},
          ),
        );
        JsonObject? section;
        try {
          await sections.load();
          if (!mounted) return;
          if (sections.error != null) throw StateError(sections.error!);
          section = await _quickPick(
            'Choose a section',
            sections.rows.where((r) => r['status'] != 'ARCHIVED').toList(),
          );
        } finally {
          sections.dispose();
        }
        if (section == null || !mounted) return;
        resource = CatalogueResource(
          CatalogueKind.items,
          parameters: {
            'menuId': menu['id'].toString(),
            'sectionId': section['id'].toString(),
          },
        );
      }
      if (!mounted) return;
      await orderPage<void>(
        context,
        CatalogueScreen(
          runtime: widget.runtime,
          resource: resource,
          currency: c.business['currency']?.toString() ?? 'NGN',
          createOnOpen: true,
        ),
      );
      if (mounted) await c.loadCatalogue();
    } catch (e) {
      if (mounted) {
        await showOrderFeedback(
          context,
          title: 'Could not open catalogue',
          message: CatalogueController.message(e),
        );
      }
    } finally {
      if (mounted) setState(() => _quickOpening = false);
    }
  }

  Widget _wallet() => DashboardBalance(
    visible: widget.runtime.balancesVisible,
    onToggleVisibility: widget.runtime.toggleBalances,
    available: dashboardMoney(c.wallet['available_kobo'], c.wallet['currency']),
    pending: dashboardMoney(c.wallet['pending_kobo'], c.wallet['currency']),
    onTap: _tab == 3 ? null : () => _select(3),
  );
  List<Widget> _orders() {
    final allowed = switch (_filter) {
      'Incoming' => DashboardController.incoming,
      'In progress' => DashboardController.working,
      'Ready' => DashboardController.ready,
      'Delivery' => {'HANDED_OVER', 'PICKED_UP', 'DELIVERING', 'DELIVERED'},
      'Completed' => {'COMPLETED'},
      'Cancelled' => {'CANCELLED', 'REJECTED'},
      _ => <String>{},
    };
    final orders = c.orders
        .where((order) => allowed.isEmpty || allowed.contains(order['status']))
        .toList();
    return [
      _heading(
        'Orders',
        '${c.branch?['display_name'] ?? 'Selected branch'} / Latest 50 orders',
      ),
      Padding(
        padding: const EdgeInsets.only(bottom: 16),
        child: DashboardAction(
          title: 'Scheduled orders',
          subtitle: 'Upcoming deliveries and recurring meals',
          icon: Icons.calendar_month_outlined,
          iconColor: const Color(0xFF3775C5),
          onTap: () => orderPage<void>(
            context,
            SchedulingWorkspaceScreen(runtime: widget.runtime),
          ),
        ),
      ),
      SingleChildScrollView(
        scrollDirection: Axis.horizontal,
        child: Row(
          children: [
            for (final filter in [
              'All',
              'Incoming',
              'In progress',
              'Ready',
              'Delivery',
              'Completed',
              'Cancelled',
            ])
              Padding(
                padding: const EdgeInsets.only(right: 8),
                child: ChoiceChip(
                  label: Text(filter),
                  selected: _filter == filter,
                  showCheckmark: false,
                  selectedColor: Color(0xFFE0F8F1).forSurface(context),
                  backgroundColor: Colors.white.forSurface(context),
                  side: BorderSide(
                    color:
                        (_filter == filter
                                ? KarryamColors.teal.withValues(alpha: .4)
                                : const Color(0xFFE1E9E4))
                            .forOutline(context),
                  ),
                  labelStyle: TextStyle(
                    color:
                        (_filter == filter
                                ? KarryamColors.tealInk
                                : KarryamColors.indigo)
                            .forInk(context),
                    fontWeight: FontWeight.w700,
                  ),
                  onSelected: (_) => setState(() => _filter = filter),
                ),
              ),
          ],
        ),
      ),
      const SizedBox(height: 20),
      _resource(
        'orders',
        orders.isEmpty
            ? _notice(
                c.awaitingActivation
                    ? 'Ordering opens after activation'
                    : _filter == 'All'
                    ? 'No orders yet'
                    : 'No ${_filter.toLowerCase()} orders',
                c.awaitingActivation
                    ? 'Your business is not active yet. Prepare your catalogue and track verification while you wait.'
                    : _filter == 'All'
                    ? 'Orders for this branch will appear here when they arrive.'
                    : 'There are no orders in this view. Check the full queue for other orders.',
                Icons.receipt_long_outlined,
                story: _filter == 'All'
                    ? EmptyStory.orders
                    : EmptyStory.filtered,
                action: c.awaitingActivation
                    ? 'Track verification'
                    : _filter == 'All'
                    ? 'Refresh orders'
                    : 'Show all orders',
                retry: c.awaitingActivation
                    ? _setup
                    : _filter == 'All'
                    ? _refresh
                    : () => setState(() => _filter = 'All'),
              )
            : Column(children: [for (final order in orders) _order(order)]),
      ),
    ];
  }

  Widget _order(JsonObject order) {
    final id = order['vendor_order_id']?.toString() ?? '';
    final short = id.length > 20 ? id.substring(id.length - 8) : id;
    final color = _orderColor(order['status']);
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: Material(
        color: Colors.white.forSurface(context),
        borderRadius: BorderRadius.circular(22),
        child: InkWell(
          borderRadius: BorderRadius.circular(22),
          onTap: () => _orderDetails(id),
          child: Padding(
            padding: const EdgeInsets.all(18),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    Container(
                      width: 40,
                      height: 40,
                      decoration: BoxDecoration(
                        color: Color(0xFFE7F8F3).forSurface(context),
                        borderRadius: BorderRadius.circular(13),
                      ),
                      child: Icon(
                        order['domain'] == 'SHOP'
                            ? Icons.shopping_bag_outlined
                            : Icons.restaurant_rounded,
                        color: KarryamColors.tealInk.forInk(context),
                        size: 21,
                      ),
                    ),
                    const SizedBox(width: 12),
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(
                            '#$short',
                            style: const TextStyle(fontWeight: FontWeight.w700),
                          ),
                          const SizedBox(height: 4),
                          Text(
                            '${dashboardLabel(order['domain'])} / ${dashboardLabel(order['fulfilment_mode'])}',
                            style: TextStyle(
                              fontSize: 11,
                              color: KarryamColors.indigo.forInk(context),
                            ),
                          ),
                        ],
                      ),
                    ),
                    Icon(
                      Icons.chevron_right_rounded,
                      color: KarryamColors.tealInk.forInk(context),
                      size: 21,
                    ),
                  ],
                ),
                const SizedBox(height: 16),
                Divider(
                  height: 1,
                  color: Color(0xFFEDF1ED).forOutline(context),
                ),
                const SizedBox(height: 14),
                Row(
                  children: [
                    Expanded(
                      child: Align(
                        alignment: Alignment.centerLeft,
                        child: Container(
                          padding: const EdgeInsets.symmetric(
                            horizontal: 10,
                            vertical: 7,
                          ),
                          decoration: BoxDecoration(
                            color: (color.withValues(
                              alpha: .08,
                            )).forSurface(context),
                            borderRadius: BorderRadius.circular(8),
                          ),
                          child: Text(
                            dashboardLabel(order['status']),
                            style: TextStyle(
                              fontSize: 10,
                              color: (color).forInk(context),
                              fontWeight: FontWeight.w700,
                            ),
                          ),
                        ),
                      ),
                    ),
                    const SizedBox(width: 8),
                    Flexible(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.end,
                        children: [
                          Text(
                            'Vendor payable',
                            style: TextStyle(
                              fontSize: 9,
                              color: KarryamColors.indigo.forInk(context),
                            ),
                          ),
                          const SizedBox(height: 3),
                          Text(
                            dashboardMoney(
                              order['vendor_payable_minor'],
                              order['currency'],
                            ),
                            textAlign: TextAlign.right,
                            style: const TextStyle(
                              fontSize: 12,
                              fontWeight: FontWeight.w700,
                            ),
                          ),
                        ],
                      ),
                    ),
                  ],
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  Future<void> _activity() => showModalBottomSheet<void>(
    context: context,
    useSafeArea: true,
    isScrollControlled: true,
    showDragHandle: true,
    backgroundColor: KarryamColors.canvas.forSurface(context),
    builder: (_) => AnimatedBuilder(
      animation: c,
      builder: (context, _) => ConstrainedBox(
        constraints: BoxConstraints(
          maxHeight: MediaQuery.sizeOf(context).height * .75,
        ),
        child: ListView(
          shrinkWrap: true,
          padding: const EdgeInsets.fromLTRB(24, 0, 24, 24),
          children: [
            const Text(
              'Order activity',
              style: TextStyle(
                fontSize: 26,
                letterSpacing: -.8,
                fontWeight: FontWeight.w700,
              ),
            ),
            const SizedBox(height: 10),
            Text(
              '${c.branch?['display_name'] ?? 'Selected branch'} / Recent order states',
            ),
            const SizedBox(height: 18),
            _resource(
              'orders',
              c.orders.isEmpty
                  ? _notice(
                      'No order activity',
                      'Orders will appear here when they reach your business.',
                      Icons.notifications_none_rounded,
                    )
                  : Column(
                      children: [
                        for (final order in c.orders.take(10)) _order(order),
                      ],
                    ),
            ),
            TextButton.icon(
              onPressed: c.loading ? null : _refresh,
              icon: const Icon(Icons.refresh_rounded),
              label: const Text('Refresh activity'),
            ),
          ],
        ),
      ),
    ),
  );

  Future<void> _orderDetails(String id) async {
    var changed = false;
    await orderPage<void>(
      context,
      OrderDetailScreen(
        runtime: widget.runtime,
        id: id,
        onChanged: () => changed = true,
      ),
    );
    if (changed && mounted) await _refresh();
  }

  List<Widget> _catalogue() {
    final food = c.food;
    final type = food && c.shop
        ? _catalogueType
        : food
        ? 'Food'
        : 'Shop';
    final records = type == 'Food' ? c.menus : c.products;
    final key = type == 'Food' ? 'menus' : 'products';
    return [
      _heading('Catalogue', 'Shape what your customers come back for.'),
      if (c.errors['capabilities'] != null)
        _notice(
          'Business types unavailable',
          c.errors['capabilities']!,
          story: c.restricted.contains('capabilities')
              ? EmptyStory.restricted
              : EmptyStory.unavailable,
          Icons.info_outline,
          error: true,
          retry: _refresh,
        )
      else if (c.loading || c.catalogueLoading)
        const _LoadingBlock()
      else if (!food && !c.shop)
        _notice(
          'No catalogue available',
          'Review your business types to see your available catalogue.',
          Icons.widgets_outlined,
          retry: _setup,
          action: 'Review business types',
          story: EmptyStory.setup,
        )
      else ...[
        if (food && c.shop)
          SizedBox(
            width: double.infinity,
            child: SegmentedButton<String>(
              segments: const [
                ButtonSegment(
                  value: 'Food',
                  label: Text('Food menus'),
                  icon: Icon(Icons.restaurant_rounded),
                ),
                ButtonSegment(
                  value: 'Shop',
                  label: Text('Shop products'),
                  icon: Icon(Icons.shopping_bag_outlined),
                ),
              ],
              selected: {_catalogueType},
              showSelectedIcon: false,
              style: ButtonStyle(
                backgroundColor: WidgetStateProperty.resolveWith(
                  (states) => states.contains(WidgetState.selected)
                      ? Color(0xFFE0F8F1).forSurface(context)
                      : Colors.white.forSurface(context),
                ),
                foregroundColor: WidgetStateProperty.resolveWith(
                  (states) => states.contains(WidgetState.selected)
                      ? KarryamColors.tealInk.forInk(context)
                      : KarryamColors.indigo.forInk(context),
                ),
                side: WidgetStatePropertyAll(
                  BorderSide(color: Color(0xFFDDE9E2).forOutline(context)),
                ),
              ),
              onSelectionChanged: (values) =>
                  setState(() => _catalogueType = values.single),
            ),
          ),
        const SizedBox(height: 18),
        FilledButton.icon(
          onPressed: () => _catalogueWorkspace(
            type == 'Food' ? CatalogueKind.menus : CatalogueKind.products,
          ),
          icon: const Icon(Icons.edit_note_rounded),
          label: Text(type == 'Food' ? 'Manage menus' : 'Manage products'),
        ),
        const SizedBox(height: 10),
        Wrap(
          spacing: 8,
          children: [
            if (type == 'Food')
              TextButton.icon(
                onPressed: () => _catalogueWorkspace(CatalogueKind.groups),
                icon: const Icon(Icons.tune_rounded),
                label: const Text('Modifier groups'),
              ),
            if (type != 'Food')
              TextButton.icon(
                onPressed: () => _catalogueWorkspace(CatalogueKind.categories),
                icon: const Icon(Icons.category_outlined),
                label: const Text('Categories'),
              ),
            if (type != 'Food' && c.branchId != null)
              TextButton.icon(
                onPressed: () => Navigator.push(
                  context,
                  setupRoute<void>(
                    StockScreen(
                      runtime: widget.runtime,
                      branchId: c.branchId!,
                      branchName:
                          c.branch?['display_name']?.toString() ??
                          'Your branch',
                    ),
                  ),
                ),
                icon: const Icon(Icons.inventory_2_outlined),
                label: const Text('Inventory'),
              ),
            if (type != 'Food' && c.branchId != null)
              TextButton.icon(
                onPressed: () => _catalogueWorkspace(CatalogueKind.listings),
                icon: const Icon(Icons.storefront_rounded),
                label: const Text('Branch listings'),
              ),
          ],
        ),
        const SizedBox(height: 22),
        Text(
          type == 'Food'
              ? 'Menus at ${c.branch?['display_name'] ?? 'your branch'}'
              : 'Products across your business',
          style: const TextStyle(
            fontSize: 18,
            fontWeight: FontWeight.w700,
            letterSpacing: -.4,
          ),
        ),
        const SizedBox(height: 8),
        Text(
          'Tap an entry to view its details.',
          style: TextStyle(
            fontSize: 12,
            color: KarryamColors.indigo.forInk(context),
          ),
        ),
        const SizedBox(height: 18),
        if (c.errors[key] != null)
          _notice(
            'Catalogue unavailable',
            c.errors[key]!,
            Icons.info_outline,
            error: true,
            retry: c.loadCatalogue,
          )
        else if (records.isEmpty)
          _notice(
            type == 'Food' ? 'No menus in this branch' : 'No products yet',
            'Your saved catalogue entries will appear here.',
            Icons.widgets_outlined,
          )
        else
          for (final row in records) _catalogueRow(row, type == 'Food'),
        const SizedBox(height: 14),
        Text(
          'Showing up to 50 entries.',
          style: TextStyle(
            fontSize: 11,
            color: KarryamColors.indigo.forInk(context),
          ),
        ),
      ],
    ];
  }

  Future<void> _catalogueWorkspace(CatalogueKind kind) async {
    await orderPage<void>(
      context,
      CatalogueScreen(
        runtime: widget.runtime,
        resource: CatalogueResource(
          kind,
          branchId: c.branchId,
          parameters: kind == CatalogueKind.listings
              ? {'branchId': c.branchId!}
              : const {},
        ),
        currency: c.business['currency']?.toString() ?? 'NGN',
        subtitle: kind == CatalogueKind.listings
            ? (c.branch?['display_name'])?.toString()
            : null,
      ),
    );
    if (mounted) await c.loadCatalogue();
  }

  Widget _catalogueRow(JsonObject row, bool food) => Padding(
    padding: const EdgeInsets.only(bottom: 12),
    child: Material(
      color: Colors.white.forSurface(context),
      borderRadius: BorderRadius.circular(22),
      child: InkWell(
        borderRadius: BorderRadius.circular(22),
        onTap: () async {
          await orderPage<void>(
            context,
            CatalogueDetailScreen(
              runtime: widget.runtime,
              resource: CatalogueResource(
                food ? CatalogueKind.menus : CatalogueKind.products,
                branchId: c.branchId,
              ),
              row: row,
              currency: c.business['currency']?.toString() ?? 'NGN',
            ),
          );
          if (mounted) await c.loadCatalogue();
        },
        child: Padding(
          padding: const EdgeInsets.all(18),
          child: Row(
            children: [
              if (widget.runtime.mode == VendorDataMode.mock)
                MockCataloguePhoto(
                  name: row['name']?.toString() ?? '',
                  id: row['id'].toString(),
                )
              else
                Container(
                  width: 48,
                  height: 54,
                  decoration: BoxDecoration(
                    color:
                        (food
                                ? const Color(0xFFFFF0DB)
                                : const Color(0xFFE6F1FF))
                            .forSurface(context),
                    borderRadius: BorderRadius.circular(16),
                  ),
                  child: Icon(
                    food
                        ? Icons.restaurant_menu_rounded
                        : Icons.shopping_bag_outlined,
                    color:
                        (food
                                ? const Color(0xFFAE680C)
                                : const Color(0xFF3475D5))
                            .forInk(context),
                  ),
                ),
              const SizedBox(width: 14),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      row['name']?.toString() ?? '',
                      style: const TextStyle(
                        fontSize: 15,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                    const SizedBox(height: 6),
                    Text(
                      dashboardLabel(row['status']),
                      style: TextStyle(
                        color: (_orderColor(row['status'])).forInk(context),
                        fontSize: 11,
                      ),
                    ),
                    const SizedBox(height: 6),
                    Text(
                      food ? 'View menu sections' : 'View product variants',
                      style: TextStyle(
                        fontSize: 11,
                        color: KarryamColors.indigo.forInk(context),
                      ),
                    ),
                  ],
                ),
              ),
              Icon(
                Icons.arrow_forward_rounded,
                color: KarryamColors.tealInk.forInk(context),
                size: 19,
              ),
            ],
          ),
        ),
      ),
    ),
  );
  List<Widget> _more() => [
    SettingsHub(
      runtime: widget.runtime,
      business: c.business,
      onSetup: _setup,
      onBranches: _branches,
      onSettings: widget.onSettings,
      onSignOut: _signOut,
      onChanged: c.load,
      branchName: c.branch?['display_name']?.toString(),
    ),
  ];
}

Color _orderColor(Object? status) =>
    ['CANCELLED', 'REJECTED', 'SUSPENDED', 'FAILED'].contains(status)
    ? KarryamColors.error
    : [
        'ACTIVE',
        'COMPLETED',
        'READY',
        'READY_FOR_PICKUP',
        'VERIFIED',
      ].contains(status)
    ? KarryamColors.tealInk
    : [
        'PREPARING',
        'PICKING',
        'PACKING',
        'AWAITING_CUSTOMER',
        'SUBSTITUTION_REQUIRED',
      ].contains(status)
    ? const Color(0xFF986000)
    : ['PAID', 'SENT_TO_VENDOR', 'ACCEPTED', 'DELIVERING'].contains(status)
    ? const Color(0xFF2457A6)
    : KarryamColors.indigo;

class _LoadingBlock extends StatelessWidget {
  const _LoadingBlock();
  @override
  Widget build(BuildContext context) => const PremiumSkeleton(rows: 2);
}
