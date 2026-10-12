// GENERATED from actual request schemas/Go JSON fields.
import '../../../core/contracts/vendor_contract.dart';

class PutTaxonomyNamespacesNamespaceCodeMappingsRequest extends VendorPayload {
  PutTaxonomyNamespacesNamespaceCodeMappingsRequest({
    String? facetCode,
    String? sourceType,
    String? sourceRef,
    List<dynamic>? mappings,
  }) : super({
         'facet_code': ?facetCode,
         'source_type': ?sourceType,
         'source_ref': ?sourceRef,
         'mappings': ?mappings,
       });
  PutTaxonomyNamespacesNamespaceCodeMappingsRequest.fromJson(super.json);
  String? get facetCode => value('facet_code') as String?;
  String? get sourceType => value('source_type') as String?;
  String? get sourceRef => value('source_ref') as String?;
  List<dynamic>? get mappings => value('mappings') as List<dynamic>?;
}

class TaxonomySection13TaxonomyError extends VendorPayload {
  TaxonomySection13TaxonomyError.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get error =>
      value('error') == null ? null : jsonObject(value('error'));
}

class TaxonomySection13TaxonomyMappingCandidate extends VendorPayload {
  TaxonomySection13TaxonomyMappingCandidate.fromJson(super.json);
  String? get nodeCode => value('node_code') as String?;
  String? get mappingType => value('mapping_type') as String?;
  String? get confidenceBasis => value('confidence_basis') as String?;
  int? get confidenceScore => value('confidence_score') as int?;
  JsonObject? get metadata =>
      value('metadata') == null ? null : jsonObject(value('metadata'));
}

class TaxonomySection13TaxonomyReplaceFacetMappingsRequest
    extends VendorPayload {
  TaxonomySection13TaxonomyReplaceFacetMappingsRequest.fromJson(super.json);
  String? get facetCode => value('facet_code') as String?;
  String? get sourceType => value('source_type') as String?;
  String? get sourceRef => value('source_ref') as String?;
  List<dynamic>? get mappings => value('mappings') as List<dynamic>?;
}

class TaxonomySection13TaxonomySuccessEnvelope extends VendorPayload {
  TaxonomySection13TaxonomySuccessEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get data =>
      value('data') == null ? null : jsonObject(value('data'));
}
