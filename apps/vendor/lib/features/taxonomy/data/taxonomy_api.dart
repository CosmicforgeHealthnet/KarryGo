// GENERATED. Methods correspond to exact public Vendor contracts.
import '../../../core/network/vendor_api_client.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/network/vendor_response.dart';
import '../models/taxonomy_models.dart';
import 'taxonomy_routes.dart';

class VendorTaxonomyApi extends VendorFeatureApi {
  VendorTaxonomyApi(VendorApiClient client) : super(client, taxonomyOperations);

  /// Source: section13-taxonomy.openapi.json
  Future<VendorResponse> getTaxonomyNamespacesNamespaceCodeClassifications({
    required String namespaceCode,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/taxonomy/namespaces/{namespaceCode}/classifications',
    pathParameters: {'namespaceCode': namespaceCode},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section13-taxonomy.openapi.json
  Future<VendorResponse>
  getTaxonomyNamespacesNamespaceCodeDietaryClaimAssignments({
    required String namespaceCode,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/taxonomy/namespaces/{namespaceCode}/dietary-claim-assignments',
    pathParameters: {'namespaceCode': namespaceCode},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section13-taxonomy.openapi.json
  Future<VendorResponse> getTaxonomyNamespacesNamespaceCodeDietaryClaims({
    required String namespaceCode,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/taxonomy/namespaces/{namespaceCode}/dietary-claims',
    pathParameters: {'namespaceCode': namespaceCode},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section13-taxonomy.openapi.json
  Future<VendorResponse> getTaxonomyNamespacesNamespaceCodeFacets({
    required String namespaceCode,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/taxonomy/namespaces/{namespaceCode}/facets',
    pathParameters: {'namespaceCode': namespaceCode},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section13-taxonomy.openapi.json
  Future<VendorResponse> getTaxonomyNamespacesNamespaceCodeMappings({
    required String namespaceCode,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/taxonomy/namespaces/{namespaceCode}/mappings',
    pathParameters: {'namespaceCode': namespaceCode},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: services/vendor-service/internal/features/taxonomy/http.go#replaceMappings
  Future<VendorResponse> putTaxonomyNamespacesNamespaceCodeMappings({
    required String namespaceCode,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
    required PutTaxonomyNamespacesNamespaceCodeMappingsRequest body,
  }) => request(
    'PUT',
    '/api/v1/vendor/taxonomy/namespaces/{namespaceCode}/mappings',
    pathParameters: {'namespaceCode': namespaceCode},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
    body: body,
  );

  /// Source: section13-taxonomy.openapi.json
  Future<VendorResponse> getTaxonomyNamespacesNamespaceCodeNodes({
    required String namespaceCode,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/taxonomy/namespaces/{namespaceCode}/nodes',
    pathParameters: {'namespaceCode': namespaceCode},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section13-taxonomy.openapi.json
  Future<VendorResponse> getTaxonomyNamespacesNamespaceCodeNodesNodeId({
    required String namespaceCode,
    required String nodeId,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/taxonomy/namespaces/{namespaceCode}/nodes/{nodeId}',
    pathParameters: {'namespaceCode': namespaceCode, 'nodeId': nodeId},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section13-taxonomy.openapi.json
  Future<VendorResponse> getTaxonomyNamespacesNamespaceCodePublishedVersion({
    required String namespaceCode,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/taxonomy/namespaces/{namespaceCode}/published-version',
    pathParameters: {'namespaceCode': namespaceCode},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );

  /// Source: section13-taxonomy.openapi.json
  Future<VendorResponse> getTaxonomyNamespacesNamespaceCodeTagAssignments({
    required String namespaceCode,
    Map<String, String> query = const {},
    VendorCommand? command,
    Map<String, String> contractHeaders = const {},
  }) => request(
    'GET',
    '/api/v1/vendor/taxonomy/namespaces/{namespaceCode}/tag-assignments',
    pathParameters: {'namespaceCode': namespaceCode},
    query: query,
    command: command,
    contractHeaders: contractHeaders,
  );
}
