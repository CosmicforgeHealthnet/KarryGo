import '../../../core/contracts/vendor_contract.dart';

class OrderAction {
  const OrderAction(
    this.route,
    this.label,
    this.statuses, {
    this.destructive = false,
    this.executorOnly = false,
  });
  final String route, label;
  final Set<String> statuses;
  final bool destructive, executorOnly;
}

const orderActions = [
  OrderAction('accept', 'Accept order', {'CREATED', 'PAID', 'SENT_TO_VENDOR'}),
  OrderAction('preparing', 'Start preparing', {'CREATED', 'ACCEPTED'}),
  OrderAction('picking', 'Start picking', {'ACCEPTED', 'PREPARING'}),
  OrderAction('pack', 'Mark packed', {
    'ACCEPTED',
    'PREPARING',
    'PICKING',
    'SUBSTITUTION_REQUIRED',
    'AWAITING_CUSTOMER',
  }),
  OrderAction('ready', 'Mark ready', {'PREPARING'}),
  OrderAction('ready-for-pickup', 'Ready for pickup', {
    'PACKING',
    'PREPARING',
    'READY',
  }),
  OrderAction('handover', 'Mark handed over', {'READY'}, executorOnly: true),
  OrderAction('pickup', 'Confirm pickup', {
    'READY',
    'READY_FOR_PICKUP',
    'HANDED_OVER',
  }, executorOnly: true),
  OrderAction('delivering', 'Start delivery', {
    'PICKED_UP',
    'HANDED_OVER',
  }, executorOnly: true),
  OrderAction('delivered', 'Confirm delivered', {
    'PICKED_UP',
    'HANDED_OVER',
    'DELIVERING',
  }, executorOnly: true),
  OrderAction('complete', 'Complete order', {'DELIVERED'}),
  OrderAction('reject', 'Reject order', {
    'CREATED',
    'PAID',
    'SENT_TO_VENDOR',
    'ACCEPTED',
  }, destructive: true),
  OrderAction('cancel', 'Cancel order', {
    'CREATED',
    'PENDING_PAYMENT',
    'PAID',
    'SENT_TO_VENDOR',
    'ACCEPTED',
    'PREPARING',
    'PICKING',
    'SUBSTITUTION_REQUIRED',
    'AWAITING_CUSTOMER',
    'PACKING',
    'READY_FOR_PICKUP',
    'READY',
  }, destructive: true),
];

List<OrderAction> availableOrderActions(JsonObject order) {
  final version = order['version'];
  if (version is! int || version < 1) return const [];
  return orderActions
      .where(
        (action) =>
            action.statuses.contains(order['status']) &&
            (!action.executorOnly ||
                [
                  'VENDOR_OWN_DELIVERY',
                  'NONE',
                ].contains(order['delivery_responsibility'])),
      )
      .toList();
}
