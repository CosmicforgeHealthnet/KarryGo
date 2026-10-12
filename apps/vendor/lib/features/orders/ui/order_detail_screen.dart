import '../../readiness/ui/order_readiness_screen.dart';
import '../../../core/ui/vendor_toast.dart';
import '../../../core/ui/premium_loading.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/widgets/vendor_widgets.dart';
import '../../dashboard/ui/dashboard_empty_state.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../state/order_actions.dart';
import '../state/order_detail_controller.dart';
import 'order_handover_sheet.dart';
import 'order_substitution_screen.dart';
import 'order_form_sheet.dart';
import 'order_design.dart';
import 'order_feedback_dialog.dart';
import '../../support/ui/order_support_screen.dart';
import '../../onboarding/ui/setup_design.dart';

class OrderDetailScreen extends StatefulWidget {
  const OrderDetailScreen({
    super.key,
    required this.runtime,
    required this.id,
    required this.onChanged,
  });
  final VendorRuntime runtime;
  final String id;
  final VoidCallback onChanged;
  @override
  State<OrderDetailScreen> createState() => _OrderDetailScreenState();
}

class _OrderDetailScreenState extends State<OrderDetailScreen> {
  late final c = OrderDetailController(widget.runtime.backend, widget.id);
  @override
  void initState() {
    super.initState();
    c.addListener(_update);
    c.load();
  }

  void _update() {
    if (c.changed) widget.onChanged();
    if (mounted) setState(() {});
  }

  @override
  void dispose() {
    c.removeListener(_update);
    c.dispose();
    super.dispose();
  }

  List<OrderAction> get actions =>
      c.order == null ? [] : availableOrderActions(c.order!);
  OrderAction? get primary {
    final preferred = switch (c.order?['status']) {
      'CREATED' || 'PAID' || 'SENT_TO_VENDOR' => 'accept',
      'ACCEPTED' => c.order?['domain'] == 'SHOP' ? 'picking' : 'preparing',
      'PREPARING' => c.order?['domain'] == 'SHOP' ? 'picking' : 'ready',
      'PICKING' => 'pack',
      'PACKING' => 'ready-for-pickup',
      'READY' || 'READY_FOR_PICKUP' => 'pickup',
      'PICKED_UP' || 'HANDED_OVER' => 'delivering',
      'DELIVERING' => 'delivered',
      'DELIVERED' => 'complete',
      _ => '',
    };
    return actions.where((a) => a.route == preferred).firstOrNull;
  }

  bool _presenting = false;
  Future<void> _result(
    bool success,
    String label, {
    bool milestone = false,
    bool ending = false,
    String? message,
  }) async {
    if (!mounted) return;
    if (!success || c.error != null) {
      final refresh = await showOrderFeedback(
        context,
        title: success
            ? 'Saved. Refresh needed'
            : c.restricted
            ? 'Access required'
            : c.stale
            ? 'Refresh before continuing'
            : 'Could not finish',
        message: success
            ? 'Your change was saved, but the latest order information could not be loaded.'
            : c.error ?? 'Please refresh this order and try again.',
        warning: success || c.restricted || c.stale,
        action: 'Refresh order',
      );
      if (refresh == true && mounted) await c.load();
      return;
    }
    if (milestone) {
      await showOrderFeedback(
        context,
        title: label,
        message:
            message ??
            (ending
                ? 'This vendor order has ended. Its latest status and reason are available in the order details.'
                : 'The order has been updated. You can continue from its latest status.'),
        success: true,
        ending: ending,
      );
    } else {
      showVendorToast(context, label);
    }
  }

