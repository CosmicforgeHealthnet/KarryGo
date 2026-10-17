import '../../../core/contracts/vendor_contract.dart';
import '../state/advanced_resource.dart';
import '../state/advanced_vocabulary.dart';
import 'catalogue_fields.dart';
import '../../dashboard/ui/dashboard_format.dart';
import 'advanced_inputs.dart';

CatalogueField _choice(
  String key,
  String label,
  List<String> options, {
  bool required = true,
  String? source,
  String section = 'Details',
  bool readOnly = false,
}) => CatalogueField(
  key,
  label,
  type: CatalogueInput.choice,
  required: required,
  options: options,
  source: source,
  section: section,
  readOnly: readOnly,
);
CatalogueField _number(
  String key,
  String label, {
  bool required = false,
  int min = 0,
  String? source,
  String section = 'Details',
  bool allowClear = false,
}) => CatalogueField(
  key,
  label,
  allowClear: allowClear,
  type: CatalogueInput.integer,
  required: required,
  min: min,
  source: source,
  section: section,
);
CatalogueField _money(
  String key,
  String label, {
  bool required = false,
  String? source,
  String section = 'Pricing',
}) => CatalogueField(
  key,
  label,
  type: CatalogueInput.money,
  required: required,
  source: source,
  section: section,
);
CatalogueField _toggle(
  String key,
  String label, {
  String? source,
  String section = 'Details',
}) => CatalogueField(
  key,
  label,
  type: CatalogueInput.toggle,
  source: source,
  section: section,
);
CatalogueField _date(
  String key,
  String label, {
  bool clock = false,
  bool timestamp = false,
  bool required = false,
}) => CatalogueField(
  key,
  label,
  type: CatalogueInput.custom,
  required: required,
  section: 'Timing',
  builder: (context, value, onChanged) => CatalogueTemporalInput(
    label: label,
    value: value,
    onChanged: onChanged,
    clock: clock,
    timestamp: timestamp,
    required: required,
  ),
);
List<CatalogueField> advancedFields(
  AdvancedResource r, {
  JsonObject? record,
  String? freshType,
  List<String>? eligibleModels,
}) {
  final create = record == null;
  return switch (r.kind) {
    AdvancedKind.schedules => [
      _choice('label', 'Schedule', [
        'BREAKFAST',
        'LUNCH',
        'DINNER',
        'SEASONAL',
        'CUSTOM',
      ]),
      CatalogueField(
        'days_of_week',
        'Days',
        type: CatalogueInput.custom,
        required: true,
        section: 'Timing',
        builder: (context, value, onChanged) =>
            CatalogueWeekdays(value: value, onChanged: onChanged),
      ),
      _date('start_local_time', 'Starts', clock: true, required: true),
      _date('end_local_time', 'Ends', clock: true, required: true),
      _date('effective_start_date', 'First date'),
      _date('effective_end_date', 'Last date'),
    ],
    AdvancedKind.availability => [
      _choice('status', 'Availability', ['AVAILABLE', 'SOLD_OUT', 'PAUSED']),
      const CatalogueField('reason', 'Reason', maxLength: 280),
      _date('effective_until', 'Restore at', timestamp: true),
    ],
    AdvancedKind.source => [
      _choice('mode', 'Availability source', ['MANUAL', 'INVENTORY_LINKED']),
      _choice('inventory_recipe_ref', 'Recipe', [], required: false),
    ],
    AdvancedKind.override => [
      _money('base_price_override_minor', 'Branch price'),
      _number(
        'prep_min_minutes',
        'Minimum preparation (minutes)',
        section: 'Preparation',
        allowClear: true,
      ),
      _number(
        'prep_max_minutes',
        'Maximum preparation (minutes)',
        section: 'Preparation',
        allowClear: true,
      ),
    ],
    AdvancedKind.prep => [
      _number('prep_min_minutes', 'Minimum (minutes)', required: true),
      _number('prep_max_minutes', 'Maximum (minutes)', required: true),
    ],
    AdvancedKind.logistics => [
      _choice('temperature_class', 'Temperature', [
        'AMBIENT',
        'HOT',
        'COLD',
        'FROZEN',
      ]),
      _choice('package_size_class', 'Package size', [
        'SMALL',
        'MEDIUM',
        'LARGE',
      ]),
      _number('approx_weight_grams', 'Approximate weight (grams)', min: 1),
      _toggle('spill_risk', 'Spill risk'),
      _toggle('fragile', 'Fragile'),
      const CatalogueField(
        'vehicle_hints',
        'Suitable vehicle hints',
        type: CatalogueInput.multi,
        options: ['MOTORCYCLE', 'CAR', 'VAN', 'TRUCK'],
      ),
    ],
    AdvancedKind.measures => [
      if (create) ...[
        const CatalogueField('code', 'Code', required: true, maxLength: 40),
        _choice(
          'family',
          'Measurement family',
          catalogueVocabularyList('MeasurementFamilies'),
          source: 'measurement_family',
        ),
        _number('base_quantity', 'Exact base quantity', min: 1),
      ],
      const CatalogueField(
        'display_name',
        'Display name',
        required: true,
        maxLength: 80,
      ),
      const CatalogueField('region_code', 'Region code', maxLength: 16),
    ],
    AdvancedKind.aliases => [
      const CatalogueField('alias', 'Name', required: true, maxLength: 80),
      const CatalogueField('region_code', 'Region code', maxLength: 16),
    ],
    AdvancedKind.calibrations => [
      if (create) ...[
        _choice(
          'branch_id',
          'Branch',
          [],
          required: false,
          source: 'branch_ref',
        ),
        _choice(
          'variant_id',
          'Variant',
          [],
          required: false,
          source: 'variant_ref',
        ),
        _choice('measure_id', 'Measure', [], source: 'measure_ref'),
      ],
      _number(
        'weight_grams',
        'Approximate weight (grams)',
        min: 1,
        source: 'approximate_weight_grams',
      ),
      _number(
        'volume_ml',
        'Approximate volume (ml)',
        min: 1,
        source: 'approximate_volume_ml',
      ),
      _number('tolerance_bps', 'Tolerance (basis points)'),
      _choice(
        'source',
        'Calibration source',
        catalogueVocabularyList('CalibrationSources'),
        source: 'calibration_source',
      ),
    ],
    AdvancedKind.sellUnits => [
      if (create) ...[
        _choice('variant_id', 'Variant', [], source: 'variant_ref'),
        _choice('measure_id', 'Measure', [], source: 'measure_ref'),
        _number('fraction_numerator', 'Fraction numerator', min: 1),
        _number('fraction_denominator', 'Fraction denominator', min: 1),
      ],
      const CatalogueField(
        'display_name',
        'Customer label',
        required: true,
        maxLength: 80,
      ),
      _choice(
        'measurement_mode',
        'Measurement mode',
        catalogueVocabularyList('MeasurementModes'),
      ),
      _choice(
        'calibration_id',
        'Calibration',
        [],
        required: false,
        source: 'calibration_ref',
      ),
      _number(
        'estimated_base_qty',
        'Estimated base quantity',
        min: 1,
        source: 'estimated_base_quantity',
      ),
      _number(
        'underfill_bps',
        'Underfill tolerance (basis points)',
        source: 'underfill_tolerance_bps',
      ),
      _number(
        'overfill_bps',
        'Overfill tolerance (basis points)',
        source: 'overfill_tolerance_bps',
      ),
      _toggle(
        'measurement_required',
        'Actual measurement required',
        source: 'actual_measurement_required',
      ),
      _number('display_precision', 'Decimal precision'),
      _money(
        'price_minor',
        'Retail price',
        required: record?['price_minor'] != null,
      ),
      _money(
        'wholesale_price_minor',
        'Wholesale price',
        required: record?['wholesale_price_minor'] != null,
      ),
      _toggle('retail_enabled', 'Retail enabled', section: 'Selling'),
      _toggle('wholesale_enabled', 'Wholesale enabled', section: 'Selling'),
      _number(
        'minimum_order_qty',
        'Minimum order',
        min: 1,
        source: 'minimum_order_quantity',
        section: 'Selling',
      ),
      _number('order_multiple', 'Order multiple', min: 1, section: 'Selling'),
      _number(
        'maximum_order_qty',
        'Maximum order',
        min: 1,
        source: 'maximum_order_quantity',
        section: 'Selling',
      ),
      if (!create)
        _choice('status', 'Status', ['ACTIVE', 'PAUSED'], section: 'Selling'),
    ],
    AdvancedKind.packaging => [
      _choice(
        'parent_variant_id',
        'Outer pack',
        [],
        source: 'parent_variant_ref',
      ),
      _choice(
        'child_variant_id',
        'Inner pack',
        [],
        source: 'child_variant_ref',
      ),
      _number(
        'units_per_parent',
        'Inner units per outer pack',
        required: true,
        min: 1,
      ),
    ],
    AdvancedKind.fresh => [
      _choice('attribute_type', 'Attribute', [
        freshType ?? record?['attribute_type']?.toString() ?? '',
      ], readOnly: true),
      _toggle('required', 'A choice is required', source: 'selection_required'),
      _number('position', 'Position'),
      CatalogueField(
        'options',
        'Customer choices',
        type: CatalogueInput.custom,
        required: true,
        builder: (context, value, onChanged) => CatalogueFreshOptions(
          value: value,
          onChanged: onChanged,
          allowed:
              catalogueVocabularyMap('FreshAttributeOptions')[freshType ??
                  record?['attribute_type']] ??
              [],
        ),
      ),
    ],
    AdvancedKind.identifiers => [
      _choice(
        'identifier_type',
        'Type',
        catalogueVocabularyList('IdentifierTypes'),
      ),
      const CatalogueField(
        'identifier_value',
        'Identifier',
        required: true,
        maxLength: 120,
      ),
      _choice('package_level', 'Package level', packageLevels, required: false),
    ],
    AdvancedKind.quantityBreaks => [
      if (create)
        _choice(
          'sell_unit_id',
          'Sell unit',
          [],
          required: false,
          source: 'sell_unit_ref',
        ),
      _number('min_quantity', 'From quantity', required: true, min: 1),
      _number('max_quantity', 'Through quantity', min: 1),
      _money('unit_price_minor', 'Price per unit', required: true),
      _toggle('wholesale_only', 'Wholesale only'),
      _number('lead_time_days', 'Lead time (days)'),
    ],
    AdvancedKind.listingUnits => [
      _toggle('enabled', 'Sell this unit at this branch'),
      _choice(
        'calibration_id',
        'Branch calibration',
        [],
        required: false,
        source: 'calibration_ref',
      ),
      _money(
        'price_override',
        'Retail price override',
        required: record?['price_override_minor'] != null,
        source: 'price_override_minor',
      ),
      _money(
        'wholesale_override',
        'Wholesale price override',
        required: record?['wholesale_price_override_minor'] != null,
        source: 'wholesale_price_override_minor',
      ),
    ],
    AdvancedKind.models => [
      CatalogueField(
        'models',
        'Selling models',
        type: CatalogueInput.multi,
        options: eligibleModels ?? shopModels,
      ),
      _choice(
        'primary_model',
        'Primary model',
        eligibleModels ?? shopModels,
        required: false,
      ),
    ],
    _ => [],
  };
}

