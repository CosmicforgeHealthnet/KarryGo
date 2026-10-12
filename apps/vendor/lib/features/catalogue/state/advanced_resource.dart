import '../../../core/contracts/vendor_contract.dart';

enum AdvancedKind {
  schedules,
  availability,
  source,
  override,
  prep,
  logistics,
  effective,
  measures,
  aliases,
  calibrations,
  sellUnits,
  packaging,
  fresh,
  identifiers,
  quantityBreaks,
  listingUnits,
  models,
  foodHistory,
  shopHistory,
}

class AdvancedResource {
  const AdvancedResource(
    this.kind, {
    this.parameters = const {},
    this.parent = const {},
    this.writable = true,
  });
  final AdvancedKind kind;
  final Map<String, String> parameters;
  final JsonObject parent;
  final bool writable;
  bool get food =>
      kind.index <= AdvancedKind.effective.index ||
      kind == AdvancedKind.foodHistory;
  bool get singleton => {
    AdvancedKind.availability,
    AdvancedKind.source,
    AdvancedKind.override,
    AdvancedKind.prep,
    AdvancedKind.logistics,
    AdvancedKind.effective,
    AdvancedKind.models,
  }.contains(kind);
  bool get readOnly => {
    AdvancedKind.effective,
    AdvancedKind.foodHistory,
    AdvancedKind.shopHistory,
  }.contains(kind);
  bool get paginated => {
    AdvancedKind.measures,
    AdvancedKind.foodHistory,
    AdvancedKind.shopHistory,
  }.contains(kind);
  String get title => switch (kind) {
    AdvancedKind.schedules => 'Menu schedules',
    AdvancedKind.availability => 'Item availability',
    AdvancedKind.source => 'Availability source',
    AdvancedKind.override => 'Branch overrides',
    AdvancedKind.prep => 'Preparation time',
    AdvancedKind.logistics => 'Delivery handling',
    AdvancedKind.effective => 'Selling status',
    AdvancedKind.measures => 'Trade measures',
    AdvancedKind.aliases => 'Measure names',
    AdvancedKind.calibrations => 'Measure calibrations',
    AdvancedKind.sellUnits => 'Sell units',
    AdvancedKind.packaging => 'Package relationships',
    AdvancedKind.fresh => 'Fresh options',
    AdvancedKind.identifiers => 'Product identifiers',
    AdvancedKind.quantityBreaks => 'Quantity pricing',
    AdvancedKind.listingUnits => 'Branch sell units',
    AdvancedKind.models => 'Branch selling models',
    AdvancedKind.foodHistory || AdvancedKind.shopHistory => 'Price history',
  };
  String get singular => switch (kind) {
    AdvancedKind.schedules => 'schedule',
    AdvancedKind.measures => 'measure',
    AdvancedKind.aliases => 'measure name',
    AdvancedKind.calibrations => 'calibration',
    AdvancedKind.sellUnits => 'sell unit',
    AdvancedKind.packaging => 'package relationship',
    AdvancedKind.fresh => 'fresh option',
    AdvancedKind.identifiers => 'identifier',
    AdvancedKind.quantityBreaks => 'price tier',
    AdvancedKind.listingUnits => 'branch sell unit',
    _ => title.toLowerCase(),
  };
  String get description => switch (kind) {
    AdvancedKind.schedules => 'Set the days and times this menu is offered.',
    AdvancedKind.availability =>
      'Manage availability without changing publication.',
    AdvancedKind.source => 'Choose how availability is checked.',
    AdvancedKind.override =>
      'Use branch-specific price and preparation settings.',
    AdvancedKind.prep => 'Set the kitchen preparation range.',
    AdvancedKind.logistics => 'Describe how this item should be handled.',
    AdvancedKind.effective => 'Check whether this item can currently be sold.',
    AdvancedKind.measures => 'Shared units for your Shop catalogue.',
    AdvancedKind.aliases => 'Add the names your customers use.',
    AdvancedKind.calibrations =>
      'Record approximate weight or volume for a measure.',
    AdvancedKind.sellUnits => 'Define the portions customers can buy.',
    AdvancedKind.packaging => 'Set the actual ratio between two packs.',
    AdvancedKind.fresh => 'Offer the choices appropriate to this product.',
    AdvancedKind.identifiers => 'Add SKUs, barcodes and trade identifiers.',
    AdvancedKind.quantityBreaks => 'Set unit prices for quantity ranges.',
    AdvancedKind.listingUnits =>
      "Set this branch's selling and pricing overrides.",
    AdvancedKind.models =>
      'Configure the selling models admitted for this branch.',
    _ => 'Review recorded catalogue price changes.',
  };
  String get path =>
      '/api/v1/vendor/${switch (kind) {
        AdvancedKind.schedules => 'food/menus/{menuId}/schedules',
        AdvancedKind.availability => 'food/items/{itemId}/availability',
        AdvancedKind.source => 'food/items/{itemId}/availability-source',
        AdvancedKind.override => 'food/items/{itemId}/branch-override',
        AdvancedKind.prep => 'food/items/{itemId}/prep-profile',
        AdvancedKind.logistics => 'food/items/{itemId}/logistics-profile',
        AdvancedKind.effective => 'food/items/{itemId}/availability/effective',
        AdvancedKind.measures => 'shop/measurements',
        AdvancedKind.aliases => 'shop/measurements/{measureId}/aliases',
        AdvancedKind.calibrations => 'shop/products/{productId}/measure-calibrations',
        AdvancedKind.sellUnits => 'shop/products/{productId}/sell-units',
        AdvancedKind.packaging => 'shop/products/{productId}/packaging',
        AdvancedKind.fresh => 'shop/products/{productId}/fresh-attributes',
        AdvancedKind.identifiers => 'shop/variants/{variantId}/identifiers',
        AdvancedKind.quantityBreaks => 'shop/listings/{listingId}/quantity-breaks',
        AdvancedKind.listingUnits => 'shop/listings/{listingId}',
        AdvancedKind.models => 'shop/branches/{branchId}/operating-models',
        AdvancedKind.foodHistory => 'food/items/{itemId}/pricing/history',
        AdvancedKind.shopHistory => 'shop/price-history',
      }}';
  String get idKey => switch (kind) {
    AdvancedKind.schedules => 'scheduleId',
    AdvancedKind.measures => 'measureId',
    AdvancedKind.aliases => 'aliasId',
    AdvancedKind.calibrations => 'calibrationId',
    AdvancedKind.sellUnits => 'sellUnitId',
    AdvancedKind.packaging => 'relationId',
    AdvancedKind.fresh => 'attributeId',
    AdvancedKind.identifiers => 'identifierId',
    AdvancedKind.quantityBreaks => 'breakId',
    AdvancedKind.listingUnits => 'listingUnitId',
    _ => 'id',
  };
  String get detail => singleton
      ? path
      : kind == AdvancedKind.listingUnits
      ? '$path/sell-units/{$idKey}'
      : '$path/{$idKey}';
  String get saveMethod =>
      singleton ||
          {AdvancedKind.fresh, AdvancedKind.listingUnits}.contains(kind)
      ? 'PUT'
      : 'POST';
  String get editMethod =>
      singleton ||
          {AdvancedKind.fresh, AdvancedKind.listingUnits}.contains(kind)
      ? 'PUT'
      : 'PATCH';
  String get editPath => kind == AdvancedKind.fresh ? path : detail;
  String get removePath => kind == AdvancedKind.models
      ? '$path/archive'
      : kind == AdvancedKind.aliases || singleton
      ? detail
      : '$detail/archive';
  String get removeMethod => kind == AdvancedKind.models
      ? 'POST'
      : kind == AdvancedKind.aliases || singleton
      ? 'DELETE'
      : 'POST';
  bool get canEdit =>
      !{
        AdvancedKind.aliases,
        AdvancedKind.packaging,
        AdvancedKind.identifiers,
      }.contains(kind) &&
      !readOnly;
  bool get canRemove => !{
    AdvancedKind.availability,
    AdvancedKind.effective,
    AdvancedKind.listingUnits,
    AdvancedKind.foodHistory,
    AdvancedKind.shopHistory,
  }.contains(kind);
  bool get smallEditor => {
    AdvancedKind.availability,
    AdvancedKind.source,
    AdvancedKind.override,
    AdvancedKind.prep,
    AdvancedKind.aliases,
    AdvancedKind.identifiers,
    AdvancedKind.packaging,
    AdvancedKind.listingUnits,
    AdvancedKind.quantityBreaks,
  }.contains(kind);
  Map<String, String> params([JsonObject? row]) => {
    ...parameters,
    if (row != null && !singleton) idKey: row['id'].toString(),
  };
}