  Future<void> _confirm(OrderAction action) async {
    if (_presenting ||
        !c.can(
          '/api/v1/vendor/commerce/orders/{vendorOrderId}/${action.route}',
        )) {
      return;
    }
    _presenting = true;
    try {
      JsonObject body = {};
      if (action.destructive ||
          [
            'accept',
            'pickup',
            'handover',
            'delivered',
            'complete',
          ].contains(action.route)) {
        final values = await showModalBottomSheet<JsonObject>(
          context: context,
          isScrollControlled: true,
          useSafeArea: true,
          showDragHandle: true,
          backgroundColor: Colors.white.forSurface(context),
          builder: (_) => OrderFormSheet(
            title: action.label,
            message: action.destructive
                ? '${orderActionHint(action.route)} Other vendor orders in this purchase are unaffected.'
                : orderActionHint(action.route),
            fields: action.destructive
                ? const [OrderInput('reason_code', 'Reason', required: true)]
                : const [],
            destructive: action.destructive,
            icon: orderActionIcon(action.route),
          ),
        );
        if (values == null || !mounted) return;
        body = values;
      }
      final success = await c.submit(
        action.route,
        body,
        version: c.order!['version'] as int,
      );
      final label = switch (action.route) {
        'accept' => 'Order accepted',
        'preparing' => 'Preparation started',
        'picking' => 'Picking started',
        'pack' => 'Order packed',
        'ready' || 'ready-for-pickup' => 'Ready for collection',
        'pickup' || 'handover' => 'Pickup recorded',
        'delivering' => 'Delivery started',
        'delivered' => 'Delivery recorded',
        'complete' => 'Order completed',
        'cancel' => 'Order cancelled',
        'reject' => 'Order declined',
        _ => 'Order updated',
      };
      await _result(
        success,
        label,
        ending: action.destructive,
        milestone: [
          'accept',
          'complete',
          'cancel',
          'reject',
        ].contains(action.route),
      );
    } finally {
      _presenting = false;
    }
  }

  Future<void> _line(JsonObject row, String action) async {
    final lineId = row['OrderLineID']?.toString();
    final version = row['Version'];
    if (lineId == null || version is! int) return;
    final availability = action == 'availability';
    final body = await showModalBottomSheet<JsonObject>(
      context: context,
      isScrollControlled: true,
      useSafeArea: true,
      showDragHandle: true,
      builder: (_) => OrderFormSheet(
        title: availability ? 'Record availability' : 'Record quality',
        message: availability
            ? 'Record how many of the ordered ${row['OrderedQuantity']} units you found.'
            : 'Record what you observed. A quality result does not approve a replacement.',
        fields: availability
            ? [
                OrderInput(
                  'picked_quantity',
                  'Units found',
                  required: true,
                  number: true,
                  max: row['OrderedQuantity'] as int?,
                  initialText: row['QuantityFound']?.toString(),
                ),
              ]
            : const [
                OrderInput('observed_value', 'Observed value'),
                OrderInput('reason', 'Reason'),
              ],
        choices: availability
            ? const {}
            : const {
                'result': ['MET', 'PARTIAL', 'NOT_AVAILABLE', 'SUBSTITUTE'],
              },
      ),
    );
    if (body == null || !mounted) return;
    if (availability) {
      final picked = body['picked_quantity'] as int;
      final ordered = row['OrderedQuantity'] as int;
      body['unavailable_quantity'] = ordered - picked;
      body['outcome'] = picked == ordered
          ? 'FULLY_AVAILABLE'
          : picked == 0
          ? 'UNAVAILABLE'
          : 'PARTIALLY_AVAILABLE';
    }
    final success = await c.line(lineId, action, body, version);
    await _result(
      success,
      availability ? 'Availability recorded' : 'Quality recorded',
    );
  }

  Future<void> _measurement(JsonObject row) async {
    final evidence = jsonObject(row['Measurement']);
    final version = evidence['version'];
    if (version is! int || version <= 0) return;
    final body = await showModalBottomSheet<JsonObject>(
      context: context,
      isScrollControlled: true,
      useSafeArea: true,
      showDragHandle: true,
      builder: (_) => OrderFormSheet(
        title: 'Record measured quantity',
        icon: Icons.scale_outlined,
        message:
            'Use the original measurement units for this order. The server calculates the amount and checks payment limits.',
        fields: [
          OrderInput(
            'actual_quantity',
            'Measured quantity',
            required: true,
            number: true,
            max: evidence['reserved_upper_quantity'] as int?,
            initialText:
                evidence['actual_quantity'] is int &&
                    (evidence['actual_quantity'] as int) > 0
                ? '${evidence['actual_quantity']}'
                : '${evidence['estimated_quantity']}',
          ),
        ],
      ),
    );
    if (body == null || !mounted) return;
    final saved = await c.line(
      '${row['OrderLineID']}',
      'actual-quantity',
      body,
      version,
    );
    await _result(saved, 'Measured quantity saved');
  }

