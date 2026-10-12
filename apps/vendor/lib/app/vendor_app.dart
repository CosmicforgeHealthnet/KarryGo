import 'package:http/http.dart' as http;
import '../core/config/vendor_app_config.dart';
import '../core/network/vendor_api_client.dart';
import '../core/state/vendor_scope.dart';
import '../features/auth/data/vendor_session_store.dart';
import '../features/auth/state/vendor_auth_controller.dart';
import '../features/access/state/vendor_access_controller.dart';
import '../features/access/data/access_api.dart';
import '../features/access/state/access_controller.dart';
import '../features/auth/data/auth_api.dart';
import '../features/auth/state/auth_controller.dart';
import '../features/business/data/business_api.dart';
import '../features/business/state/business_controller.dart';
import '../features/cloud_kitchen/data/cloud_kitchen_api.dart';
import '../features/cloud_kitchen/state/cloud_kitchen_controller.dart';
import '../features/crm/data/crm_api.dart';
import '../features/crm/state/crm_controller.dart';
import '../features/delivery/data/delivery_api.dart';
import '../features/delivery/state/delivery_controller.dart';
import '../features/finance/data/finance_api.dart';
import '../features/finance/state/finance_controller.dart';
import '../features/food_catalogue/data/food_catalogue_api.dart';
import '../features/food_catalogue/state/food_catalogue_controller.dart';
import '../features/handover/data/handover_api.dart';
import '../features/handover/state/handover_controller.dart';
import '../features/inventory/data/inventory_api.dart';
import '../features/inventory/state/inventory_controller.dart';
import '../features/onboarding/data/onboarding_api.dart';
import '../features/onboarding/state/onboarding_controller.dart';
import '../features/operational_cost/data/operational_cost_api.dart';
import '../features/operational_cost/state/operational_cost_controller.dart';
import '../features/orchestration/data/orchestration_api.dart';
import '../features/orchestration/state/orchestration_controller.dart';
import '../features/orders/data/orders_api.dart';
import '../features/orders/state/orders_controller.dart';
import '../features/pastry/data/pastry_api.dart';
import '../features/pastry/state/pastry_controller.dart';
import '../features/production/data/production_api.dart';
import '../features/production/state/production_controller.dart';
import '../features/production_batches/data/production_batches_api.dart';
import '../features/production_batches/state/production_batches_controller.dart';
import '../features/production_cost/data/production_cost_api.dart';
import '../features/production_cost/state/production_cost_controller.dart';
import '../features/production_master/data/production_master_api.dart';
import '../features/production_master/state/production_master_controller.dart';
import '../features/production_stock_policies/data/production_stock_policies_api.dart';
import '../features/production_stock_policies/state/production_stock_policies_controller.dart';
import '../features/promotions/data/promotions_api.dart';
import '../features/promotions/state/promotions_controller.dart';
import '../features/purchasing/data/purchasing_api.dart';
import '../features/purchasing/state/purchasing_controller.dart';
import '../features/readiness/data/readiness_api.dart';
import '../features/readiness/state/readiness_controller.dart';
import '../features/recipe_cost/data/recipe_cost_api.dart';
import '../features/recipe_cost/state/recipe_cost_controller.dart';
import '../features/recipes/data/recipes_api.dart';
import '../features/recipes/state/recipes_controller.dart';
import '../features/scheduled_commerce/data/scheduled_commerce_api.dart';
import '../features/scheduled_commerce/state/scheduled_commerce_controller.dart';
import '../features/shop_catalogue/data/shop_catalogue_api.dart';
import '../features/shop_catalogue/state/shop_catalogue_controller.dart';
import '../features/support/data/support_api.dart';
import '../features/support/state/support_controller.dart';
import '../features/taxonomy/data/taxonomy_api.dart';
import '../features/taxonomy/state/taxonomy_controller.dart';

