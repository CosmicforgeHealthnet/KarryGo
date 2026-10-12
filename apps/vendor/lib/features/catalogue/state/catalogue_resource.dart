import '../../../core/contracts/vendor_contract.dart';

enum CatalogueKind {
  menus,
  sections,
  items,
  foodVariants,
  groups,
  modifiers,
  categories,
  products,
  shopVariants,
  listings,
}

class CatalogueResource {
  const CatalogueResource(
    this.kind, {
    this.parameters = const {},
    this.branchId,
    this.writable = true,
  });
  final CatalogueKind kind;
  final Map<String, String> parameters;
  final String? branchId;
  final bool writable;
  bool get food => kind.index <= CatalogueKind.modifiers.index;
  String get title => switch (kind) {
    CatalogueKind.menus => 'Food menus',
    CatalogueKind.sections => 'Menu sections',
    CatalogueKind.items => 'Food items',
    CatalogueKind.foodVariants || CatalogueKind.shopVariants => 'Variants',
    CatalogueKind.groups => 'Modifier groups',
    CatalogueKind.modifiers => 'Modifiers',
    CatalogueKind.categories => 'Categories',
    CatalogueKind.products => 'Shop products',
    CatalogueKind.listings => 'Branch listings',
  };
  String get singular => switch (kind) {
    CatalogueKind.menus => 'menu',
    CatalogueKind.sections => 'section',
    CatalogueKind.items => 'item',
    CatalogueKind.foodVariants || CatalogueKind.shopVariants => 'variant',
    CatalogueKind.groups => 'modifier group',
    CatalogueKind.modifiers => 'modifier',
    CatalogueKind.categories => 'category',
    CatalogueKind.products => 'product',
    CatalogueKind.listings => 'listing',
  };
  String get collection =>
      '/api/v1/vendor/${switch (kind) {
        CatalogueKind.menus => 'food/menus',
        CatalogueKind.sections => 'food/menus/{menuId}/sections',
        CatalogueKind.items => 'food/menus/{menuId}/sections/{sectionId}/items',
        CatalogueKind.foodVariants => 'food/items/{itemId}/variants',
        CatalogueKind.groups => 'food/modifier-groups',
        CatalogueKind.modifiers => 'food/modifier-groups/{groupId}/modifiers',
        CatalogueKind.categories => 'shop/categories',
        CatalogueKind.products => 'shop/products',
        CatalogueKind.shopVariants => 'shop/products/{productId}/variants',
        CatalogueKind.listings => 'shop/branches/{branchId}/listings',
      }}';
  String get idKey => switch (kind) {
    CatalogueKind.menus => 'menuId',
    CatalogueKind.sections => 'sectionId',
    CatalogueKind.items => 'itemId',
    CatalogueKind.foodVariants || CatalogueKind.shopVariants => 'variantId',
    CatalogueKind.groups => 'groupId',
    CatalogueKind.modifiers => 'modifierId',
    CatalogueKind.categories => 'categoryId',
    CatalogueKind.products => 'productId',
    CatalogueKind.listings => 'listingId',
  };
  String get detail => switch (kind) {
    CatalogueKind.items => '/api/v1/vendor/food/items/{itemId}',
    CatalogueKind.listings => '/api/v1/vendor/shop/listings/{listingId}',
    _ => '$collection/{$idKey}',
  };
  String? get readDetail => switch (kind) {
    CatalogueKind.sections ||
    CatalogueKind.foodVariants ||
    CatalogueKind.modifiers => null,
    CatalogueKind.shopVariants => '/api/v1/vendor/shop/variants/{variantId}',
    _ => detail,
  };
  Map<String, String> detailParameters(String id) =>
      kind == CatalogueKind.items || kind == CatalogueKind.listings
      ? {idKey: id}
      : {...parameters, idKey: id};
  String get rowsKey => switch (kind) {
    CatalogueKind.menus => 'menus',
    CatalogueKind.sections => 'sections',
    CatalogueKind.foodVariants => 'variants',
    CatalogueKind.groups => 'modifier_groups',
    CatalogueKind.modifiers => 'modifiers',
    _ => 'items',
  };
  bool get paginated => [
    CatalogueKind.menus,
    CatalogueKind.items,
    CatalogueKind.groups,
    CatalogueKind.products,
    CatalogueKind.categories,
    CatalogueKind.listings,
  ].contains(kind);
  bool get reorderable => [
    CatalogueKind.sections,
    CatalogueKind.items,
    CatalogueKind.foodVariants,
    CatalogueKind.modifiers,
  ].contains(kind);
  CatalogueResource? child(JsonObject row) => switch (kind) {
    CatalogueKind.menus => CatalogueResource(
      CatalogueKind.sections,
      parameters: {'menuId': row['id'].toString()},
      branchId: branchId,
      writable: writable && row['status'] != 'ARCHIVED',
    ),
    CatalogueKind.sections => CatalogueResource(
      CatalogueKind.items,
      parameters: {...parameters, 'sectionId': row['id'].toString()},
      branchId: branchId,
      writable: writable && row['status'] != 'ARCHIVED',
    ),
    CatalogueKind.items => CatalogueResource(
      CatalogueKind.foodVariants,
      parameters: {'itemId': row['id'].toString()},
      branchId: branchId,
      writable: writable && row['status'] != 'ARCHIVED',
    ),
    CatalogueKind.groups => CatalogueResource(
      CatalogueKind.modifiers,
      parameters: {'groupId': row['id'].toString()},
      branchId: branchId,
      writable: writable && row['status'] != 'ARCHIVED',
    ),
    CatalogueKind.products => CatalogueResource(
      CatalogueKind.shopVariants,
      parameters: {'productId': row['id'].toString()},
      branchId: branchId,
      writable: writable && row['status'] != 'ARCHIVED',
    ),
    _ => null,
  };
}
