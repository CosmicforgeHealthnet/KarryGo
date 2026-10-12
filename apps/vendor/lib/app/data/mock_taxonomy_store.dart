import 'dart:convert';
import 'package:http/http.dart' as http;
import '../../core/contracts/vendor_contract.dart';
import '../../features/catalogue/state/catalogue_resource.dart';
import 'mock_catalogue_store.dart';

class MockTaxonomyStore {
  MockTaxonomyStore(this.catalogue, this.vendorId);
  final MockCatalogueStore catalogue;
  final String vendorId;
  final mappings = <String, List<JsonObject>>{};
  final commands = <String, JsonObject>{};
  int sequence = 0;
  static final seeds = jsonObject(
    jsonDecode(
      r'''{"namespaces":{"FOOD":{"schema_version":1,"namespace_code":"FOOD","seed_key":"KARRYGO_FOOD_V1","facets":[{"code":"PROVIDER_TYPE","display_name":"Provider Type","description":"Food provider discovery classification.","structure_type":"FLAT","position":10,"metadata":{}},{"code":"CUISINE","display_name":"Cuisine","description":"Cuisine and regional food discovery classification.","structure_type":"HIERARCHICAL","position":20,"metadata":{}},{"code":"MEAL_PERIOD","display_name":"Meal Period","description":"Discovery-time meal period classification; not branch-opening authority.","structure_type":"FLAT","position":30,"metadata":{"discovery_only":true,"branch_opening_authority":false}},{"code":"DISH_TYPE","display_name":"Dish / Food Type","description":"Hierarchical canonical food and dish discovery vocabulary.","structure_type":"HIERARCHICAL","position":40,"metadata":{}},{"code":"OCCASION","display_name":"Occasion / Commercial Usage","description":"Commercial and occasion-based Food discovery classification.","structure_type":"FLAT","position":50,"metadata":{}}],"nodes":[{"canonical_code":"FOOD_PROVIDER_TYPE_RESTAURANT","facet_code":"PROVIDER_TYPE","display_name":"Restaurants","description":"","parent_code":"","position":10,"metadata":{"kind":"PROVIDER_TYPE"}},{"canonical_code":"FOOD_PROVIDER_TYPE_FAST_FOOD","facet_code":"PROVIDER_TYPE","display_name":"Fast Food","description":"","parent_code":"","position":20,"metadata":{"kind":"PROVIDER_TYPE"}},{"canonical_code":"FOOD_PROVIDER_TYPE_CLOUD_KITCHEN","facet_code":"PROVIDER_TYPE","display_name":"Cloud Kitchens","description":"","parent_code":"","position":30,"metadata":{"kind":"PROVIDER_TYPE"}},{"canonical_code":"FOOD_PROVIDER_TYPE_PASTRY_BAKERY","facet_code":"PROVIDER_TYPE","display_name":"Pastry & Bakery","description":"","parent_code":"","position":40,"metadata":{"kind":"PROVIDER_TYPE"}},{"canonical_code":"FOOD_CUISINE_AFRICAN_FOOD","facet_code":"CUISINE","display_name":"African Food","description":"","parent_code":"","position":10,"metadata":{"kind":"CUISINE"}},{"canonical_code":"FOOD_CUISINE_NIGERIAN_LOCAL_DISHES","facet_code":"CUISINE","display_name":"Nigerian / Local Dishes","description":"","parent_code":"FOOD_CUISINE_AFRICAN_FOOD","position":20,"metadata":{"kind":"CUISINE"}},{"canonical_code":"FOOD_CUISINE_CONTINENTAL_INTERNATIONAL","facet_code":"CUISINE","display_name":"Continental / International","description":"","parent_code":"","position":30,"metadata":{"kind":"CUISINE"}},{"canonical_code":"FOOD_MEAL_PERIOD_BREAKFAST_BRUNCH","facet_code":"MEAL_PERIOD","display_name":"Breakfast & Brunch","description":"","parent_code":"","position":10,"metadata":{"kind":"MEAL_PERIOD","discovery_only":true,"branch_opening_authority":false}},{"canonical_code":"FOOD_MEAL_PERIOD_LUNCH","facet_code":"MEAL_PERIOD","display_name":"Lunch","description":"","parent_code":"","position":20,"metadata":{"kind":"MEAL_PERIOD","discovery_only":true,"branch_opening_authority":false}},{"canonical_code":"FOOD_MEAL_PERIOD_DINNER","facet_code":"MEAL_PERIOD","display_name":"Dinner","description":"","parent_code":"","position":30,"metadata":{"kind":"MEAL_PERIOD","discovery_only":true,"branch_opening_authority":false}},{"canonical_code":"FOOD_MEAL_PERIOD_LATE_NIGHT","facet_code":"MEAL_PERIOD","display_name":"Late Night","description":"","parent_code":"","position":40,"metadata":{"kind":"MEAL_PERIOD","discovery_only":true,"branch_opening_authority":false}},{"canonical_code":"FOOD_DISH_TYPE_RICE_MEALS","facet_code":"DISH_TYPE","display_name":"Rice & Meals","description":"","parent_code":"","position":10,"metadata":{"kind":"GROUP"}},{"canonical_code":"FOOD_DISH_TYPE_SOUPS_SWALLOW","facet_code":"DISH_TYPE","display_name":"Soups & Swallow","description":"","parent_code":"","position":20,"metadata":{"kind":"GROUP"}},{"canonical_code":"FOOD_DISH_TYPE_GRILLS_BBQ","facet_code":"DISH_TYPE","display_name":"Grills & BBQ","description":"","parent_code":"","position":30,"metadata":{"kind":"GROUP"}},{"canonical_code":"FOOD_DISH_TYPE_PIZZA","facet_code":"DISH_TYPE","display_name":"Pizza","description":"","parent_code":"","position":40,"metadata":{"kind":"GROUP"}},{"canonical_code":"FOOD_DISH_TYPE_BURGERS","facet_code":"DISH_TYPE","display_name":"Burgers","description":"","parent_code":"","position":50,"metadata":{"kind":"GROUP"}},{"canonical_code":"FOOD_DISH_TYPE_SHAWARMA","facet_code":"DISH_TYPE","display_name":"Shawarma","description":"","parent_code":"","position":60,"metadata":{"kind":"GROUP"}},{"canonical_code":"FOOD_DISH_TYPE_CAKES_CELEBRATIONS","facet_code":"DISH_TYPE","display_name":"Cakes & Celebrations","description":"","parent_code":"","position":70,"metadata":{"kind":"GROUP"}},{"canonical_code":"FOOD_DISH_TYPE_PASTRIES","facet_code":"DISH_TYPE","display_name":"Pastries","description":"","parent_code":"","position":80,"metadata":{"kind":"GROUP"}},{"canonical_code":"FOOD_DISH_TYPE_SMALL_CHOPS","facet_code":"DISH_TYPE","display_name":"Small Chops","description":"","parent_code":"","position":90,"metadata":{"kind":"GROUP"}},{"canonical_code":"FOOD_DISH_TYPE_SNACKS","facet_code":"DISH_TYPE","display_name":"Snacks","description":"","parent_code":"","position":100,"metadata":{"kind":"GROUP"}},{"canonical_code":"FOOD_DISH_TYPE_DESSERTS","facet_code":"DISH_TYPE","display_name":"Desserts","description":"","parent_code":"","position":110,"metadata":{"kind":"GROUP"}},{"canonical_code":"FOOD_DISH_TYPE_ICE_CREAM","facet_code":"DISH_TYPE","display_name":"Ice Cream","description":"","parent_code":"","position":120,"metadata":{"kind":"GROUP"}},{"canonical_code":"FOOD_DISH_TYPE_DRINKS","facet_code":"DISH_TYPE","display_name":"Drinks","description":"","parent_code":"","position":130,"metadata":{"kind":"GROUP"}},{"canonical_code":"FOOD_DISH_TYPE_JUICES","facet_code":"DISH_TYPE","display_name":"Juices","description":"","parent_code":"","position":140,"metadata":{"kind":"GROUP"}},{"canonical_code":"FOOD_DISH_TYPE_SMOOTHIES","facet_code":"DISH_TYPE","display_name":"Smoothies","description":"","parent_code":"","position":150,"metadata":{"kind":"GROUP"}},{"canonical_code":"FOOD_DISH_TYPE_HEALTHY_MEALS","facet_code":"DISH_TYPE","display_name":"Healthy Meals","description":"","parent_code":"","position":160,"metadata":{"kind":"GROUP"}},{"canonical_code":"FOOD_OCCASION_FAMILY_MEALS","facet_code":"OCCASION","display_name":"Family Meals","description":"","parent_code":"","position":10,"metadata":{"kind":"OCCASION"}},{"canonical_code":"FOOD_OCCASION_GROUP_ORDERS","facet_code":"OCCASION","display_name":"Group Orders","description":"","parent_code":"","position":20,"metadata":{"kind":"OCCASION"}},{"canonical_code":"FOOD_OCCASION_BULK_FOOD","facet_code":"OCCASION","display_name":"Bulk Food","description":"","parent_code":"","position":30,"metadata":{"kind":"OCCASION"}},{"canonical_code":"FOOD_OCCASION_CORPORATE_MEALS","facet_code":"OCCASION","display_name":"Corporate Meals","description":"","parent_code":"","position":40,"metadata":{"kind":"OCCASION"}},{"canonical_code":"FOOD_OCCASION_SCHEDULED_FOOD","facet_code":"OCCASION","display_name":"Scheduled Food","description":"","parent_code":"","position":50,"metadata":{"kind":"OCCASION"}},{"canonical_code":"FOOD_OCCASION_CELEBRATION_FOOD","facet_code":"OCCASION","display_name":"Celebration Food","description":"","parent_code":"","position":60,"metadata":{"kind":"OCCASION"}},{"canonical_code":"FOOD_OCCASION_PARTY_PACKS","facet_code":"OCCASION","display_name":"Party Packs","description":"","parent_code":"","position":70,"metadata":{"kind":"OCCASION"}},{"canonical_code":"FOOD_DISH_TYPE_RICE_MEALS_JOLLOF_RICE","facet_code":"DISH_TYPE","display_name":"Jollof Rice","description":"","parent_code":"FOOD_DISH_TYPE_RICE_MEALS","position":10,"metadata":{"kind":"DISH","market":"NG"}},{"canonical_code":"FOOD_DISH_TYPE_RICE_MEALS_FRIED_RICE","facet_code":"DISH_TYPE","display_name":"Fried Rice","description":"","parent_code":"FOOD_DISH_TYPE_RICE_MEALS","position":20,"metadata":{"kind":"DISH","market":"NG"}},{"canonical_code":"FOOD_DISH_TYPE_RICE_MEALS_OFADA_RICE","facet_code":"DISH_TYPE","display_name":"Ofada Rice","description":"","parent_code":"FOOD_DISH_TYPE_RICE_MEALS","position":30,"metadata":{"kind":"DISH","market":"NG"}},{"canonical_code":"FOOD_DISH_TYPE_RICE_MEALS_NATIVE_RICE","facet_code":"DISH_TYPE","display_name":"Native Rice","description":"","parent_code":"FOOD_DISH_TYPE_RICE_MEALS","position":40,"metadata":{"kind":"DISH","market":"NG"}},{"canonical_code":"FOOD_DISH_TYPE_SOUPS_SWALLOW_EGUSI_SOUP","facet_code":"DISH_TYPE","display_name":"Egusi Soup","description":"","parent_code":"FOOD_DISH_TYPE_SOUPS_SWALLOW","position":10,"metadata":{"kind":"DISH","market":"NG"}},{"canonical_code":"FOOD_DISH_TYPE_SOUPS_SWALLOW_OGBONO_SOUP","facet_code":"DISH_TYPE","display_name":"Ogbono Soup","description":"","parent_code":"FOOD_DISH_TYPE_SOUPS_SWALLOW","position":20,"metadata":{"kind":"DISH","market":"NG"}},{"canonical_code":"FOOD_DISH_TYPE_SOUPS_SWALLOW_AFANG","facet_code":"DISH_TYPE","display_name":"Afang","description":"","parent_code":"FOOD_DISH_TYPE_SOUPS_SWALLOW","position":30,"metadata":{"kind":"DISH","market":"NG"}},{"canonical_code":"FOOD_DISH_TYPE_SOUPS_SWALLOW_EDIKANG_IKONG","facet_code":"DISH_TYPE","display_name":"Edikang Ikong","description":"","parent_code":"FOOD_DISH_TYPE_SOUPS_SWALLOW","position":40,"metadata":{"kind":"DISH","market":"NG"}},{"canonical_code":"FOOD_DISH_TYPE_SOUPS_SWALLOW_BANGA","facet_code":"DISH_TYPE","display_name":"Banga","description":"","parent_code":"FOOD_DISH_TYPE_SOUPS_SWALLOW","position":50,"metadata":{"kind":"DISH","market":"NG"}},{"canonical_code":"FOOD_DISH_TYPE_SOUPS_SWALLOW_OHA","facet_code":"DISH_TYPE","display_name":"Oha","description":"","parent_code":"FOOD_DISH_TYPE_SOUPS_SWALLOW","position":60,"metadata":{"kind":"DISH","market":"NG"}},{"canonical_code":"FOOD_DISH_TYPE_SOUPS_SWALLOW_NSALA","facet_code":"DISH_TYPE","display_name":"Nsala","description":"","parent_code":"FOOD_DISH_TYPE_SOUPS_SWALLOW","position":70,"metadata":{"kind":"DISH","market":"NG"}},{"canonical_code":"FOOD_DISH_TYPE_SOUPS_SWALLOW_FISHERMAN_SOUP","facet_code":"DISH_TYPE","display_name":"Fisherman Soup","description":"","parent_code":"FOOD_DISH_TYPE_SOUPS_SWALLOW","position":80,"metadata":{"kind":"DISH","market":"NG"}},{"canonical_code":"FOOD_DISH_TYPE_SOUPS_SWALLOW_EBA","facet_code":"DISH_TYPE","display_name":"Eba","description":"","parent_code":"FOOD_DISH_TYPE_SOUPS_SWALLOW","position":90,"metadata":{"kind":"DISH","market":"NG"}},{"canonical_code":"FOOD_DISH_TYPE_SOUPS_SWALLOW_AMALA","facet_code":"DISH_TYPE","display_name":"Amala","description":"","parent_code":"FOOD_DISH_TYPE_SOUPS_SWALLOW","position":100,"metadata":{"kind":"DISH","market":"NG"}},{"canonical_code":"FOOD_DISH_TYPE_SOUPS_SWALLOW_POUNDED_YAM","facet_code":"DISH_TYPE","display_name":"Pounded Yam","description":"","parent_code":"FOOD_DISH_TYPE_SOUPS_SWALLOW","position":110,"metadata":{"kind":"DISH","market":"NG"}},{"canonical_code":"FOOD_DISH_TYPE_SOUPS_SWALLOW_SEMO","facet_code":"DISH_TYPE","display_name":"Semo","description":"","parent_code":"FOOD_DISH_TYPE_SOUPS_SWALLOW","position":120,"metadata":{"kind":"DISH","market":"NG"}},{"canonical_code":"FOOD_DISH_TYPE_SOUPS_SWALLOW_FUFU","facet_code":"DISH_TYPE","display_name":"Fufu","description":"","parent_code":"FOOD_DISH_TYPE_SOUPS_SWALLOW","position":130,"metadata":{"kind":"DISH","market":"NG"}},{"canonical_code":"FOOD_DISH_TYPE_SOUPS_SWALLOW_TUWO","facet_code":"DISH_TYPE","display_name":"Tuwo","description":"","parent_code":"FOOD_DISH_TYPE_SOUPS_SWALLOW","position":140,"metadata":{"kind":"DISH","market":"NG"}},{"canonical_code":"FOOD_DISH_TYPE_GRILLS_BBQ_SUYA","facet_code":"DISH_TYPE","display_name":"Suya","description":"","parent_code":"FOOD_DISH_TYPE_GRILLS_BBQ","position":10,"metadata":{"kind":"DISH","market":"NG"}},{"canonical_code":"FOOD_DISH_TYPE_GRILLS_BBQ_KILISHI","facet_code":"DISH_TYPE","display_name":"Kilishi","description":"","parent_code":"FOOD_DISH_TYPE_GRILLS_BBQ","position":20,"metadata":{"kind":"DISH","market":"NG"}},{"canonical_code":"FOOD_DISH_TYPE_SNACKS_AKARA","facet_code":"DISH_TYPE","display_name":"Akara","description":"","parent_code":"FOOD_DISH_TYPE_SNACKS","position":10,"metadata":{"kind":"DISH","market":"NG"}},{"canonical_code":"FOOD_DISH_TYPE_SNACKS_MOI_MOI","facet_code":"DISH_TYPE","display_name":"Moi Moi","description":"","parent_code":"FOOD_DISH_TYPE_SNACKS","position":20,"metadata":{"kind":"DISH","market":"NG"}},{"canonical_code":"FOOD_DISH_TYPE_SNACKS_PUFF_PUFF","facet_code":"DISH_TYPE","display_name":"Puff Puff","description":"","parent_code":"FOOD_DISH_TYPE_SNACKS","position":30,"metadata":{"kind":"DISH","market":"NG"}},{"canonical_code":"FOOD_DISH_TYPE_SNACKS_CHIN_CHIN","facet_code":"DISH_TYPE","display_name":"Chin Chin","description":"","parent_code":"FOOD_DISH_TYPE_SNACKS","position":40,"metadata":{"kind":"DISH","market":"NG"}},{"canonical_code":"FOOD_DISH_TYPE_BREAKFAST_MEALS","facet_code":"DISH_TYPE","display_name":"Breakfast Meals","description":"","parent_code":"","position":170,"metadata":{"kind":"GROUP"}},{"canonical_code":"FOOD_DISH_TYPE_BREAKFAST_MEALS_AKAMU","facet_code":"DISH_TYPE","display_name":"Akamu","description":"","parent_code":"FOOD_DISH_TYPE_BREAKFAST_MEALS","position":10,"metadata":{"kind":"DISH","market":"NG"}}],"terms":[{"namespace_code":"FOOD","node_code":"FOOD_DISH_TYPE_SNACKS_MOI_MOI","term":"Moin Moin","relationship_type":"ALIAS","locale":"en-NG","region":"NG","metadata":{"market":"NG"}},{"namespace_code":"FOOD","node_code":"FOOD_DISH_TYPE_SNACKS_MOI_MOI","term":"Moimoi","relationship_type":"ALIAS","locale":"en-NG","region":"NG","metadata":{"market":"NG"}},{"namespace_code":"FOOD","node_code":"FOOD_DISH_TYPE_SNACKS_MOI_MOI","term":"Moi-Moi","relationship_type":"ALIAS","locale":"en-NG","region":"NG","metadata":{"market":"NG"}},{"namespace_code":"FOOD","node_code":"FOOD_DISH_TYPE_SNACKS_PUFF_PUFF","term":"Puff-Puff","relationship_type":"ALIAS","locale":"en-NG","region":"NG","metadata":{"market":"NG"}},{"namespace_code":"FOOD","node_code":"FOOD_DISH_TYPE_BREAKFAST_MEALS_AKAMU","term":"Pap","relationship_type":"SYNONYM","locale":"en-NG","region":"NG","metadata":{"market":"NG"}},{"namespace_code":"FOOD","node_code":"FOOD_DISH_TYPE_BREAKFAST_MEALS_AKAMU","term":"Ogi","relationship_type":"SYNONYM","locale":"en-NG","region":"NG","metadata":{"market":"NG"}},{"namespace_code":"FOOD","node_code":"FOOD_DISH_TYPE_GRILLS_BBQ_SUYA","term":"Grilled Meat","relationship_type":"RELATED_TERM","locale":"en-NG","region":"NG","metadata":{"market":"NG","exact_equivalence":false}}]},"SHOP":{"schema_version":1,"namespace_code":"SHOP","seed_key":"KARRYGO_SHOP_V1","facets":[{"code":"CATEGORY","display_name":"Shop Category","description":"Canonical Shop discovery category hierarchy.","structure_type":"HIERARCHICAL","position":10,"metadata":{"discovery_namespace":"SHOP"}}],"nodes":[{"canonical_code":"SHOP_CATEGORY_FOODSTUFF","facet_code":"CATEGORY","display_name":"Foodstuff","description":"","parent_code":"","position":10,"metadata":{"kind":"CATEGORY"}},{"canonical_code":"SHOP_CATEGORY_FRESH_PRODUCE","facet_code":"CATEGORY","display_name":"Fresh Produce","description":"","parent_code":"","position":20,"metadata":{"kind":"CATEGORY"}},{"canonical_code":"SHOP_CATEGORY_PROTEIN","facet_code":"CATEGORY","display_name":"Protein","description":"","parent_code":"","position":30,"metadata":{"kind":"CATEGORY"}},{"canonical_code":"SHOP_CATEGORY_COOKING_ESSENTIALS","facet_code":"CATEGORY","display_name":"Cooking Essentials","description":"","parent_code":"","position":40,"metadata":{"kind":"CATEGORY"}},{"canonical_code":"SHOP_CATEGORY_FROZEN_CHILLED","facet_code":"CATEGORY","display_name":"Frozen & Chilled","description":"","parent_code":"","position":50,"metadata":{"kind":"CATEGORY"}},{"canonical_code":"SHOP_CATEGORY_BEVERAGES","facet_code":"CATEGORY","display_name":"Beverages","description":"","parent_code":"","position":60,"metadata":{"kind":"CATEGORY"}},{"canonical_code":"SHOP_CATEGORY_DAIRY_EGGS","facet_code":"CATEGORY","display_name":"Dairy & Eggs","description":"","parent_code":"","position":70,"metadata":{"kind":"CATEGORY"}},{"canonical_code":"SHOP_CATEGORY_PACKAGED_GROCERY","facet_code":"CATEGORY","display_name":"Packaged Grocery","description":"","parent_code":"","position":80,"metadata":{"kind":"CATEGORY"}},{"canonical_code":"SHOP_CATEGORY_BULK_WHOLESALE","facet_code":"CATEGORY","display_name":"Bulk / Wholesale","description":"","parent_code":"","position":90,"metadata":{"kind":"CATEGORY"}},{"canonical_code":"SHOP_CATEGORY_FOODSTUFF_RICE_GRAINS","facet_code":"CATEGORY","display_name":"Rice & Grains","description":"","parent_code":"SHOP_CATEGORY_FOODSTUFF","position":10,"metadata":{"kind":"CATEGORY"}},{"canonical_code":"SHOP_CATEGORY_FOODSTUFF_BEANS_PULSES","facet_code":"CATEGORY","display_name":"Beans & Pulses","description":"","parent_code":"SHOP_CATEGORY_FOODSTUFF","position":20,"metadata":{"kind":"CATEGORY"}},{"canonical_code":"SHOP_CATEGORY_FOODSTUFF_GARRI_CASSAVA","facet_code":"CATEGORY","display_name":"Garri & Cassava","description":"","parent_code":"SHOP_CATEGORY_FOODSTUFF","position":30,"metadata":{"kind":"CATEGORY"}},{"canonical_code":"SHOP_CATEGORY_FOODSTUFF_YAM_TUBERS","facet_code":"CATEGORY","display_name":"Yam & Tubers","description":"","parent_code":"SHOP_CATEGORY_FOODSTUFF","position":40,"metadata":{"kind":"CATEGORY"}},{"canonical_code":"SHOP_CATEGORY_FOODSTUFF_PLANTAIN","facet_code":"CATEGORY","display_name":"Plantain","description":"","parent_code":"SHOP_CATEGORY_FOODSTUFF","position":50,"metadata":{"kind":"CATEGORY","quality_preferences":{"RIPENESS":["GREEN","TURNING","RIPE","VERY_RIPE"]}}},{"canonical_code":"SHOP_CATEGORY_FRESH_PRODUCE_VEGETABLES","facet_code":"CATEGORY","display_name":"Vegetables","description":"","parent_code":"SHOP_CATEGORY_FRESH_PRODUCE","position":10,"metadata":{"kind":"CATEGORY"}},{"canonical_code":"SHOP_CATEGORY_FRESH_PRODUCE_FRUITS","facet_code":"CATEGORY","display_name":"Fruits","description":"","parent_code":"SHOP_CATEGORY_FRESH_PRODUCE","position":20,"metadata":{"kind":"CATEGORY"}},{"canonical_code":"SHOP_CATEGORY_PROTEIN_MEAT_POULTRY","facet_code":"CATEGORY","display_name":"Meat & Poultry","description":"","parent_code":"SHOP_CATEGORY_PROTEIN","position":10,"metadata":{"kind":"CATEGORY"}},{"canonical_code":"SHOP_CATEGORY_PROTEIN_FISH_SEAFOOD","facet_code":"CATEGORY","display_name":"Fish & Seafood","description":"","parent_code":"SHOP_CATEGORY_PROTEIN","position":20,"metadata":{"kind":"CATEGORY","quality_preferences":{"PREPARATION":["WHOLE","CLEAN_ONLY","CLEAN_AND_CUT"],"CUT_SIZE":["SMALL_CUT","MEDIUM_CUT","LARGE_CUT"]}}},{"canonical_code":"SHOP_CATEGORY_COOKING_ESSENTIALS_COOKING_OILS","facet_code":"CATEGORY","display_name":"Cooking Oils","description":"","parent_code":"SHOP_CATEGORY_COOKING_ESSENTIALS","position":10,"metadata":{"kind":"CATEGORY"}},{"canonical_code":"SHOP_CATEGORY_COOKING_ESSENTIALS_SPICES_SEASONING","facet_code":"CATEGORY","display_name":"Spices & Seasoning","description":"","parent_code":"SHOP_CATEGORY_COOKING_ESSENTIALS","position":20,"metadata":{"kind":"CATEGORY"}},{"canonical_code":"SHOP_CATEGORY_COOKING_ESSENTIALS_FLOUR_BAKING","facet_code":"CATEGORY","display_name":"Flour & Baking","description":"","parent_code":"SHOP_CATEGORY_COOKING_ESSENTIALS","position":30,"metadata":{"kind":"CATEGORY"}},{"canonical_code":"SHOP_CATEGORY_FROZEN_CHILLED_FROZEN_FOODS","facet_code":"CATEGORY","display_name":"Frozen Foods","description":"","parent_code":"SHOP_CATEGORY_FROZEN_CHILLED","position":10,"metadata":{"kind":"CATEGORY"}},{"canonical_code":"SHOP_CATEGORY_BEVERAGES_DRINKS_WATER","facet_code":"CATEGORY","display_name":"Drinks & Water","description":"","parent_code":"SHOP_CATEGORY_BEVERAGES","position":10,"metadata":{"kind":"CATEGORY"}},{"canonical_code":"SHOP_CATEGORY_DAIRY_EGGS_DAIRY","facet_code":"CATEGORY","display_name":"Dairy","description":"","parent_code":"SHOP_CATEGORY_DAIRY_EGGS","position":10,"metadata":{"kind":"CATEGORY"}},{"canonical_code":"SHOP_CATEGORY_DAIRY_EGGS_EGGS","facet_code":"CATEGORY","display_name":"Eggs","description":"","parent_code":"SHOP_CATEGORY_DAIRY_EGGS","position":20,"metadata":{"kind":"CATEGORY"}},{"canonical_code":"SHOP_CATEGORY_PACKAGED_GROCERY_CANNED_FOODS","facet_code":"CATEGORY","display_name":"Canned Foods","description":"","parent_code":"SHOP_CATEGORY_PACKAGED_GROCERY","position":10,"metadata":{"kind":"CATEGORY"}},{"canonical_code":"SHOP_CATEGORY_PACKAGED_GROCERY_PACKAGED_GROCERIES","facet_code":"CATEGORY","display_name":"Packaged Groceries","description":"","parent_code":"SHOP_CATEGORY_PACKAGED_GROCERY","position":20,"metadata":{"kind":"CATEGORY"}},{"canonical_code":"SHOP_CATEGORY_BULK_WHOLESALE_FOODSTUFF","facet_code":"CATEGORY","display_name":"Bulk / Wholesale Foodstuff","description":"","parent_code":"SHOP_CATEGORY_BULK_WHOLESALE","position":10,"metadata":{"kind":"CATEGORY"}},{"canonical_code":"SHOP_CATEGORY_FOODSTUFF_GARRI_CASSAVA_GARRI","facet_code":"CATEGORY","display_name":"Garri","description":"","parent_code":"SHOP_CATEGORY_FOODSTUFF_GARRI_CASSAVA","position":10,"metadata":{"kind":"PRODUCT_CONCEPT","market":"NG"}},{"canonical_code":"SHOP_CATEGORY_FRESH_PRODUCE_VEGETABLES_OKRA","facet_code":"CATEGORY","display_name":"Okra","description":"","parent_code":"SHOP_CATEGORY_FRESH_PRODUCE_VEGETABLES","position":10,"metadata":{"kind":"PRODUCT_CONCEPT","market":"NG"}},{"canonical_code":"SHOP_CATEGORY_PROTEIN_MEAT_POULTRY_PONMO","facet_code":"CATEGORY","display_name":"Ponmo","description":"","parent_code":"SHOP_CATEGORY_PROTEIN_MEAT_POULTRY","position":10,"metadata":{"kind":"PRODUCT_CONCEPT","market":"NG"}},{"canonical_code":"SHOP_CATEGORY_FOODSTUFF_BEANS_PULSES_GROUNDNUT","facet_code":"CATEGORY","display_name":"Groundnut","description":"","parent_code":"SHOP_CATEGORY_FOODSTUFF_BEANS_PULSES","position":10,"metadata":{"kind":"PRODUCT_CONCEPT","market":"NG"}}],"authority_vocabulary":[{"authority_type":"SECTION12_MEASUREMENT","authority_ref":"smd_paint","authority_code":"PAINT","term":"Paint","locale":"en-NG","region":"NG","metadata":{"authority":"SECTION_12","source_table":"shop_measure_definitions","read_only_reference":true,"search_namespace":"SHOP","fractional_composition":true}},{"authority_type":"SECTION12_MEASUREMENT","authority_ref":"smd_paint_rubber","authority_code":"PAINT_RUBBER","term":"Paint Rubber","locale":"en-NG","region":"NG","metadata":{"authority":"SECTION_12","source_table":"shop_measure_definitions","read_only_reference":true,"search_namespace":"SHOP","fractional_composition":true}},{"authority_type":"SECTION12_MEASUREMENT","authority_ref":"smd_derica","authority_code":"DERICA","term":"Derica","locale":"en-NG","region":"NG","metadata":{"authority":"SECTION_12","source_table":"shop_measure_definitions","read_only_reference":true,"search_namespace":"SHOP","fractional_composition":false}},{"authority_type":"SECTION12_MEASUREMENT","authority_ref":"smd_mudu","authority_code":"MUDU","term":"Mudu","locale":"en-NG","region":"NG","metadata":{"authority":"SECTION_12","source_table":"shop_measure_definitions","read_only_reference":true,"search_namespace":"SHOP","fractional_composition":false}},{"authority_type":"SECTION12_MEASUREMENT","authority_ref":"smd_congo","authority_code":"CONGO","term":"Congo","locale":"en-NG","region":"NG","metadata":{"authority":"SECTION_12","source_table":"shop_measure_definitions","read_only_reference":true,"search_namespace":"SHOP","fractional_composition":false}},{"authority_type":"SECTION12_MEASUREMENT","authority_ref":"smd_tiya","authority_code":"TIYA","term":"Tiya","locale":"en-NG","region":"NG","metadata":{"authority":"SECTION_12","source_table":"shop_measure_definitions","read_only_reference":true,"search_namespace":"SHOP","fractional_composition":false}},{"authority_type":"SECTION12_MEASUREMENT","authority_ref":"smd_kobiowu","authority_code":"KOBIOWU","term":"Kobiowu","locale":"en-NG","region":"NG","metadata":{"authority":"SECTION_12","source_table":"shop_measure_definitions","read_only_reference":true,"search_namespace":"SHOP","fractional_composition":false}},{"authority_type":"SECTION12_MEASUREMENT","authority_ref":"smd_basket","authority_code":"BASKET","term":"Basket","locale":"en-NG","region":"NG","metadata":{"authority":"SECTION_12","source_table":"shop_measure_definitions","read_only_reference":true,"search_namespace":"SHOP","fractional_composition":false}},{"authority_type":"SECTION12_MEASUREMENT","authority_ref":"smd_bowl","authority_code":"BOWL","term":"Bowl","locale":"en-NG","region":"NG","metadata":{"authority":"SECTION_12","source_table":"shop_measure_definitions","read_only_reference":true,"search_namespace":"SHOP","fractional_composition":false}},{"authority_type":"SECTION12_MEASUREMENT","authority_ref":"smd_basin","authority_code":"BASIN","term":"Basin","locale":"en-NG","region":"NG","metadata":{"authority":"SECTION_12","source_table":"shop_measure_definitions","read_only_reference":true,"search_namespace":"SHOP","fractional_composition":false}},{"authority_type":"SECTION12_MEASUREMENT","authority_ref":"smd_bunch","authority_code":"BUNCH","term":"Bunch","locale":"en-NG","region":"NG","metadata":{"authority":"SECTION_12","source_table":"shop_measure_definitions","read_only_reference":true,"search_namespace":"SHOP","fractional_composition":false}},{"authority_type":"SECTION12_MEASUREMENT","authority_ref":"smd_tuber","authority_code":"TUBER","term":"Tuber","locale":"en-NG","region":"NG","metadata":{"authority":"SECTION_12","source_table":"shop_measure_definitions","read_only_reference":true,"search_namespace":"SHOP","fractional_composition":false}},{"authority_type":"SECTION12_MEASUREMENT","authority_ref":"smd_bag","authority_code":"BAG","term":"Bag","locale":"en-NG","region":"NG","metadata":{"authority":"SECTION_12","source_table":"shop_measure_definitions","read_only_reference":true,"search_namespace":"SHOP","fractional_composition":false}},{"authority_type":"SECTION12_MEASUREMENT","authority_ref":"smd_sack","authority_code":"SACK","term":"Sack","locale":"en-NG","region":"NG","metadata":{"authority":"SECTION_12","source_table":"shop_measure_definitions","read_only_reference":true,"search_namespace":"SHOP","fractional_composition":false}},{"authority_type":"SECTION12_MEASUREMENT","authority_ref":"smd_carton","authority_code":"CARTON","term":"Carton","locale":"en-NG","region":"NG","metadata":{"authority":"SECTION_12","source_table":"shop_measure_definitions","read_only_reference":true,"search_namespace":"SHOP","fractional_composition":false}},{"authority_type":"SECTION12_MEASUREMENT","authority_ref":"smd_crate","authority_code":"CRATE","term":"Crate","locale":"en-NG","region":"NG","metadata":{"authority":"SECTION_12","source_table":"shop_measure_definitions","read_only_reference":true,"search_namespace":"SHOP","fractional_composition":false}},{"authority_type":"SECTION12_MEASUREMENT","authority_ref":"smd_jerrycan","authority_code":"JERRYCAN","term":"Jerrycan","locale":"en-NG","region":"NG","metadata":{"authority":"SECTION_12","source_table":"shop_measure_definitions","read_only_reference":true,"search_namespace":"SHOP","fractional_composition":false}}],"terms":[{"namespace_code":"SHOP","node_code":"SHOP_CATEGORY_FOODSTUFF_GARRI_CASSAVA_GARRI","term":"Gari","relationship_type":"ALIAS","locale":"en-NG","region":"NG","metadata":{"market":"NG"}},{"namespace_code":"SHOP","node_code":"SHOP_CATEGORY_FRESH_PRODUCE_VEGETABLES_OKRA","term":"Okro","relationship_type":"ALIAS","locale":"en-NG","region":"NG","metadata":{"market":"NG","regional_spelling":true}},{"namespace_code":"SHOP","node_code":"SHOP_CATEGORY_PROTEIN_MEAT_POULTRY_PONMO","term":"Kpomo","relationship_type":"ALIAS","locale":"en-NG","region":"NG","metadata":{"market":"NG","contextual":true}},{"namespace_code":"SHOP","node_code":"SHOP_CATEGORY_PROTEIN_MEAT_POULTRY_PONMO","term":"Kanda","relationship_type":"ALIAS","locale":"en-NG","region":"NG","metadata":{"market":"NG","contextual":true}},{"namespace_code":"SHOP","node_code":"SHOP_CATEGORY_FOODSTUFF_BEANS_PULSES_GROUNDNUT","term":"Peanut","relationship_type":"SYNONYM","locale":"en-NG","region":"NG","metadata":{"market":"NG"}}]}},"semantics":{"schema_version":1,"seed_key":"KARRYGO_SEMANTICS_V1","tags":[{"namespace":"FOOD","code":"SPICY","display_name":"Spicy","tag_type":"ATTRIBUTE"},{"namespace":"FOOD","code":"VERY_SPICY","display_name":"Very Spicy","tag_type":"ATTRIBUTE"},{"namespace":"FOOD","code":"GRILLED","display_name":"Grilled","tag_type":"PREPARATION"},{"namespace":"FOOD","code":"FRIED","display_name":"Fried","tag_type":"PREPARATION"},{"namespace":"FOOD","code":"BAKED","display_name":"Baked","tag_type":"PREPARATION"},{"namespace":"FOOD","code":"READY_TO_EAT","display_name":"Ready to Eat","tag_type":"DISCOVERY"},{"namespace":"FOOD","code":"FAMILY_SIZE","display_name":"Family Size","tag_type":"DISCOVERY"},{"namespace":"FOOD","code":"SINGLE_SERVING","display_name":"Single Serving","tag_type":"DISCOVERY"},{"namespace":"SHOP","code":"FRESH","display_name":"Fresh","tag_type":"ATTRIBUTE"},{"namespace":"SHOP","code":"FROZEN","display_name":"Frozen","tag_type":"ATTRIBUTE"},{"namespace":"SHOP","code":"CHILLED","display_name":"Chilled","tag_type":"ATTRIBUTE"},{"namespace":"SHOP","code":"READY_TO_COOK","display_name":"Ready to Cook","tag_type":"DISCOVERY"},{"namespace":"SHOP","code":"BULK_PACK","display_name":"Bulk Pack","tag_type":"DISCOVERY"},{"namespace":"SHOP","code":"SINGLE_UNIT","display_name":"Single Unit","tag_type":"DISCOVERY"},{"namespace":"SHOP","code":"LOCAL_PRODUCE","display_name":"Local Produce","tag_type":"ATTRIBUTE"},{"namespace":"SHOP","code":"IMPORTED","display_name":"Imported","tag_type":"ATTRIBUTE"}],"dietary_claims":[{"namespace":"FOOD","code":"VEGETARIAN","display_name":"Vegetarian","verification_policy":"DECLARATION","risk_level":"LOW"},{"namespace":"FOOD","code":"VEGAN","display_name":"Vegan","verification_policy":"DECLARATION","risk_level":"LOW"},{"namespace":"FOOD","code":"HALAL","display_name":"Halal","verification_policy":"EVIDENCE_REQUIRED","risk_level":"MODERATE"},{"namespace":"FOOD","code":"GLUTEN_FREE","display_name":"Gluten Free","verification_policy":"PLATFORM_VERIFIED","risk_level":"HIGH"},{"namespace":"FOOD","code":"DAIRY_FREE","display_name":"Dairy Free","verification_policy":"EVIDENCE_REQUIRED","risk_level":"HIGH"},{"namespace":"FOOD","code":"NUT_FREE","display_name":"Nut Free","verification_policy":"PLATFORM_VERIFIED","risk_level":"HIGH"},{"namespace":"FOOD","code":"SUGAR_FREE","display_name":"Sugar Free","verification_policy":"EVIDENCE_REQUIRED","risk_level":"MODERATE"},{"namespace":"FOOD","code":"LOW_SUGAR","display_name":"Low Sugar","verification_policy":"EVIDENCE_REQUIRED","risk_level":"MODERATE"},{"namespace":"FOOD","code":"KETO","display_name":"Keto","verification_policy":"DECLARATION","risk_level":"LOW"},{"namespace":"FOOD","code":"HIGH_PROTEIN","display_name":"High Protein","verification_policy":"EVIDENCE_REQUIRED","risk_level":"MODERATE"},{"namespace":"SHOP","code":"VEGETARIAN","display_name":"Vegetarian","verification_policy":"DECLARATION","risk_level":"LOW"},{"namespace":"SHOP","code":"VEGAN","display_name":"Vegan","verification_policy":"DECLARATION","risk_level":"LOW"},{"namespace":"SHOP","code":"HALAL","display_name":"Halal","verification_policy":"EVIDENCE_REQUIRED","risk_level":"MODERATE"},{"namespace":"SHOP","code":"GLUTEN_FREE","display_name":"Gluten Free","verification_policy":"PLATFORM_VERIFIED","risk_level":"HIGH"},{"namespace":"SHOP","code":"DAIRY_FREE","display_name":"Dairy Free","verification_policy":"EVIDENCE_REQUIRED","risk_level":"HIGH"},{"namespace":"SHOP","code":"NUT_FREE","display_name":"Nut Free","verification_policy":"PLATFORM_VERIFIED","risk_level":"HIGH"},{"namespace":"SHOP","code":"SUGAR_FREE","display_name":"Sugar Free","verification_policy":"EVIDENCE_REQUIRED","risk_level":"MODERATE"},{"namespace":"SHOP","code":"LOW_SUGAR","display_name":"Low Sugar","verification_policy":"EVIDENCE_REQUIRED","risk_level":"MODERATE"},{"namespace":"SHOP","code":"KETO","display_name":"Keto","verification_policy":"DECLARATION","risk_level":"LOW"},{"namespace":"SHOP","code":"HIGH_PROTEIN","display_name":"High Protein","verification_policy":"EVIDENCE_REQUIRED","risk_level":"MODERATE"}]}}''',
    ),
  );
  JsonObject ok(Object data) => {
    '_status': 200,
    '_data': jsonDecode(jsonEncode(data)),
  };
  JsonObject fail(int status, String message) => {
    '_status': status,
    '_code': 'TAXONOMY_REQUEST_REFUSED',
    '_message': message,
  };
  JsonObject? handle(http.BaseRequest request, JsonObject body) {
    final match = RegExp(
      r'^/api/v1/vendor/taxonomy/namespaces/(FOOD|SHOP)/([^/]+)(?:/([^/]+))?$',
    ).firstMatch(request.url.path);
    if (match == null) return null;
    final ns = match[1]!, route = match[2]!;
    final seed = jsonObject(seeds['namespaces'][ns]);
    final versionId = 'mock-taxonomy-$ns-v1';
    final namespaceId = 'mock-taxonomy-$ns';
    final facets = (seed['facets'] as List)
        .map(
          (f) => <String, dynamic>{
            'ID': '$ns-${f['code']}',
            'NamespaceID': namespaceId,
            'VersionID': versionId,
            'Code': f['code'],
            'DisplayName': f['display_name'],
            'Description': f['description'],
            'StructureType': f['structure_type'],
            'Position': f['position'],
            'Status': 'ACTIVE',
            'Metadata': f['metadata'],
            'Revision': 1,
          },
        )
        .toList();
    final nodes = (seed['nodes'] as List)
        .map(
          (n) => <String, dynamic>{
            'ID': '$ns-${n['canonical_code']}',
            'NamespaceID': namespaceId,
            'VersionID': versionId,
            'FacetID': '$ns-${n['facet_code']}',
            'CanonicalCode': n['canonical_code'],
            'DisplayName': n['display_name'],
            'Description': n['description'],
            'ParentID': n['parent_code'] == null || n['parent_code'] == ''
                ? null
                : '$ns-${n['parent_code']}',
            'Position': n['position'],
            'Status': 'ACTIVE',
            'Metadata': n['metadata'],
            'Revision': 1,
          },
        )
        .toList();
    final claims = (seeds['semantics']['dietary_claims'] as List)
        .where((c) => c['namespace'] == ns)
        .map(
          (c) => <String, dynamic>{
            'ID': '$ns-${c['code']}',
            'NamespaceID': namespaceId,
            'Code': c['code'],
            'DisplayName': c['display_name'],
            'VerificationPolicy': c['verification_policy'],
            'RiskLevel': c['risk_level'],
            'Status': 'ACTIVE',
            'Revision': 1,
          },
        )
        .toList();
    if (request.method == 'GET') {
      if (route == 'published-version') {
        return ok({
          'namespace_code': ns,
          'version': {
            'ID': versionId,
            'NamespaceID': namespaceId,
            'VersionNumber': 1,
            'Status': 'PUBLISHED',
            'Revision': 1,
          },
        });
      }
      if (route == 'facets') {
        return ok({
          'namespace_code': ns,
          'version_id': versionId,
          'facets': facets,
        });
      }
      if (route == 'nodes') {
        if (match[3] != null) {
          final node = nodes.where((n) => n['ID'] == match[3]).firstOrNull;
          return node == null
              ? fail(404, 'Category not found.')
              : ok({'node': node, 'terms': []});
        }
        final facet = request.url.queryParameters['facet_id'];
        return ok({
          'namespace_code': ns,
          'version_id': versionId,
          'nodes': nodes
              .where((n) => facet == null || n['FacetID'] == facet)
              .toList(),
        });
      }
      if (route == 'dietary-claims') {
        return ok({'namespace_code': ns, 'dietary_claims': claims});
      }
    }
    final type = request.method == 'GET'
        ? request.url.queryParameters['source_type']
        : body['source_type'];
    final ref = request.method == 'GET'
        ? request.url.queryParameters['source_ref']
        : body['source_ref'];
    final kind = switch (type) {
      'FOOD_ITEM' => CatalogueKind.items,
      'SHOP_PRODUCT' => CatalogueKind.products,
      'SHOP_VARIANT' => CatalogueKind.shopVariants,
      _ => null,
    };
    if (kind == null ||
        ref is! String ||
        (type == 'FOOD_ITEM') != (ns == 'FOOD') ||
        catalogue.advancedRecord(kind, ref) == null) {
      return fail(404, 'Catalogue item not found.');
    }
    final source = '$type|$ref';
    final saved = mappings.putIfAbsent(source, () => []);
    final classification = {
      'namespace_code': ns,
      'source_type': type,
      'source_ref': ref,
      'business_ref': vendorId,
      'mappings': saved,
      'tag_assignments': [
        {
          'id': 'mock-tag-$source',
          'namespace_id': namespaceId,
          'tag_id':
              '$ns-${(seeds['semantics']['tags'] as List).firstWhere((t) => t['namespace'] == ns)['code']}',
          'tag_code': (seeds['semantics']['tags'] as List).firstWhere(
            (t) => t['namespace'] == ns,
          )['code'],
          'display_name': (seeds['semantics']['tags'] as List).firstWhere(
            (t) => t['namespace'] == ns,
          )['display_name'],
          'tag_type': (seeds['semantics']['tags'] as List).firstWhere(
            (t) => t['namespace'] == ns,
          )['tag_type'],
          'source_type': type,
          'source_ref': ref,
          'business_ref': vendorId,
          'status': 'ACTIVE',
          'revision': 1,
        },
      ],
      'dietary_claim_assignments': <JsonObject>[],
    };
    if (request.method == 'GET') {
      if (route == 'classifications') return ok(classification);
      if (route == 'mappings') {
        return ok({
          'namespace_code': ns,
          'source_type': type,
          'source_ref': ref,
          'mappings': saved,
        });
      }
      if (route == 'tag-assignments') {
        return ok({
          'namespace_code': ns,
          'source_type': type,
          'source_ref': ref,
          'tag_assignments': classification['tag_assignments'],
        });
      }
      if (route == 'dietary-claim-assignments') {
        return ok({
          'namespace_code': ns,
          'source_type': type,
          'source_ref': ref,
          'dietary_claim_assignments':
              classification['dietary_claim_assignments'],
        });
      }
    }
    if (route != 'mappings' || request.method != 'PUT') {
      return fail(404, 'Unsupported operation.');
    }
    final key = request.headers.entries
        .where((e) => e.key.toLowerCase() == 'idempotency-key')
        .firstOrNull
        ?.value;
    if (key == null || key.isEmpty) {
      return fail(400, 'A request key is required.');
    }
    final fingerprint = jsonEncode(body);
    if (commands.containsKey(key)) {
      return commands[key]!['fingerprint'] == fingerprint
          ? ok({...jsonObject(commands[key]!['result']), 'replayed': true})
          : fail(409, 'The request key belongs to another change.');
    }
    final facet = facets
        .where((f) => f['Code'] == body['facet_code'])
        .firstOrNull;
    final candidates = body['mappings'];
    if (facet == null ||
        candidates is! List ||
        body.keys.any(
          (k) => ![
            'facet_code',
            'source_type',
            'source_ref',
            'mappings',
          ].contains(k),
        )) {
      return fail(400, 'Invalid classification.');
    }
    if (candidates.where((m) => m['mapping_type'] == 'PRIMARY').length > 1 ||
        candidates.map((m) => m['node_code']).toSet().length !=
            candidates.length ||
        candidates.any(
          (m) =>
              ![
                'PRIMARY',
                'SECONDARY',
                'DISCOVERY',
              ].contains(m['mapping_type']) ||
              !nodes.any(
                (n) =>
                    n['CanonicalCode'] == m['node_code'] &&
                    n['FacetID'] == facet['ID'],
              ),
        )) {
      return fail(400, 'Invalid selected categories.');
    }
    final facetNodes = nodes
        .where((n) => n['FacetID'] == facet['ID'])
        .map((n) => n['ID'])
        .toSet();
    for (final mapping in saved.where(
      (m) => m['Status'] == 'ACTIVE' && facetNodes.contains(m['NodeID']),
    )) {
      mapping['Status'] = 'ARCHIVED';
    }
    final ids = <String>[];
    for (final c in candidates) {
      final node = nodes.firstWhere(
        (n) => n['CanonicalCode'] == c['node_code'],
      );
      final id = 'mock-mapping-${++sequence}';
      ids.add(id);
      saved.add({
        'ID': id,
        'NamespaceID': namespaceId,
        'VersionID': versionId,
        'NodeID': node['ID'],
        'SourceType': type,
        'SourceRef': ref,
        'BusinessRef': vendorId,
        'MappingType': c['mapping_type'],
        'ConfidenceBasis': c['confidence_basis'] ?? 'MANUAL',
        'ConfidenceScore': c['confidence_score'],
        'Metadata': c['metadata'] ?? {},
        'Status': 'ACTIVE',
        'Revision': 1,
      });
    }
    final result = {
      'namespace_code': ns,
      'namespace_id': namespaceId,
      'version_id': versionId,
      'facet_id': facet['ID'],
      'facet_code': facet['Code'],
      'source_type': type,
      'source_ref': ref,
      'mapping_ids': ids,
      'replayed': false,
    };
    commands[key] = {'fingerprint': fingerprint, 'result': result};
    return ok(result);
  }
}
