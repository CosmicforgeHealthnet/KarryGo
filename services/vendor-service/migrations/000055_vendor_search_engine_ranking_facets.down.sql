DROP TRIGGER IF EXISTS shop_search_terms_refresh_trg
    ON shop_search_documents;

DROP FUNCTION IF EXISTS refresh_shop_search_terms();

DROP TRIGGER IF EXISTS food_search_terms_refresh_trg
    ON food_search_documents;

DROP FUNCTION IF EXISTS refresh_food_search_terms();

DROP TABLE IF EXISTS shop_search_terms;
DROP TABLE IF EXISTS food_search_terms;
DROP TABLE IF EXISTS search_ranking_profiles;