JsonObject advancedPresets(AdvancedResource r, {String? freshType}) =>
    switch (r.kind) {
      AdvancedKind.schedules => {
        'label': 'LUNCH',
        'days_of_week': [1, 2, 3, 4, 5],
        'start_local_time': '11:00',
        'end_local_time': '16:00',
      },
      AdvancedKind.availability => {'status': 'AVAILABLE', 'reason': ''},
      AdvancedKind.source => {'mode': 'MANUAL'},
      AdvancedKind.override => {'prep_min_minutes': 10, 'prep_max_minutes': 20},
      AdvancedKind.prep => {'prep_min_minutes': 10, 'prep_max_minutes': 20},
      AdvancedKind.logistics => {
        'temperature_class': 'HOT',
        'package_size_class': 'SMALL',
        'approx_weight_grams': 500,
        'spill_risk': false,
        'fragile': false,
        'vehicle_hints': ['MOTORCYCLE', 'CAR'],
      },
      AdvancedKind.measures => {
        'code': 'KITCHEN_BOWL',
        'display_name': 'Kitchen bowl',
        'family': 'LOCAL_CONTAINER',
        'region_code': 'NG',
      },
      AdvancedKind.aliases => {'alias': 'Market bowl', 'region_code': 'NG'},
      AdvancedKind.calibrations => {
        'weight_grams': 1000,
        'tolerance_bps': 500,
        'source': 'VENDOR_MEASURED',
      },
      AdvancedKind.sellUnits => {
        'display_name': 'Customer portion',
        'fraction_numerator': 1,
        'fraction_denominator': 1,
        'measurement_mode': 'FIXED',
        'underfill_bps': 0,
        'overfill_bps': 0,
        'display_precision': 0,
        'price_minor': 250000,
        'retail_enabled': true,
        'wholesale_enabled': false,
        'minimum_order_qty': 1,
        'order_multiple': 1,
      },
      AdvancedKind.packaging => {'units_per_parent': 6},
      AdvancedKind.fresh => {
        'attribute_type': freshType,
        'required': false,
        'position': 0,
        'options': [
          {
            'value': catalogueVocabularyMap(
              'FreshAttributeOptions',
            )[freshType]!.first,
            'display_name': dashboardLabel(
              catalogueVocabularyMap('FreshAttributeOptions')[freshType]!.first,
            ),
            'is_default': true,
          },
        ],
      },
      AdvancedKind.identifiers => {
        'identifier_type': 'INTERNAL_SKU',
        'identifier_value': 'KARRYAM-PACK-01',
        'package_level': 'EACH',
      },
      AdvancedKind.quantityBreaks => {
        'min_quantity': 6,
        'unit_price_minor': 225000,
        'wholesale_only': true,
        'lead_time_days': 1,
      },
      AdvancedKind.models => {
        'models': ['GROCERY_SHOP'],
        'primary_model': 'GROCERY_SHOP',
      },
      _ => {},
    };
