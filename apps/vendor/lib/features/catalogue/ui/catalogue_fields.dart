import 'package:flutter/material.dart';
import '../state/catalogue_resource.dart';

enum CatalogueInput {
  text,
  paragraph,
  integer,
  money,
  codes,
  choice,
  multi,
  toggle,
  custom,
}

class CatalogueField {
  const CatalogueField(
    this.key,
    this.label, {
    this.type = CatalogueInput.text,
    this.required = false,
    this.maxLength,
    this.min = 0,
    this.options = const [],
    this.source,
    this.section = 'Details',
    this.builder,
    this.readOnly = false,
    this.allowClear = false,
  });
  final String key, label, section;
  final CatalogueInput type;
  final bool required, readOnly, allowClear;
  final int? maxLength;
  final int min;
  final List<String> options;
  final String? source;
  final Widget Function(BuildContext, Object?, ValueChanged<Object?>?)? builder;
}

const shopModels = [
  'SUPERMARKET',
  'GROCERY_SHOP',
  'FOODSTUFF_DEALER',
  'MARKET_TRADER',
  'WHOLESALER',
];
const packageLevels = [
  'EACH',
  'SACHET',
  'PACK',
  'ROLL',
  'BUNDLE',
  'TRAY',
  'BAG',
  'SACK',
  'CARTON',
  'CASE',
  'CRATE',
  'DRUM',
  'PALLET',
  'OTHER_CONTROLLED',
];
const productClasses = [
  'PACKAGED',
  'DRY_BULK',
  'FRESH_PRODUCE',
  'MEAT',
  'FISH',
  'FROZEN',
  'CHILLED',
  'DAIRY',
  'BEVERAGE',
  'BAKED_PACKAGED',
  'HOUSEHOLD_GROCERY',
  'LOCAL_FOODSTUFF',
  'OTHER',
];