  Widget _evidenceStatus(
    String label,
    String value,
    IconData icon,
    Color color,
  ) => Padding(
    padding: const EdgeInsets.only(top: 12),
    child: Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: color.forInk(context).withValues(alpha: 0.09),
        borderRadius: BorderRadius.circular(16),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Icon(icon, color: color.forInk(context), size: 22),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  label,
                  style: TextStyle(
                    fontSize: 12,
                    color: Theme.of(context).colorScheme.onSurfaceVariant,
                  ),
                ),
                const SizedBox(height: 4),
                Text(
                  value,
                  style: const TextStyle(fontWeight: FontWeight.w600),
                ),
              ],
            ),
          ),
        ],
      ),
    ),
  );

  List<Widget> _savedEvidence(JsonObject row) {
    final measurement = jsonObject(row['Measurement'] ?? {});
    final state = '${measurement['state'] ?? ''}';
    final quality = row['QualityResult'];
    final results = (row['QualityResults'] as List? ?? []).map(jsonObject);
    final recorded = state != 'ESTIMATED' && state.isNotEmpty;
    return [
      if (measurement.isNotEmpty) ...[
        _evidenceStatus(
          'Measured quantity',
          recorded
              ? '${measurement['actual_quantity']} / ${measurement['estimated_quantity']} estimated'
              : '${measurement['estimated_quantity']} estimated; awaiting measurement',
          Icons.scale_outlined,
          KarryamColors.indigo,
        ),
        Padding(
          padding: const EdgeInsets.only(top: 8),
          child: Wrap(
            spacing: 14,
            runSpacing: 6,
            children: [
              Text(
                dashboardLabel(state),
                style: TextStyle(
                  fontSize: 12,
                  color:
                      (state == 'REAPPROVAL_REQUIRED' ||
                                  state == 'FAILED_RECOVERABLE'
                              ? const Color(0xFFAE4D37)
                              : state == 'FINALIZED'
                              ? KarryamColors.tealInk
                              : const Color(0xFF986013))
                          .forInk(context),
                ),
              ),
              if (recorded)
                Text(
                  dashboardMoney(
                    measurement['final_amount_minor'],
                    c.order!['currency'],
                  ),
                  style: const TextStyle(fontWeight: FontWeight.w600),
                ),
            ],
          ),
        ),
        if (state == 'REAPPROVAL_REQUIRED')
          const Padding(
            padding: EdgeInsets.only(top: 8),
            child: Text('Customer reapproval is required before settlement.'),
          ),
      ] else if (row['MeasurementMode'] != null &&
          row['MeasurementMode'] != 'FIXED_UNIT')
        const Padding(
          padding: EdgeInsets.only(top: 8),
          child: Text(
            'Measurement evidence is unavailable. Refresh before recording a quantity.',
          ),
        ),
      if (quality != null)
        _evidenceStatus(
          'Quality status',
          dashboardLabel(quality),
          quality == 'MET' ? Icons.fact_check_outlined : Icons.report_outlined,
          quality == 'MET'
              ? KarryamColors.tealInk
              : quality == 'NOT_AVAILABLE'
              ? const Color(0xFFAE4D37)
              : const Color(0xFF986013),
        ),
      for (final result in results)
        Padding(
          padding: const EdgeInsets.only(top: 10),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                '${dashboardLabel(result['preference_type'])}: ${dashboardLabel(result['result'])}',
                style: const TextStyle(fontWeight: FontWeight.w600),
              ),
              if ('${result['observed_value'] ?? ''}'.isNotEmpty)
                Text('${result['observed_value']}'),
              if ('${result['reason'] ?? ''}'.isNotEmpty)
                Text(
                  '${result['reason']}',
                  style: TextStyle(
                    fontSize: 12,
                    color: Theme.of(context).colorScheme.onSurfaceVariant,
                  ),
                ),
            ],
          ),
        ),
    ];
  }

  Future<void> _substitute(JsonObject row) async {
    final body = await orderPage<JsonObject>(
      context,
      OrderSubstitutionScreen(
        runtime: widget.runtime,
        branchId: c.order!['branch_id'].toString(),
      ),
    );
    if (body == null || !mounted) return;
    final success = await c.line(
      row['OrderLineID'].toString(),
      'substitutions',
      body,
      c.order!['version'] as int,
    );
    await _result(
      success,
      'Replacement suggested',
      milestone: true,
      message:
          'Your suggestion has been sent. It has not been approved on behalf of the customer.',
    );
  }

  Future<void> _handover() async {
    final changed = await showModalBottomSheet<bool>(
      context: context,
      isScrollControlled: true,
      useSafeArea: true,
      showDragHandle: true,
      builder: (_) => OrderHandoverSheet(
        runtime: widget.runtime,
        orderId: widget.id,
        missionId: c.order!['logistics_mission_ref'].toString(),
      ),
    );
    if (changed == true && mounted) {
      c.changed = true;
      widget.onChanged();
      await c.load();
      await _result(
        true,
        'Pickup verified',
        milestone: true,
        message:
            'The pickup credential has been verified. Order progress updates when the delivery provider reports it.',
      );
    }
  }

  Widget _card(List<Widget> children) => Container(
    padding: const EdgeInsets.all(18),
    margin: const EdgeInsets.only(bottom: 14),
    decoration: BoxDecoration(
      color: Color(0xFFF7F9FB).forSurface(context),
      borderRadius: BorderRadius.circular(20),
      border: Border.all(color: Color(0xFFE7ECEF).forOutline(context)),
    ),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: children,
    ),
  );
  @override
  Widget build(BuildContext context) {
    final order = c.order;
    final destructive = actions.where((a) => a.destructive).toList();
    final other = actions.where((a) => !a.destructive && a != primary).toList();
    return PopScope(
      canPop: !c.busy,
      child: Scaffold(
        backgroundColor: Color(0xFFF5F7FA).forSurface(context),
        appBar: AppBar(
          backgroundColor: Color(0xFFF5F7FA).forSurface(context),
          surfaceTintColor: Colors.transparent.forSurface(context),
          leading: IconButton(
            tooltip: 'Back to orders',
            onPressed: c.busy ? null : () => Navigator.pop(context),
            icon: const Icon(Icons.arrow_back_rounded),
          ),
          title: const Text(
            'Order details',
            style: TextStyle(fontSize: 19, fontWeight: FontWeight.w700),
          ),
          actions: [
            IconButton(
              tooltip: 'Refresh order',
              onPressed: c.loading || c.busy ? null : c.load,
              icon: Icon(
                Icons.refresh_rounded,
                color: KarryamColors.tealInk.forInk(context),
              ),
            ),
          ],
        ),
        body: SafeArea(
          top: false,
          bottom: false,
          child: c.loading && order == null
              ? const SingleChildScrollView(
                  padding: EdgeInsets.all(22),
                  child: PremiumSkeleton(detail: true, rows: 2),
                )
              : RefreshIndicator(
                  color: KarryamColors.tealInk.forInk(context),
                  onRefresh: c.load,
                  child: ListView(
                    physics: const AlwaysScrollableScrollPhysics(),
                    padding: const EdgeInsets.fromLTRB(20, 6, 20, 28),
                    children: [
                      if (c.loading) const ContentRefresh(),
                      Row(
                        children: [
                          Expanded(
                            child: Text(
                              '#${widget.id}',
                              style: TextStyle(
                                fontSize: 12,
                                fontWeight: FontWeight.w600,
                                color: KarryamColors.indigo.forInk(context),
                              ),
                            ),
                          ),
                          IconButton(
                            tooltip: 'Copy order reference',
                            icon: Icon(
                              Icons.copy_rounded,
                              size: 17,
                              color: KarryamColors.tealInk.forInk(context),
                            ),
                            onPressed: () {
                              Clipboard.setData(ClipboardData(text: widget.id));
                              showVendorToast(
                                context,
                                'Order reference copied',
                              );
                            },
                          ),
                        ],
                      ),
                      if (c.error != null)
                        DashboardEmptyState(
                          story: c.restricted
                              ? EmptyStory.restricted
                              : EmptyStory.unavailable,
                          title: c.stale
                              ? 'Refresh this order'
                              : c.restricted
                              ? 'Access required'
                              : 'Order unavailable',
                          message: c.error!,
                          action: 'Refresh order',
                          onAction: c.busy || c.loading ? null : c.load,
                        ),
                      if (order != null) ...[
                        TweenAnimationBuilder<double>(
                          tween: Tween(begin: 0, end: 1),
                          duration: MediaQuery.disableAnimationsOf(context)
                              ? Duration.zero
                              : const Duration(milliseconds: 420),
                          curve: Curves.easeOutCubic,
                          builder: (_, value, child) => Opacity(
                            opacity: value,
                            child: Transform.translate(
                              offset: Offset(0, 14 * (1 - value)),
                              child: child,
                            ),
                          ),
                          child: OrderHero(order: order),
                        ),
                        OrderSection(
                          title: 'Items in this order',
                          icon: order['domain'] == 'SHOP'
                              ? Icons.shopping_basket_outlined
                              : Icons.restaurant_rounded,
                          color:
                              (order['domain'] == 'SHOP'
                                      ? const Color(0xFF285EB0)
                                      : const Color(0xFFB05F29))
                                  .forInk(context),
                          child: Column(
                            children: [
                              for (final raw in order['lines'] as List? ?? [])
                                _item(jsonObject(raw), order),
                            ],
                          ),
                        ),
                        OrderSection(
                          title: 'Fulfilment',
                          icon: Icons.route_outlined,
                          color: Color(0xFF5264A0).forInk(context),
                          child: Column(
                            children: [
                              _detail(
                                Icons.local_shipping_outlined,
                                'Fulfilment',
                                dashboardLabel(order['fulfilment_mode']),
                              ),
                              _detail(
                                Icons.handshake_outlined,
                                'Delivery responsibility',
                                dashboardLabel(
                                  order['delivery_responsibility'],
                                ),
                              ),
                              if (order['created_at'] != null)
                                _detail(
                                  Icons.schedule_rounded,
                                  'Placed',
                                  _time(order['created_at']),
                                ),
                              if (order['cancel_reason_code']
                                      ?.toString()
                                      .isNotEmpty ==
                                  true)
                                _detail(
                                  Icons.info_outline,
                                  'Cancellation reason',
                                  order['cancel_reason_code'].toString(),
                                ),
                              if (order['rejection_reason']
                                      ?.toString()
                                      .isNotEmpty ==
                                  true)
                                _detail(
                                  Icons.info_outline,
                                  'Rejection reason',
                                  order['rejection_reason'].toString(),
                                ),
                            ],
                          ),
                        ),
                        if (order['domain'] == 'SHOP')
                          OrderSection(
                            title: 'Picking checklist',
                            icon: Icons.playlist_add_check_rounded,
                            color: Color(0xFF986013).forInk(context),
                            child: Column(
                              children: [
                                if (c.pickingError != null)
                                  DashboardEmptyState(
                                    story: c.pickingRestricted
                                        ? EmptyStory.restricted
                                        : EmptyStory.unavailable,
                                    title: c.pickingRestricted
                                        ? 'Picking access required'
                                        : 'Picking unavailable',
                                    message: c.pickingError!,
                                    action: 'Refresh picking',
                                    onAction: c.load,
                                  )
                                else if (c.picking.isEmpty)
                                  const Text(
                                    'There are no picking items for this order.',
                                  )
                                else
                                  for (final row in c.picking) _picking(row),
                              ],
                            ),
                          ),
                        if (order['delivery_responsibility'] ==
                                'PLATFORM_LOGISTICS' &&
                            [
                              'READY',
                              'READY_FOR_PICKUP',
                              'HANDED_OVER',
                              'PICKED_UP',
                              'DELIVERING',
                            ].contains(order['status']))
                          OrderSection(
                            title: 'Platform delivery',
                            icon: Icons.local_shipping_outlined,
                            child: Column(
                              crossAxisAlignment: CrossAxisAlignment.start,
                              children: [
                                const Text(
                                  'The delivery provider controls collection and delivery progress. Refresh for the latest update.',
                                ),
                                if (order['status'] == 'READY' &&
                                    order['logistics_mission_ref']
                                            ?.toString()
                                            .isNotEmpty ==
                                        true)
                                  Padding(
                                    padding: const EdgeInsets.only(top: 14),
                                    child: OutlinedButton.icon(
                                      onPressed: c.loading || c.busy
                                          ? null
                                          : _handover,
                                      icon: const Icon(
                                        Icons.verified_user_outlined,
                                      ),
                                      label: const Text(
                                        'Verify pickup handover',
                                      ),
                                    ),
                                  ),
                              ],
                            ),
                          ),
                        OrderSection(
                          title: 'Preparation',
                          icon: Icons.soup_kitchen_outlined,
                          child: OutlinedButton.icon(
                            onPressed: c.busy || c.loading || c.stale
                                ? null
                                : () async {
                                    await Navigator.push(
                                      context,
                                      setupRoute<void>(
                                        OrderReadinessScreen(
                                          runtime: widget.runtime,
                                          order: order,
                                        ),
                                      ),
                                    );
                                    if (mounted) await c.load();
                                  },
                            icon: const Icon(Icons.timelapse_rounded),
                            label: const Text('Preparation and ready time'),
                          ),
                        ),
                        OrderSection(
                          title: 'Order support',
                          icon: Icons.support_agent_rounded,
                          child: OutlinedButton.icon(
                            onPressed: c.busy || c.loading || c.stale
                                ? null
                                : () => Navigator.push(
                                    context,
                                    setupRoute<void>(
                                      OrderSupportScreen(
                                        runtime: widget.runtime,
                                        order: order,
                                      ),
                                    ),
                                  ),
                            icon: const Icon(Icons.forum_outlined),
                            label: const Text('Reports and issues'),
                          ),
                        ),
                        if (other.isNotEmpty)
                          OrderSection(
                            title: 'Available actions',
                            icon: Icons.touch_app_outlined,
                            child: Column(
                              children: [
                                for (final action in other) _actionRow(action),
                              ],
                            ),
                          ),
                      ],
                    ],
                  ),
                ),
        ),
        bottomNavigationBar: order == null || actions.isEmpty
            ? null
            : Container(
                decoration: BoxDecoration(
                  color: Colors.white.forSurface(context),
                  border: Border(
                    top: BorderSide(
                      color: Color(0xFFE5EAEE).forOutline(context),
                    ),
                  ),
                ),
                child: SafeArea(
                  top: false,
                  child: Padding(
                    padding: const EdgeInsets.fromLTRB(20, 14, 20, 10),
                    child: Column(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        if (primary != null)
                          PrimaryAction(
                            label: primary!.label,
                            busy: c.busy,
                            onPressed: _can(primary!)
                                ? () => _confirm(primary!)
                                : null,
                          ),
                        if (destructive.isNotEmpty)
                          Padding(
                            padding: EdgeInsets.only(
                              top: primary == null ? 0 : 6,
                            ),
                            child: Wrap(
                              alignment: WrapAlignment.center,
                              spacing: 12,
                              children: [
                                for (final action in destructive)
                                  TextButton.icon(
                                    style: TextButton.styleFrom(
                                      foregroundColor: (KarryamColors.error)
                                          .forInk(context),
                                    ),
                                    onPressed: _can(action)
                                        ? () => _confirm(action)
                                        : null,
                                    icon: Icon(
                                      orderActionIcon(action.route),
                                      size: 17,
                                    ),
                                    label: Text(
                                      action.label,
                                      style: const TextStyle(
                                        fontWeight: FontWeight.w600,
                                        fontSize: 12,
                                      ),
                                    ),
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
    );
  }

  bool _can(OrderAction action) =>
      c.can('/api/v1/vendor/commerce/orders/{vendorOrderId}/${action.route}');
  Widget _actionRow(OrderAction action) => Padding(
    padding: const EdgeInsets.only(bottom: 8),
    child: Material(
      color: (KarryamColors.mint.withValues(alpha: .5)).forSurface(context),
      borderRadius: BorderRadius.circular(16),
      child: InkWell(
        borderRadius: BorderRadius.circular(16),
        onTap: _can(action) ? () => _confirm(action) : null,
        child: Padding(
          padding: const EdgeInsets.all(14),
          child: Row(
            children: [
              Icon(
                orderActionIcon(action.route),
                color:
                    (_can(action) ? KarryamColors.tealInk : KarryamColors.gray)
                        .forInk(context),
                size: 23,
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      action.label,
                      style: const TextStyle(fontWeight: FontWeight.w700),
                    ),
                    const SizedBox(height: 3),
                    Text(
                      orderActionHint(action.route),
                      style: const TextStyle(fontSize: 11),
                    ),
                  ],
                ),
              ),
              const SizedBox(width: 8),
              Icon(
                Icons.chevron_right_rounded,
                color: KarryamColors.tealInk.forInk(context),
                size: 18,
              ),
            ],
          ),
        ),
      ),
    ),
  );
  Widget _detail(IconData icon, String label, String value) => Padding(
    padding: const EdgeInsets.only(bottom: 12),
    child: Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Icon(icon, size: 20, color: KarryamColors.tealInk.forInk(context)),
        const SizedBox(width: 12),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(label, style: const TextStyle(fontSize: 11)),
              Text(value, style: const TextStyle(fontWeight: FontWeight.w600)),
            ],
          ),
        ),
      ],
    ),
  );
  Widget _item(JsonObject line, JsonObject order) => Padding(
    padding: const EdgeInsets.only(bottom: 16),
    child: Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Container(
          width: 34,
          height: 34,
          alignment: Alignment.center,
          decoration: BoxDecoration(
            color: KarryamColors.mint.forSurface(context),
            borderRadius: BorderRadius.circular(10),
          ),
          child: Text(
            '${line['quantity'] ?? ''}',
            style: TextStyle(
              color: KarryamColors.tealInk.forInk(context),
              fontWeight: FontWeight.w700,
            ),
          ),
        ),
        const SizedBox(width: 12),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                line['item_name']?.toString() ?? '',
                style: const TextStyle(fontWeight: FontWeight.w600),
              ),
              if (line['unit_price_minor'] is int)
                Text(
                  '${dashboardMoney(line['unit_price_minor'], order['currency'])} each',
                  style: const TextStyle(fontSize: 11),
                ),
              if (line['quoted_total_minor'] is int)
                Padding(
                  padding: const EdgeInsets.only(top: 6),
                  child: Text(
                    dashboardMoney(
                      line['quoted_total_minor'],
                      order['currency'],
                    ),
                    style: const TextStyle(
                      fontSize: 13,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                ),
            ],
          ),
        ),
      ],
    ),
  );
  Widget _picking(JsonObject row) {
    final line = (c.order!['lines'] as List? ?? [])
        .map(jsonObject)
        .where((l) => l['id'] == row['OrderLineID'])
        .firstOrNull;
    final mutable = [
      'ACCEPTED',
      'PREPARING',
      'PICKING',
      'SUBSTITUTION_REQUIRED',
      'AWAITING_CUSTOMER',
    ].contains(c.order!['status']);
    return _card([
      Text(
        line?['item_name']?.toString() ?? row['OrderLineID']?.toString() ?? '',
        style: const TextStyle(fontWeight: FontWeight.w700),
      ),
      const SizedBox(height: 8),
      Text('${row['QuantityFound']} found / ${row['OrderedQuantity']} ordered'),
      if (row['QuantityFound'] is int &&
          row['OrderedQuantity'] is int &&
          (row['OrderedQuantity'] as int) > 0)
        Padding(
          padding: const EdgeInsets.symmetric(vertical: 12),
          child: ClipRRect(
            borderRadius: BorderRadius.circular(8),
            child: LinearProgressIndicator(
              value:
                  ((row['QuantityFound'] as int) /
                          (row['OrderedQuantity'] as int))
                      .clamp(0.0, 1.0),
              minHeight: 6,
              backgroundColor: Color(0xFFE5EBEE).forSurface(context),
              color:
                  (row['State'] == 'BLOCKED'
                          ? const Color(0xFF986013)
                          : KarryamColors.tealInk)
                      .forInk(context),
            ),
          ),
        ),
      Text(
        dashboardLabel(row['State']),
        style: TextStyle(
          color:
              (row['State'] == 'BLOCKED'
                      ? const Color(0xFF9A650D)
                      : row['State'] == 'COMPLETE'
                      ? KarryamColors.tealInk
                      : KarryamColors.indigo)
                  .forInk(context),
        ),
      ),
      for (final raw in row['QualityPreferences'] as List? ?? [])
        Padding(
          padding: const EdgeInsets.only(top: 8),
          child: Text(
            '${dashboardLabel((raw as Map)['type'])}: ${raw['value']}',
          ),
        ),
      if (row['SubstitutionPolicy'] != null)
        Padding(
          padding: const EdgeInsets.only(top: 8),
          child: Text(
            'Replacement policy: ${dashboardLabel(row['SubstitutionPolicy'])}',
            style: const TextStyle(fontSize: 12),
          ),
        ),
      ..._savedEvidence(row),
      if (mutable &&
          [
            'SPECIFIC_REPLACEMENT',
            'SAME_BRAND',
            'SIMILAR_WITHIN_PRICE_TOLERANCE',
            'VENDOR_SUGGEST_AND_APPROVE',
            'CONTACT_FIRST',
          ].contains(row['SubstitutionPolicy']))
        TextButton(
          onPressed:
              c.can(
                '/api/v1/vendor/commerce/orders/{vendorOrderId}/lines/{lineId}/substitutions',
              )
              ? () => _substitute(row)
              : null,
          child: const Text('Suggest replacement'),
        ),
      if (mutable &&
          row['Measurement'] is Map &&
          ![
            'FINALIZED',
            'FINANCIAL_PENDING',
          ].contains(jsonObject(row['Measurement'])['state']))
        TextButton.icon(
          onPressed:
              c.can(
                '/api/v1/vendor/commerce/orders/{vendorOrderId}/lines/{lineId}/actual-quantity',
              )
              ? () => _measurement(row)
              : null,
          icon: const Icon(Icons.scale_outlined),
          label: const Text('Record measurement'),
        ),
      if (mutable)
        Wrap(
          spacing: 8,
          children: [
            TextButton(
              onPressed:
                  row['MeasurementMode'] == 'FIXED_UNIT' &&
                      c.can(
                        '/api/v1/vendor/commerce/orders/{vendorOrderId}/lines/{lineId}/availability',
                      )
                  ? () => _line(row, 'availability')
                  : null,
              child: const Text('Availability'),
            ),
            TextButton(
              onPressed:
                  c.can(
                    '/api/v1/vendor/commerce/orders/{vendorOrderId}/lines/{lineId}/quality',
                  )
                  ? () => _line(row, 'quality')
                  : null,
              child: const Text('Quality'),
            ),
          ],
        ),
    ]);
  }

  String _time(Object value) {
    final time = DateTime.tryParse(value.toString())?.toLocal();
    return time == null
        ? value.toString()
        : '${time.day}/${time.month}/${time.year} ${time.hour.toString().padLeft(2, '0')}:${time.minute.toString().padLeft(2, '0')}';
  }
}