JsonObject normalizeAdvancedRecord(AdvancedKind kind, JsonObject record) {
  final row = Map<String, dynamic>.of(record);
  if (kind == AdvancedKind.fresh) {
    row['options'] = (row['options'] as List? ?? []).map((o) {
      final v = jsonObject(o);
      return {
        'value': v['option_value'] ?? v['value'],
        'display_name': v['display_name'],
        'is_default': v['is_default'] == true,
      };
    }).toList();
  }
  if (kind == AdvancedKind.models) {
    row['models'] = (row['models'] as List? ?? [])
        .map((m) => m is Map ? m['code'] : m)
        .toList();
  }
  return row;
}

JsonObject advancedPayload(
  AdvancedResource r,
  JsonObject body,
  JsonObject? record,
) {
  final out = Map<String, dynamic>.of(body);
  if (r.kind == AdvancedKind.schedules && record != null) {
    for (final key in ['effective_start_date', 'effective_end_date']) {
      if (!out.containsKey(key) && record[key] != null) {
        out['clear_$key'] = true;
      }
    }
  }
  if (r.kind == AdvancedKind.override && record != null) {
    if (!out.containsKey('base_price_override_minor') &&
        record['base_price_override_minor'] != null) {
      out['clear_base_price_override'] = true;
    }
    if (!out.containsKey('prep_min_minutes') &&
        !out.containsKey('prep_max_minutes') &&
        record['prep_min_minutes'] != null) {
      out['clear_prep_override'] = true;
    }
  }
  if (r.kind == AdvancedKind.source && out['mode'] == 'MANUAL') {
    out['inventory_recipe_ref'] = null;
  }
  if (r.kind == AdvancedKind.measures) {
    if (out['code'] is String) {
      out['code'] = out['code'].toString().toUpperCase();
    }
    if (out['region_code'] is String) {
      out['region_code'] = out['region_code'].toString().toUpperCase();
    }
  }
  if (record != null &&
      {'calibration_ref'}.any((k) => record[k] != null) &&
      !out.containsKey('calibration_id')) {
    out['calibration_id'] = '';
  }
  return out;
}

