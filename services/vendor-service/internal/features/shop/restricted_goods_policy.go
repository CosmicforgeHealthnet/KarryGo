package shop

import "cosmicforge/logistics/services/vendor-service/internal/features/taxonomy"

const restrictedGoodsReasonPharmacyExcluded = taxonomy.ReasonMedicineExternalDomain

// IsPharmacyExcludedProduct reports whether a Shop product belongs to the
// explicitly out-of-scope pharmacy/medicine commerce domain. This is a backend
// commerce guard, not a presentation hint.
func IsPharmacyExcludedProduct(product Product) bool {
	return taxonomy.EvaluateRestrictedGoods(productRestrictedGoodsFacts(product)).ReasonCode == taxonomy.ReasonMedicineExternalDomain
}

// IsRestrictedGoodsProduct applies the canonical platform taxonomy projection,
// including pharmacy exclusion and general prohibited/restricted classes.
func IsRestrictedGoodsProduct(product Product) bool {
	return !taxonomy.EvaluateRestrictedGoods(productRestrictedGoodsFacts(product)).Allowed
}

func productRestrictedGoodsFacts(product Product) taxonomy.RestrictedGoodsFacts {
	return taxonomy.RestrictedGoodsFacts{
		ProductClass: product.ProductClass, Name: product.Name,
		Description: product.Description, Brand: product.Brand,
		Manufacturer:        product.Manufacturer,
		RegulatoryReference: product.RegulatoryReference,
		ManufacturerCode:    product.ManufacturerCode,
	}
}

func validateRestrictedGoodsProduct(field string, product Product) error {
	decision := taxonomy.EvaluateRestrictedGoods(productRestrictedGoodsFacts(product))
	if decision.Allowed {
		return nil
	}
	return fieldError(field, "This product is not eligible for Vendor Commerce ("+decision.ReasonCode+").")
}

func validatePharmacyMedicineExclusion(field string, values ...string) error {
	decision := taxonomy.EvaluateRestrictedGoods(taxonomy.RestrictedGoodsFacts{
		Name: valuesAt(values, 0), Description: valuesAt(values, 1),
		Brand: valuesAt(values, 2), Manufacturer: valuesAt(values, 3),
		RegulatoryReference: valuesAt(values, 4), ManufacturerCode: valuesAt(values, 5),
	})
	if !decision.Allowed {
		return fieldError(field, "This product is not eligible for Vendor Commerce ("+decision.ReasonCode+").")
	}
	return nil
}

func valuesAt(values []string, index int) string {
	if index < len(values) {
		return values[index]
	}
	return ""
}