class VendorApp {
  VendorApp({
    required VendorAppConfig config,
    VendorSessionStore? sessions,
    http.Client? httpClient,
  }) {
    client = VendorApiClient(
      config: config,
      sessions: sessions ?? SecureVendorSessionStore(),
      client: httpClient,
      onSessionChanged: (session) {
        if (session == null) {
          scope.clear();
          access.clear();
        } else {
          if (scope.vendorId != session.vendorId) {
            scope.select(vendorId: session.vendorId);
          }
        }
        auth.sessionChanged(session);
      },
      onForbidden: access.recordDenial,
    );
    accessApi = VendorAccessApi(client);
    accessData = VendorAccessFeatureController(scope, accessApi);
    authApi = VendorAuthApi(client);
    authData = VendorAuthFeatureController(scope, authApi);
    businessApi = VendorBusinessApi(client);
    business = VendorBusinessFeatureController(scope, businessApi);
    cloudKitchenApi = VendorCloudKitchenApi(client);
    cloudKitchen = VendorCloudKitchenFeatureController(scope, cloudKitchenApi);
    crmApi = VendorCrmApi(client);
    crm = VendorCrmFeatureController(scope, crmApi);
    deliveryApi = VendorDeliveryApi(client);
    delivery = VendorDeliveryFeatureController(scope, deliveryApi);
    financeApi = VendorFinanceApi(client);
    finance = VendorFinanceFeatureController(scope, financeApi);
    foodCatalogueApi = VendorFoodCatalogueApi(client);
    foodCatalogue = VendorFoodCatalogueFeatureController(
      scope,
      foodCatalogueApi,
    );
    handoverApi = VendorHandoverApi(client);
    handover = VendorHandoverFeatureController(scope, handoverApi);
    inventoryApi = VendorInventoryApi(client);
    inventory = VendorInventoryFeatureController(scope, inventoryApi);
    onboardingApi = VendorOnboardingApi(client);
    onboarding = VendorOnboardingFeatureController(scope, onboardingApi);
    operationalCostApi = VendorOperationalCostApi(client);
    operationalCost = VendorOperationalCostFeatureController(
      scope,
      operationalCostApi,
    );
    orchestrationApi = VendorOrchestrationApi(client);
    orchestration = VendorOrchestrationFeatureController(
      scope,
      orchestrationApi,
    );
    ordersApi = VendorOrdersApi(client);
    orders = VendorOrdersFeatureController(scope, ordersApi);
    pastryApi = VendorPastryApi(client);
    pastry = VendorPastryFeatureController(scope, pastryApi);
    productionApi = VendorProductionApi(client);
    production = VendorProductionFeatureController(scope, productionApi);
    productionBatchesApi = VendorProductionBatchesApi(client);
    productionBatches = VendorProductionBatchesFeatureController(
      scope,
      productionBatchesApi,
    );
    productionCostApi = VendorProductionCostApi(client);
    productionCost = VendorProductionCostFeatureController(
      scope,
      productionCostApi,
    );
    productionMasterApi = VendorProductionMasterApi(client);
    productionMaster = VendorProductionMasterFeatureController(
      scope,
      productionMasterApi,
    );
    productionStockPoliciesApi = VendorProductionStockPoliciesApi(client);
    productionStockPolicies = VendorProductionStockPoliciesFeatureController(
      scope,
      productionStockPoliciesApi,
    );
    promotionsApi = VendorPromotionsApi(client);
    promotions = VendorPromotionsFeatureController(scope, promotionsApi);
    purchasingApi = VendorPurchasingApi(client);
    purchasing = VendorPurchasingFeatureController(scope, purchasingApi);
    readinessApi = VendorReadinessApi(client);
    readiness = VendorReadinessFeatureController(scope, readinessApi);
    recipeCostApi = VendorRecipeCostApi(client);
    recipeCost = VendorRecipeCostFeatureController(scope, recipeCostApi);
    recipesApi = VendorRecipesApi(client);
    recipes = VendorRecipesFeatureController(scope, recipesApi);
    scheduledCommerceApi = VendorScheduledCommerceApi(client);
    scheduledCommerce = VendorScheduledCommerceFeatureController(
      scope,
      scheduledCommerceApi,
    );
    shopCatalogueApi = VendorShopCatalogueApi(client);
    shopCatalogue = VendorShopCatalogueFeatureController(
      scope,
      shopCatalogueApi,
    );
    supportApi = VendorSupportApi(client);
    support = VendorSupportFeatureController(scope, supportApi);
    taxonomyApi = VendorTaxonomyApi(client);
    taxonomy = VendorTaxonomyFeatureController(scope, taxonomyApi);
    auth = VendorAuthController(api: authApi, client: client);
  }
  final scope = VendorScope();
  final access = VendorAccessController();
  late final VendorApiClient client;
  late final VendorAuthController auth;
  late final VendorAccessApi accessApi;
  late final VendorAccessFeatureController accessData;
  late final VendorAuthApi authApi;
  late final VendorAuthFeatureController authData;
  late final VendorBusinessApi businessApi;
  late final VendorBusinessFeatureController business;
  late final VendorCloudKitchenApi cloudKitchenApi;
  late final VendorCloudKitchenFeatureController cloudKitchen;
  late final VendorCrmApi crmApi;
  late final VendorCrmFeatureController crm;
  late final VendorDeliveryApi deliveryApi;
  late final VendorDeliveryFeatureController delivery;
  late final VendorFinanceApi financeApi;
  late final VendorFinanceFeatureController finance;
  late final VendorFoodCatalogueApi foodCatalogueApi;
  late final VendorFoodCatalogueFeatureController foodCatalogue;
  late final VendorHandoverApi handoverApi;
  late final VendorHandoverFeatureController handover;
  late final VendorInventoryApi inventoryApi;
  late final VendorInventoryFeatureController inventory;
  late final VendorOnboardingApi onboardingApi;
  late final VendorOnboardingFeatureController onboarding;
  late final VendorOperationalCostApi operationalCostApi;
  late final VendorOperationalCostFeatureController operationalCost;
  late final VendorOrchestrationApi orchestrationApi;
  late final VendorOrchestrationFeatureController orchestration;
  late final VendorOrdersApi ordersApi;
  late final VendorOrdersFeatureController orders;
  late final VendorPastryApi pastryApi;
  late final VendorPastryFeatureController pastry;
  late final VendorProductionApi productionApi;
  late final VendorProductionFeatureController production;
  late final VendorProductionBatchesApi productionBatchesApi;
  late final VendorProductionBatchesFeatureController productionBatches;
  late final VendorProductionCostApi productionCostApi;
  late final VendorProductionCostFeatureController productionCost;
  late final VendorProductionMasterApi productionMasterApi;
  late final VendorProductionMasterFeatureController productionMaster;
  late final VendorProductionStockPoliciesApi productionStockPoliciesApi;
  late final VendorProductionStockPoliciesFeatureController
  productionStockPolicies;
  late final VendorPromotionsApi promotionsApi;
  late final VendorPromotionsFeatureController promotions;
  late final VendorPurchasingApi purchasingApi;
  late final VendorPurchasingFeatureController purchasing;
  late final VendorReadinessApi readinessApi;
  late final VendorReadinessFeatureController readiness;
  late final VendorRecipeCostApi recipeCostApi;
  late final VendorRecipeCostFeatureController recipeCost;
  late final VendorRecipesApi recipesApi;
  late final VendorRecipesFeatureController recipes;
  late final VendorScheduledCommerceApi scheduledCommerceApi;
  late final VendorScheduledCommerceFeatureController scheduledCommerce;
  late final VendorShopCatalogueApi shopCatalogueApi;
  late final VendorShopCatalogueFeatureController shopCatalogue;
  late final VendorSupportApi supportApi;
  late final VendorSupportFeatureController support;
  late final VendorTaxonomyApi taxonomyApi;
  late final VendorTaxonomyFeatureController taxonomy;
  Future<void> initialize() async {
    await auth.restore();
    final session = auth.session;
    if (session != null && scope.vendorId != session.vendorId) {
      scope.select(vendorId: session.vendorId);
    }
  }

  void dispose() {
    auth.dispose();
    accessData.dispose();
    authData.dispose();
    business.dispose();
    cloudKitchen.dispose();
    crm.dispose();
    delivery.dispose();
    finance.dispose();
    foodCatalogue.dispose();
    handover.dispose();
    inventory.dispose();
    onboarding.dispose();
    operationalCost.dispose();
    orchestration.dispose();
    orders.dispose();
    pastry.dispose();
    production.dispose();
    productionBatches.dispose();
    productionCost.dispose();
    productionMaster.dispose();
    productionStockPolicies.dispose();
    promotions.dispose();
    purchasing.dispose();
    readiness.dispose();
    recipeCost.dispose();
    recipes.dispose();
    scheduledCommerce.dispose();
    shopCatalogue.dispose();
    support.dispose();
    taxonomy.dispose();
    scope.dispose();
    access.dispose();
    client.close();
  }
}