String? validateAdvanced(
  AdvancedResource r,
  JsonObject body,
  JsonObject? record,
) {
  switch (r.kind) {
    case AdvancedKind.schedules:
      if (body['start_local_time'] == body['end_local_time']) {
        return 'Start and end times must be different.';
      }
      if (body['effective_start_date'] != null &&
          body['effective_end_date'] != null &&
          body['effective_start_date'].toString().compareTo(
                body['effective_end_date'].toString(),
              ) >
              0) {
        return 'The last date must follow the first date.';
      }
    case AdvancedKind.availability:
      if (body['effective_until'] != null &&
          DateTime.parse(body['effective_until']).isBefore(DateTime.now())) {
        return 'Choose a restore time in the future.';
      }
    case AdvancedKind.source:
      if (body['mode'] == 'INVENTORY_LINKED' &&
          (body['inventory_recipe_ref']?.toString().trim().isEmpty ?? true)) {
        return 'Choose the recipe that controls this item\'s stock.';
      }
    case AdvancedKind.override:
    case AdvancedKind.prep:
      final min = body['prep_min_minutes'], max = body['prep_max_minutes'];
      if ((min == null) != (max == null)) {
        return 'Enter both preparation times or clear both.';
      }
      if (min != null && (min > 1440 || max > 1440 || min > max)) {
        return 'Use 0-1440 minutes, with the maximum at least the minimum.';
      }
    case AdvancedKind.logistics:
      if ((body['approx_weight_grams'] as int? ?? 0) > 200000) {
        return 'Weight cannot exceed 200000 grams.';
      }
    case AdvancedKind.measures:
      if (body['family'] != null) {
        final exact = ['MASS', 'VOLUME', 'COUNT'].contains(body['family']);
        if (exact && body['base_quantity'] == null) {
          return 'Enter the exact quantity in grams, millilitres or units.';
        }
        if (!exact && body['base_quantity'] != null) {
          return 'This family uses a product calibration, not a universal base quantity.';
        }
      }
    case AdvancedKind.calibrations:
      if (body['weight_grams'] == null && body['volume_ml'] == null) {
        return 'Enter an approximate weight or volume.';
      }
      if ((body['tolerance_bps'] as int? ?? 0) > 10000) {
        return 'Tolerance cannot exceed 10000 basis points.';
      }
    case AdvancedKind.sellUnits:
      if (body['retail_enabled'] != true && body['wholesale_enabled'] != true) {
        return 'Enable retail, wholesale or both.';
      }
      if ((body['fraction_numerator'] as int? ?? 1) > 1000000 ||
          (body['fraction_denominator'] as int? ?? 1) > 1000) {
        return 'Use a numerator up to 1000000 and denominator up to 1000.';
      }
      if ((body['display_precision'] as int? ?? 0) > 3) {
        return 'Decimal precision must be 0-3.';
      }
      for (final key in ['underfill_bps', 'overfill_bps']) {
        if ((body[key] as int? ?? 0) > 10000) {
          return 'Tolerance cannot exceed 10000 basis points.';
        }
      }
      if (body['maximum_order_qty'] != null &&
          body['maximum_order_qty'] < (body['minimum_order_qty'] ?? 1)) {
        return 'Maximum order must be at least the minimum.';
      }
    case AdvancedKind.packaging:
      if (body['parent_variant_id'] == body['child_variant_id']) {
        return 'Choose two different packs.';
      }
    case AdvancedKind.quantityBreaks:
      if (body['max_quantity'] != null &&
          body['max_quantity'] < body['min_quantity']) {
        return 'The upper quantity must be at least the lower quantity.';
      }
      if ((body['lead_time_days'] as int? ?? 0) > 365) {
        return 'Lead time must be 0-365 days.';
      }
    case AdvancedKind.models:
      final models = body['models'] as List? ?? [];
      if (models.isEmpty) return 'Choose at least one selling model.';
      if (body['primary_model'] != null &&
          !models.contains(body['primary_model'])) {
        return 'Choose a primary model from your enabled models.';
      }
    default:
      break;
  }
  return null;
}