List<CatalogueField> catalogueFields(
  CatalogueKind kind, {
  required bool creating,
}) {
  const name = CatalogueField('name', 'Name', required: true, maxLength: 160);
  const description = CatalogueField(
    'description',
    'Description',
    type: CatalogueInput.paragraph,
    maxLength: 4000,
  );
  return switch (kind) {
    CatalogueKind.menus => [
      name,
      const CatalogueField(
        'description',
        'Description',
        type: CatalogueInput.paragraph,
        maxLength: 2000,
      ),
      if (creating)
        const CatalogueField(
          'capability_type',
          'Business type',
          type: CatalogueInput.choice,
          required: true,
        ),
    ],
    CatalogueKind.sections => [
      name,
      const CatalogueField(
        'description',
        'Description',
        type: CatalogueInput.paragraph,
        maxLength: 2000,
      ),
    ],
    CatalogueKind.items => [
      name,
      description,
      const CatalogueField(
        'base_price_minor',
        'Base price',
        type: CatalogueInput.money,
        required: true,
        section: 'Pricing',
      ),
      const CatalogueField(
        'dietary_codes',
        'Dietary codes',
        type: CatalogueInput.codes,
        section: 'Food information',
      ),
      const CatalogueField(
        'allergen_codes',
        'Allergen codes',
        type: CatalogueInput.codes,
        section: 'Food information',
      ),
    ],
    CatalogueKind.foodVariants => [
      name,
      const CatalogueField(
        'price_override_minor',
        'Price override',
        type: CatalogueInput.money,
        section: 'Pricing',
      ),
    ],
    CatalogueKind.groups => [
      name,
      description,
      const CatalogueField(
        'min_select',
        'Minimum selections',
        type: CatalogueInput.integer,
        required: true,
        section: 'Selection rules',
      ),
      const CatalogueField(
        'max_select',
        'Maximum selections',
        type: CatalogueInput.integer,
        required: true,
        min: 1,
        section: 'Selection rules',
      ),
    ],
    CatalogueKind.modifiers => [
      name,
      const CatalogueField(
        'price_delta_minor',
        'Price adjustment',
        type: CatalogueInput.money,
        required: true,
        section: 'Pricing',
      ),
    ],
    CatalogueKind.categories => [
      const CatalogueField('name', 'Name', required: true, maxLength: 120),
      const CatalogueField('slug', 'Slug', maxLength: 160),
      description,
      const CatalogueField(
        'parent_id',
        'Parent category',
        source: 'parent_ref',
        type: CatalogueInput.choice,
      ),
      const CatalogueField(
        'position',
        'Position',
        type: CatalogueInput.integer,
      ),
    ],
    CatalogueKind.products => [
      const CatalogueField(
        'name',
        'Product name',
        required: true,
        maxLength: 200,
      ),
      description,
      const CatalogueField(
        'category_id',
        'Category',
        source: 'category_ref',
        type: CatalogueInput.choice,
        required: true,
      ),
      const CatalogueField(
        'product_class',
        'Product class',
        type: CatalogueInput.choice,
        required: true,
        options: productClasses,
      ),
      const CatalogueField(
        'brand',
        'Brand',
        maxLength: 120,
        section: 'Product information',
      ),
      const CatalogueField(
        'manufacturer',
        'Manufacturer',
        maxLength: 160,
        section: 'Product information',
      ),
      const CatalogueField(
        'origin_country',
        'Country of origin (two-letter code)',
        maxLength: 2,
        section: 'Product information',
      ),
      const CatalogueField(
        'regulatory_reference',
        'Regulatory reference',
        maxLength: 80,
        section: 'Product information',
      ),
      const CatalogueField(
        'manufacturer_code',
        'Manufacturer code',
        maxLength: 120,
        section: 'Product information',
      ),
      const CatalogueField(
        'operating_models',
        'Operating models',
        type: CatalogueInput.multi,
        options: shopModels,
        section: 'Selling',
      ),
    ],
    CatalogueKind.shopVariants => [
      if (!creating)
        const CatalogueField(
          'status',
          'Variant status',
          type: CatalogueInput.choice,
          required: true,
          options: ['ACTIVE', 'PAUSED'],
        ),
      const CatalogueField(
        'name',
        'Variant name',
        required: true,
        maxLength: 200,
      ),
      const CatalogueField('sku', 'SKU', maxLength: 64),
      const CatalogueField(
        'package_level',
        'Package',
        type: CatalogueInput.choice,
        required: true,
        options: packageLevels,
      ),
      const CatalogueField(
        'pack_quantity',
        'Units in pack',
        type: CatalogueInput.integer,
        min: 1,
        required: true,
      ),
      const CatalogueField(
        'retail_price_minor',
        'Retail price',
        type: CatalogueInput.money,
        section: 'Pricing',
      ),
      const CatalogueField(
        'wholesale_price_minor',
        'Wholesale price',
        type: CatalogueInput.money,
        section: 'Pricing',
      ),
      const CatalogueField(
        'net_weight_grams',
        'Net weight (g)',
        type: CatalogueInput.integer,
        section: 'Size & handling',
      ),
      const CatalogueField(
        'net_volume_ml',
        'Net volume (ml)',
        type: CatalogueInput.integer,
        section: 'Size & handling',
      ),
      const CatalogueField(
        'length_mm',
        'Length (mm)',
        type: CatalogueInput.integer,
        section: 'Size & handling',
      ),
      const CatalogueField(
        'width_mm',
        'Width (mm)',
        type: CatalogueInput.integer,
        section: 'Size & handling',
      ),
      const CatalogueField(
        'height_mm',
        'Height (mm)',
        type: CatalogueInput.integer,
        section: 'Size & handling',
      ),
      const CatalogueField(
        'temperature_zone',
        'Temperature',
        type: CatalogueInput.choice,
        options: ['AMBIENT', 'FRESH', 'CHILLED', 'FROZEN'],
        section: 'Size & handling',
      ),
      const CatalogueField(
        'consolidation_class',
        'Consolidation class',
        type: CatalogueInput.choice,
        options: [
          'GENERAL',
          'DRY_GOODS',
          'FRESH_PRODUCE',
          'RAW_PROTEIN',
          'FROZEN',
          'CHILLED',
          'LIQUID',
          'FRAGILE',
          'BULK_LOAD',
        ],
        section: 'Size & handling',
      ),
      const CatalogueField(
        'separation_group',
        'Separation group',
        maxLength: 60,
        section: 'Size & handling',
      ),
      const CatalogueField(
        'stackable',
        'Stackable',
        type: CatalogueInput.toggle,
        section: 'Size & handling',
      ),
      const CatalogueField(
        'large_load',
        'Large load',
        type: CatalogueInput.toggle,
        section: 'Size & handling',
      ),
      const CatalogueField(
        'handling_flags',
        'Handling requirements',
        type: CatalogueInput.multi,
        options: [
          'FRAGILE',
          'UPRIGHT_REQUIRED',
          'CRUSH_SENSITIVE',
          'LEAK_RISK',
          'LIQUID',
          'ODOR_SENSITIVE',
          'RAW_PROTEIN',
          'KEEP_DRY',
          'KEEP_COLD',
          'STACK_RESTRICTED',
        ],
        section: 'Size & handling',
      ),
    ],
    CatalogueKind.listings => [
      if (!creating)
        const CatalogueField(
          'status',
          'Listing status',
          type: CatalogueInput.choice,
          required: true,
          options: ['ACTIVE', 'INACTIVE'],
        ),
      if (creating)
        const CatalogueField(
          'variant_id',
          'Product variant',
          type: CatalogueInput.choice,
          required: true,
        ),
      const CatalogueField(
        'retail_enabled',
        'Sell retail',
        type: CatalogueInput.toggle,
        section: 'Selling',
      ),
      const CatalogueField(
        'wholesale_enabled',
        'Sell wholesale',
        type: CatalogueInput.toggle,
        section: 'Selling',
      ),
      const CatalogueField(
        'retail_override',
        'Retail price override',
        source: 'retail_price_override_minor',
        type: CatalogueInput.money,
        section: 'Pricing',
      ),
      const CatalogueField(
        'wholesale_override',
        'Wholesale price override',
        source: 'wholesale_price_override_minor',
        type: CatalogueInput.money,
        section: 'Pricing',
      ),
      const CatalogueField(
        'minimum_order_qty',
        'Minimum order quantity',
        source: 'minimum_order_quantity',
        type: CatalogueInput.integer,
        min: 1,
        section: 'Order rules',
      ),
      const CatalogueField(
        'order_multiple',
        'Order multiple',
        type: CatalogueInput.integer,
        min: 1,
        section: 'Order rules',
      ),
      const CatalogueField(
        'maximum_order_qty',
        'Maximum order quantity',
        source: 'maximum_order_quantity',
        type: CatalogueInput.integer,
        min: 1,
        section: 'Order rules',
      ),
      const CatalogueField(
        'auto_hide_unavailable',
        'Hide when unavailable',
        source: 'auto_hide_when_unavailable',
        type: CatalogueInput.toggle,
        section: 'Selling',
      ),
      const CatalogueField(
        'logistics_available',
        'Logistics available',
        type: CatalogueInput.toggle,
        section: 'Selling',
      ),
    ],
  };
}

int? catalogueMinor(String input) {
  final match = RegExp(r'^(-?)(\d+)(?:\.(\d{1,2}))?$').firstMatch(input.trim());
  if (match == null) return null;
  final whole = int.tryParse(match[2]!);
  if (whole == null) return null;
  final value = whole * 100 + int.parse((match[3] ?? '').padRight(2, '0'));
  return match[1] == '-' ? -value : value;
}
